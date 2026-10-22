package app

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/render"
)

// Fog under water and in lava, set before Hooks.Render.
var (
	WaterFog = render.Fog{Color: mgl32.Vec3{0.05, 0.18, 0.4}, Start: 0, End: 28}
	LavaFog  = render.Fog{Color: mgl32.Vec3{0.6, 0.1, 0}, Start: 0, End: 2.5}
)

// frameStats tracks worst-case frame times between title updates.
type frameStats struct {
	frameStart time.Time
	// cost: CPU work per frame up to EndFrame (excludes vsync waits).
	costMax   time.Duration
	slowCosts int
	phase     [6]time.Duration // worst: ticks, update, acquire, render, swap, pacing
	maxFrame  time.Duration
	frames    int
	fpsTime   time.Time
}

// tick runs one 20 Hz step of the world.
func (a *App) tick() {
	for _, s := range a.screens {
		s.Update()
	}
	if a.Paused() {
		return
	}
	a.Tick++
	a.Player.tick()
	a.Entities.tick()
	if a.Hooks.Tick != nil {
		a.Hooks.Tick()
	}
	a.Entities.sweep() // removed by the game's tick
}

// loop runs frames until the window closes, Quit or NewGame.
func (a *App) loop() {
	st := &a.stats
	dbg := &a.Config.Debug
	last := time.Now()
	acc := time.Duration(0)
	st.fpsTime = time.Now()
	start := time.Now()
	var benchFrames, benchCPU []time.Duration
	for !a.Win.ShouldClose() && !a.quit && !a.restart {
		if dbg.Verbose && !a.readyLogged && a.Ready() {
			a.readyLogged = true
			log.Printf("startup: terrain ready %v after the session started (tick %d)", time.Since(a.sessionStart).Round(time.Millisecond), a.Tick)
		}
		if dbg.Bench > 0 {
			if el := time.Since(start); el > dbg.Bench {
				printBench(benchFrames, benchCPU)
				return
			} else if el > 3*time.Second { // skip startup
				benchFrames = append(benchFrames, time.Since(last))
			}
		}
		glfw.PollEvents()
		now := time.Now()
		st.frameStart = now
		if ft := now.Sub(last); a.Tick > 40 {
			st.maxFrame = max(st.maxFrame, ft)
		}
		acc = min(acc+now.Sub(last), time.Second)
		last = now
		t0 := time.Now()
		for acc >= TickDur {
			a.tick()
			acc -= TickDur
		}
		if a.Paused() {
			acc = time.Duration(a.alpha * float64(TickDur)) // hold the pose
		}
		t1 := time.Now()
		alpha := float64(acc) / float64(TickDur)
		a.alpha = alpha
		p := a.Player
		ix, iy, iz := p.Lerp(alpha)
		a.Eng.Update(ix, iy, iz)
		t2 := time.Now()

		fw, fh := a.Win.GetFramebufferSize()
		shot := dbg.Shot != "" && a.Tick >= dbg.ShotAt
		if a.shotReq || shot {
			render.CaptureNextFrame()
		}
		tAcq := time.Now()
		// With vsync on, BeginFrame blocks for the next drawable (up to a
		// refresh): that wait is not CPU cost.
		render.BeginFrame(fw, fh, 0.75, 0.85, 1)
		acquire := time.Since(tAcq)
		a.Cam.X, a.Cam.Y, a.Cam.Z = ix, iy+p.EyeHeight, iz
		a.Cam.Aspect = float64(fw) / float64(max(fh, 1))
		opt := &a.Eng.Renderer.Opt
		switch p.EyeIn {
		case character.LiquidWater:
			f := WaterFog
			f.Color = f.Color.Mul(0.1 + 0.9*opt.Daylight)
			opt.Fog = &f
		case character.LiquidLava:
			opt.Fog = &LavaFog
		default:
			opt.Fog = nil
		}
		if a.Hooks.Render != nil {
			a.Hooks.Render(alpha, opt)
		}
		if a.Mixer != nil {
			a.Mixer.SetListener(a.Cam.X, a.Cam.Y, a.Cam.Z, a.Cam.Yaw)
		}
		a.Eng.Render(&a.Cam)
		if h, ent, ok := a.Target(); ok && ent == nil && !a.Paused() {
			a.Eng.Renderer.DrawBlockOutline(&a.Cam, h.Block.X, h.Block.Y, h.Block.Z)
		}
		a.drawEntities(alpha)
		if a.Hooks.Draw != nil {
			a.Hooks.Draw(alpha)
		}
		if !a.Paused() {
			a.Eng.Renderer.DrawCrosshair(float32(a.Cam.Aspect))
		}
		if a.shotReq { // before the HUD and screens, so they never hide the scene
			a.shotReq = false
			a.saveDebugShot(fw, fh)
		}
		if a.HUD != nil {
			a.HUD.Draw(fw, fh)
		}
		for _, s := range a.screens {
			s.Draw(fw, fh)
		}
		if shot {
			if err := writePNG(dbg.Shot, render.ReadFrame(fw, fh)); err != nil {
				log.Print(err)
			}
			s := a.Eng.Renderer.Stats
			fmt.Printf("shot saved: %s | %d visible / %d sections, %d draw calls, %d ranges, %d quads, %d entities\n",
				dbg.Shot, s.Visible, s.Sections, s.DrawCalls, s.DrawRanges, s.Quads, len(a.Entities.List))
			a.quit = true
			return
		}
		t3 := time.Now()
		if dbg.Bench > 0 && time.Since(start) > 3*time.Second {
			benchCPU = append(benchCPU, t3.Sub(st.frameStart)-acquire)
		}
		render.EndFrame(a.Win)
		tSwap := time.Now()
		if a.frameLimit > 0 {
			// Sleep most of the remainder, spin the last bit for precision.
			end := st.frameStart.Add(a.frameLimit)
			if d := time.Until(end) - 500*time.Microsecond; d > 0 {
				osSleep(d)
			}
			for time.Now().Before(end) {
			}
		}
		t4 := time.Now()
		if cost := t3.Sub(t0) - acquire; a.Tick > 40 {
			st.costMax = max(st.costMax, cost)
			if cost > 12*time.Millisecond {
				st.slowCosts++
			}
		}
		for i, d := range [6]time.Duration{t1.Sub(t0), t2.Sub(t1), acquire, t3.Sub(t2) - acquire, tSwap.Sub(t3), t4.Sub(tSwap)} {
			st.phase[i] = max(st.phase[i], d)
		}
		st.frames++
		if el := time.Since(st.fpsTime); el >= 500*time.Millisecond {
			fps := int(float64(st.frames) / el.Seconds())
			st.frames, st.fpsTime = 0, time.Now()
			a.updateTitle(fps)
		}
	}
}

