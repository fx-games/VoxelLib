package main

// Zombies: the demo's undead come in waves. A zombie is the vanilla biped
// (mp.Body) in the pack's zombie skin, posed as ZombieModel, always hostile
// and never giving up; it fights through combat.go's melee (no sword, a
// bare swing). A zombie killed while a gun is held drops a magazine's worth
// of that gun's ammunition, picked up into the stock by walking over it.
// Hooks elsewhere: setupZombies after the guns, tickWaves after the
// creatures, poseCreature for the pose, kill for the loot, tickItems for
// the pickup.

import (
	"fmt"
	"image/png"
	"log"
	"math"
	"os"
	"strings"

	"github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/mobs"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/mp"
)

const (
	zombieSkinFile = "assets/pixelperfection/assets/minecraft/textures/entity/zombie/zombie.png"
	// A wave of n holds firstWave + (n-1)·waveGrowth zombies, appearing
	// waveNear..waveFar blocks away; the next one starts waveRest ticks
	// after the last falls.
	firstWave  = 4
	waveGrowth = 2
	waveNear   = 16
	waveFar    = 28
	waveRest   = 5 * 20
	// zombieSwingTicks is how long a blow takes (the reference's swing
	// duration), zombieAttackTicks the whole attack (blow and recovery).
	zombieSwingTicks  = 6
	zombieAttackTicks = 10
	// ammoItem prefixes the item name of an ammunition pickup ("ammo:" + Ammo.ID).
	ammoItem = "ammo:"
)

type wavesState struct {
	zombie  *species // nil: no waves
	wave    int      // the wave running (0 before the first)
	toSpawn int      // zombies of this wave still to appear
	rest    int      // ticks until the next wave
	// ammoLoot is the rounds an ammunition pickup holds by Ammo.ID: the
	// largest magazine taking it.
	ammoLoot map[string]int
}

// setupZombies builds the zombie species and registers the ammo pickups.
func (g *game) setupZombies() {
	if err := g.loadZombie(); err != nil {
		log.Printf("zombies disabled: %v", err)
	}
	if g.gunPack == nil {
		return
	}
	g.ammoLoot = map[string]int{}
	for _, gun := range g.gunPack.Guns {
		g.ammoLoot[gun.Ammo] = max(g.ammoLoot[gun.Ammo], gun.Mag, 1)
	}
	for _, a := range g.gunPack.Ammo {
		img, err := g.gunPack.Icon(a.Icon)
		if err != nil {
			log.Printf("ammo %s: %v", a.ID, err)
			continue
		}
		g.addItem(ammoItem+a.ID, img)
		g.unscattered = append(g.unscattered, ammoItem+a.ID)
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
	m.Tags = map[string]bool{"humanoid": true}
	h, err := g.Models.Add(m)
	if err != nil {
		return err
	}
	g.zombie = &species{model: m, handle: h, kind: mobs.Walker, width: 0.6, height: 1.95, body: body}
	return nil
}

// tickWaves starts waves and lets their zombies appear.
func (g *game) tickWaves() {
	if g.zombie == nil || !g.Ready() {
		return
	}
	alive := 0
	for _, c := range g.creatures {
		if c.sp == g.zombie && c.dying == 0 {
			alive++
		}
	}
	if alive == 0 && g.toSpawn == 0 {
		if g.rest > 0 {
			g.rest--
			return
		}
		g.wave++
		g.toSpawn, g.rest = firstWave+(g.wave-1)*waveGrowth, waveRest
		g.debugMsg = fmt.Sprintf("wave %d: %d zombies", g.wave, g.toSpawn)
	}
	for tries := 0; g.toSpawn > 0 && tries < 8; tries++ {
		x, y, z, ok := g.spawnSpot(waveNear, waveFar)
		if !ok {
			continue
		}
		m := mobs.New(mobs.Walker, x, y, z, g.zombie.width, g.zombie.height)
		m.Yaw = g.mobRand.Float64() * 2 * math.Pi
		c := &creature{sp: g.zombie, mob: m, anim: model.NewAnimator(g.zombie.model)}
		g.addCreature(c)
		g.provoke(c)
		g.toSpawn--
	}
	for _, c := range g.creatures {
		if c.sp == g.zombie && c.dying == 0 {
			c.prevLimb = c.limb
			c.limb.Walk(c.mob.X-c.mob.PrevX, c.mob.Z-c.mob.PrevZ)
			c.age++
		}
	}
}

// zombieSkin poses a zombie between the last two ticks.
func (c *creature) zombieSkin(alpha float64) {
	m := c.mob
	attack := 0.0
	if m.State == mobs.Attack {
		attack = min((float64(m.StateTicks)+alpha)/zombieSwingTicks, 1)
	}
	c.skin = c.sp.body.ZombieSkin(c.prevLimb.Lerp(c.limb, alpha), m.Hostile, attack, float64(c.age)+alpha, c.skin)
}

// dropAmmo leaves a magazine of the held gun's ammunition where a zombie fell.
func (g *game) dropAmmo(c *creature) {
	if c.sp != g.zombie || !g.gunOn() || g.itemRend == nil {
		return
	}
	if item := ammoItem + g.gun.Gun.Ammo; g.itemRend.Has(item) {
		m := c.mob
		g.drops.Spawn(item, m.X, m.Y+0.5, m.Z)
	}
}

// takeAmmo moves touched ammunition pickups into the stock.
func (g *game) takeAmmo() {
	g.drops.Take(g.Player.Box(), func(item string) bool {
		id, ok := strings.CutPrefix(item, ammoItem)
		if !ok {
			return false
		}
		g.stock[id] += max(g.ammoLoot[id], 1)
		return true
	})
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
