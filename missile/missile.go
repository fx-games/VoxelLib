// Package missile is reference-exact projectile physics for arrows, bolts,
// thrown weapons, stones and spells: the launch headings, the per-tick
// flight (block ray, entity sweep, sticking in blocks, drag, water,
// gravity), impact damage and knockback, and the pickup test.
//
// A Missile carries only physics. What it is (texture, damage factor,
// poison, what it drops) is the game's: the game calls Begin, resolves an
// entity or block hit with the helpers here, then calls End.
//
// Angles are the reference's: degrees, yaw 0 along +Z and growing towards
// -X, pitch positive up for missiles.
package missile

import (
	"math"

	"github.com/memmaker/voxellib/fmath"
	"github.com/memmaker/voxellib/particles"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/physics"
)

// Rand is the random source (math/rand/v2's *Rand is one).
type Rand interface {
	Float32() float32
	IntN(n int) int
	NormFloat64() float64
}

// World is what a missile reads of the world.
type World interface {
	Block(x, y, z int) voxel.StateID
	// Ray is the block ray trace from (x0, y0, z0) to (x1, y1, z1): the
	// first block whose ray boxes it meets; passNoBox also passes blocks
	// without a collision box (plants, torches).
	Ray(x0, y0, z0, x1, y1, z1 float64, passNoBox bool) (hx, hy, hz float64, h physics.Hit, ok bool)
	// Inside reports whether a point lies in the collision box of the
	// block at (x, y, z).
	Inside(x, y, z int, px, py, pz float64) bool
	// Water reports whether water touches the box.
	Water(b collide.AABB) bool
	// Spark spawns a particle ("crit", "bubble").
	Spark(name string, x, y, z, vx, vy, vz float64)
}

// Spec is a kind of missile's flight.
type Spec struct {
	Gravity float32 // taken off the vertical motion each tick, after drag
	Drag    float32 // motion kept per tick in air; water keeps WaterDrag
	// PassNoBox: the block ray passes blocks without a collision box.
	PassNoBox bool
	// Life is how many ticks a missile stays stuck in a block; 0: it
	// never sticks (the game ends it on impact).
	Life int
	// Classic is the base arrow's maths: a random sign on each heading
	// jitter and float literals (0.1F, 0.05F, 0.6F). Off, it is the
	// derived projectiles' (no sign, double literals).
	Classic bool
}

// The reference's specs. A Throwable's Gravity is per kind (0.03 by
// default).
var (
	Arrow      = Spec{Gravity: 0.05, Drag: 0.99, PassNoBox: true, Life: 1200, Classic: true}
	Projectile = Spec{Gravity: 0.05, Drag: 0.99, PassNoBox: true, Life: 1200}
	Throwable  = Spec{Gravity: 0.03, Drag: 0.99, Classic: true}
)

// WaterDrag is the motion kept per tick in water.
const WaterDrag = float32(0.8)

// Missile is one projectile in flight or stuck.
type Missile struct {
	Spec
	X, Y, Z                  float64
	PX, PY, PZ               float64 // last tick's position (render lerp)
	MX, MY, MZ               float64
	Yaw, Pitch, PYaw, PPitch float64
	W                        float64 // box edge; the box stands on Y
	Air, Ground, Shake       int     // ticks in air, ticks stuck, wobble
	InGround                 bool
	TX, TY, TZ               int // the block it is stuck in
	Tile                     voxel.StateID
	TileSet                  bool
	Crit, Dead               bool
}

func deg(r float64) float64 { return r * 180 / math.Pi }
func f32(v float64) float64 { return float64(float32(v)) }

// lit is a literal the classic maths writes as a float.
func (m *Missile) lit(v float64) float64 {
	if m.Classic {
		return f32(v)
	}
	return v
}

// Box is the missile's bounding box.
func (m *Missile) Box() collide.AABB {
	h := m.W / 2
	return collide.AABB{MinX: m.X - h, MinY: m.Y, MinZ: m.Z - h, MaxX: m.X + h, MaxY: m.Y + m.W, MaxZ: m.Z + h}
}

