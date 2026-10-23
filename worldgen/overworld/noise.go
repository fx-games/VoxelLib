// Package gen is a reference-flavoured terrain generator: 2D noise for
// continents/erosion/peaks, a 3D density field for overhangs and cheese/
// spaghetti caves, surface rules, ores and trees.
//
// Like the reference's NoiseChunk, the 3D density is only evaluated on a coarse
// 4x8x4 lattice and trilinearly interpolated in between, cutting noise calls
// per chunk by ~128x.
package overworld

import "math"

// Perlin is Ken Perlin's improved noise with a seeded permutation table.
type Perlin struct {
	perm       [512]uint8
	ox, oy, oz float64
}

func NewPerlin(seed int64) *Perlin {
	p := &Perlin{}
	r := newRand(seed)
	var src [256]uint8
	for i := range src {
		src[i] = uint8(i)
	}
	for i := 255; i > 0; i-- {
		j := int(r.next() % uint64(i+1))
		src[i], src[j] = src[j], src[i]
	}
	for i := 0; i < 512; i++ {
		p.perm[i] = src[i&255]
	}
	p.ox = float64(r.next()%100000) / 100000 * 256
	p.oy = float64(r.next()%100000) / 100000 * 256
	p.oz = float64(r.next()%100000) / 100000 * 256
	return p
}

func fade(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }
func lerp(t, a, b float64) float64 {
	return a + t*(b-a)
}

func grad(h uint8, x, y, z float64) float64 {
	switch h & 15 {
	case 0:
		return x + y
	case 1:
		return -x + y
	case 2:
		return x - y
	case 3:
		return -x - y
	case 4:
		return x + z
	case 5:
		return -x + z
	case 6:
		return x - z
	case 7:
		return -x - z
	case 8:
		return y + z
	case 9:
		return -y + z
	case 10:
		return y - z
	case 11:
		return -y - z
	case 12:
		return y + x
	case 13:
		return -y + z
	case 14:
		return y - x
	}
	return -y - z
}

// Noise3 returns noise in roughly [-1, 1].
func (p *Perlin) Noise3(x, y, z float64) float64 {
	x += p.ox
	y += p.oy
	z += p.oz
	fx, fy, fz := math.Floor(x), math.Floor(y), math.Floor(z)
	X, Y, Z := int(fx)&255, int(fy)&255, int(fz)&255
	x -= fx
	y -= fy
	z -= fz
	u, v, w := fade(x), fade(y), fade(z)
	pm := &p.perm
	A := int(pm[X]) + Y
	AA := int(pm[A]) + Z
	AB := int(pm[A+1]) + Z
	B := int(pm[X+1]) + Y
	BA := int(pm[B]) + Z
	BB := int(pm[B+1]) + Z
	return lerp(w,
		lerp(v, lerp(u, grad(pm[AA], x, y, z), grad(pm[BA], x-1, y, z)),
			lerp(u, grad(pm[AB], x, y-1, z), grad(pm[BB], x-1, y-1, z))),
		lerp(v, lerp(u, grad(pm[AA+1], x, y, z-1), grad(pm[BA+1], x-1, y, z-1)),
			lerp(u, grad(pm[AB+1], x, y-1, z-1), grad(pm[BB+1], x-1, y-1, z-1))))
}

func (p *Perlin) Noise2(x, z float64) float64 { return p.Noise3(x, 0.5, z) }

// Octaves sums several Perlin layers (fBm).
type Octaves struct {
	layers []*Perlin
	freq   float64
}

func NewOctaves(seed int64, n int, freq float64) *Octaves {
	o := &Octaves{freq: freq}
	for i := 0; i < n; i++ {
		o.layers = append(o.layers, NewPerlin(seed*31+int64(i)*7919+1))
	}
	return o
}

func (o *Octaves) At2(x, z float64) float64 {
	sum, amp, f, norm := 0.0, 1.0, o.freq, 0.0
	for _, l := range o.layers {
		sum += l.Noise2(x*f, z*f) * amp
		norm += amp
		amp *= 0.5
		f *= 2
	}
	return sum / norm
}

func (o *Octaves) At3(x, y, z float64) float64 {
	sum, amp, f, norm := 0.0, 1.0, o.freq, 0.0
	for _, l := range o.layers {
		sum += l.Noise3(x*f, y*f, z*f) * amp
		norm += amp
		amp *= 0.5
		f *= 2
	}
	return sum / norm
}

// splitmix64 random source, deterministic per seed.
type rng struct{ s uint64 }

func newRand(seed int64) *rng { return &rng{uint64(seed)*0x9E3779B97F4A7C15 + 1} }

func (r *rng) next() uint64 {
	r.s += 0x9E3779B97F4A7C15
	z := r.s
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

func (r *rng) intn(n int) int        { return int(r.next() % uint64(n)) }
func (r *rng) float() float64        { return float64(r.next()>>11) / (1 << 53) }
func (r *rng) chance(p float64) bool { return r.float() < p }

func hash2(seed int64, x, z int) uint64 {
	r := rng{uint64(seed) ^ uint64(x)*0x632BE59BD9B4E019 ^ uint64(z)*0x8CB92BA72F3D8DD7}
	return r.next()
}
