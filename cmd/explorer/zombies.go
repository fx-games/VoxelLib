package main

// Zombie defense on the client (the rules and the simulation are the
// server's: mp/zdefense.go, mp/server_zombies.go; single player is a
// private server). Here: the zombie model (the vanilla biped in the pack's
// zombie skin, posed as ZombieModel), the zombies drawn from the snapshots
// with the players' hurt flash and death fall, and the HUD line pointing
// at the nearest pickup (what a pickup gives: inventory.go).

import (
	"fmt"
	"image/png"
	"log"
	"math"
	"os"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/actors"
	"github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/voxel/render"
)

const zombieSkinFile = "assets/pixelperfection/assets/minecraft/textures/entity/zombie/zombie.png"

// zombieState is the zombie model and the pickups' sprites.
type zombieState struct {
	zombieBody  *mp.Body
	zombieModel int // ModelRenderer handle; -1: none
}

// remoteZombie is a snapshot zombie's drawing state.
type remoteZombie struct {
	mp.Anim
	x, z float64
	// hurt counts down the red flash (ticks, from actors.MaxHurtTime on an
	// mp.Hit); dying counts ticks since the death was seen.
	hurt, dying float64
	skin        []mgl32.Mat4
	seen        bool // in this frame's snapshot
}

// setupZombies builds the zombie model (the pickups' sprites are the
// items', setupItems).
func (g *game) setupZombies() {
	g.zombieModel = -1
	if err := g.loadZombie(); err != nil {
		log.Printf("zombies not drawn: %v", err)
	}
}

func (g *game) loadZombie() error {
	f, err := os.Open(zombieSkinFile)
	if err != nil {
		return err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("%s: %w", zombieSkinFile, err)
	}
	body := mp.NewBody()
	bipedLimbs(body.Geo)
	m, err := cuboid.Build(body.Geo, img)
	if err != nil {
		return err
	}
	m.Name = "zombie"
	h, err := g.Models.Add(m)
	if err != nil {
		return err
	}
	g.zombieBody, g.zombieModel = body, h
	return nil
}

// poseZombies poses the snapshot zombies for this frame (netPose).
func (g *game) poseZombies() {
	n := g.net
	n.zinsts = n.zinsts[:0]
	n.zviews = n.cl.Zombies(n.zviews)
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, rz := range n.zombies {
		rz.seen = false
	}
	for _, v := range n.zviews {
		rz := n.zombies[v.ID]
		if rz == nil {
			rz = &remoteZombie{x: v.X, z: v.Z}
			n.zombies[v.ID] = rz
		}
		rz.seen = true
		rz.hurt = max(0, rz.hurt-n.frameTicks)
		rz.Advance(v.X-rz.x, v.Z-rz.z, n.frameTicks)
		rz.x, rz.z = v.X, v.Z
		tilt := 0.0
		if v.Dead {
			rz.dying += n.frameTicks
			tilt = deathTiltAt(rz.dying)
		}
		if g.zombieModel < 0 {
			continue
		}
		attack := 0.0
		if v.Attack >= 0 {
			attack = min(float64(v.Attack)/mp.ZombieSwingTicks, 1)
		}
		rz.skin = g.zombieBody.ZombieSkin(rz.Anim, v.Hostile, attack, n.cl.RenderTime(), rz.skin)
		sky, blk := g.Light(v.X, v.Y+1, v.Z)
		in := render.ModelInstance{Model: g.zombieModel, X: v.X, Y: v.Y, Z: v.Z, Yaw: v.Yaw, Tilt: tilt, TiltYaw: v.Yaw - math.Pi/2,
			Skin: rz.skin, Sky: sky, Block: blk}
		if rz.hurt > 0 || v.Dead { // getOverlayCoords: hurtTime > 0 || deathTime > 0
			in.Overlay = hurtOverlay
		}
		n.zinsts = append(n.zinsts, in)
	}
	for id, rz := range n.zombies {
		if !rz.seen {
			delete(n.zombies, id)
		}
	}
}

// zombieHurt starts a zombie's hurt flash (an mp.Hit on it).
func (g *game) zombieHurt(id uint32) {
	n := g.net
	n.mu.Lock()
	if rz := n.zombies[id]; rz != nil {
		rz.hurt = actors.MaxHurtTime
	}
	n.mu.Unlock()
}

func (g *game) ammoName(id string) string {
	for _, a := range g.gunPack.Ammo {
		if a.ID == id && a.Name != "" {
			return a.Name
		}
	}
	return id
}

// note shows a line in the kill feed.
func (g *game) note(s string) {
	if g.ui != nil {
		g.ui.HUD.Message(s)
	}
}

// compass names the direction from the camera's look to a point: ahead,
// behind, left or right.
func compass(yaw, dx, dz float64) string {
	a := math.Remainder(math.Atan2(dx, -dz)-yaw, 2*math.Pi)
	switch {
	case math.Abs(a) < math.Pi/4:
		return "ahead"
	case math.Abs(a) > 3*math.Pi/4:
		return "behind"
	case a < 0:
		return "left"
	}
	return "right"
}

// wantedPickup is the pickup to fetch: the nearest ammunition one of the
// owned guns takes, else the nearest gun.
func (g *game) wantedPickup() (mp.PickupView, bool) {
	takes := map[string]bool{}
	for _, st := range g.inv().Main {
		if st == nil {
			continue
		}
		if spec := g.gunSpec(st.Item.Key); spec != nil {
			takes[spec.Ammo] = true
		}
	}
	var gun, ammo mp.PickupView
	gd, ad := math.Inf(1), math.Inf(1)
	for _, pk := range g.net.cl.Pickups() {
		d := math.Hypot(pk.X-g.Player.X, pk.Z-g.Player.Z)
		switch {
		case pk.Kind == mp.PickupGun && d < gd:
			gun, gd = pk, d
		case pk.Kind == mp.PickupAmmo && takes[pk.Pickup.ID] && d < ad:
			ammo, ad = pk, d
		}
	}
	if !math.IsInf(ad, 1) {
		return ammo, true
	}
	return gun, !math.IsInf(gd, 1)
}

// defenseLine is zombie defense's top line: the zombies about, and where
// the wanted pickup lies.
func (g *game) defenseLine() string {
	n := g.net
	alive := 0
	for _, v := range n.zviews {
		if !v.Dead {
			alive++
		}
	}
	l := fmt.Sprintf("§cZombies: %d", alive)
	if pk, ok := g.wantedPickup(); ok {
		what := "gun"
		if pk.Kind == mp.PickupAmmo {
			what = "ammo"
		}
		p := g.Player
		l += fmt.Sprintf("§r  Nearest %s: %.0f m %s", what, math.Hypot(pk.X-p.X, pk.Z-p.Z), compass(g.Cam.Yaw, pk.X-p.X, pk.Z-p.Z))
	}
	return l
}

// bipedLimbs gives a player geometry ModelBiped's left limbs, which the
// zombie skin has no own texture for: the right limb's UV, mirrored.
func bipedLimbs(g *cuboid.Geometry) {
	right := map[string]string{"leftArm": "rightArm", "leftLeg": "rightLeg"}
	uv := map[string][2]int{}
	for _, p := range g.Parts {
		if len(p.Boxes) > 0 {
			uv[p.Name] = p.Boxes[0].UV
		}
	}
	for i := range g.Parts {
		if r, ok := right[g.Parts[i].Name]; ok {
			b := &g.Parts[i].Boxes[0]
			b.UV, b.Mirror = uv[r], true
		}
	}
}
