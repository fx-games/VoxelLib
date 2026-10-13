package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/render"
)

// frameStats tracks worst-case frame times between title updates.
type frameStats struct {
	frameStart time.Time
	// GPU timer queries, read back 3 frames later so nothing waits.
	tq     [4]uint32
	fq     int
	gpuMax time.Duration
	// cost: CPU work per frame up to SwapBuffers (excludes vsync waits).
	costMax   time.Duration
	slowCosts int
	phase     [5]time.Duration // worst: ticks, update, render, swap, pacing
	maxFrame  time.Duration
}

var (
	waterFog = render.Fog{Color: mgl32.Vec3{0.05, 0.18, 0.4}, Start: 0, End: 28}
	lavaFog  = render.Fog{Color: mgl32.Vec3{0.6, 0.1, 0}, Start: 0, End: 2.5}
)

func (g *game) run(shotPath string, shotAt int) {
	st := &g.stats
	last := time.Now()
	acc := time.Duration(0)
	frames, fpsTime := 0, time.Now()
	for !g.win.ShouldClose() {
		glfw.PollEvents()
		now := time.Now()
		st.frameStart = now
		if ft := now.Sub(last); g.tick > 40 {
			st.maxFrame = max(st.maxFrame, ft)
		}
		acc = min(acc+now.Sub(last), time.Second)
		last = now
		t0 := time.Now()
		for acc >= tickDur {
			g.tickPlayer()
			g.tickItems()
			g.tickCreatures()
			g.tickArrows()
			g.tickTorches()
			g.tickPlayerHealth()
			acc -= tickDur
		}
		t1 := time.Now()
		alpha := float64(acc) / float64(tickDur)
		ix, iy, iz := g.player.Lerp(alpha)
		g.eng.Update(ix, iy, iz)
		t2 := time.Now()

		fw, fh := g.win.GetFramebufferSize()
		gl.Viewport(0, 0, int32(fw), int32(fh))
		gl.ClearColor(0.75, 0.85, 1, 1)
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
		g.cam.X, g.cam.Y, g.cam.Z = ix, iy+g.player.EyeHeight, iz
		g.cam.Aspect = float64(fw) / float64(max(fh, 1))
		opt := &g.eng.Renderer.Opt
		switch g.player.EyeIn {
		case character.LiquidWater:
			f := waterFog
			f.Color = f.Color.Mul(0.1 + 0.9*opt.Daylight)
			opt.Fog = &f
		case character.LiquidLava:
			opt.Fog = &lavaFog
		default:
			opt.Fog = nil
		}
		g.gpuTimer(func() {
			g.eng.Render(&g.cam)
			if h, ok := g.target(); ok {
				g.eng.Renderer.DrawBlockOutline(&g.cam, h.Block.X, h.Block.Y, h.Block.Z)
			}
			g.drawSigns()
			g.drawCreatures(alpha)
			g.drawWeapons()
			g.drawArrows(alpha)
			g.drawItems(alpha)
			g.drawTorches(alpha)
		})
		g.eng.Renderer.DrawCrosshair(float32(g.cam.Aspect))
		if g.shotReq { // before the help overlay, so it never hides the scene
			g.shotReq = false
			g.saveDebugShot(fw, fh)
		}
		g.drawHUD(fw, fh)
		g.drawHelp(fw, fh)

		if shotPath != "" && g.tick >= shotAt {
			saveShot(shotPath, fw, fh)
			s := g.eng.Renderer.Stats
			fmt.Printf("shot saved: %s | %d visible / %d sections, %d draw calls, %d ranges, %d quads, %d drops, holding %q\n",
				shotPath, s.Visible, s.Sections, s.DrawCalls, s.DrawRanges, s.Quads, len(g.drops.Drops), g.held)
			return
		}
		t3 := time.Now()
		g.win.SwapBuffers()
		tSwap := time.Now()
		if g.frameLimit > 0 {
			// Sleep most of the remainder, spin the last bit for precision.
			end := st.frameStart.Add(g.frameLimit)
			if d := time.Until(end) - 500*time.Microsecond; d > 0 {
				osSleep(d)
			}
			for time.Now().Before(end) {
			}
		}
		t4 := time.Now()
		if cost := t3.Sub(t0); g.tick > 40 {
			st.costMax = max(st.costMax, cost)
			if cost > 12*time.Millisecond {
				st.slowCosts++
			}
		}
		for i, d := range []time.Duration{t1.Sub(t0), t2.Sub(t1), t3.Sub(t2), tSwap.Sub(t3), t4.Sub(tSwap)} {
			st.phase[i] = max(st.phase[i], d)
		}

		frames++
		if time.Since(fpsTime) >= 500*time.Millisecond {
			fps := int(float64(frames) / time.Since(fpsTime).Seconds())
			frames, fpsTime = 0, time.Now()
			g.updateTitle(fps)
		}
	}
}

