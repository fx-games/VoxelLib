package main

import (
	"math"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
)

// Debug keys (also listed in README.md):
//
//	F / double Space  toggle flying      G  toggle no-clip (fly through blocks)
//	N                 day/night          R  respawn at the spawn point
//	Q                 drop held item     F3 toggle frame stats on stdout
//	F1                help screen (help.go; keep helpLines in sync)
//	K                 preview the nearest creature's animations one by one
func (g *game) setupInput(grab bool) {
	if grab {
		g.capture(true)
	}
	g.win.SetKeyCallback(func(w *glfw.Window, k glfw.Key, sc int, a glfw.Action, m glfw.ModifierKey) {
		if a != glfw.Press {
			return
		}
		p := g.player
		switch {
		case k == glfw.KeyEscape:
			if g.captured {
				g.capture(false)
			} else {
				w.SetShouldClose(true)
			}
		case k == glfw.KeyF:
			p.ToggleFly()
		case k == glfw.KeyG:
			p.NoClip = !p.NoClip
			if p.NoClip {
				p.Flying = true
			}
		case k == glfw.KeyN:
			if g.eng.Renderer.Opt.Daylight > 0.5 {
				g.eng.Renderer.Opt.Daylight = 0.2
			} else {
				g.eng.Renderer.Opt.Daylight = 1
			}
		case k == glfw.KeyR:
			p.Teleport(g.spawnX, g.spawnY, g.spawnZ)
			g.findGround = true
		case k == glfw.KeyQ:
			g.dropHeld()
		case k == glfw.KeyF1:
			g.toggleHelp()
		case k == glfw.KeyK:
			g.cycleAnimation()
		case k == glfw.KeyF2:
			g.shotReq = true
		case k == glfw.KeyF3:
			g.verbose = !g.verbose
		case k == glfw.KeySpace:
			if time.Since(g.lastJump) < 300*time.Millisecond && !p.InWater {
				p.ToggleFly()
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
		if a != glfw.Press {
			return
		}
		if !g.captured {
			g.capture(true)
			return
		}
		switch b {
		case glfw.MouseButtonLeft:
			g.breakBlock()
		case glfw.MouseButtonRight:
			g.placeBlock()
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
