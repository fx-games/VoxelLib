// Package particles simulates short-lived particles on the CPU: sparks,
// shards, smoke, splashes, blood. Each particle carries its own colour and
// size over life, gravity, drag and bounce, and collides with solid blocks.
// voxel/render.ParticleRenderer draws them.
//
// The Props shape follows the user's go-fps particle presets (origin and
// velocity variation, radial velocity, begin/end colour and size); the
// simulation is new: go-fps simulates on the GPU without gravity, alpha or
// collisions.
package particles

import (
	"math"
	"math/rand"
)

// Props describes a burst of particles; Emit spawns Count of them.
type Props struct {
	Count  int
	Spread float64    // spawn offset, ± per axis (blocks)
	Vel    [3]float64 // base velocity (blocks/s)
	VelVar float64    // ± per axis
	Radial float64    // extra speed outward from the origin
	// Gravity in blocks/s² (positive pulls down; negative rises, e.g.
	// smoke), Drag per second (0..1 of velocity lost), Bounce keeps that
	// share of speed off a block (0 = stick).
	Gravity, Drag, Bounce float64
	Life, LifeVar         float64 // seconds
	Size0, Size1, SizeVar float64 // blocks, over life
	Col0, Col1            [4]float32
	// Additive particles glow (fire, sparks): they add light and ignore the
	// world's light. The rest are lit like terrain and alpha blended.
	Additive bool
}

// Particle is one live particle.
type Particle struct {
	X, Y, Z, PrevX, PrevY, PrevZ float64
	VX, VY, VZ                   float64
	Age, Life                    float64
	Size0, Size1                 float32
	Col0, Col1                   [4]float32
	Gravity, Drag, Bounce        float64
	Additive                     bool
	// Sprite is 1 + a cell of the renderer's particle atlas (Minecraft's
	// particles.png: 16 x 16 cells, index = row*16 + column); 0 draws a
	// plain square.
	Sprite uint16
	// Sheet, when non-zero, draws UV (u0, v0, u1, v1) of the renderer's
	// sheet Sheet instead of an atlas cell (Minecraft's block and item
	// particles: a quarter of a terrain or item icon).
	Sheet uint8
	UV    [4]float32
}

// Lerp is the particle's position between the last two ticks, its size and
// its colour (alpha in [3]).
func (p *Particle) Lerp(alpha float64) (x, y, z float64, size float32, col [4]float32) {
	t := float32(math.Min(1, p.Age/p.Life))
	for i := range col {
		col[i] = p.Col0[i] + (p.Col1[i]-p.Col0[i])*t
	}
	return p.PrevX + (p.X-p.PrevX)*alpha, p.PrevY + (p.Y-p.PrevY)*alpha, p.PrevZ + (p.Z-p.PrevZ)*alpha,
		p.Size0 + (p.Size1-p.Size0)*t, col
}

// System holds live particles.
type System struct {
	P   []Particle
	Max int // cap on live particles (0 = 4000); the oldest go first
	rng *rand.Rand
}

func (s *System) r() float64 {
	if s.rng == nil {
		s.rng = rand.New(rand.NewSource(1))
	}
	return s.rng.Float64()*2 - 1
}

// Emit spawns a burst at x,y,z.
func (s *System) Emit(x, y, z float64, p Props) {
	for i := 0; i < p.Count; i++ {
		ox, oy, oz := s.r()*p.Spread, s.r()*p.Spread, s.r()*p.Spread
		vx, vy, vz := p.Vel[0]+s.r()*p.VelVar, p.Vel[1]+s.r()*p.VelVar, p.Vel[2]+s.r()*p.VelVar
		if p.Radial != 0 {
			dx, dy, dz := s.r(), s.r(), s.r()
			if l := math.Sqrt(dx*dx + dy*dy + dz*dz); l > 1e-6 {
				vx, vy, vz = vx+dx/l*p.Radial, vy+dy/l*p.Radial, vz+dz/l*p.Radial
			}
		}
		k := float32(1 + s.r()*p.SizeVar/math.Max(p.Size0, 1e-6))
		q := Particle{X: x + ox, Y: y + oy, Z: z + oz, VX: vx, VY: vy, VZ: vz,
			Life:  math.Max(0.05, p.Life+s.r()*p.LifeVar),
			Size0: float32(p.Size0) * k, Size1: float32(p.Size1) * k, Col0: p.Col0, Col1: p.Col1,
			Gravity: p.Gravity, Drag: p.Drag, Bounce: p.Bounce, Additive: p.Additive}
		q.PrevX, q.PrevY, q.PrevZ = q.X, q.Y, q.Z
		s.Add(q) // ponytail: O(n) drop of the oldest, only when full
	}
}

// Add appends one particle, dropping the oldest when full.
func (s *System) Add(p Particle) {
	limit := s.Max
	if limit == 0 {
		limit = 4000
	}
	if len(s.P) >= limit {
		copy(s.P, s.P[1:])
		s.P = s.P[:len(s.P)-1]
	}
	s.P = append(s.P, p)
}

// Tick advances every particle by dt seconds; solid reports blocks they
// bounce off (nil: no collisions).
func (s *System) Tick(dt float64, solid func(x, y, z int) bool) {
	live := s.P[:0]
	for _, p := range s.P {
		p.Age += dt
		if p.Age >= p.Life {
			continue
		}
		p.PrevX, p.PrevY, p.PrevZ = p.X, p.Y, p.Z
		p.VY -= p.Gravity * dt
		d := math.Max(0, 1-p.Drag*dt)
		p.VX, p.VY, p.VZ = p.VX*d, p.VY*d, p.VZ*d
		// Move one axis at a time so a hit only reflects that axis.
		move := func(pos, vel *float64, step float64) bool {
			old := *pos
			*pos += step
			if solid != nil && solid(int(math.Floor(p.X)), int(math.Floor(p.Y)), int(math.Floor(p.Z))) {
				*pos = old
				*vel *= -p.Bounce
				return true
			}
			return false
		}
		move(&p.X, &p.VX, p.VX*dt)
		if move(&p.Y, &p.VY, p.VY*dt) && p.VY >= 0 { // landed: rub along the floor
			p.VX, p.VZ = p.VX*0.6, p.VZ*0.6
		}
		move(&p.Z, &p.VZ, p.VZ*dt)
		live = append(live, p)
	}
	clear(s.P[len(live):])
	s.P = live
}