// Place sets the position and the last tick's to it.
func (m *Missile) Place(x, y, z float64) {
	m.X, m.Y, m.Z = x, y, z
	m.PX, m.PY, m.PZ = x, y, z
}

// Speed is the motion's length (float).
func (m *Missile) Speed() float64 {
	return f32(math.Sqrt(m.MX*m.MX + m.MY*m.MY + m.MZ*m.MZ))
}

// Heading is setThrowableHeading: the direction normalised, jittered by
// gaussian*0.0075*inacc per axis, times speed; the angles follow.
func (m *Missile) Heading(r Rand, x, y, z, speed, inacc float64) {
	f := f32(math.Sqrt(x*x + y*y + z*z))
	x, y, z = x/f, y/f, z/f
	j := func() float64 {
		if !m.Classic {
			return r.NormFloat64() * 0.0075 * inacc
		}
		s := 1.0
		if r.IntN(2) == 0 {
			s = -1
		}
		return r.NormFloat64() * s * 0.007499999832361937 * inacc
	}
	x += j()
	y += j()
	z += j()
	m.MX, m.MY, m.MZ = x*speed, y*speed, z*speed
	h := f32(math.Sqrt(x*x + z*z))
	m.Yaw = f32(deg(math.Atan2(x, z)))
	m.Pitch = f32(deg(math.Atan2(y, h)))
	m.PYaw, m.PPitch = m.Yaw, m.Pitch
	m.Ground = 0
}

func sinD(d float64) float64 { return float64(fmath.Sin(float32(d) / 180 * fmath.Pi)) }
func cosD(d float64) float64 { return float64(fmath.Cos(float32(d) / 180 * fmath.Pi)) }

// Shoot launches from a thrower's eye (ex, ey, ez) looking along the
// reference yaw and pitch (degrees, pitch positive down as entities look):
// 0.16 to the side, 0.1 down, then Heading(speed, inaccuracy 1). scale
// multiplies the look vector first (the thrown kinds' 0.4).
func (m *Missile) Shoot(r Rand, ex, ey, ez, yaw, pitch, scale, speed float64) {
	m.Place(ex-cosD(yaw)*0.16, ey-m.lit(0.1), ez-sinD(yaw)*0.16)
	m.Yaw, m.Pitch = f32(yaw), f32(pitch)
	k := f32(scale)
	mx := f32(f32(-sinD(yaw)*cosD(pitch)) * k)
	mz := f32(f32(cosD(yaw)*cosD(pitch)) * k)
	my := f32(-sinD(pitch) * k)
	m.Heading(r, mx, my, mz, speed, 1)
}

// Aim launches from a shooter at (sx, sz) with launch height y at a point
// (tx, ty, tz), lifted by a fifth of the horizontal distance and starting
// one block towards it (the shooter-at-target constructors). False when
// the target is straight above or below.
func (m *Missile) Aim(r Rand, sx, y, sz, tx, ty, tz, speed, inacc float64) bool {
	dx, dy, dz := tx-sx, ty-y, tz-sz
	d := f32(math.Sqrt(dx*dx + dz*dz))
	if d < 1e-7 {
		return false
	}
	m.Place(sx+dx/d, y, sz+dz/d)
	m.Heading(r, dx, dy+f32(d*0.2), dz, speed, inacc)
	return true
}

// Intercept is the nearest point (as a share of the segment) where the
// segment crosses one of the box's faces.
func Intercept(b collide.AABB, x0, y0, z0, x1, y1, z1 float64) (float64, bool) {
	best, ok := 0.0, false
	for _, p := range [][3]float64{{b.MinX, x0, x1}, {b.MaxX, x0, x1}, {b.MinY, y0, y1}, {b.MaxY, y0, y1}, {b.MinZ, z0, z1}, {b.MaxZ, z0, z1}} {
		d := p[2] - p[1]
		if d*d < 1e-14 {
			continue
		}
		t := (p[0] - p[1]) / d
		if t < 0 || t > 1 {
			continue
		}
		x, y, z := x0+(x1-x0)*t, y0+(y1-y0)*t, z0+(z1-z0)*t
		const e = 1e-12
		if x >= b.MinX-e && x <= b.MaxX+e && y >= b.MinY-e && y <= b.MaxY+e && z >= b.MinZ-e && z <= b.MaxZ+e && (!ok || t < best) {
			best, ok = t, true
		}
	}
	return best, ok
}

