package main

import (
	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/platform/glfw"
)

// The explorer's keys on top of app.DefaultBindings (also listed in
// README.md and help.go's helpLines):
//
//	F / double Space  toggle flying      G  toggle no-clip (fly through blocks)
//	N                 day/night          R  respawn at the spawn point
//	Q                 drop held item     E  open/close door
//	F1                help screen        C  crossbow
//	K                 preview the nearest creature's animations one by one
func (g *game) bindKeys() {
	for name, k := range map[string]glfw.Key{"fly": glfw.KeyF, "noclip": glfw.KeyG, "daynight": glfw.KeyN,
		"respawn": glfw.KeyR, "drop": glfw.KeyQ, "door": glfw.KeyE, "help": glfw.KeyF1, "anim": glfw.KeyK, "crossbow": glfw.KeyC} {
		g.Input.Bind(name, app.Key(k))
	}
}

// action handles the explorer's own actions, and left clicks while the
// crossbow is out.
func (g *game) action(name string, pressed bool) bool {
	if !pressed {
		return false
	}
	p := g.Player
	switch name {
	case app.ActAttack:
		if !g.crossbowOn() {
			return false
		}
		g.fire()
	case "door":
		if h, ent, ok := g.Target(); ok && ent == nil {
			g.toggleDoor(h.Block)
		}
	case "fly":
		p.ToggleFly()
	case "noclip":
		p.NoClip = !p.NoClip
		if p.NoClip {
			p.Flying = true
		}
	case "daynight":
		if g.Eng.Renderer.Opt.Daylight > 0.5 {
			g.Eng.Renderer.Opt.Daylight = 0.2
		} else {
			g.Eng.Renderer.Opt.Daylight = 1
		}
	case "respawn":
		p.Respawn()
	case "drop":
		g.dropHeld()
	case "help":
		g.toggleHelp()
	case "anim":
		g.cycleAnimation()
	case "crossbow":
		g.toggleCrossbow()
	default:
		return false
	}
	return true
}
