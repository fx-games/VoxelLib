package projectile

import (
	"math"
	"math/rand"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/physics"
	"github.com/memmaker/voxellib/weapon"
)

// Bullet is TACZ's EntityKineticBullet: a gun's round flying tick by tick
// with its own speed, gravity, friction and life, hitting blocks and the
// targets along each tick's segment. The server (or single player) runs
// Tick with a BulletCollider; a client drawing a bullet from its spawn
// runs Tick without one (TACZ's client-side entity tick: motion only).
type Bullet struct {
	Pos, Prev, Vel [3]float64 // Prev: Pos before the last tick (drawing)
	Start          [3]float64 // startPos: damage falls off with the distance from here
	Gravity        float64    // blocks/tick²
	Friction       float64    // fraction of the velocity lost per tick
	Life           int        // ticks
	Age            int        // tickCount
	Pierce         int        // targets it can pass through, at least 1
	Tracer         bool
	Gone           bool
	// Yaw, Pitch are the model's heading (radians, atan2(x, z) and up),
	// eased toward the velocity each tick (lerpRotation); PrevYaw,
	// PrevPitch the last tick's (yRotO, xRotO), for drawing between ticks.
	Yaw, Pitch, PrevYaw, PrevPitch float64
	// Gun is the shooter's spec when fired: damage, falloff, head shot,
	// knockback. Nil on a client (a bullet drawn from its spawn).
	Gun *weapon.Gun
	// DamageScale is applyShotgunDamageSpread: 1/pellets for a shotgun round.
	DamageScale float32
}

const (
	// inaccuracyScale is Projectile.shoot's spread per unit of inaccuracy
	// (a triangle distribution on each axis of the unit direction).
	inaccuracyScale = 0.0172275
	// In water the friction is waterFriction and gravity scaled by waterGravity.
	waterFriction = 0.4
	waterGravity  = 0.6
	// TicksPerSecond converts BulletData's speed and life.
	TicksPerSecond = 20
)

// NewBullet is EntityKineticBullet's constructor and shootFromRotation:
// at the spawn point (BulletSpawn), along the unit look dir with the
// gun's speed, turned by the vanilla spread for inaccuracy (rng's three
// triangle samples, x, y then z).
//
// The shooter's own motion is not added: TACZ adds the server-side
// player's deltaMovement, which does not hold the client's walking.
func NewBullet(at, dir [3]float64, g *weapon.Gun, inaccuracy float64, tracer bool, rng *rand.Rand) *Bullet {
	b := &Bullet{Pos: at, Prev: at, Start: at, Gun: g, Tracer: tracer, DamageScale: 1,
		Gravity: max(float64(g.Bullet.Gravity), 0), Friction: max(float64(g.Bullet.Friction), 0),
		Life: max(int(g.Bullet.Life*TicksPerSecond), 1), Pierce: max(g.Bullet.Pierce, 1)}
	if g.Bullet.Pellets > 1 {
		b.DamageScale = 1 / float32(g.Bullet.Pellets)
	}
	speed := max(float64(g.Bullet.Speed)/TicksPerSecond, 0)
	n := norm(dir)
	tri := func() float64 { return inaccuracyScale * inaccuracy * (rng.Float64() - rng.Float64()) }
	for i := range 3 {
		n[i] += tri()
	}
	for i := range 3 {
		b.Vel[i] = n[i] * speed
	}
	b.Face()
	return b
}

// rotationEase is Projectile.lerpRotation's step toward the velocity's heading.
const rotationEase = 0.2

// heading is the yaw and pitch of v (EntityKineticBullet.tick, shoot).
func heading(v [3]float64) (yaw, pitch float64) {
	return math.Atan2(v[0], v[2]), math.Atan2(v[1], math.Hypot(v[0], v[2]))
}

// Face turns a new bullet to its velocity, this tick and the last (shoot,
// or the spawn packet's rotation on a client).
func (b *Bullet) Face() {
	b.Yaw, b.Pitch = heading(b.Vel)
	b.PrevYaw, b.PrevPitch = b.Yaw, b.Pitch
}

// lerpRotation is Projectile.lerpRotation: a step of rotationEase from a
// toward b the short way round.
func lerpRotation(a, b float64) float64 {
	for b-a < -math.Pi {
		a -= 2 * math.Pi
	}
	for b-a >= math.Pi {
		a += 2 * math.Pi
	}
	return a + (b-a)*rotationEase
}

