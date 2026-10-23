package particles

import (
	"math"
	"math/rand"

	"github.com/memmaker/voxellib/voxel/javarand"
)

// Ground is what rain lands on in a column (RainSplashes).
type Ground struct {
	// Top is World.getPrecipitationHeight.
	Top int
	// Rains is canSpawnLightningBolt && getFloatTemperature(x, Top, z) >=
	// 0.15: it rains, not snows, there.
	Rains bool
	// Air, Lava: the block at Top-1. MinY is its bounds' minimum y.
	Air, Lava bool
	MinY      float64
}

// Splash is a rain particle RainSplashes spawns: a splash, or smoke when
// rain hits lava.
type Splash struct {
	X, Y, Z float64
	Smoke   bool
}

// RainSplashes is the 1.7 reference's EntityRenderer.addRainParticles for one
// client tick: where splashes spawn around feet. strength is
// getRainStrength(1); fancy is fancy graphics; setting is the particle
// setting (0 all, 1 decreased, 2 minimal). ground describes a column.
func RainSplashes(tick int, strength float32, fancy bool, setting int, feet [3]float64, ground func(x, z int) Ground, out []Splash) []Splash {
	f := strength
	if !fancy {
		f /= 2
	}
	if f == 0 {
		return out
	}
	var r javarand.Random
	r.SetSeed(int64(tick) * 312987231)
	i, j, k := int(math.Floor(feet[0])), int(math.Floor(feet[1])), int(math.Floor(feet[2]))
	const b0 = 10
	n := int(100 * f * f)
	switch setting {
	case 1:
		n >>= 1
	case 2:
		n = 0
	}
	l := int32(0)
	for range n {
		x := i + int(r.NextInt(b0)) - int(r.NextInt(b0))
		z := k + int(r.NextInt(b0)) - int(r.NextInt(b0))
		g := ground(x, z)
		if g.Top > j+b0 || g.Top < j-b0 || !g.Rains {
			continue
		}
		f1, f2 := r.NextFloat(), r.NextFloat()
		s := Splash{X: float64(float32(x) + f1), Y: float64(float32(g.Top)+0.1) - g.MinY, Z: float64(float32(z) + f2)}
		switch {
		case g.Lava:
			s.Smoke = true
		case !g.Air:
			l++
			r.NextInt(l) // picks the rain sound's position
		default:
			continue
		}
		out = append(out, s)
	}
	return out
}

// RainDrop is EntityRainFX at x,y,z: a 19..22 sprite of particles.png
// thrown up a little, falling with gravity 0.06 and drag 0.98 per tick
// (converted to seconds). Not modelled: its 50% chance per tick to die on
// the ground and dying inside liquids.
func RainDrop(x, y, z float64, rnd *rand.Rand) Particle {
	// EntityFX(world, x, y, z, 0, 0, 0): random direction, speed 0.06..0.18.
	mx, my, mz := float64(float32(rnd.Float64()*2-1)*0.4), float64(float32(rnd.Float64()*2-1)*0.4), float64(float32(rnd.Float64()*2-1)*0.4)
	sp := float64(float32(rnd.Float64()+rnd.Float64()+1) * 0.15)
	l := float64(float32(math.Sqrt(mx*mx + my*my + mz*mz)))
	mx, mz = mx/l*sp*0.4*0.3, mz/l*sp*0.4*0.3
	my = float64(float32(rnd.Float64())*0.2 + 0.1)
	scale := (rnd.Float32()*0.5 + 0.5) * 2
	age := int(8 / (rnd.Float64()*0.8 + 0.2))
	const tps = 20
	return Particle{X: x, Y: y, Z: z, PrevX: x, PrevY: y, PrevZ: z,
		VX: mx * tps, VY: my * tps, VZ: mz * tps,
		Life:  float64(age+1) / tps, // dies when particleMaxAge-- <= 0
		Size0: 0.2 * scale, Size1: 0.2 * scale,
		Col0: [4]float32{1, 1, 1, 1}, Col1: [4]float32{1, 1, 1, 1},
		Gravity: 0.06 * tps * tps, Drag: 0.02 * tps,
		Sprite: uint16(19+rnd.Intn(4)) + 1}
}
