// Package mobs gives creatures simple ambient behaviour: walkers wander
// around their home with gravity, collisions and jumps; flyers drift
// between points in the air. Each mob reports a logical animation State;
// mapping states to a model's clips is up to the caller.
//
// It knows nothing about models or rendering and depends only on
// voxel/collide. Tick at 20 Hz like the character controller.
package mobs

import (
	"math"
	"math/rand"

	"github.com/memmaker/voxellib/voxel/collide"
)

// Kind is how a mob moves.
type Kind uint8

const (
	Walker Kind = iota
	Flyer
)

// State is the mob's current activity, used to pick an animation.
type State uint8

const (
	Idle State = iota
	Walk
	Run
	Jump
	Trick // a one-off flourish while idling
)

func (s State) String() string { return [...]string{"Idle", "Walk", "Run", "Jump", "Trick"}[s] }

// Mob is one creature.
type Mob struct {
	Kind                Kind
	X, Y, Z             float64 // feet centre
	VX, VY, VZ          float64
	PrevX, PrevY, PrevZ float64
	Yaw, PrevYaw        float64 // radians; forward is (sin, 0, -cos) as the camera
	Width, Height       float64
	WalkSpeed, RunSpeed float64 // blocks per tick
	HomeX, HomeY, HomeZ float64
	Range               float64 // wander radius around home
	OnGround            bool
	State               State
	// StateTicks counts ticks in the current state; TrickTicks is how long
	// a Trick lasts (set it to the clip length).
	StateTicks, TrickTicks int

	timer          int
	tx, ty, tz     float64
	hasTarget, run bool
}

// New returns a mob at home position x,y,z with sensible defaults
// (Cube Pets walk at 1.12 m/s and run at 2.24 m/s).
func New(kind Kind, x, y, z, width, height float64) *Mob {
	return &Mob{Kind: kind, X: x, Y: y, Z: z, PrevX: x, PrevY: y, PrevZ: z,
		Width: width, Height: height, WalkSpeed: 1.12 / 20, RunSpeed: 2.24 / 20,
		HomeX: x, HomeY: y, HomeZ: z, Range: 10, TrickTicks: 30}
}

// Box returns the collision box.
func (m *Mob) Box() collide.AABB { return collide.Box(m.X, m.Y, m.Z, m.Width, m.Height) }

// Lerp interpolates position and yaw for rendering.
func (m *Mob) Lerp(alpha float64) (x, y, z, yaw float64) {
	dy := math.Remainder(m.Yaw-m.PrevYaw, 2*math.Pi)
	return m.PrevX + (m.X-m.PrevX)*alpha, m.PrevY + (m.Y-m.PrevY)*alpha, m.PrevZ + (m.Z-m.PrevZ)*alpha, m.PrevYaw + dy*alpha
}

func (m *Mob) set(s State) {
	if m.State != s {
		m.State, m.StateTicks = s, 0
	}
}

// Tick advances one 50 ms step.
func (m *Mob) Tick(w collide.Solids, rnd *rand.Rand) {
	m.PrevX, m.PrevY, m.PrevZ, m.PrevYaw = m.X, m.Y, m.Z, m.Yaw
	m.StateTicks++
	if m.timer > 0 {
		m.timer--
	}
	if !collide.Fits(w, m.Box()) { // buried by a placed block: pop up
		m.Y += 0.2
		m.VY = 0
		return
	}
	if m.Kind == Flyer {
		m.tickFlyer(w, rnd)
	} else {
		m.tickWalker(w, rnd)
	}
}

func (m *Mob) pickTarget(rnd *rand.Rand) {
	a := rnd.Float64() * 2 * math.Pi
	r := rnd.Float64() * m.Range
	m.tx, m.tz = m.HomeX+math.Cos(a)*r, m.HomeZ+math.Sin(a)*r
	m.ty = m.HomeY + 1.5 + rnd.Float64()*4
	m.hasTarget, m.run = true, rnd.Intn(4) == 0
	m.timer = 200 // give up after 10 s
}