// LerpRotation is the heading between ticks (the renderer's Mth.lerp of
// yRotO..yRot and xRotO..xRot).
func (b *Bullet) LerpRotation(alpha float64) (yaw, pitch float64) {
	return b.PrevYaw + (b.Yaw-b.PrevYaw)*alpha, b.PrevPitch + (b.Pitch-b.PrevPitch)*alpha
}

// BulletSpawn is where a shooter's bullet starts: halfway between the
// feet's position at the last tick and now, at eye height (the
// constructor's posX/posY/posZ).
func BulletSpawn(prev, now [3]float64, eyeHeight float64) [3]float64 {
	return [3]float64{prev[0] + (now[0]-prev[0])/2, prev[1] + (now[1]-prev[1])/2 + eyeHeight, prev[2] + (now[2]-prev[2])/2}
}

// Shooter is a shooter's tracer count (ShooterDataHolder.shootCount),
// reset when a gun is drawn.
type Shooter struct{ Shots int }

// NextIsTracer counts a bullet and reports whether it is a tracer
// (nextBulletIsTracer).
func (s *Shooter) NextIsTracer(interval int) bool {
	s.Shots++
	return interval != weapon.NoTracer && s.Shots%(interval+1) == 0
}

// Fire is one round of g (shootOnce's bullet loop): bullet_amount
// bullets from at along the look dir, each rolled its own spread.
func Fire(at, dir [3]float64, g *weapon.Gun, inaccuracy float64, s *Shooter, rng *rand.Rand) []*Bullet {
	spec := *g
	g = &spec // the spec when fired: a later re-resolve (fire select) leaves the round as it was
	n := max(g.Bullet.Pellets, 1)
	out := make([]*Bullet, 0, n)
	for range n {
		tracer := g.Bullet.TracerInterval != weapon.NoTracer && s.NextIsTracer(g.Bullet.TracerInterval)
		out = append(out, NewBullet(at, dir, g, inaccuracy, tracer, rng))
	}
	return out
}

// BulletWorld is what a bullet flies through: blocks and water.
type BulletWorld interface {
	physics.Blocks
	Liquid(x, y, z int) character.Liquid
}

// BulletTarget is an entity a bullet can hit this tick.
type BulletTarget struct {
	// Box is HitboxHelper.getFixedBoundingBox (see HitBox).
	Box collide.AABB
	// Feet is entity.position(), EyeHeight its eye above it: a hit within
	// headBand of the eye height is a head shot.
	Feet      [3]float64
	EyeHeight float64
}

// headBand is EntityUtil.getHitResult's default head zone: the eye height ± this.
const headBand = 0.25

// HitboxHelper's box adjustments.
const (
	playerHitboxGrow   = 0.0625 // a standing player's box is this much taller
	playerHitboxOffset = 3      // OtherConfig.SERVER_HITBOX_OFFSET's default: a player's box moves this many ticks of its velocity ahead
	hitboxLag          = -5     // every box moves back this many ticks of its velocity
)

// HitBox is HitboxHelper.getFixedBoundingBox: an entity's box (a
// player's as the shooter saw it) grown up a little for a standing
// player, stretched along its velocity (blocks per tick) and moved
// against it.
func HitBox(box collide.AABB, vel [3]float64, player, crouching bool) collide.AABB {
	if player && !crouching {
		box.MaxY += playerHitboxGrow
	}
	box = box.Expand(vel[0], vel[1], vel[2])
	k := float64(hitboxLag)
	if player {
		k += playerHitboxOffset
	}
	return box.Offset(vel[0]*k, vel[1]*k, vel[2]*k)
}

// BulletHit is a target a bullet hit.
type BulletHit struct {
	Target int        // index into the tick's targets
	Pos    [3]float64 // where it entered the box
	Head   bool
}

// BulletCollider is the server side of a bullet's tick (onBulletTick).
type BulletCollider struct {
	Boxes   physics.BoxFunc
	Targets []BulletTarget
	// Hit is called for each target hit, nearest first; Block for the
	// block that stopped the bullet.
	Hit   func(b *Bullet, h BulletHit)
	Block func(b *Bullet, h physics.Hit)
}