// Step is one tick's flight segment.
type Step struct {
	X1, Y1, Z1 float64 // the segment's end: the block hit, else pos+motion
	Block      bool    // a block was hit
	HX, HY, HZ float64
	Hit        physics.Hit
}

// Begin starts a tick: the stuck test, the wobble, the stuck tick (false:
// no flight this tick), else the air tick and the block ray.
func (m *Missile) Begin(w World, r Rand) (Step, bool) {
	m.PX, m.PY, m.PZ = m.X, m.Y, m.Z
	m.PYaw, m.PPitch = m.Yaw, m.Pitch
	if m.Life > 0 && m.TileSet && w.Block(m.TX, m.TY, m.TZ) != voxel.Air && w.Inside(m.TX, m.TY, m.TZ, m.X, m.Y, m.Z) {
		m.InGround = true
	}
	if m.Shake > 0 {
		m.Shake--
	}
	if m.InGround {
		if w.Block(m.TX, m.TY, m.TZ) == m.Tile {
			if m.Ground++; m.Ground >= m.Life {
				m.Dead = true
			}
			return Step{}, false
		}
		m.InGround = false
		m.MX *= float64(r.Float32() * 0.2)
		m.MY *= float64(r.Float32() * 0.2)
		m.MZ *= float64(r.Float32() * 0.2)
		m.Ground, m.Air = 0, 0
		return Step{}, false
	}
	m.Air++
	s := Step{X1: m.X + m.MX, Y1: m.Y + m.MY, Z1: m.Z + m.MZ}
	if hx, hy, hz, h, ok := w.Ray(m.X, m.Y, m.Z, s.X1, s.Y1, s.Z1, m.PassNoBox); ok {
		s.Block, s.HX, s.HY, s.HZ, s.Hit = true, hx, hy, hz, h
		s.X1, s.Y1, s.Z1 = hx, hy, hz
	}
	return s, true
}

// SweepArea is the area whose entities the tick tests: the box stretched
// by the motion and grown by 1.
func (m *Missile) SweepArea() collide.AABB {
	return m.Box().Expand(m.MX, m.MY, m.MZ).Grow(1, 1, 1)
}

// Sweep is the nearest of the boxes (grown by 0.3) the step's segment
// crosses; -1 for none. The caller leaves out what cannot be hit (the
// shooter while Air < 5, the dead).
func (m *Missile) Sweep(s Step, boxes []collide.AABB) int {
	best, bd := -1, 0.0
	for i, b := range boxes {
		t, ok := Intercept(b.Grow(0.3, 0.3, 0.3), m.X, m.Y, m.Z, s.X1, s.Y1, s.Z1)
		if !ok {
			continue
		}
		dx, dy, dz := (s.X1-m.X)*t, (s.Y1-m.Y)*t, (s.Z1-m.Z)*t
		if d := math.Sqrt(dx*dx + dy*dy + dz*dz); best < 0 || d < bd || bd == 0 {
			best, bd = i, d
		}
	}
	return best
}

// Damage is the impact damage for a base (speed times the missile's
// damage): rounded up, and a critical one adds 0..damage/2+1.
func (m *Missile) Damage(r Rand, base float64) int {
	k := int(math.Ceil(base))
	if m.Crit {
		k += r.IntN(k/2 + 2)
	}
	return k
}

// Push is the knockback velocity of strength k along the horizontal
// motion: 0.6*k per unit, 0.1 up; zero when there is none.
func (m *Missile) Push(k int) (vx, vy, vz float64) {
	f := f32(math.Sqrt(m.MX*m.MX + m.MZ*m.MZ))
	if k <= 0 || f <= 0 {
		return 0, 0, 0
	}
	p := m.lit(0.6)
	return m.MX * float64(k) * p / f, 0.1, m.MZ * float64(k) * p / f
}

