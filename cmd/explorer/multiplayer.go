package main

// LAN multiplayer on package mp: host (a listen server in this process,
// joined over loopback like any client) or join a game, then play it in a
// world generated from the server's seed. The server runs the game
// (zombie defense: zombies, pickups); this side predicts the local player and draws the
// others with their skins. Hooks: ride (commands and prediction), tickNet
// after the player tick, drawNet in the world pass, blockBroken/blockPlaced
// and toggleDoor send block edits.

import (
	"bytes"
	"fmt"
	"github.com/memmaker/voxellib/ui/nametag"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/actors"
	"github.com/memmaker/voxellib/audio"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/inventory"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxellib/voxel/respack"
)

// skin is a player skin to choose from.
type skin struct {
	name string
	png  []byte
	img  image.Image
}

// defaultSkinFile is the vanilla default skin (DefaultPlayerSkin: Steve,
// wide arms), worn by a player without a custom one.
const defaultSkinFile = "assets/pixelperfection/assets/minecraft/textures/entity/player/wide/steve.png"

// readSkin reads a 64x64 PNG skin.
func readSkin(path string) (skin, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return skin{}, err
	}
	img, err := decodeSkin(b)
	if err != nil {
		return skin{}, fmt.Errorf("%s: %w", path, err)
	}
	return skin{name: strings.TrimSuffix(filepath.Base(path), ".png"), png: b, img: img}, nil
}

// decodeSkin decodes a 64x64 PNG skin.
func decodeSkin(b []byte) (image.Image, error) {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
		return nil, fmt.Errorf("skin is %v, not 64x64", img.Bounds().Size())
	}
	return img, nil
}