// Tick runs one tick of b in w (EntityKineticBullet.tick): with c, the
// collisions along this tick's segment first; then the move, friction
// and gravity, and the end of its life.
func (b *Bullet) Tick(w BulletWorld, c *BulletCollider) {
	if b.Gone {
		return
	}
	b.Age++
	if c != nil {
		if b.collide(c, w); b.Gone {
			return
		}
	}
	// baseTick keeps the last rotation; the heading of this tick's motion
	// is eased toward (EntityKineticBullet.tick).
	b.PrevYaw, b.PrevPitch = b.Yaw, b.Pitch
	yaw, pitch := heading(b.Vel)
	b.Yaw, b.Pitch = lerpRotation(b.PrevYaw, yaw), lerpRotation(b.PrevPitch, pitch)
	b.Prev = b.Pos
	for i := range 3 {
		b.Pos[i] += b.Vel[i]
	}
	friction, gravity := b.Friction, b.Gravity
	if w.Liquid(int(math.Floor(b.Pos[0])), int(math.Floor(b.Pos[1])), int(math.Floor(b.Pos[2]))) == character.LiquidWater {
		friction, gravity = waterFriction, gravity*waterGravity
	}
	for i := range 3 {
		b.Vel[i] *= 1 - friction
	}
	b.Vel[1] -= gravity
	if b.Age >= b.Life-1 {
		b.Gone = true
	}
}

// collide is onBulletTick: the block along the segment shortens it; every
// target on what is left is hit nearest first until the pierce runs out
// (one target only at pierce 1); a bullet still flying stops at the block.
func (b *Bullet) collide(c *BulletCollider, w BulletWorld) {
	start, seg := b.Pos, b.Vel
	l := math.Sqrt(seg[0]*seg[0] + seg[1]*seg[1] + seg[2]*seg[2])
	block, blocked := physics.Hit{}, false
	if l > 0 {
		block, blocked = physics.RaycastBox(w, start[0], start[1], start[2], seg[0], seg[1], seg[2], l, c.Boxes)
		if blocked {
			for i := range 3 {
				seg[i] *= block.Dist / l
			}
		}
	}
	var hits []BulletHit
	var dists []float64
	for i, t := range c.Targets {
		f, n, ok := collide.RayAABB(start, seg, t.Box)
		if !ok || f > 1 || f == 0 && n == ([3]float64{}) {
			continue // missed, or started inside (AABB.clip)
		}
		p := [3]float64{start[0] + seg[0]*f, start[1] + seg[1]*f, start[2] + seg[2]*f}
		y := p[1] - t.Feet[1]
		hits = append(hits, BulletHit{Target: i, Pos: p, Head: t.EyeHeight-headBand < y && y < t.EyeHeight+headBand})
		dists = append(dists, f)
	}
	if b.Pierce <= 1 && len(hits) > 1 { // findEntityOnPath: the nearest only
		k := 0
		for i := range hits {
			if dists[i] < dists[k] {
				k = i
			}
		}
		hits = hits[k : k+1]
	}
	// Sorted nearest first (the bullet's selection sort over the first pierce entries).
	for i := 0; (i < b.Pierce || i < 1) && i < len(hits)-1; i++ {
		k := i
		for j := i + 1; j < len(hits); j++ {
			if dists[j] < dists[k] {
				k = j
			}
		}
		hits[i], hits[k], dists[i], dists[k] = hits[k], hits[i], dists[k], dists[i]
	}
	for _, h := range hits {
		if c.Hit != nil {
			c.Hit(b, h)
		}
		if b.Pierce--; b.Pierce < 1 {
			b.Gone = true
			return
		}
	}
	if blocked {
		if c.Block != nil {
			c.Block(b, block)
		}
		b.Gone = true
	}
}

// Damage is getDamage at a hit point: the gun's falloff at the distance
// from Start, scaled by DamageScale, times the head shot multiplier on a
// head hit (EntityKineticBullet.onHitEntity).
func (b *Bullet) Damage(at [3]float64, head bool) float32 {
	d := math.Sqrt(sq(at[0]-b.Start[0]) + sq(at[1]-b.Start[1]) + sq(at[2]-b.Start[2]))
	dmg := max(b.Gun.DamageAt(float32(d))*b.DamageScale, 0)
	if head {
		dmg *= max(b.Gun.Bullet.Headshot, 0)
	}
	return dmg
}

// Lerp is the bullet's position between the last two ticks.
func (b *Bullet) Lerp(alpha float64) [3]float64 {
	return [3]float64{b.Prev[0] + (b.Pos[0]-b.Prev[0])*alpha, b.Prev[1] + (b.Pos[1]-b.Prev[1])*alpha, b.Prev[2] + (b.Pos[2]-b.Prev[2])*alpha}
}

func sq(v float64) float64 { return v * v }

func norm(v [3]float64) [3]float64 {
	l := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if l == 0 {
		return v
	}
	return [3]float64{v[0] / l, v[1] / l, v[2] / l}
}