// steer turns toward the target and returns the horizontal distance to it.
func (m *Mob) steer(maxTurn float64) float64 {
	dx, dz := m.tx-m.X, m.tz-m.Z
	want := math.Atan2(dx, -dz)
	d := math.Remainder(want-m.Yaw, 2*math.Pi)
	m.Yaw += math.Max(-maxTurn, math.Min(maxTurn, d))
	return math.Hypot(dx, dz)
}

func (m *Mob) tickWalker(w collide.Solids, rnd *rand.Rand) {
	switch {
	case m.State == Trick:
		if m.StateTicks >= m.TrickTicks {
			m.set(Idle)
			m.timer = 20 + rnd.Intn(60)
		}
	case !m.hasTarget:
		if m.timer == 0 {
			if rnd.Intn(5) == 0 {
				m.set(Trick)
			} else {
				m.pickTarget(rnd)
			}
		}
	}
	speed := 0.0
	if m.hasTarget {
		dist := m.steer(0.25)
		if dist < 0.4 || m.timer == 0 {
			m.hasTarget = false
			m.timer = 40 + rnd.Intn(100)
		} else {
			speed = m.WalkSpeed
			if m.run {
				speed = m.RunSpeed
			}
		}
	}
	// Speed is a target velocity; ground friction makes it snappy.
	fx, fz := math.Sin(m.Yaw)*speed, -math.Cos(m.Yaw)*speed
	m.VX += (fx - m.VX) * 0.5
	m.VZ += (fz - m.VZ) * 0.5
	m.VY -= 0.08
	mx, my, mz := collide.Move(w, m.Box(), m.VX, m.VY, m.VZ)
	blocked := (mx != m.VX || mz != m.VZ) && speed > 0
	m.OnGround = m.VY < 0 && my > m.VY
	if my != m.VY {
		m.VY = 0
	}
	m.X, m.Y, m.Z = m.X+mx, m.Y+my, m.Z+mz
	m.VY *= 0.98
	if blocked && m.OnGround {
		m.VY = 0.42 // hop up one block
		m.set(Jump)
	}
	switch {
	case m.State == Trick:
	case !m.OnGround && m.State == Jump:
	case speed == 0:
		m.set(Idle)
	case m.run:
		m.set(Run)
	default:
		m.set(Walk)
	}
}

func (m *Mob) tickFlyer(w collide.Solids, rnd *rand.Rand) {
	if !m.hasTarget && m.timer == 0 {
		m.pickTarget(rnd)
	}
	speed := 0.0
	if m.hasTarget {
		dist := m.steer(0.12)
		if math.Hypot(dist, m.ty-m.Y) < 0.6 || m.timer == 0 {
			m.hasTarget = false
			m.timer = 10 + rnd.Intn(60)
		} else {
			speed = m.WalkSpeed * 1.5
			if m.run {
				speed = m.RunSpeed * 1.5
			}
		}
	}
	fx, fz := math.Sin(m.Yaw)*speed, -math.Cos(m.Yaw)*speed
	fy := 0.0
	if m.hasTarget {
		fy = math.Max(-0.05, math.Min(0.05, (m.ty-m.Y)*0.1))
	}
	fy += math.Sin(float64(m.StateTicks)*0.3) * 0.02 // flutter
	m.VX += (fx - m.VX) * 0.1
	m.VY += (fy - m.VY) * 0.2
	m.VZ += (fz - m.VZ) * 0.1
	mx, my, mz := collide.Move(w, m.Box(), m.VX, m.VY, m.VZ)
	if mx != m.VX || mz != m.VZ {
		m.ty += 1 // fly over obstacles
		m.VY = 0.05
	}
	m.X, m.Y, m.Z = m.X+mx, m.Y+my, m.Z+mz
	m.OnGround = false
	if speed > m.WalkSpeed*1.5 {
		m.set(Run)
	} else if speed > 0 {
		m.set(Walk)
	} else {
		m.set(Idle)
	}
}
