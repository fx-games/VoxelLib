package render

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel/javarand"
)

// Precip is what falls in one column.
type Precip uint8

const (
	PrecipNone Precip = iota
	PrecipRain
	PrecipSnow
	PrecipAsh  // LOTR: Mordor, Dagorlad, volcanoes
	PrecipSand // LOTR: sandstorms
	PrecipCount
)

// Weather is the falling rain and snow around the viewer, drawn like the
// LOTR mod's LOTRWeatherRenderer (Minecraft 1.7's renderRainSnow plus ash
// and sand): one camera-facing strip per column within Radius, from the
// precipitation height up to the viewer's feet + Radius.
type Weather struct {
	// Strength is World.getRainStrength(partial); 0 draws nothing.
	Strength float32
	// Ticks and Partial are the client tick counter and the partial tick:
	// they scroll the textures.
	Ticks   int
	Partial float32
	// Feet is the view entity's position, interpolated.
	Feet [3]float64
	// Radius is 5, or 10 with fancy graphics.
	Radius int
	// Column is the kind falling in column x,z whose strip starts at y,
	// from its biome: PrecipNone when it neither rains nor snows there,
	// rain when getFloatTemperature(x, y, z) >= 0.15, else snow.
	Column func(x, y, z int) Precip
	// Textures names each kind's texture (Options.SkyTextures).
	Textures [PrecipCount]string
}

// DrawWeather draws Opt.Weather. Minecraft draws it last in the world,
// after particles, so call it after models and particles.
func (r *Renderer) DrawWeather(cam *Camera) {
	if w := r.Opt.Weather; w != nil && w.Strength > 0 && w.Column != nil {
		r.weatherDraws = r.weather(w, cam, r.weatherDraws[:0])
		r.drawSky(cam, r.weatherDraws)
	}
}

// weatherPlane is LOTRWeatherRenderer's rainXCoords/rainYCoords: per
// column offset (dx, dz) from the viewer, half the unit vector across the
// line of sight (NaN on the viewer's own column, as in Java).
func weatherPlane(dx, dz int) (float32, float32) {
	l := float32(math.Sqrt(float64(dx*dx + dz*dz)))
	return -float32(dz) / l * 0.5, float32(dx) / l * 0.5
}