// Bounce is the miss: an entity that took no damage turns the missile
// back at a tenth of its speed.
func (m *Missile) Bounce() {
	k := -m.lit(0.1)
	m.MX, m.MY, m.MZ = m.MX*k, m.MY*k, m.MZ*k
	m.Yaw += 180
	m.PYaw += 180
	m.Air = 0
}

// Stick lodges the missile in the block the step hit: backed out 0.05
// along the motion, wobbling for 7 ticks, no longer critical.
func (m *Missile) Stick(s Step) {
	m.TX, m.TY, m.TZ, m.Tile, m.TileSet = s.Hit.Block.X, s.Hit.Block.Y, s.Hit.Block.Z, s.Hit.State, true
	m.MX, m.MY, m.MZ = f32(s.HX-m.X), f32(s.HY-m.Y), f32(s.HZ-m.Z)
	f := m.Speed()
	b := m.lit(0.05)
	m.X -= m.MX / f * b
	m.Y -= m.MY / f * b
	m.Z -= m.MZ / f * b
	m.InGround, m.Shake, m.Crit = true, 7, false
}

// End finishes a flying tick: the crit trail, the move, the turn into the
// flight (extraPitch added), drag (bubbles in water) and gravity.
func (m *Missile) End(w World, extraPitch float64) {
	if m.Dead {
		return
	}
	if m.Crit {
		particles.Trail(4, m.X, m.Y, m.Z, m.MX, m.MY, m.MZ, 0.2, func(x, y, z, vx, vy, vz float64) {
			w.Spark("crit", x, y, z, vx, vy, vz)
		})
	}
	m.X, m.Y, m.Z = m.X+m.MX, m.Y+m.MY, m.Z+m.MZ
	m.TurnFlight(extraPitch)
	f := m.Drag
	if m.InWater(w) {
		for range 4 {
			w.Spark("bubble", m.X-m.MX*0.25, m.Y-m.MY*0.25, m.Z-m.MZ*0.25, m.MX, m.MY, m.MZ)
		}
		f = WaterDrag
	}
	m.MX, m.MY, m.MZ = m.MX*float64(f), m.MY*float64(f)-float64(m.Gravity), m.MZ*float64(f)
}

// InWater is the water test on the box shrunk by 0.4 vertically.
func (m *Missile) InWater(w World) bool {
	return w.Water(m.Box().Grow(-0.001, -0.4, -0.001))
}

// TurnFlight sets the angles from the motion, eased in by a fifth from
// last tick's; extraPitch is added to the pitch.
func (m *Missile) TurnFlight(extraPitch float64) {
	f := f32(math.Sqrt(m.MX*m.MX + m.MZ*m.MZ))
	m.Yaw = f32(deg(math.Atan2(m.MX, m.MZ)))
	m.Pitch = f32(deg(math.Atan2(m.MY, f)))
	for m.Pitch-m.PPitch < -180 {
		m.PPitch -= 360
	}
	for m.Pitch-m.PPitch >= 180 {
		m.PPitch += 360
	}
	for m.Yaw-m.PYaw < -180 {
		m.PYaw -= 360
	}
	for m.Yaw-m.PYaw >= 180 {
		m.PYaw += 360
	}
	m.Pitch = m.PPitch + (m.Pitch-m.PPitch)*0.2 + extraPitch
	m.Yaw = m.PYaw + (m.Yaw-m.PYaw)*0.2
}

// Pickable reports whether a stuck, settled missile touches a player's
// box (grown by 1, 0.5, 1 as players collect).
func (m *Missile) Pickable(player collide.AABB) bool {
	return m.InGround && m.Shake <= 0 && m.Box().Intersects(player.Grow(1, 0.5, 1))
}

// PopPitch is the pickup sound's pitch.
func PopPitch(r Rand) float64 {
	return float64(((r.Float32()-r.Float32())*0.7 + 1) * 2)
}

// HitPitch is the impact sound's pitch.
func HitPitch(r Rand) float64 {
	return float64(1.2 / (r.Float32()*0.2 + 0.9))
}
