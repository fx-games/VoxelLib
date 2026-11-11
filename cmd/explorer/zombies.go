package main

// Zombie defense on the client (the rules and the simulation are the
// server's: mp/zdefense.go, mp/server_zombies.go; single player is a
// private server). Here: the zombie model (the vanilla biped in the pack's
// zombie skin, posed as ZombieModel), the zombies drawn from the snapshots
// with the players' hurt flash and death fall, the pickup sprites, what a
// pickup gives, and the HUD line pointing at the nearest pickup.

import (
	"fmt"
	"image/png"
	"log"
	"math"
	"os"
	"slices"

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

// setupZombies builds the zombie model and registers the pickups' sprites:
// each gun's and ammunition's slot icon under mp.Pickup.Sprite.
func (g *game) setupZombies() {
	g.zombieModel = -1
	if err := g.loadZombie(); err != nil {
		log.Printf("zombies not drawn: %v", err)
	}
	if g.gunPack == nil {
		return
	}
	for _, gun := range g.gunPack.Guns {
		g.addIcon(mp.Pickup{Kind: mp.PickupGun, ID: gun.ID}, gun.Icon)
	}
	for _, a := range g.gunPack.Ammo {
		g.addIcon(mp.Pickup{Kind: mp.PickupAmmo, ID: a.ID}, a.Icon)
	}
}

func (g *game) addIcon(p mp.Pickup, file string) {
	img, err := g.gunPack.Icon(file)
	if err != nil {
		log.Printf("pickup %s: %v", p.Sprite(), err)
		return
	}
	g.addItem(p.Sprite(), img)
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

// arsenal is the guns the player has picked up, in pickup order (V cycles
// through them, then the bare hand).
type arsenal struct{ owned []string }

// add adds a gun; it reports whether to draw it: the hand is empty.
func (a *arsenal) add(id string, holding bool) (draw bool) {
	if !slices.Contains(a.owned, id) {
		a.owned = append(a.owned, id)
	}
	return !holding
}

// next is the gun after cur ("": none) in the arsenal, "" past the last.
func (a *arsenal) next(cur string) string {
	i := slices.Index(a.owned, cur) // -1 for none: the first
	if i+1 < len(a.owned) {
		return a.owned[i+1]
	}
	return ""
}

// takePickup gives the player what they picked up (mp.Client.OnPickup).
func (g *game) takePickup(p mp.Pickup) {
	switch p.Kind {
	case mp.PickupGun:
		spec := g.gunSpec(p.ID)
		if spec == nil {
			log.Printf("pickup: no gun %q in this pack", p.ID)
			return
		}
		if g.arsenal.add(p.ID, g.gunOn() || g.switching) {
			g.selectGunID(p.ID)
		}
		g.note("§aPicked up " + spec.Name)
	case mp.PickupAmmo:
		g.stock[p.ID] += p.Rounds
		g.note(fmt.Sprintf("§aPicked up %d rounds of %s", p.Rounds, g.ammoName(p.ID)))
	default:
		log.Printf("pickup: unknown kind %d", p.Kind)
	}
}

func (g *game) ammoName(id string) string {
	for _, a := range g.gunPack.Ammo {
		if a.ID == id && a.Name != "" {
			return a.Name
		}
	}
	return id
}

// loseArsenal empties the hands and the pockets (a death in zombie
// defense: back to bare hands).
func (g *game) loseArsenal() {
	g.takeGun(-1)
	g.arsenal = arsenal{}
	clear(g.stock)
	clear(g.loaded)
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

// defenseLine is zombie defense's top line: the zombies about, and where
// the nearest gun (or, holding guns, ammunition) pickup lies.
func (g *game) defenseLine() string {
	n := g.net
	alive := 0
	for _, v := range n.zviews {
		if !v.Dead {
			alive++
		}
	}
	l := fmt.Sprintf("§cZombies: %d", alive)
	want := mp.PickupGun
	if len(g.arsenal.owned) > 0 {
		want = mp.PickupAmmo
	}
	best, bd := mp.PickupView{}, math.Inf(1)
	p := g.Player
	for _, pk := range n.cl.Pickups() {
		if d := math.Hypot(pk.X-p.X, pk.Z-p.Z); pk.Kind == want && d < bd {
			best, bd = pk, d
		}
	}
	if !math.IsInf(bd, 1) {
		what := "gun"
		if want == mp.PickupAmmo {
			what = "ammo"
		}
		l += fmt.Sprintf("§r  Nearest %s: %.0f m %s", what, bd, compass(g.Cam.Yaw, best.X-p.X, best.Z-p.Z))
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
