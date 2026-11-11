package main

// LAN multiplayer on package mp: host (a listen server in this process,
// joined over loopback like any client) or join a game, then play it in a
// world generated from the server's seed. Everyone has the crossbow; the
// server runs the game, this side predicts the local player and draws the
// others with their skins. Hooks: ride (commands and prediction), tickNet
// after the player tick, drawNet in the world pass, blockBroken/blockPlaced
// and toggleDoor send block edits.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
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
	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/audio"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/importer/tacz"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/render"
	firearm "github.com/memmaker/voxellib/weapon"
)

// skin is a player skin to choose from.
type skin struct {
	name string
	png  []byte
	img  image.Image
}

// loadSkins reads the 64x64 PNG skins in dir (cmd/fetchskins fills it);
// without any there is one plain skin.
func loadSkins(dir string) []skin {
	var out []skin
	paths, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	slices.Sort(paths)
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil || img.Bounds().Dx() != 64 || img.Bounds().Dy() != 64 {
			log.Printf("skins: %s: not a 64x64 png", p)
			continue
		}
		out = append(out, skin{name: strings.TrimSuffix(filepath.Base(p), ".png"), png: b, img: img})
	}
	if len(out) == 0 {
		img := plainSkin()
		var b bytes.Buffer
		png.Encode(&b, img)
		out = append(out, skin{name: "Plain", png: b.Bytes(), img: img})
	}
	return out
}

// plainSkin is a stand-in skin: skin-coloured head and hands, a blue shirt
// and dark trousers on the 64x64 layout, overlays empty.
func plainSkin() image.Image {
	const plainHand = 3 // hand length in pixels
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fill := func(x0, y0, x1, y1 int, c color.NRGBA) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.Set(x, y, c)
			}
		}
	}
	skinC, shirt, pants := color.NRGBA{200, 150, 110, 255}, color.NRGBA{40, 110, 180, 255}, color.NRGBA{50, 50, 90, 255}
	fill(0, 0, 32, 16, skinC)   // head
	fill(16, 16, 40, 32, shirt) // body
	fill(40, 16, 56, 32, shirt) // right arm
	fill(32, 48, 48, 64, shirt) // left arm
	fill(0, 16, 16, 32, pants)  // right leg
	fill(16, 48, 32, 64, pants) // left leg
	// hands: each arm's bottom face and the lowest plainHand rows of its sides
	for _, uv := range [][2]int{{40, 16}, {32, 48}} { // right arm, left arm
		x, y := uv[0], uv[1]
		fill(x+8, y, x+12, y+4, skinC)
		fill(x, y+16-plainHand, x+16, y+16, skinC)
	}
	fill(8, 12, 16, 13, color.NRGBA{120, 70, 50, 255})
	return img
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
	guns    map[string]*gunView // other players' gun views by gun ID + tacz.Choice.Key
	fired   []int               // slots whose gun went off, to show and play

	anims      [4]remoteAnim
	lastFrame  time.Time
	frameTicks float64 // the last frame's length in ticks
	arrows     []mp.ArrowPose
	insts      []render.ModelInstance
	bows       []render.ModelInstance
	mats       []mgl32.Mat4 // per bows entry
	arrowInsts []render.ModelInstance
	skins      [][]mgl32.Mat4
}

type remoteAnim struct {
	mp.Anim
	x, z float64
	seen bool
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
	var gun func(string, tacz.Choice, firearm.FireMode) (firearm.Gun, error)
	if g.gunPack != nil {
		gun = g.gunPack.Gun
	}
	srv, err := mp.StartServer(mp.ServerConfig{Name: name, Seed: seed, Generator: gen, Mode: mode, Reg: voxel.NewRegistry(), Gun: gun})
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
	g.Config.Start = &[3]float64{w.SpawnX, w.SpawnY, w.SpawnZ}
	g.net = &netGame{cl: cl, bodyModel: -1, guns: map[string]*gunView{}}
	log.Printf("mp: joined %s as slot %d (seed %d) at %.1f,%.1f,%.1f", addr, w.Slot, w.Seed, w.SpawnX, w.SpawnY, w.SpawnZ)
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
	if g.bowModel >= 0 {
		g.equipped = true
	}
	n.body = mp.NewBody()
	var err error
	if n.bodyModel, err = g.Models.Add(n.body.Model); err != nil {
		log.Printf("mp: player model: %v", err)
		n.bodyModel = -1
	}
	n.defTex = g.Models.AddTexture(plainSkin())
	cl.SendSkin(g.skins[g.skinIdx].png)
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
		case h.Victim == me:
			g.hurt = hurtTicks
		case h.Attacker == me:
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
			n.mu.Lock()
			n.lines = append(n.lines, "§e"+msg)
			n.mu.Unlock()
		}
	}
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
	attack := g.crossbowOn() && (g.Input.Down(app.ActAttack) && !g.ScreenOpen() || g.autoShot())
	n.cmd, n.haveCmd = n.cl.Command(in, attack), true
	return false
}

