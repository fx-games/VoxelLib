package mp

// Zombie defense, the rules: players together against an endless stream of
// zombies, with the guns and ammunition they need lying around. Every
// SpawnInterval one zombie appears ZombieMinDist..ZombieMaxDist blocks from
// a random player, and one gun and one ammunition pickup
// PickupMinDist..PickupMaxDist away, each on loaded ground outside every
// player's view frustum. Zombies chase the nearest player; a zombie killed
// by a gun holder drops a magazine of that gun's ammunition. The server
// runs it (server_zombies.go); single player is a private server.

import (
	"math"
	"math/rand"

	"github.com/memmaker/voxellib/importer/tacz"
)

const (
	// SpawnInterval is the ticks between spawns.
	SpawnInterval = 15 * TickRate
	// A zombie appears this far from the player it spawns for (blocks,
	// horizontal), and at least ZombieMinDist from everyone.
	ZombieMinDist = 20
	ZombieMaxDist = 200
	// Pickups lie closer, so they can be found on foot (the HUD points
	// to the nearest).
	PickupMinDist = 8
	PickupMaxDist = 40
	// SpotTries is how many random spots a spawn tries before it waits
	// for the next interval.
	SpotTries = 64
	// pickupLifespan: a pickup disappears after ItemEntity's 6000 ticks.
	pickupLifespan = 6000
	// pickupRadius is a pickup's half size; players take it within their
	// box grown by pickupGrow (Player.aiStep's inflate(1, 0.5, 1)).
	pickupRadius = 0.125
	pickupGrowXZ = 1
	pickupGrowY  = 0.5
	// loadPerTick is how many unloaded columns the server generates per
	// tick around each player, out to ZombieMaxDist, so spawn spots exist.
	loadPerTick = 2
)

// The zombie (Zombie.createAttributes, its size and ZombieAttackGoal).
const (
	ZombieHealth      = 20
	ZombieDamage      = 3 // ATTACK_DAMAGE on normal difficulty
	ZombieWidth       = 0.6
	ZombieHeight      = 1.95
	ZombieEyeHeight   = 1.74
	ZombieReach       = 2.2 // where it starts a blow
	ZombieHitReach    = ZombieReach + 0.6
	ZombieCooldown    = 8
	ZombieAttackTicks = 10 // a whole attack, blow and recovery
	ZombieSwingTicks  = 6  // the blow (the arm swing's length)
	ZombieDeathTicks  = 20 // LivingEntity.tickDeath: removed this long after dying
)

// View is a player's view frustum: the eye, the look and the unzoomed
// projection (vertical field of view in radians, aspect ratio).
type View struct {
	Eye         [3]float64
	Look        Look
	FOV, Aspect float64
}

// defaultView is the projection assumed until a client reports its own
// (the app's 70° camera on a 16:9 window).
var defaultView = View{FOV: 70 * math.Pi / 180, Aspect: 16.0 / 9}

func (v View) valid() bool { return v.FOV > 0 && v.FOV < math.Pi && v.Aspect > 0.1 && v.Aspect < 10 }

// Sees reports whether a sphere at p of radius r is at least partly inside
// the frustum. It has no far plane: the frustum reaches past every spawn.
func (v View) Sees(p [3]float64, r float64) bool {
	f := v.Look.Dir()
	right := [3]float64{math.Cos(v.Look.Yaw), 0, math.Sin(v.Look.Yaw)}
	up := cross(right, f)
	d := [3]float64{p[0] - v.Eye[0], p[1] - v.Eye[1], p[2] - v.Eye[2]}
	z := dot(d, f)
	tv := math.Tan(v.FOV / 2)
	th := tv * v.Aspect
	// Each side plane passes through the eye; the sphere is outside when
	// its centre lies more than r beyond one.
	return z >= -r && math.Abs(dot(d, up))-z*tv <= r*math.Hypot(1, tv) && math.Abs(dot(d, right))-z*th <= r*math.Hypot(1, th)
}