// loadSkins returns the default skin and the 64x64 PNG skins in dir
// (cmd/fetchskins fills it); without any, the choice is the default.
func loadSkins(dir string) (def skin, choice []skin, err error) {
	if def, err = readSkin(defaultSkinFile); err != nil {
		return def, nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.png"))
	if err != nil {
		return def, nil, err
	}
	slices.Sort(paths)
	for _, p := range paths {
		s, err := readSkin(p)
		if err != nil {
			return def, nil, err
		}
		choice = append(choice, s)
	}
	if len(choice) == 0 {
		choice = []skin{def}
	}
	return def, choice, nil
}

// netGame is a multiplayer session's state.
type netGame struct {
	srv     *mp.Server // hosting: the in-process server
	cl      *mp.Client
	cmd     mp.Cmd
	haveCmd bool
	dead    bool

	body      *mp.Body
	bodyModel int
	defTex    int    // the default skin texture
	tex       [4]int // per slot; 0: defTex

	mu      sync.Mutex // the client's callbacks (tick) vs drawing
	skinNew [4][]byte  // skins to upload (nil entry: none pending)
	skinSet [4]bool
	lines   []string            // kill feed lines to show
	held    [4]mp.Hold          // by slot: the gun in hand (PlayerInfo.Held)
	names   [4]string           // by slot: the player's name (PlayerInfo.Name)
	guns    map[string]*gunView // other players' gun views by gun ID + tacz.Choice.Key
	fired   []int               // slots whose gun went off, to show and play

	anims      [4]remoteAnim
	lastFrame  time.Time
	frameTicks float64 // the last frame's length in ticks
	insts      []render.ModelInstance
	bows       []render.ModelInstance // the other players' guns
	mats       []mgl32.Mat4           // per bows entry
	gunAt      [4]mgl32.Mat4          // by slot: the held gun's model→world this frame (remoteGunMat)
	gunPosed   [4]bool                // gunAt is set
	tags       []nametag.Tag
	skins      [][]mgl32.Mat4
	viewAspect float64 // the aspect last sent (SendView)

	zombies map[uint32]*remoteZombie // by entity ID; under mu
	zviews  []mp.ZombieView
	zinsts  []render.ModelInstance
}

type remoteAnim struct {
	mp.Anim
	x, z float64
	seen bool
	// hurt counts down the red hurt flash (ticks, from actors.MaxHurtTime on
	// an mp.Hit); dying counts ticks since the death was seen (deathTime).
	hurt, dying float64
}

// spWorld is the single-player configuration to return to.
type spWorld struct {
	engine engine.Config
	start  *[3]float64
	hotbar []voxel.StateID
}

// hostGame starts a server in this process and joins it.
// An empty name is the player's, a zero seed random.
func (g *game) hostGame(name string, seed int64, gen string, mode mp.Mode) error {
	if name == "" {
		name = g.playerName + "'s game"
	}
	if seed == 0 {
		seed = time.Now().UnixNano() & 0xFFFFFFF
	}
	return g.serve(mp.ServerConfig{Name: name, Seed: seed, Generator: gen, Mode: mode})
}

// playSolo is single player: zombie defense on a private server in this
// process (on loopback and a free port: not found by LAN discovery).
func (g *game) playSolo(gen string, seed int64) error {
	if seed == 0 {
		seed = time.Now().UnixNano() & 0xFFFFFFF
	}
	return g.serve(mp.ServerConfig{Addr: "127.0.0.1:0", Name: g.playerName + "'s game", Seed: seed, Generator: gen, Mode: mp.ZombieDefense})
}

// serve starts a server for cfg (with this game's guns) and joins it.
func (g *game) serve(cfg mp.ServerConfig) error {
	cfg.Reg, cfg.Zombies = voxel.NewRegistry(), g.opt.zombies
	if g.gunPack != nil {
		cfg.Gun, cfg.Loot = g.gunPack.Gun, mp.NewLoot(g.gunPack)
	}
	srv, err := mp.StartServer(cfg)
	if err != nil {
		return err
	}
	if err := g.joinGame(fmt.Sprintf("127.0.0.1:%d", srv.Addr().Port)); err != nil {
		srv.Close()
		return err
	}
	g.net.srv = srv
	return nil
}

// joinGame connects to a server and switches to its world.
func (g *game) joinGame(addr string) error {
	cl, err := mp.Connect(addr, g.playerName, netcode.Config{})
	if err != nil {
		return err
	}
	w := cl.Welcome
	if g.net == nil {
		g.sp = &spWorld{engine: g.Config.Engine, start: g.Config.Start, hotbar: g.hotbar}
	}
	reg := g.sp.engine.Registry
	if mp.MapName(w.Generator) != "" {
		reg = voxel.NewRegistry() // the map's blocks only, as single player opens it
	}
	gen, err := mp.NewGenerator(reg, w.Generator, w.Seed)
	if err != nil {
		cl.Close()
		return err
	}
	c := &g.Config.Engine
	c.WorldDir, c.SaveDir, c.ReadOnly = "", "", true
	c.MinY, c.Height = mp.Heights(w.Generator)
	c.Patch = func(w *voxel.World, ch *voxel.Chunk) { cl.Edits.Apply(w.Reg, ch) }
	if mg, ok := gen.(*mp.MapGen); ok {
		// a map game is the map, as single player walks it
		c.Map, c.Dimension, c.Generator, c.Textures = mg.Map, mg.Map.Spawn.Dim, nil, nil
		c.Registry = reg
		g.hotbar = nil
		for _, n := range mapHotbar {
			g.hotbar = append(g.hotbar, mg.Map.State(n))
		}
	} else {
		c.Map, c.Generator, c.Textures = nil, gen, g.sp.engine.Textures
		c.Registry, c.Dimension = reg, g.sp.engine.Dimension
		g.hotbar = g.sp.hotbar
		if c.Textures == nil {
			c.Textures = g.sp.engine.Map
		}
	}
	g.Config.Start = &[3]float64{w.Own.X, w.Own.Y, w.Own.Z} // placed by the server
	g.net = &netGame{cl: cl, bodyModel: -1, guns: map[string]*gunView{}, zombies: map[uint32]*remoteZombie{}}
	log.Printf("mp: joined %s as slot %d (seed %d) at %.1f,%.1f,%.1f", addr, w.Slot, w.Seed, w.Own.X, w.Own.Y, w.Own.Z)
	g.Switch()
	return nil
}

// leaveGame disconnects (stopping a hosted server) and returns to the
// single-player world.
func (g *game) leaveGame(reason string) {
	n := g.net
	if n == nil {
		return
	}
	n.cl.Close()
	for _, v := range n.guns {
		if v != nil {
			g.freeGunView(v)
		}
	}
	if n.srv != nil {
		n.srv.Close()
	}
	g.net = nil
	if g.sp != nil {
		g.Config.Engine, g.Config.Start = g.sp.engine, g.sp.start
		g.hotbar = g.sp.hotbar
	}
	if reason != "" {
		log.Printf("mp: %s", reason)
		g.pendingNote = reason
	}
	g.Switch()
}

// netStart sets up a multiplayer session (from start, after the imports).
func (g *game) netStart() {
	n := g.net
	cl := n.cl
	g.Player.CanFly, g.Player.Flying, g.Player.NoClip = false, false, false
	n.body = mp.NewBody()
	var err error
	if n.bodyModel, err = g.Models.Add(n.body.Model); err != nil {
		log.Printf("mp: player model: %v", err)
		n.bodyModel = -1
	}
	n.defTex = g.Models.AddTexture(g.defaultSkin.img)
	cl.SendSkin(g.skins[g.skinIdx].png)
	g.sendHold()
	g.sendView()
	cl.OnPickup = g.takePickup
	cl.OnExplosion = func(e mp.Explosion) { g.explosionFX(e.Pos, e.Radius) }
	cl.OnBlock = func(e mp.BlockEdit) {
		reg := g.Eng.World.Reg
		if id := reg.Parse(e.State); g.Eng.Block(e.X, e.Y, e.Z) != id {
			g.Eng.SetBlock(e.X, e.Y, e.Z, id)
		}
	}
	cl.OnPlayer = func(p mp.PlayerInfo) {
		n.mu.Lock()
		n.skinNew[p.Slot], n.skinSet[p.Slot] = p.Skin, true
		n.held[p.Slot] = p.Held // no gun when the slot was freed
		n.names[p.Slot] = p.Name
		n.mu.Unlock()
	}
	cl.OnFired = func(slot int) {
		n.mu.Lock()
		n.fired = append(n.fired, slot)
		n.mu.Unlock()
	}
	cl.OnText = func(s string) {
		n.mu.Lock()
		n.lines = append(n.lines, s)
		n.mu.Unlock()
	}
	cl.OnHit = func(h mp.Hit) {
		me := cl.Slot()
		switch {
		case h.ByZombie && h.Victim == me:
			g.hurt = hurtTicks
		case h.ByZombie && h.Victim >= 0 && h.Victim < len(n.anims):
			n.anims[h.Victim].hurt = actors.MaxHurtTime
		case h.Zombie != 0:
			g.zombieHurt(h.Zombie)
			if h.Attacker == me {
				g.hitFeedback(h)
			}
		case h.Victim == me:
			g.hurt = hurtTicks
		case h.Victim >= 0 && h.Victim < len(n.anims):
			n.anims[h.Victim].hurt = actors.MaxHurtTime
		case h.Attacker == me:
			g.hitFeedback(h)
		}
	}
}

// hitFeedback is the shooter's side of a hit: the hit marker and a line.
func (g *game) hitFeedback(h mp.Hit) {
	if h.Gun {
		g.hitMark(h.Head, h.Killed)
	}
	msg := "Hit"
	if h.Head {
		msg = "Head shot"
	}
	if h.Killed {
		msg += " - kill!"
	}
	n := g.net
	n.mu.Lock()
	n.lines = append(n.lines, "§e"+msg)
	n.mu.Unlock()
}

// sendView tells the server the unzoomed view (zombie defense spawns out
// of it) when the window's aspect changed.
func (g *game) sendView() {
	n := g.net
	if g.Cam.Aspect == n.viewAspect || g.Cam.Aspect <= 0 {
		return
	}
	n.viewAspect = g.Cam.Aspect
	n.cl.SendView(baseFOV*math.Pi/180, g.Cam.Aspect)
}

// ride turns the tick's input into a command (Hooks.Ride): first the
// prediction is reset to the server's newest state and replayed, then the
// app moves the player with the input as usual (false), unless dead.
func (g *game) ride(in character.Input) bool {
	n := g.net
	if n == nil {
		return false
	}
	n.cl.Reconcile(g.Player.Character, g.Terrain)
	if n.cl.Own().Dead {
		return true
	}
	n.cmd, n.haveCmd = n.cl.Command(in), true
	return false
}

// sendCmd runs first after the player's tick: send the command. The server runs the reliable messages riding on
// a packet before its commands, so a shot queued after this send is fired
// with this tick's input (sneak, position), as in single player.
func (g *game) sendCmd() {
	n := g.net
	if n.haveCmd {
		n.cl.Send(n.cmd)
		n.haveCmd = false
	}
}

// tickNet takes in what arrived and follows the server's health and death.
func (g *game) tickNet() {
	n := g.net
	n.cl.Poll()
	n.cl.TickBullets(bulletWorld{g.Eng.World, g.Terrain})
	if reason := n.cl.Gone(); reason != "" {
		g.leaveGame("disconnected: " + reason)
		return
	}
	own := n.cl.Own()
	g.health = own.Health
	g.Player.Dead = own.Dead
	if own.Dead && !n.dead {
		g.loseInventory() // back to bare hands
		n.mu.Lock()
		if r := n.cl.Round(); r.Mode == mp.BattleRoyale && r.Phase == mp.PhasePlaying {
			n.lines = append(n.lines, "§cYou are out - watching until the next round")
		} else {
			n.lines = append(n.lines, "§cYou died - respawning...")
		}
		n.mu.Unlock()
	}
	n.dead = own.Dead
}

// netFrame advances the client's clock and uploads new skins (render
// thread).
func (g *game) netFrame() {
	n := g.net
	now := time.Now()
	if !n.lastFrame.IsZero() {
		dt := now.Sub(n.lastFrame)
		n.cl.Advance(dt)
		n.frameTicks = math.Min(dt.Seconds()*mp.TickRate, 2)
	}
	n.lastFrame = now
	g.sendView()
	t, f := n.cl.WorldTime() // the day is the server's
	g.Eng.Renderer.Opt.Daylight = mp.Daylight(t, f)
	n.mu.Lock()
	for slot := range n.skinNew {
		if !n.skinSet[slot] {
			continue
		}
		n.tex[slot] = 0
		if b := n.skinNew[slot]; b != nil {
			if img, err := decodeSkin(b); err != nil {
				log.Printf("mp: player %d's skin: %v (drawn with the default)", slot, err)
			} else {
				n.tex[slot] = g.Models.AddTexture(img)
			}
		}
		n.skinNew[slot], n.skinSet[slot] = nil, false
	}
	lines, fired, held := n.lines, n.fired, n.held
	n.lines, n.fired = nil, nil
	n.mu.Unlock()
	for _, slot := range fired {
		g.remoteShot(slot, held[slot])
	}
	if g.ui != nil {
		for _, l := range lines {
			g.ui.HUD.Message(l)
		}
	}
}

// netPose poses the other players and the zombies for this frame's views
// (drawNet), advancing their walk cycles.
func (g *game) netPose() {
	n := g.net
	g.poseZombies()
	n.insts, n.bows, n.mats, n.tags = n.insts[:0], n.bows[:0], n.mats[:0], n.tags[:0]
	views := n.cl.Players()
	if len(n.skins) < len(views) {
		n.skins = make([][]mgl32.Mat4, len(views))
	}
	n.gunPosed = [4]bool{}
	for i, v := range views {
		ra := &n.anims[v.Slot]
		ra.hurt = max(0, ra.hurt-n.frameTicks)
		if !v.Dead {
			ra.dying = 0
		} else if ra.dying += n.frameTicks; ra.dying >= deathTicks {
			continue // removed after the death animation (LivingEntity.tickDeath)
		}
		if n.bodyModel < 0 {
			continue
		}
		n.mu.Lock()
		held, name := n.held[v.Slot], n.names[v.Slot]
		n.mu.Unlock()
		gv := g.remoteGun(held)
		if !ra.seen {
			ra.x, ra.z, ra.seen = v.X, v.Z, true
		}
		ra.Advance(v.X-ra.x, v.Z-ra.z, n.frameTicks)
		ra.x, ra.z, ra.Age = v.X, v.Z, n.cl.RenderTime()
		ra.Pitch, ra.Sneak, ra.Gun, ra.Aim = v.Pitch, v.Sneak, held.Gun != "", v.Aim
		if gv != nil {
			ra.Hold = gv.spec.ThirdPersonHold
		}
		n.skins[i] = n.body.Skin(ra.Anim, n.skins[i])
		// The body falls over onto its left side while dying (setupRotations' Z turn).
		tilt, tiltYaw := 0.0, v.Yaw-math.Pi/2
		if v.Dead {
			tilt = deathTiltAt(ra.dying)
		}
		root := creatureMatrix(v.X, v.Y, v.Z, v.Yaw, tilt, tiltYaw)
		if gv != nil {
			n.gunAt[v.Slot], n.gunPosed[v.Slot] = remoteGunMat(n.body, n.skins[i], root, gv), true
		}
		sky, blk := g.Light(v.X, v.Y+1, v.Z)
		if !v.Dead {
			top := character.PlayerHeight
			if v.Sneak {
				top = character.PlayerCrouchHeight
			}
			n.tags = append(n.tags, nametag.Tag{Text: name, X: v.X, Y: v.Y + top, Z: v.Z, Discrete: v.Sneak, Sky: sky, Block: blk})
		}
		if v.Protected && time.Now().UnixMilli()/150%2 == 0 {
			continue // blinks while spawn-protected (the walk cycle runs on)
		}
		tex := n.tex[v.Slot]
		if tex == 0 {
			tex = n.defTex
		}
		in := render.ModelInstance{Model: n.bodyModel, X: v.X, Y: v.Y, Z: v.Z, Yaw: v.Yaw, Tilt: tilt, TiltYaw: tiltYaw,
			Skin: n.skins[i], Texture: tex, Sky: sky, Block: blk}
		if ra.hurt > 0 || v.Dead { // getOverlayCoords: hurtTime > 0 || deathTime > 0
			in.Overlay = hurtOverlay
		}
		n.insts = append(n.insts, in)
		if gv != nil {
			m := n.gunAt[v.Slot]
			t := m.Col(3)
			m.SetCol(3, mgl32.Vec4{0, 0, 0, 1})
			skin := gv.rest
			if v.Empty {
				skin = gv.restEmpty
			}
			parts := []int{gv.handle}
			if l := gv.lens; l != nil {
				// BedrockAttachmentModel out of first person: the scope
				// body and ocular ring drawn plainly, no oculars or reticles.
				parts = append(parts, l.body, l.ring)
			}
			for _, h := range parts {
				if h >= 0 {
					n.mats = append(n.mats, m)
					n.bows = append(n.bows, render.ModelInstance{Model: h, X: float64(t[0]), Y: float64(t[1]), Z: float64(t[2]), Skin: skin, Sky: sky, Block: blk})
				}
			}
		}
	}
	for i := range n.bows {
		n.bows[i].Transform = &n.mats[i]
	}
}

// drawNet draws the other players, their guns and the zombies as netPose
// posed them, and the battle royale wall, for cam.
func (g *game) drawNet(cam *render.Camera) {
	n := g.net
	g.Models.Draw(cam, n.insts)
	g.Models.Draw(cam, n.bows)
	g.Models.Draw(cam, n.zinsts)
	if g.nameTags != nil {
		g.nameTags.Draw(cam, n.tags)
	}
	if r := n.cl.Round(); r.Mode == mp.BattleRoyale && r.Phase == mp.PhasePlaying {
		g.drawWall(cam, r.Zone, r.Shrinking)
	}
}

// sendHold tells the server what is in hand: the gun or nothing.
func (g *game) sendHold() {
	if g.net == nil {
		return
	}
	var h mp.Hold
	if g.gunOn() {
		h = mp.Hold{Gun: g.gun.Gun.ID, Choice: g.stackChoice(g.gunStack), Mode: g.gun.FireMode()}
	}
	g.net.cl.SendHold(h)
}

// sendNextHold tells the server gun stack s (nil: none) is in
// hand as the switch to it starts: in TACZ the item changes at once and
// others see it (ClientMessagePlayerDrawGun); put_away plays in first person only.
func (g *game) sendNextHold(s *inventory.Stack) {
	if g.net == nil {
		return
	}
	var h mp.Hold
	if s != nil {
		h = mp.Hold{Gun: s.Item.Key, Choice: g.stackChoice(s), Mode: g.stackMode(s)}
	}
	g.net.cl.SendHold(h)
}

// remoteGun is another player's held gun with its attachments (nil: none,
// or not in this pack), built once per gun and choice.
func (g *game) remoteGun(h mp.Hold) *gunView {
	if g.gunSpec(h.Gun) == nil {
		return nil
	}
	key := h.Gun + "|" + h.Choice.Key()
	if v, ok := g.net.guns[key]; ok {
		return v
	}
	v, err := g.newGunView(h.Gun, h.Choice, h.Mode)
	if err != nil {
		log.Printf("gun %s %v: %v", h.Gun, h.Choice, err)
	}
	g.net.guns[key] = v // nil on error: logged once, not rebuilt every frame
	return v
}

// remoteGunMat maps another player's gun model to the world, as TACZ
// draws a gun in the right hand: ItemInHandLayer (translateToHand, then
// ThirdPersonHeld with builtin/entity's empty display), then
// GunItemTransforms for THIRD_PERSON_RIGHT_HAND, which puts the
// thirdperson_hand node at (0.5, 0.5, 0.5) of the item, axes aligned, and
// scales by the display's thirdperson scale.
func remoteGunMat(body *mp.Body, skin []mgl32.Mat4, root mgl32.Mat4, gv *gunView) mgl32.Mat4 {
	s := gv.spec.ThirdPersonScale
	return root.Mul4(body.RightArm(skin)).
		Mul4(mgl32.Scale3D(pixelsPerBlock, pixelsPerBlock, pixelsPerBlock)).
		Mul4(render.ThirdPersonHeld(false, respack.HeldDisplay{})).
		Mul4(mgl32.Translate3D(0.5, 0.5, 0.5)).Mul4(mgl32.Scale3D(s, s, s)).Mul4(gv.thirdHand)
}

// pixelsPerBlock turns the reference's ModelPart pixels into blocks' worth.
const pixelsPerBlock = 16

// remoteShot plays another player's shot: the gun's sound where they stand
// and its muzzle flash at their gun's muzzle.
func (g *game) remoteShot(slot int, h mp.Hold) {
	gv := g.remoteGun(h)
	if gv == nil {
		return
	}
	spec := &gv.spec
	dist := spec.SoundDistance()
	if dist <= 0 {
		return // unheard (ModernKineticGunScriptAPI.shoot: soundDistance > 0)
	}
	if !g.net.gunPosed[slot] {
		return // not posed this frame (dead, or no body)
	}
	m := g.net.gunAt[slot]
	t := m.Col(3)
	g.sounds.play(spec.Sounds[spec.ShotSound3P()], audio.At(float64(t[0]), float64(t[1]), float64(t[2]), float32(dist)/blocksPerVolume, g.soundPitch()))
	if b := gv.m.Bone(muzzleBone); b >= 0 {
		m = m.Mul4(gv.rest[b]).Mul4(gv.m.Bones[b].InvBind.Inv()).Mul4(mgl32.HomogRotate3DZ(math.Pi)) // see drawMuzzle
	}
	f := frameOf(m)
	g.startMuzzle(spec, &f)
}

// blocksPerVolume is how far a sound of volume 1 reaches (audio.Params.Volume):
// a remote shot's volume is its weapon.Gun.SoundDistance over it.
const blocksPerVolume = 16

// sendEdit reports a block the player changed at pos.
func (g *game) sendEdit(pos voxel.BlockPos) {
	if g.net == nil {
		return
	}
	reg := g.Eng.World.Reg
	g.net.cl.SendEdit(mp.BlockEdit{X: pos.X, Y: pos.Y, Z: pos.Z, State: reg.StateString(g.Eng.Block(pos.X, pos.Y, pos.Z))})
}

func (g *game) blockBroken(pos voxel.BlockPos, _ voxel.StateID) { g.sendEdit(pos) }
func (g *game) blockPlaced(pos voxel.BlockPos, _ voxel.StateID) { g.sendEdit(pos) }

// watchChestHeight is where the -watch camera looks at a player (blocks
// above the feet).
const watchChestHeight = 1.2

// watchCam is the -watch debug camera: it views the first other player
// from the set angle and distance at chest height, and the hand is not
// drawn while it does.
func (g *game) watchCam() {
	g.watching = false
	w := g.opt.watch
	if w == nil {
		return
	}
	views := g.net.cl.Players()
	if len(views) == 0 {
		return
	}
	v := views[0]
	a := v.Yaw + w[0]
	tx, ty, tz := v.X, v.Y+watchChestHeight, v.Z
	g.Cam.X, g.Cam.Y, g.Cam.Z = tx+math.Sin(a)*w[1], ty, tz-math.Cos(a)*w[1]
	g.Cam.Yaw, g.Cam.Pitch = math.Atan2(tx-g.Cam.X, -(tz-g.Cam.Z)), 0
	g.Cam.Bob = mgl32.Mat4{} // no view bobbing
	g.watching = true
}
