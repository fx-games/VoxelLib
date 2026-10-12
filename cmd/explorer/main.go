// Command explorer is a first-person demo of voxellib: Minecraft-like
// terrain, walking/flying, block breaking and placing.
//
//	go run ./cmd/explorer                       # generated world saved to saves/demo
//	go run ./cmd/explorer -world ~/path/to/mc/world -pack client.jar
//
// Controls: WASD move, mouse look, Space jump (double tap or F: fly),
// Ctrl sprint, Shift sneak/descend, LMB break, RMB place, 1-9 / wheel select,
// N day/night, Esc release mouse (Esc again quits).
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/gen"
	"github.com/memmaker/voxellib/voxel/mcres"
	"github.com/memmaker/voxellib/voxel/physics"
	"github.com/memmaker/voxellib/voxel/render"
)

func init() { runtime.LockOSThread() }

var hotbarNames = []string{
	"grass_block[snowy=false]", "dirt", "stone", "cobblestone", "oak_planks", "oak_log[axis=y]",
	"glass", "bricks", "glowstone", "oak_leaves[distance=1,persistent=true,waterlogged=false]",
	"sand", "stone_bricks", "water[level=0]",
}

func main() {
	var (
		worldDir = flag.String("world", "saves/demo", "Minecraft world directory to load/save ('' = memory only)")
		readOnly = flag.Bool("readonly", false, "never write to the world directory")
		packs    = flag.String("pack", "", "extra resource packs (dir/zip/jar), comma separated, highest priority first")
		seed     = flag.Int64("seed", 1337, "terrain seed")
		dist     = flag.Int("dist", 12, "render distance in chunks")
		shot     = flag.String("shot", "", "save a screenshot PNG at tick -shot-at and quit")
		shotAt   = flag.Int("shot-at", 200, "tick for -shot")
		startPos = flag.String("pos", "", "start position x,y,z")
		look     = flag.String("look", "", "start yaw,pitch in degrees")
		night    = flag.Bool("night", false, "start at night")
		fly      = flag.Bool("fly", false, "start flying")
		edits    = flag.String("edits", "", "debug: 'dx,dy,dz,block;...' edits relative to the player, applied once terrain is ready")
		autofly  = flag.Float64("autofly", 0, "debug: fly forward automatically at this many blocks/s (streaming stress test)")
		cpuprof  = flag.String("cpuprofile", "", "write a CPU profile")
		verbose  = flag.Bool("verbose", false, "print frame stats to stdout")
		vsync    = flag.String("vsync", "on", "frame pacing: on (GL vsync), limit (no GL vsync, sleep to max(display rate, 120Hz)), off (unlimited)")
	)
	flag.Parse()

	if *cpuprof != "" {
		f, err := os.Create(*cpuprof)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if err := glfw.Init(); err != nil {
		log.Fatal(err)
	}
	defer glfw.Terminate()
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Samples, 0)
	win, err := glfw.CreateWindow(1280, 720, "voxellib explorer", nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	win.MakeContextCurrent()
	frameLimit := time.Duration(0)
	if *vsync == "on" {
		glfw.SwapInterval(1)
	} else {
		glfw.SwapInterval(0)
		if *vsync == "limit" {
			hz := 120 // never pace slower than this: 8.3ms frames
			if m := glfw.GetPrimaryMonitor(); m != nil && m.GetVideoMode().RefreshRate > hz {
				hz = m.GetVideoMode().RefreshRate
			}
			frameLimit = time.Second / time.Duration(hz)
		}
	}

	var paths []string
	if *packs != "" {
		paths = strings.Split(*packs, ",")
	}
	paths = append(paths, "assets/rpg16pack")
	pack, err := mcres.Open(paths...)
	if err != nil {
		log.Fatal(err)
	}
	defer pack.Close()

	reg := voxel.NewRegistry()
	reg.Resolver = pack.Resolver()
	generator := gen.New(reg, *seed)
	eng, err := engine.New(engine.Config{
		Registry:       reg,
		RenderDistance: *dist,
		Generator:      generator,
		WorldDir:       *worldDir,
		ReadOnly:       *readOnly,
		Textures:       pack,
		RenderOptions:  render.Options{Missing: mcres.MissingTexture, IsGreyscale: mcres.IsGreyscale},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer eng.Close()

	g := &game{eng: eng, win: win, flying: *fly, verbose: *verbose, edits: *edits, autofly: *autofly, frameLimit: frameLimit}
	for _, n := range hotbarNames {
		g.hotbar = append(g.hotbar, reg.Parse(n))
	}
	g.cam = render.Camera{FOV: 70 * math.Pi / 180, Near: 0.05}
	g.spawnKnown = false
	if *startPos != "" {
		fmt.Sscanf(*startPos, "%f,%f,%f", &g.x, &g.y, &g.z)
		g.spawnKnown = true
	} else if sx, sy, sz, ok := eng.Spawn(); ok {
		g.x, g.y, g.z = float64(sx)+0.5, float64(sy), float64(sz)+0.5
		g.spawnKnown = true
		g.findGround = true
	} else {
		lx, lz := generator.FindLand(0, 0)
		g.x, g.z = float64(lx)+0.5, float64(lz)+0.5
		g.findGround = true
	}
	if *look != "" {
		var yd, pd float64
		fmt.Sscanf(*look, "%f,%f", &yd, &pd)
		g.cam.Yaw, g.cam.Pitch = yd*math.Pi/180, pd*math.Pi/180
	}
	if *night {
		eng.Renderer.Opt.Daylight = 0.2
	}
	g.setupInput(*shot == "")
	g.run(*shot, *shotAt)
}

type game struct {
	eng *engine.Engine
	win *glfw.Window
	cam render.Camera

	x, y, z          float64 // feet position
	px, py, pz       float64 // previous tick, for interpolation
	vx, vy, vz       float64
	onGround, flying bool
	findGround       bool
	spawnKnown       bool
	frozen           bool

	verbose bool
	edits   string
	autofly float64
	// frameLimit paces frames without GL vsync (0 = unlimited).
	frameLimit time.Duration
	frameStart time.Time
	// GPU timer queries, read back 3 frames later so nothing waits.
	tq     [4]uint32
	fq     int
	gpuMax time.Duration
	// cost: CPU work per frame up to SwapBuffers (excludes vsync waits).
	costMax   time.Duration
	slowCosts int
	phase     [5]time.Duration // worst: physics, update, render, swap, pacing
	maxFrame  time.Duration
	hotbar    []voxel.StateID
	selected  int
	captured  bool
	lastJump  time.Time
	tick      int
	breakCD   int
	placeCD   int
}

const (
	eyeHeight = 1.62
	halfWidth = 0.3
	height    = 1.8
	tickDur   = time.Second / 20
)

func (g *game) setupInput(grab bool) {
	if grab {
		g.capture(true)
	}
	g.win.SetKeyCallback(func(w *glfw.Window, k glfw.Key, sc int, a glfw.Action, m glfw.ModifierKey) {
		if a != glfw.Press {
			return
		}
		switch {
		case k == glfw.KeyEscape:
			if g.captured {
				g.capture(false)
			} else {
				w.SetShouldClose(true)
			}
		case k == glfw.KeyF:
			g.flying = !g.flying
		case k == glfw.KeyN:
			if g.eng.Renderer.Opt.Daylight > 0.5 {
				g.eng.Renderer.Opt.Daylight = 0.2
			} else {
				g.eng.Renderer.Opt.Daylight = 1
			}
		case k == glfw.KeySpace:
			if time.Since(g.lastJump) < 300*time.Millisecond {
				g.flying = !g.flying
				g.vy = 0
			}
			g.lastJump = time.Now()
		case k >= glfw.Key1 && k <= glfw.Key9:
			if i := int(k - glfw.Key1); i < len(g.hotbar) {
				g.selected = i
			}
		}
	})
	g.win.SetScrollCallback(func(w *glfw.Window, dx, dy float64) {
		n := len(g.hotbar)
		if dy < 0 {
			g.selected = (g.selected + 1) % n
		} else if dy > 0 {
			g.selected = (g.selected + n - 1) % n
		}
	})
	g.win.SetMouseButtonCallback(func(w *glfw.Window, b glfw.MouseButton, a glfw.Action, m glfw.ModifierKey) {
		if a == glfw.Press && !g.captured {
			g.capture(true)
			return
		}
		if a == glfw.Press {
			switch b {
			case glfw.MouseButtonLeft:
				g.breakBlock()
				g.breakCD = 5
			case glfw.MouseButtonRight:
				g.placeBlock()
				g.placeCD = 4
			}
		}
	})
	lastX, lastY := g.win.GetCursorPos()
	g.win.SetCursorPosCallback(func(w *glfw.Window, x, y float64) {
		dx, dy := x-lastX, y-lastY
		lastX, lastY = x, y
		if !g.captured {
			return
		}
		const sens = 0.0025
		g.cam.Yaw += dx * sens
		g.cam.Pitch = math.Max(-1.55, math.Min(1.55, g.cam.Pitch-dy*sens))
	})
}

func (g *game) capture(on bool) {
	g.captured = on
	if on {
		g.win.SetInputMode(glfw.CursorMode, glfw.CursorDisabled)
		if glfw.RawMouseMotionSupported() {
			g.win.SetInputMode(glfw.RawMouseMotion, glfw.True)
		}
	} else {
		g.win.SetInputMode(glfw.CursorMode, glfw.CursorNormal)
	}
}

func (g *game) eye() (float64, float64, float64) { return g.x, g.y + eyeHeight, g.z }

func (g *game) target() (physics.Hit, bool) {
	ex, ey, ez := g.eye()
	f := g.cam.Forward()
	return g.eng.Raycast(ex, ey, ez, float64(f[0]), float64(f[1]), float64(f[2]), 5)
}

func (g *game) breakBlock() {
	if h, ok := g.target(); ok {
		if g.eng.World.Reg.Info(h.State).Name == "minecraft:bedrock" {
			return
		}
		g.eng.SetBlock(h.Block.X, h.Block.Y, h.Block.Z, voxel.Air)
		// plants can't float: pop the one standing on the removed block
		above := g.eng.World.Reg.Info(g.eng.Block(h.Block.X, h.Block.Y+1, h.Block.Z))
		if above.Shape == voxel.ShapeCross {
			g.eng.SetBlock(h.Block.X, h.Block.Y+1, h.Block.Z, voxel.Air)
		}
	}
}

func (g *game) placeBlock() {
	h, ok := g.target()
	if !ok {
		return
	}
	p := h.Block
	if !g.eng.World.Reg.Info(h.State).Def.Replaceable {
		p = p.Offset(h.Face)
	}
	cur := g.eng.World.Reg.Info(g.eng.Block(p.X, p.Y, p.Z))
	if !cur.Def.Replaceable {
		return
	}
	id := g.hotbar[g.selected]
	if g.eng.World.Reg.Info(id).Collides {
		box := g.box()
		b := physics.AABB{MinX: float64(p.X), MinY: float64(p.Y), MinZ: float64(p.Z), MaxX: float64(p.X + 1), MaxY: float64(p.Y + 1), MaxZ: float64(p.Z + 1)}
		if box.Intersects(b) {
			return
		}
	}
	g.eng.SetBlock(p.X, p.Y, p.Z, id)
}

func (g *game) box() physics.AABB {
	return physics.AABB{MinX: g.x - halfWidth, MinY: g.y, MinZ: g.z - halfWidth, MaxX: g.x + halfWidth, MaxY: g.y + height, MaxZ: g.z + halfWidth}
}

func (g *game) key(k glfw.Key) bool { return g.win.GetKey(k) == glfw.Press }

func (g *game) inWater() bool {
	info := g.eng.World.Reg.Info(g.eng.Block(int(math.Floor(g.x)), int(math.Floor(g.y+0.4)), int(math.Floor(g.z))))
	return info.Shape == voxel.ShapeLiquid
}

// physicsTick runs one 20 Hz Minecraft-style movement step.
func (g *game) physicsTick() {
	g.px, g.py, g.pz = g.x, g.y, g.z
	g.tick++
	cx, cz := int(math.Floor(g.x))>>4, int(math.Floor(g.z))>>4
	if !g.eng.ChunkReady(voxel.ChunkPos{X: cx, Z: cz}) {
		g.frozen = true
		return // wait for terrain under us
	}
	if g.findGround {
		if y, ok := g.eng.SurfaceY(int(math.Floor(g.x)), int(math.Floor(g.z))); ok {
			if !g.spawnKnown || float64(y) > g.y {
				g.y = float64(y)
			}
			g.px, g.py, g.pz = g.x, g.y, g.z
			g.findGround = false
		}
	}
	g.frozen = false
	if g.edits != "" {
		bx, by, bz := int(math.Floor(g.x)), int(math.Floor(g.y)), int(math.Floor(g.z))
		for _, e := range strings.Split(g.edits, ";") {
			var dx, dy, dz int
			var name string
			parts := strings.SplitN(e, ",", 4)
			if len(parts) != 4 {
				continue
			}
			fmt.Sscan(parts[0], &dx)
			fmt.Sscan(parts[1], &dy)
			fmt.Sscan(parts[2], &dz)
			name = parts[3]
			id := voxel.Air
			if name != "air" {
				id = g.eng.World.Reg.Parse(name)
			}
			g.eng.SetBlock(bx+dx, by+dy, bz+dz, id)
		}
		g.edits = ""
	}

	if g.captured {
		if g.win.GetMouseButton(glfw.MouseButtonLeft) == glfw.Press {
			if g.breakCD--; g.breakCD <= 0 {
				g.breakBlock()
				g.breakCD = 5
			}
		}
		if g.win.GetMouseButton(glfw.MouseButtonRight) == glfw.Press {
			if g.placeCD--; g.placeCD <= 0 {
				g.placeBlock()
				g.placeCD = 4
			}
		}
	}

	if g.autofly > 0 {
		g.flying = true
		g.x -= math.Cos(g.cam.Yaw+math.Pi/2) * g.autofly / 20
		g.z -= math.Sin(g.cam.Yaw+math.Pi/2) * g.autofly / 20
		return
	}
	var fwd, strafe float64
	if g.key(glfw.KeyW) {
		fwd++
	}
	if g.key(glfw.KeyS) {
		fwd--
	}
	if g.key(glfw.KeyD) {
		strafe++
	}
	if g.key(glfw.KeyA) {
		strafe--
	}
	sprint := g.key(glfw.KeyLeftControl) && fwd > 0
	sneak := g.key(glfw.KeyLeftShift)
	jump := g.key(glfw.KeySpace)
	if l := math.Hypot(fwd, strafe); l > 1 {
		fwd, strafe = fwd/l, strafe/l
	}
	if sneak && !g.flying {
		fwd, strafe = fwd*0.3, strafe*0.3
	}
	water := g.inWater()

	var accel, drag float64
	switch {
	case g.flying:
		accel, drag = 0.05, 0.91
		if sprint {
			accel = 0.1
		}
	case water:
		accel, drag = 0.02, 0.8
	case g.onGround:
		accel, drag = 0.1, 0.546 // 0.6 slipperiness * 0.91
		if sprint {
			accel = 0.13
		}
	default:
		accel, drag = 0.02, 0.91
		if sprint {
			accel = 0.026
		}
	}
	sy, cy := math.Sin(g.cam.Yaw), math.Cos(g.cam.Yaw)
	g.vx += (fwd*sy + strafe*cy) * accel
	g.vz += (-fwd*cy + strafe*sy) * accel

	switch {
	case g.flying:
		if jump {
			g.vy += 0.15
		}
		if sneak {
			g.vy -= 0.15
		}
	case water:
		if jump {
			g.vy += 0.04
		}
	case jump && g.onGround:
		g.vy = 0.42
		if sprint {
			g.vx += sy * 0.2
			g.vz -= cy * 0.2
		}
	}

	mx, my, mz := physics.Move(g.eng.World, g.box(), g.vx, g.vy, g.vz)
	g.onGround = g.vy < 0 && my > g.vy
	if g.onGround && g.flying {
		g.flying = false
	}
	if mx != g.vx {
		g.vx = 0
	}
	if mz != g.vz {
		g.vz = 0
	}
	if my != g.vy {
		g.vy = 0
	}
	g.x, g.y, g.z = g.x+mx, g.y+my, g.z+mz

	switch {
	case g.flying:
		g.vy *= 0.6
	case water:
		g.vy = (g.vy - 0.02) * 0.8
	default:
		g.vy = (g.vy - 0.08) * 0.98
	}
	g.vx *= drag
	g.vz *= drag
}

func (g *game) run(shotPath string, shotAt int) {
	last := time.Now()
	acc := time.Duration(0)
	frames, fpsTime, fps := 0, time.Now(), 0
	for !g.win.ShouldClose() {
		glfw.PollEvents()
		now := time.Now()
		g.frameStart = now
		if ft := now.Sub(last); g.tick > 40 {
			g.maxFrame = max(g.maxFrame, ft)
		}
		acc += now.Sub(last)
		last = now
		if acc > time.Second {
			acc = time.Second
		}
		t0 := time.Now()
		for acc >= tickDur {
			g.physicsTick()
			acc -= tickDur
		}
		t1 := time.Now()
		alpha := float64(acc) / float64(tickDur)
		ix := g.px + (g.x-g.px)*alpha
		iy := g.py + (g.y-g.py)*alpha
		iz := g.pz + (g.z-g.pz)*alpha
		g.eng.Update(ix, iy, iz)
		t2 := time.Now()

		fw, fh := g.win.GetFramebufferSize()
		gl.Viewport(0, 0, int32(fw), int32(fh))
		gl.ClearColor(0.75, 0.85, 1, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
		g.cam.X, g.cam.Y, g.cam.Z = ix, iy+eyeHeight, iz
		g.cam.Aspect = float64(fw) / float64(max(fh, 1))
		g.gpuTimer(func() { g.eng.Render(&g.cam) })
		if h, ok := g.target(); ok {
			g.eng.Renderer.DrawBlockOutline(&g.cam, h.Block.X, h.Block.Y, h.Block.Z)
		}
		g.eng.Renderer.DrawCrosshair(float32(g.cam.Aspect))

		if shotPath != "" && g.tick >= shotAt {
			saveShot(shotPath, fw, fh)
			s := g.eng.Renderer.Stats
			fmt.Printf("shot saved: %s | %d visible / %d sections, %d draw calls, %d ranges, %d quads\n",
				shotPath, s.Visible, s.Sections, s.DrawCalls, s.DrawRanges, s.Quads)
			return
		}
		t3 := time.Now()
		g.win.SwapBuffers()
		tSwap := time.Now()
		if g.frameLimit > 0 {
			// Sleep most of the remainder, spin the last bit for precision.
			end := g.frameStart.Add(g.frameLimit)
			if d := time.Until(end) - 500*time.Microsecond; d > 0 {
				osSleep(d)
			}
			for time.Now().Before(end) {
			}
		}
		t4 := time.Now()
		if cost := t3.Sub(t0); g.tick > 40 {
			g.costMax = max(g.costMax, cost)
			if cost > 12*time.Millisecond {
				g.slowCosts++
			}
		}
		for i, d := range []time.Duration{t1.Sub(t0), t2.Sub(t1), t3.Sub(t2), tSwap.Sub(t3), t4.Sub(tSwap)} {
			g.phase[i] = max(g.phase[i], d)
		}

		frames++
		if time.Since(fpsTime) >= 500*time.Millisecond {
			fps = int(float64(frames) / time.Since(fpsTime).Seconds())
			frames, fpsTime = 0, time.Now()
			s := g.eng.Renderer.Stats
			loaded, pc, pm := g.eng.Progress()
			mode := "walk"
			if g.flying {
				mode = "fly"
			}
			name := strings.TrimPrefix(g.eng.World.Reg.Info(g.hotbar[g.selected]).Name, "minecraft:")
			title := (fmt.Sprintf("voxellib explorer | %d fps | %.1f %.1f %.1f %s | [%d] %s | vis %d/%d sec, %d calls, %dk quads, %d MB | chunks %d (+%d) meshes %d",
				fps, g.x, g.y, g.z, mode, g.selected+1, name, s.Visible, s.Sections, s.DrawCalls, s.Quads/1000,
				s.VertexMemory>>20, loaded, pc, pm)) + fmt.Sprintf(" | worst frame cpu %.1fms gpu %.1fms (cpu >12ms: %d) interval %.1fms", ms(g.costMax), ms(g.gpuMax), g.slowCosts, ms(g.maxFrame))
			if g.verbose {
				title += fmt.Sprintf(" | phys %.1f upd %.1f rend %.1f swap %.1f pace %.1f", ms(g.phase[0]), ms(g.phase[1]), ms(g.phase[2]), ms(g.phase[3]), ms(g.phase[4]))
			}
			if g.verbose {
				for i, n := range engine.TimingNames {
					title += fmt.Sprintf(" %s %.1f", n, ms(g.eng.Timing[i]))
				}
				g.eng.Timing = [7]time.Duration{}
			}
			g.maxFrame, g.phase, g.costMax, g.gpuMax = 0, [5]time.Duration{}, 0, 0
			g.win.SetTitle(title)
			if g.verbose {
				fmt.Println(title)
			}
		}
	}
}

// gpuTimer measures f's GPU time with a TIME_ELAPSED query, reading the
// result of a query issued 3 frames ago so the CPU never waits for it.
func (g *game) gpuTimer(f func()) {
	if g.tq[0] == 0 {
		gl.GenQueries(int32(len(g.tq)), &g.tq[0])
	}
	g.fq++
	qi := g.fq % len(g.tq)
	if g.fq > len(g.tq) && g.tick > 40 {
		var ns uint64
		gl.GetQueryObjectui64v(g.tq[(qi+1)%len(g.tq)], gl.QUERY_RESULT, &ns)
		g.gpuMax = max(g.gpuMax, time.Duration(ns))
	}
	gl.BeginQuery(gl.TIME_ELAPSED, g.tq[qi])
	f()
	gl.EndQuery(gl.TIME_ELAPSED)
}

func saveShot(path string, w, h int) {
	px := make([]byte, w*h*4)
	gl.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		copy(img.Pix[y*w*4:(y+1)*w*4], px[(h-1-y)*w*4:(h-y)*w*4])
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	f, err := os.Create(path)
	if err != nil {
		log.Print(err)
		return
	}
	defer f.Close()
	png.Encode(f, img)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