// weather appends w's draws to out, one per run of columns that share a
// texture binding.
func (r *Renderer) weather(w *Weather, cam *Camera, out []SkyDraw) []SkyDraw {
	b0 := w.Radius
	px, pz := int(math.Floor(w.Feet[0])), int(math.Floor(w.Feet[2]))
	py := int(math.Floor(w.Feet[1]))
	k := py // floor(interpolated y); Java's var50 is the tick y, near enough
	f5 := float32(w.Ticks) + w.Partial
	r.weatherFog = r.CurrentFog()
	var rnd javarand.Random
	bound := -1 // Java's b1: 0 rain texture, 1 the others (one binding for all three)
	for l := pz - b0; l <= pz+b0; l++ {
		for i1 := px - b0; i1 <= px+b0; i1++ {
			if i1 == px && l == pz {
				continue // NaN plane: Java's quad is degenerate
			}
			f6, f7 := weatherPlane(i1-px, l-pz)
			k1 := r.world.PrecipitationHeight(i1, l)
			l1, i2 := max(py-b0, k1), max(py+b0, k1)
			if l1 == i2 {
				continue
			}
			kind := w.Column(i1, l1, l)
			if kind == PrecipNone || kind >= PrecipCount {
				continue
			}
			j2 := max(k1, k)
			x32, z32 := int32(i1), int32(l)
			rnd.SetSeed(int64(x32*x32*3121 + x32*45238971 ^ z32*z32*418711 + z32*13761))
			d4, d5 := float64(float32(i1)+0.5)-w.Feet[0], float64(float32(l)+0.5)-w.Feet[2]
			dist := float32(math.Sqrt(d4*d4+d5*d5)) / float32(b0)
			sky, blk := r.world.Light(i1, j2, l)
			var u0, v0 float32
			var c mgl32.Vec3
			var alpha float32
			if kind == PrecipRain {
				h := int32(w.Ticks) + x32*x32*3121 + x32*45238971 + z32*z32*418711 + z32*13761
				v0 = (float32(h&31) + w.Partial) / 32 * (3 + rnd.NextFloat())
				c = r.light.At(float32(sky), float32(blk))
				alpha = ((1-dist*dist)*0.5 + 0.5) * w.Strength
			} else {
				f10 := (float32(w.Ticks&511) + w.Partial) / 512
				switch kind {
				case PrecipAsh:
					u0 = rnd.NextFloat()*0.3 + f5*0.003*float32(rnd.NextGaussian())
				case PrecipSand:
					u0 = f5 * (0.07 + float32(rnd.NextGaussian())*0.01)
				default:
					u0 = rnd.NextFloat() + f5*0.01*float32(rnd.NextGaussian())
				}
				f11 := rnd.NextFloat() + f5*float32(rnd.NextGaussian())*0.001
				v0 = f10 + f11
				// (brightness*3 + 0xF000F0) / 4: three quarters toward full.
				c = r.light.At((3*float32(sky)+15)/4, (3*float32(blk)+15)/4)
				alpha = ((1-dist*dist)*0.3 + 0.5) * w.Strength
			}
			b := 0
			if kind != PrecipRain {
				b = 1
			}
			if b != bound {
				bound = b
				var verts []SkyVertex
				if len(out) < cap(out) {
					verts = out[:len(out)+1][len(out)].Verts[:0] // reuse last frame's
				}
				out = append(out, SkyDraw{Texture: w.Textures[kind], Fog: &r.weatherFog, AlphaTest: 0.1, Verts: verts})
			}
			d := &out[len(out)-1]
			col := [4]float32{c[0], c[1], c[2], alpha}
			if r.Opt.Pack != nil { // gbuffers_weather reads lmcoord.s from the colour (packWeatherWGSL)
				lvl := float32(blk)
				if kind != PrecipRain {
					lvl = (3*lvl + 15) / 4
				}
				col = [4]float32{1, (lvl*16 + 8) / 256, 1, alpha}
			}
			ox, oz := float32(float64(i1)+0.5-cam.X), float32(float64(l)+0.5-cam.Z)
			y0, y1 := float32(l1), float32(i2)
			yl, yh := float32(float64(l1)-cam.Y), float32(float64(i2)-cam.Y)
			d.Verts = append(d.Verts,
				SkyVertex{Pos: [3]float32{ox - f6, yl, oz - f7}, UV: [2]float32{u0, y0/4 + v0}, Color: col},
				SkyVertex{Pos: [3]float32{ox + f6, yl, oz + f7}, UV: [2]float32{1 + u0, y0/4 + v0}, Color: col},
				SkyVertex{Pos: [3]float32{ox + f6, yh, oz + f7}, UV: [2]float32{1 + u0, y1/4 + v0}, Color: col},
				SkyVertex{Pos: [3]float32{ox - f6, yh, oz - f7}, UV: [2]float32{u0, y1/4 + v0}, Color: col})
		}
	}
	return out
}

// At is the light colour at fractional sky and block levels, filtered
// linearly between levels like Minecraft's lightmap texture.
func (lm *Lightmap) At(sky, block float32) mgl32.Vec3 {
	lerp := func(v float32) (int, int, float32) {
		v = min(max(v, 0), 15)
		i := min(int(v), 14)
		return i, i + 1, v - float32(i)
	}
	s0, s1, fs := lerp(sky)
	b0, b1, fb := lerp(block)
	var c mgl32.Vec3
	for i := range c {
		lo := lm[s0][b0][i] + (lm[s0][b1][i]-lm[s0][b0][i])*fb
		hi := lm[s1][b0][i] + (lm[s1][b1][i]-lm[s1][b0][i])*fb
		c[i] = lo + (hi-lo)*fs
	}
	return c
}