func (a *App) updateTitle(fps int) {
	st := &a.stats
	s := a.Eng.Renderer.Stats
	p := a.Player
	loaded, pc, pm := a.Eng.Progress()
	mode := "walk"
	switch {
	case p.NoClip:
		mode = "noclip"
	case p.Flying:
		mode = "fly"
	case p.Swimming:
		mode = "swim"
	case p.InWater:
		mode = "water"
	case p.InLava:
		mode = "lava"
	}
	title := fmt.Sprintf("%s | %d fps | %.1f %.1f %.1f %s | vis %d/%d sec, %d calls, %dk quads, %d MB | chunks %d (+%d) meshes %d | worst frame cpu %.1fms (cpu >12ms: %d) interval %.1fms",
		a.Config.Title, fps, p.X, p.Y, p.Z, mode, s.Visible, s.Sections, s.DrawCalls, s.Quads/1000,
		s.VertexMemory>>20, loaded, pc, pm, ms(st.costMax), st.slowCosts, ms(st.maxFrame))
	if a.Hooks.Status != nil {
		if t := a.Hooks.Status(); t != "" {
			title += " | " + t
		}
	}
	if a.Config.Debug.Verbose {
		title += fmt.Sprintf(" | tick %.1f upd %.1f acq %.1f rend %.1f swap %.1f pace %.1f", ms(st.phase[0]), ms(st.phase[1]), ms(st.phase[2]), ms(st.phase[3]), ms(st.phase[4]), ms(st.phase[5]))
		for i, n := range engine.TimingNames {
			title += fmt.Sprintf(" %s %.1f", n, ms(a.Eng.Timing[i]))
		}
		a.Eng.Timing = [len(engine.TimingNames)]time.Duration{}
		fmt.Println(title)
	}
	st.maxFrame, st.phase, st.costMax = 0, [6]time.Duration{}, 0
	a.Win.SetTitle(title)
}

// ShotDir is where F2 screenshots go.
const ShotDir = "screenshots"

// saveDebugShot (F2) saves the frame as screenshots/<timestamp>.png with a
// .txt next to it: position, look and the flags that restore the view.
// Encoding runs in the background so the frame doesn't hitch.
func (a *App) saveDebugShot(w, h int) {
	if err := os.MkdirAll(ShotDir, 0o755); err != nil {
		log.Print(err)
		return
	}
	base := filepath.Join(ShotDir, time.Now().Format("2006-01-02_15-04-05.000"))
	img := render.ReadFrame(w, h)
	p := a.Player
	yaw, pitch := a.Cam.Yaw*180/math.Pi, a.Cam.Pitch*180/math.Pi
	info := fmt.Sprintf("pos %.2f,%.2f,%.2f\nlook %.1f,%.1f\nflying %v\ntick %d\n"+
		"repro: -pos %.2f,%.2f,%.2f -look %.1f,%.1f%s\n",
		p.X, p.Y, p.Z, yaw, pitch, p.Flying, a.Tick,
		p.X, p.Y, p.Z, yaw, pitch, map[bool]string{true: " -fly"}[p.Flying])
	go func() {
		if err := writePNG(base+".png", img); err != nil {
			log.Print(err)
			return
		}
		os.WriteFile(base+".txt", []byte(info), 0o644)
		log.Printf("screenshot saved: %s.png", base)
	}()
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// printBench reports Debug.Bench: frame interval and CPU cost (frame start
// to EndFrame, without the drawable wait), mean and 99th percentile.
func printBench(frames, cpu []time.Duration) {
	if len(frames) == 0 || len(cpu) == 0 {
		return
	}
	stat := func(ds []time.Duration) (mean, p99 time.Duration) {
		var sum time.Duration
		for _, d := range ds {
			sum += d
		}
		slices.Sort(ds)
		return sum / time.Duration(len(ds)), ds[len(ds)*99/100]
	}
	fm, fp := stat(frames)
	cm, cp := stat(cpu)
	fmt.Printf("bench %s: %d frames, %.0f fps, interval mean %.2fms p99 %.2fms, cpu mean %.2fms p99 %.2fms\n",
		render.Backend, len(frames), float64(time.Second)/float64(fm), ms(fm), ms(fp), ms(cm), ms(cp))
}
