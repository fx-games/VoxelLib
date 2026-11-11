package main

// -autoplay plays zombie defense by itself, for scripted test runs: it
// walks to the nearest gun (then to ammunition when out of it), shoots the
// nearest zombie in range, reloads, and logs each change of plan. It drives
// the game through Input.Hold and the camera, as a player would.

import (
	"log"
	"math"

	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/mp"
)

// autoplayRange is how near a zombie must be to be shot at (blocks).
const autoplayRange = 30

// autoplayChest is where on a zombie autoplay aims (blocks above the feet).
const autoplayChest = 1.4

// autoplay is the plan last logged.
type autoplay struct{ plan string }

func (g *game) tickAutoplay() {
	n := g.net
	if !g.opt.autoplay || n == nil {
		return
	}
	in := g.Input
	fire, walk, reload := false, false, false
	plan := "idle"
	switch {
	case n.cl.Own().Dead:
		plan = "dead"
	case g.gunOn() && g.gun.Ammo == 0 && g.ammoCount(g.gun.Gun.Ammo) > 0:
		plan, reload = "reload "+g.gun.Gun.ID, true
	default:
		if z, ok := g.nearestZombie(); ok && g.gunOn() && g.gun.Ammo > 0 {
			g.face(z.X, z.Y+autoplayChest, z.Z)
			plan, fire = "shoot a zombie", true
			break
		}
		if p, ok := g.wantedPickup(); ok {
			g.face(p.X, g.Cam.Y, p.Z)
			plan, walk = "walk to "+p.Sprite(), true
		}
	}
	in.Hold(app.ActForward, walk)
	in.Hold(app.ActAttack, fire && g.Tick%4 < 2) // presses: semi-automatic guns too
	in.Hold("reload", reload)
	if plan != g.autoplay.plan {
		log.Printf("autoplay: %s", plan)
		g.autoplay.plan = plan
	}
}

// face turns the camera to look at a point.
func (g *game) face(x, y, z float64) {
	dx, dy, dz := x-g.Cam.X, y-g.Cam.Y, z-g.Cam.Z
	g.Cam.Yaw = math.Atan2(dx, -dz)
	g.Cam.Pitch = math.Atan2(dy, math.Hypot(dx, dz))
}

func (g *game) nearestZombie() (mp.ZombieView, bool) {
	best, bd := mp.ZombieView{}, float64(autoplayRange)
	for _, z := range g.net.zviews {
		if d := math.Hypot(z.X-g.Player.X, z.Z-g.Player.Z); !z.Dead && d < bd {
			best, bd = z, d
		}
	}
	return best, bd < autoplayRange
}
