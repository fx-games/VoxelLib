// Package explosion is the reference's explosion: the blast rays that pick
// the blocks it destroys (each block's resistance eats ray strength), the
// exposure of an entity box to the centre, the damage and knockback by
// exposure, and the debris particles' motion.
//
// It reads the world through callbacks and changes nothing; the game
// decides whether the picked blocks break (block breaking is off unless
// the game passes Break), applies damage, motion, sound and particles.
package explosion

import (
	"math"

	"github.com/memmaker/voxellib/voxel/collide"
)

// Rand is the random source (math/rand/v2's *Rand and javarand are one).
type Rand interface{ Float32() float32 }

// Blast is one explosion.
type Blast struct {
	X, Y, Z float64
	Size    float32 // the reference's explosion strength (fire jar 2, TNT 4)
	Break   bool    // the picked blocks break ("smoking"); off by default
	Fire    bool    // air above opaque blocks catches fire, 1 in 3
}

// Blocks walks the blast rays: from each of the 1352 boundary points of a
// 16-cube grid a ray of strength Size*(0.7+r*0.6) steps 0.3 blocks out,
// every non-air block taking (resistance+0.3)*0.3 and every step 0.225;
// the blocks met while strength is left are picked, in first-met order.
// block reports a block's blast resistance and whether it is air.
func (b Blast) Blocks(rnd Rand, block func(x, y, z int) (res float32, air bool)) [][3]int {
	const n = 16
	seen := map[[3]int]bool{}
	var out [][3]int
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			for k := 0; k < n; k++ {
				if i != 0 && i != n-1 && j != 0 && j != n-1 && k != 0 && k != n-1 {
					continue
				}
				dx := float64(float32(i)/(n-1)*2 - 1)
				dy := float64(float32(j)/(n-1)*2 - 1)
				dz := float64(float32(k)/(n-1)*2 - 1)
				l := math.Sqrt(dx*dx + dy*dy + dz*dz)
				dx, dy, dz = dx/l, dy/l, dz/l
				f := b.Size * (0.7 + rnd.Float32()*0.6)
				x, y, z := b.X, b.Y, b.Z
				const step = float32(0.3)
				for ; f > 0; f -= step * 0.75 {
					p := [3]int{int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z))}
					if res, air := block(p[0], p[1], p[2]); !air {
						f -= (res + 0.3) * step
					}
					if f > 0 && !seen[p] {
						seen[p] = true
						out = append(out, p)
					}
					x += dx * float64(step)
					y += dy * float64(step)
					z += dz * float64(step)
				}
			}
		}
	}
	return out
}

// Reach is the box whose entities the blast reaches: floor(c -+ 2*Size -+ 1)
// on each axis.
func (b Blast) Reach() collide.AABB {
	s := float64(b.Size * 2)
	f := func(v float64) float64 { return math.Floor(v) }
	return collide.AABB{
		MinX: f(b.X - s - 1), MinY: f(b.Y - s - 1), MinZ: f(b.Z - s - 1),
		MaxX: f(b.X + s + 1), MaxY: f(b.Y + s + 1), MaxZ: f(b.Z + s + 1),
	}
}

// Exposure is the share of sample points of box whose ray to (cx, cy, cz)
// meets no block (the reference's block density); clear reports a ray
// that meets none. Samples step 1/(edge*2+1) per axis in float.
func Exposure(cx, cy, cz float64, box collide.AABB, clear func(x0, y0, z0, x1, y1, z1 float64) bool) float32 {
	sx := 1 / ((box.MaxX-box.MinX)*2 + 1)
	sy := 1 / ((box.MaxY-box.MinY)*2 + 1)
	sz := 1 / ((box.MaxZ-box.MinZ)*2 + 1)
	hit, all := 0, 0
	for u := float32(0); u <= 1; u = float32(float64(u) + sx) {
		for v := float32(0); v <= 1; v = float32(float64(v) + sy) {
			for w := float32(0); w <= 1; w = float32(float64(w) + sz) {
				x := box.MinX + (box.MaxX-box.MinX)*float64(u)
				y := box.MinY + (box.MaxY-box.MinY)*float64(v)
				z := box.MinZ + (box.MaxZ-box.MinZ)*float64(w)
				if clear(x, y, z, cx, cy, cz) {
					hit++
				}
				all++
			}
		}
	}
	return float32(hit) / float32(all)
}

// Hit is the blast on an entity standing at (x, y, z) with its eyes eye
// above, whose box has the given exposure: the push direction (from the
// centre to the eyes, unit), the impact (1-dist/(2*Size))*exposure and the
// damage (int)((impact²+impact)/2*8*2*Size+1). ok is false out of reach or
// at the very centre.
func (b Blast) Hit(x, y, z, eye float64, exposure float32) (dx, dy, dz, impact float64, damage float32, ok bool) {
	s := float64(b.Size * 2)
	ex, ey, ez := x-b.X, y-b.Y, z-b.Z
	d := math.Sqrt(ex*ex+ey*ey+ez*ez) / s
	if d > 1 {
		return
	}
	dx, dy, dz = x-b.X, y+eye-b.Y, z-b.Z
	l := float64(float32(math.Sqrt(dx*dx + dy*dy + dz*dz)))
	if l == 0 {
		return
	}
	impact = (1 - d) * float64(exposure)
	return dx / l, dy / l, dz / l, impact,
		float32(int((impact*impact+impact)/2*8*s + 1)), true
}

// Knock is the knockback speed after blast protection of the given total
// level: impact - floor(impact*level*0.15).
func Knock(impact float64, level int) float64 {
	if level > 0 {
		impact -= math.Floor(impact * float64(float32(level)*0.15))
	}
	return impact
}

// Debris is the reference's per-block blast particles: a random point in
// the block (three draws) and its motion away from the centre,
// 0.5/(dist/Size+0.1)*(r*r+0.3). The flash ("explode") goes at the
// midpoint of the point and the centre, the smoke at the point.
func (b Blast) Debris(rnd Rand, bx, by, bz int) (px, py, pz, vx, vy, vz float64) {
	px = float64(float32(bx) + rnd.Float32())
	py = float64(float32(by) + rnd.Float32())
	pz = float64(float32(bz) + rnd.Float32())
	vx, vy, vz = px-b.X, py-b.Y, pz-b.Z
	l := float64(float32(math.Sqrt(vx*vx + vy*vy + vz*vz)))
	vx, vy, vz = vx/l, vy/l, vz/l
	v := 0.5 / (l/float64(b.Size) + 0.1)
	v *= float64(rnd.Float32()*rnd.Float32() + 0.3)
	return px, py, pz, vx * v, vy * v, vz * v
}

// Pitch is the blast sound's pitch, (1+(r-r)*0.2)*0.7; its volume is 4.
func Pitch(rnd Rand) float32 { return (1 + (rnd.Float32()-rnd.Float32())*0.2) * 0.7 }