// tickNet runs after the player's tick: finish the command (crossbow,
// send), take in what arrived, follow the server's health and death.
func (g *game) tickNet() {
	n := g.net
	if n.haveCmd {
		if n.cl.Predicted(n.cmd, g.Player.Character, g.Terrain) {
			g.recoil = recoilTicks
		}
		n.haveCmd = false
	}
	n.cl.Poll()
	if reason := n.cl.Gone(); reason != "" {
		g.leaveGame("disconnected: " + reason)
		return
	}
	own := n.cl.Own()
	if n.cl.Ready() {
		g.health = own.Health
	}
	g.Player.Dead = own.Dead
	if own.Dead && !n.dead {
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
	if n.cl.Ready() { // the day is the server's
		t, f := n.cl.WorldTime()
		g.Eng.Renderer.Opt.Daylight = mp.Daylight(t, f)
	}
	n.mu.Lock()
	for slot := range n.skinNew {
		if !n.skinSet[slot] {
			continue
		}
		n.tex[slot] = 0
		if b := n.skinNew[slot]; b != nil {
			if img, err := png.Decode(bytes.NewReader(b)); err == nil && img.Bounds().Dx() == 64 && img.Bounds().Dy() == 64 {
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

// netPose poses the other players and every arrow for this frame's views
// (drawNet), advancing their walk cycles.
func (g *game) netPose(alpha float64) {
	n := g.net
	n.insts, n.bows, n.mats = n.insts[:0], n.bows[:0], n.mats[:0]
	views := n.cl.Players()
	if len(n.skins) < len(views) {
		n.skins = make([][]mgl32.Mat4, len(views))
	}
	for i, v := range views {
		if v.Dead || n.bodyModel < 0 {
			continue
		}
		ra := &n.anims[v.Slot]
		if !ra.seen {
			ra.x, ra.z, ra.seen = v.X, v.Z, true
		}
		ra.Advance(v.X-ra.x, v.Z-ra.z, n.frameTicks)
		ra.x, ra.z = v.X, v.Z
		ra.Pitch, ra.Sneak = v.Pitch, v.Sneak
		if v.Protected && time.Now().UnixMilli()/150%2 == 0 {
			continue // blinks while spawn-protected (the walk cycle runs on)
		}
		n.skins[i] = n.body.Skin(ra.Anim, n.skins[i])
		tex := n.tex[v.Slot]
		if tex == 0 {
			tex = n.defTex
		}
		sky, blk := g.Light(v.X, v.Y+1, v.Z)
		n.insts = append(n.insts, render.ModelInstance{Model: n.bodyModel, X: v.X, Y: v.Y, Z: v.Z, Yaw: v.Yaw,
			Skin: n.skins[i], Texture: tex, Sky: sky, Block: blk})
		n.mu.Lock()
		held := n.held[v.Slot]
		n.mu.Unlock()
		if gv := g.remoteGun(held); gv != nil {
			n.mats = append(n.mats, remoteGunPose(v, gv))
			x, y, z := remoteGunOrigin(v)
			n.bows = append(n.bows, render.ModelInstance{Model: gv.handle, X: x, Y: y, Z: z, Skin: gv.rest, Sky: sky, Block: blk})
		} else if g.bowModel >= 0 {
			// The crossbow at the right hand, aimed along the look, its
			// cut-out on edge (rolled from the first-person hold, which
			// lies flat) so it shows its profile from the side.
			hand := mgl32.TransformCoordinate(n.body.Hand(n.skins[i]), mp.Matrix(0, 0, 0, v.Yaw))
			m := mgl32.HomogRotate3DY(float32(-v.Yaw)).Mul4(mgl32.HomogRotate3DX(float32(v.Pitch))).
				Mul4(mgl32.HomogRotate3DZ(math.Pi / 2)).Mul4(bowHold).Mul4(mgl32.Scale3D(0.55, 0.55, 0.55))
			n.mats = append(n.mats, m)
			n.bows = append(n.bows, render.ModelInstance{Model: g.bowModel, X: v.X + float64(hand[0]),
				Y: v.Y + float64(hand[1]), Z: v.Z + float64(hand[2]), Sky: sky, Block: blk})
		}
	}
	for i := range n.bows {
		n.bows[i].Transform = &n.mats[i]
	}
	n.arrowInsts = n.arrowInsts[:0]
	if g.arrowModel < 0 {
		return
	}
	n.arrows = n.cl.Arrows(g.Terrain, alpha, n.arrows)
	for _, a := range n.arrows {
		if a.Fresh {
			dx, dy, dz := a.X-g.Cam.X, a.Y-g.Cam.Y, a.Z-g.Cam.Z
			if dx*dx+dy*dy+dz*dz < 3.5*3.5 {
				continue // a fresh shot leaving the eye (see drawArrows)
			}
		}
		back := 0.4 * math.Cos(a.Pitch)
		sky, blk := g.Light(a.X-math.Sin(a.Yaw)*back, a.Y-0.4*math.Sin(a.Pitch), a.Z+math.Cos(a.Yaw)*back)
		n.arrowInsts = append(n.arrowInsts, render.ModelInstance{Model: g.arrowModel, X: a.X, Y: a.Y, Z: a.Z, Yaw: a.Yaw, Pitch: a.Pitch, Sky: sky, Block: blk})
	}
}

// drawNet draws the other players, their weapons and every arrow as
// netPose posed them, and the battle royale wall, for cam.
func (g *game) drawNet(cam *render.Camera) {
	n := g.net
	g.Models.Draw(cam, n.insts)
	g.Models.Draw(cam, n.bows)
	g.Models.Draw(cam, n.arrowInsts)
	if r := n.cl.Round(); r.Mode == mp.BattleRoyale && r.Phase == mp.PhasePlaying {
		g.drawWall(cam, r.Zone, r.Shrinking)
	}
}

// sendHold tells the server which gun is in hand (none: the crossbow).
func (g *game) sendHold() {
	if g.net == nil {
		return
	}
	var h mp.Hold
	if g.gunOn() {
		h = mp.Hold{Gun: g.gun.Gun.ID, Choice: g.gunChoice(g.gun.Gun.ID), Mode: g.gun.FireMode()}
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

// Another player's gun is the first-person model as its holder sees it
// (camera at idle_view), turned to their look, lowered from their eye
// toward the hand and scaled down: the first-person model is drawn with
// the hand's field of view, its muzzle over 2 blocks out.
const (
	remoteGunDrop  = 0.35
	remoteGunScale = 0.4
)

func remoteGunOrigin(v mp.PlayerView) (x, y, z float64) {
	return v.X, v.Y + character.PlayerEyeHeight - remoteGunDrop, v.Z
}

// remoteGunPose maps gun model space to the world, less the translation
// to remoteGunOrigin: the view turned to the look (render.Camera.Forward).
func remoteGunPose(v mp.PlayerView, gv *gunView) mgl32.Mat4 {
	return mgl32.HomogRotate3DY(float32(-v.Yaw)).Mul4(mgl32.HomogRotate3DX(float32(v.Pitch))).
		Mul4(mgl32.Scale3D(remoteGunScale, remoteGunScale, remoteGunScale)).Mul4(gv.idle)
}

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
	for _, v := range g.net.cl.Players() {
		if v.Slot != slot || v.Dead {
			continue
		}
		x, y, z := remoteGunOrigin(v)
		g.sounds.play(spec.Sounds[spec.ShotSound3P()], audio.At(x, y, z, float32(dist)/blocksPerVolume, g.soundPitch()))
		m := mgl32.Translate3D(float32(x), float32(y), float32(z)).Mul4(remoteGunPose(v, gv))
		if b := gv.m.Bone(muzzleBone); b >= 0 {
			m = m.Mul4(gv.rest[b]).Mul4(gv.m.Bones[b].InvBind.Inv()).Mul4(mgl32.HomogRotate3DZ(math.Pi)) // see drawMuzzle
		}
		f := frameOf(m)
		g.startMuzzle(spec, &f)
	}
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