func dot(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

// Seen reports whether a body of height h standing at feet is inside any
// of the views (tested as its bounding sphere).
func Seen(views []View, feet [3]float64, h float64) bool {
	c := [3]float64{feet[0], feet[1] + h/2, feet[2]}
	r := math.Hypot(h/2, 0.5)
	for _, v := range views {
		if v.Sees(c, r) {
			return true
		}
	}
	return false
}

// Ground is the terrain spots are picked on.
type Ground interface {
	// StandY is the feet height of a place to stand in column x,z; false
	// where the column is not loaded or has none (a tree top, water).
	StandY(x, z int) (y int, ok bool)
}

// PickSpot picks a random place to stand minD..maxD blocks (horizontal)
// from around, at least minD from every view's eye, where a body of
// height h is in none of the views. False after SpotTries misses.
func PickSpot(rng *rand.Rand, around [3]float64, minD, maxD, h float64, views []View, g Ground) ([3]float64, bool) {
	for range SpotTries {
		a := rng.Float64() * 2 * math.Pi
		r := minD + rng.Float64()*(maxD-minD)
		bx, bz := int(math.Floor(around[0]+math.Cos(a)*r)), int(math.Floor(around[2]+math.Sin(a)*r))
		y, ok := g.StandY(bx, bz)
		if !ok {
			continue
		}
		p := [3]float64{float64(bx) + 0.5, float64(y), float64(bz) + 0.5}
		if d := math.Hypot(p[0]-around[0], p[2]-around[2]); d < minD || d > maxD {
			continue
		}
		near := false
		for _, v := range views {
			near = near || math.Hypot(p[0]-v.Eye[0], p[2]-v.Eye[2]) < minD
		}
		if near || Seen(views, p, h) {
			continue
		}
		return p, true
	}
	return [3]float64{}, false
}

// PickupKind is what a pickup gives.
type PickupKind uint8

const (
	PickupGun  PickupKind = 1 // a gun (ID: weapon.Gun.ID) with its default attachments
	PickupAmmo PickupKind = 2 // Rounds of ammunition (ID: weapon.Ammo.ID)
)

// Pickup is what lies on the ground to be taken.
type Pickup struct {
	Kind   PickupKind
	ID     string
	Rounds int // PickupAmmo
}

// Sprite is the pickup's item sprite name (the client registers the
// pack's icons under it).
func (p Pickup) Sprite() string {
	if p.Kind == PickupGun {
		return "gun:" + p.ID
	}
	return "ammo:" + p.ID
}

func (p Pickup) encode(b *wbuf) {
	b.u8(uint8(p.Kind))
	b.str(p.ID)
	b.u16(uint16(p.Rounds))
}

func decodePickup(r *rbuf) Pickup {
	return Pickup{Kind: PickupKind(r.u8()), ID: r.str(), Rounds: int(r.u16())}
}

// Loot is what zombie defense spawns: every gun of the pack, every ammo
// type, and the rounds an ammunition pickup holds.
type Loot struct {
	Guns, Ammo []string
	// Rounds by ammo ID: the largest magazine taking it.
	Rounds map[string]int
}

// NewLoot lists a pack's guns and ammunition.
func NewLoot(p *tacz.Pack) Loot {
	l := Loot{Rounds: map[string]int{}}
	for _, g := range p.Guns {
		l.Guns = append(l.Guns, g.ID)
		l.Rounds[g.Ammo] = max(l.Rounds[g.Ammo], g.Mag, 1)
	}
	for _, a := range p.Ammo {
		l.Ammo = append(l.Ammo, a.ID)
		l.Rounds[a.ID] = max(l.Rounds[a.ID], 1)
	}
	return l
}

// Gun is a random gun pickup (false: the pack has none).
func (l Loot) Gun(rng *rand.Rand) (Pickup, bool) {
	if len(l.Guns) == 0 {
		return Pickup{}, false
	}
	return Pickup{Kind: PickupGun, ID: l.Guns[rng.Intn(len(l.Guns))]}, true
}

// RandomAmmo is a random ammunition pickup (false: the pack has none).
func (l Loot) RandomAmmo(rng *rand.Rand) (Pickup, bool) {
	if len(l.Ammo) == 0 {
		return Pickup{}, false
	}
	return l.Magazine(l.Ammo[rng.Intn(len(l.Ammo))]), true
}

// Magazine is an ammunition pickup of this type.
func (l Loot) Magazine(ammo string) Pickup {
	return Pickup{Kind: PickupAmmo, ID: ammo, Rounds: max(l.Rounds[ammo], 1)}
}

// ZombieView is a zombie as the snapshot shows it.
type ZombieView struct {
	ID      uint32
	X, Y, Z float64
	Yaw     float64
	Hostile bool // chasing a player: the arms raised higher
	Dead    bool
	Attack  int // ticks into a blow; -1 none
}

func (z ZombieView) encode() []byte {
	var b wbuf
	b.u8(uint8(kindZombie))
	b.f32(z.X)
	b.f32(z.Y)
	b.f32(z.Z)
	b.u16(angle16(z.Yaw))
	b.u8(flag(z.Hostile, 1) | flag(z.Dead, 2))
	b.u8(uint8(z.Attack + 1))
	return b
}

func decodeZombieView(id uint32, p []byte) (ZombieView, bool) {
	r := rbuf{p: p}
	if entityKind(r.u8()) != kindZombie {
		return ZombieView{}, false
	}
	z := ZombieView{ID: id, X: r.f32(), Y: r.f32(), Z: r.f32(), Yaw: unangle16(r.u16())}
	f := r.u8()
	z.Hostile, z.Dead = f&1 != 0, f&2 != 0
	z.Attack = int(r.u8()) - 1
	return z, r.err == nil
}

// PickupView is a pickup lying in the world.
type PickupView struct {
	ID uint32
	Pickup
	X, Y, Z float64
}

func (p PickupView) encode() []byte {
	var b wbuf
	b.u8(uint8(kindPickup))
	p.Pickup.encode(&b)
	b.f32(p.X)
	b.f32(p.Y)
	b.f32(p.Z)
	return b
}

func decodePickupView(id uint32, p []byte) (PickupView, bool) {
	r := rbuf{p: p}
	if entityKind(r.u8()) != kindPickup {
		return PickupView{}, false
	}
	v := PickupView{ID: id, Pickup: decodePickup(&r)}
	v.X, v.Y, v.Z = r.f32(), r.f32(), r.f32()
	return v, r.err == nil
}