func (g *game) updateTitle(fps int) {
	st := &g.stats
	s := g.eng.Renderer.Stats
	p := g.player
	loaded, pc, pm := g.eng.Progress()
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
	name := strings.TrimPrefix(g.eng.World.Reg.Info(g.hotbar[g.selected]).Name, "minecraft:")
	held := g.held
	if held == "" {
		held = "-"
	}
	title := fmt.Sprintf("voxellib explorer (F1 help) | %d fps | %.1f %.1f %.1f %s | [%d] %s | hand %s | vis %d/%d sec, %d calls, %dk quads, %d MB | chunks %d (+%d) meshes %d | worst frame cpu %.1fms gpu %.1fms (cpu >12ms: %d) interval %.1fms",
		fps, p.X, p.Y, p.Z, mode, g.selected+1, name, held, s.Visible, s.Sections, s.DrawCalls, s.Quads/1000,
		s.VertexMemory>>20, loaded, pc, pm, ms(st.costMax), ms(st.gpuMax), st.slowCosts, ms(st.maxFrame))
	if g.debugMsg != "" {
		title += " | anim " + g.debugMsg
	}
	if g.verbose {
		title += fmt.Sprintf(" | tick %.1f upd %.1f rend %.1f swap %.1f pace %.1f", ms(st.phase[0]), ms(st.phase[1]), ms(st.phase[2]), ms(st.phase[3]), ms(st.phase[4]))
		for i, n := range engine.TimingNames {
			title += fmt.Sprintf(" %s %.1f", n, ms(g.eng.Timing[i]))
		}
		g.eng.Timing = [len(engine.TimingNames)]time.Duration{}
		fmt.Println(title)
	}
	st.maxFrame, st.phase, st.costMax, st.gpuMax = 0, [5]time.Duration{}, 0, 0
	g.win.SetTitle(title)
}

// gpuTimer measures f's GPU time with a TIME_ELAPSED query, reading the
// result of a query issued 3 frames ago so the CPU never waits for it.
func (g *game) gpuTimer(f func()) {
	st := &g.stats
	if st.tq[0] == 0 {
		gl.GenQueries(int32(len(st.tq)), &st.tq[0])
	}
	st.fq++
	qi := st.fq % len(st.tq)
	if st.fq > len(st.tq) && g.tick > 40 {
		var ns uint64
		gl.GetQueryObjectui64v(st.tq[(qi+1)%len(st.tq)], gl.QUERY_RESULT, &ns)
		st.gpuMax = max(st.gpuMax, time.Duration(ns))
	}
	gl.BeginQuery(gl.TIME_ELAPSED, st.tq[qi])
	f()
	gl.EndQuery(gl.TIME_ELAPSED)
}

// shotDir is where F2 screenshots go.
const shotDir = "screenshots"

// saveDebugShot (F2) saves the frame as screenshots/<timestamp>.png with a
// .txt next to it: position, look and an explorer command that restores
// the view. Encoding runs in the background so the frame doesn't hitch.
func (g *game) saveDebugShot(w, h int) {
	if err := os.MkdirAll(shotDir, 0o755); err != nil {
		log.Print(err)
		return
	}
	base := filepath.Join(shotDir, time.Now().Format("2006-01-02_15-04-05.000"))
	img := readFrame(w, h)
	p := g.player
	yaw, pitch := g.cam.Yaw*180/math.Pi, g.cam.Pitch*180/math.Pi
	info := fmt.Sprintf("pos %.2f,%.2f,%.2f\nlook %.1f,%.1f\nflying %v\nheld %q\ntick %d\n"+
		"repro: ./explorer -pos %.2f,%.2f,%.2f -look %.1f,%.1f -hold %q%s\n",
		p.X, p.Y, p.Z, yaw, pitch, p.Flying, g.held, g.tick,
		p.X, p.Y, p.Z, yaw, pitch, g.held, map[bool]string{true: " -fly"}[p.Flying])
	go func() {
		if err := writePNG(base+".png", img); err != nil {
			log.Print(err)
			return
		}
		os.WriteFile(base+".txt", []byte(info), 0o644)
		log.Printf("screenshot saved: %s.png", base)
	}()
}

func saveShot(path string, w, h int) {
	if err := writePNG(path, readFrame(w, h)); err != nil {
		log.Print(err)
	}
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// readFrame reads the back buffer into an opaque, top-down image.
func readFrame(w, h int) *image.NRGBA {
	px := make([]byte, w*h*4)
	gl.ReadPixels(0, 0, int32(w), int32(h), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		copy(img.Pix[y*w*4:(y+1)*w*4], px[(h-1-y)*w*4:(h-y)*w*4])
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	return img
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
