package main

import (
	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/platform/glfw"
)

// The explorer's keys on top of app.DefaultBindings (also listed in
// README.md and help.go's helpLines):
//
//	Ctrl+F            cheat: every gun and some ammo (zombie defense)
//	F / double Space  toggle flying      G  toggle no-clip (fly through blocks)
//	N                 day/night          R  respawn at the spawn point
//	F1                help screen        (doors: left or right click)
//	K                 preview the nearest creature's animations one by one
//	Tab (hold)        multiplayer ranking
//	(app: 1-9 / mouse wheel hotbar slot, the gun in hand; E inventory)
//	with a gun:       R reloads, H inspects, B fire mode, Z melee, X unloads,
//	                  J refits (refit.go), right click aims
func (g *game) bindKeys() {
	for name, k := range map[string]glfw.Key{"fly": glfw.KeyF, "noclip": glfw.KeyG, "daynight": glfw.KeyN,
		"respawn": glfw.KeyR, "help": glfw.KeyF1, "anim": glfw.KeyK, "scores": glfw.KeyTab,
		"reload": glfw.KeyR, "inspect": glfw.KeyH, "firemode": glfw.KeyB,
		"melee": glfw.KeyZ, "unload": glfw.KeyX, "refit": glfw.KeyJ} {
		g.Input.Bind(name, app.Key(k))
	}
}

// action handles the explorer's own actions.
func (g *game) action(name string, pressed bool) bool {
	if name == "scores" {
		g.showScores = pressed && g.net != nil
		return true
	}
	if !pressed {
		return false
	}
	if name == "fly" && g.Input.Down(app.ActSprint) && g.net != nil { // Ctrl+F: the gun cheat
		g.net.cl.SendCheatGuns()
		return true
	}
	p := g.Player
	if g.net != nil {
		switch name {
		case "fly", "noclip", "respawn", "daynight", "anim":
			return true // not in multiplayer (the time is the server's)
		}
	}
	if g.gunOn() {
		switch name {
		case app.ActAttack, app.ActUse, "respawn", "reload", "inspect", "firemode", "melee", "unload":
			return true // read by tickGun; R reloads instead of respawning
		}
	}
	switch name {
	case "refit":
		g.openRefit()
	case "reload", "inspect", "firemode", "melee", "unload":
		return false
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
	case "help":
		g.toggleHelp()
	case "anim":
		g.cycleAnimation()
	default:
		return false
	}
	return true
}
