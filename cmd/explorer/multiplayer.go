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
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/render"
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
	lines   []string // kill feed lines to show

	anims      [4]remoteAnim
	lastFrame  time.Time
	frameTicks float64 // the last frame's length in ticks
	arrows     []mp.ArrowPose
	insts      []render.ModelInstance
	bows       []render.ModelInstance
	mats       []mgl32.Mat4
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
}

// hostGame starts a server in this process and joins it.
func (g *game) hostGame() error {
	srv, err := mp.StartServer(mp.ServerConfig{Name: g.playerName + "'s game", Seed: time.Now().UnixNano() & 0xFFFFFFF,
		Generator: "overworld", Reg: voxel.NewRegistry()})
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
	reg := g.Config.Engine.Registry
	gen, err := mp.NewGenerator(reg, w.Generator, w.Seed)
	if err != nil {
		cl.Close()
		return err
	}
	if g.net == nil {
		g.sp = &spWorld{engine: g.Config.Engine, start: g.Config.Start}
	}
	c := &g.Config.Engine
	c.Generator, c.WorldDir, c.SaveDir, c.Map, c.ReadOnly = cl.Edits.Wrap(gen), "", "", nil, true
	g.Config.Start = &[3]float64{w.SpawnX, w.SpawnY, w.SpawnZ}
	g.net = &netGame{cl: cl, bodyModel: -1}
	log.Printf("mp: joined %s as slot %d (seed %d)", addr, w.Slot, w.Seed)
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
	if n.srv != nil {
		n.srv.Close()
	}
	g.net = nil
	if g.sp != nil {
		g.Config.Engine, g.Config.Start = g.sp.engine, g.sp.start
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
	attack := g.crossbowOn() && g.Input.Down(app.ActAttack) && !g.ScreenOpen()
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
		n.lines = append(n.lines, "§cYou died - respawning...")
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
	lines := n.lines
	n.lines = nil
	n.mu.Unlock()
	if g.ui != nil {
		for _, l := range lines {
			g.ui.HUD.Message(l)
		}
	}
}

// drawNet draws the other players and every arrow.
func (g *game) drawNet(alpha float64) {
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
		n.skins[i] = n.body.Skin(ra.Anim, n.skins[i])
		tex := n.tex[v.Slot]
		if tex == 0 {
			tex = n.defTex
		}
		sky, blk := g.Light(v.X, v.Y+1, v.Z)
		n.insts = append(n.insts, render.ModelInstance{Model: n.bodyModel, X: v.X, Y: v.Y, Z: v.Z, Yaw: v.Yaw,
			Skin: n.skins[i], Texture: tex, Sky: sky, Block: blk})
		if g.bowModel >= 0 {
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
	if len(n.insts) > 0 {
		g.Models.Draw(&g.Cam, n.insts)
	}
	if len(n.bows) > 0 {
		g.Models.Draw(&g.Cam, n.bows)
	}
	if g.arrowModel < 0 {
		return
	}
	n.arrows = n.cl.Arrows(g.Terrain, alpha, n.arrows)
	g.modelInsts = g.modelInsts[:0]
	for _, a := range n.arrows {
		if a.Fresh {
			dx, dy, dz := a.X-g.Cam.X, a.Y-g.Cam.Y, a.Z-g.Cam.Z
			if dx*dx+dy*dy+dz*dz < 3.5*3.5 {
				continue // a fresh shot leaving the eye (see drawArrows)
			}
		}
		back := 0.4 * math.Cos(a.Pitch)
		sky, blk := g.Light(a.X-math.Sin(a.Yaw)*back, a.Y-0.4*math.Sin(a.Pitch), a.Z+math.Cos(a.Yaw)*back)
		g.modelInsts = append(g.modelInsts, render.ModelInstance{Model: g.arrowModel, X: a.X, Y: a.Y, Z: a.Z, Yaw: a.Yaw, Pitch: a.Pitch, Sky: sky, Block: blk})
	}
	g.Models.Draw(&g.Cam, g.modelInsts)
}

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
