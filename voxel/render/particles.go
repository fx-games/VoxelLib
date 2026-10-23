package render

import (
	"image"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/particles"
)

// ParticleRenderer draws particles (package particles) as camera-facing
// squares, one instanced quad each. Normal particles are lit like terrain
// and alpha blended; additive ones glow. Neither writes depth, and alpha
// particles are not sorted (small and short-lived, so it rarely shows).
type ParticleRenderer struct {
	r   *Renderer
	gpu gpuParticleRenderer
	buf []float32
}

const particleStride = 14 // rel pos 3, size 1, rgba 4, sprite 1, sheet flag 1, uv rect 4

// particleCorners is the unit quad as two triangles.
var particleCorners = []float32{-0.5, -0.5, 0.5, -0.5, 0.5, 0.5, -0.5, -0.5, 0.5, 0.5, -0.5, 0.5}

// particleFrame is the uniform state of a particle draw.
type particleFrame struct {
	vp        mgl32.Mat4
	right, up mgl32.Vec3
	fog       Fog
}

// NewParticleRenderer creates the particle renderer.
func NewParticleRenderer(r *Renderer) (*ParticleRenderer, error) {
	pr := &ParticleRenderer{r: r}
	if err := pr.initGPU(); err != nil {
		return nil, err
	}
	return pr, nil
}

// SetAtlas sets the sprite atlas Particle.Sprite indexes: 16 x 16 cells
// like Minecraft's particles.png.
func (pr *ParticleRenderer) SetAtlas(img image.Image) { pr.setAtlas(img) }

// SetSheet sets sheet i (1..255) that particles with Sheet i sample by
// their UV rectangle; a later call replaces it.
func (pr *ParticleRenderer) SetSheet(i int, img image.Image) { pr.setSheet(i, img) }

// Draw renders the particles, interpolated alpha (0..1) between ticks.
// light gives the sky and block light (0..15) at a world position. Call
// after the opaque world and models, with the same camera.
func (pr *ParticleRenderer) Draw(cam *Camera, ps []particles.Particle, alpha float64, light func(x, y, z float64) (sky, block uint8)) {
	if len(ps) == 0 {
		return
	}
	f := cam.Forward()
	right := f.Cross(mgl32.Vec3{0, 1, 0}).Normalize()
	pf := particleFrame{vp: cam.ViewProj(), right: right, up: right.Cross(f).Normalize(), fog: pr.r.CurrentFog()}
	pr.begin(&pf)
	sheets := 0
	for i := range ps {
		sheets = max(sheets, int(ps[i].Sheet))
	}
	for _, additive := range []bool{false, true} {
		for sheet := 0; sheet <= sheets; sheet++ {
			pr.buf = pr.buf[:0]
			for i := range ps {
				q := &ps[i]
				if q.Additive != additive || int(q.Sheet) != sheet {
					continue
				}
				x, y, z, size, col := q.Lerp(alpha)
				lc := mgl32.Vec3{1, 1, 1}
				flag := float32(0)
				if sheet > 0 {
					flag = 1
				}
				if pack := pr.r.Opt.Pack != nil; pack && light != nil {
					flag += 2 * lightCode(light(x, y, z)) // the pack lights them (packParticleWGSL)
				} else if pack {
					flag += 2 * 15
				} else if !additive && light != nil {
					lc = pr.r.LightColor(light(x, y, z))
				}
				pr.buf = append(pr.buf, float32(x-cam.X), float32(y-cam.Y), float32(z-cam.Z), size,
					col[0]*lc[0], col[1]*lc[1], col[2]*lc[2], col[3], float32(q.Sprite), flag, q.UV[0], q.UV[1], q.UV[2], q.UV[3])
			}
			if len(pr.buf) > 0 {
				pr.drawPass(&pf, additive, sheet, pr.buf)
			}
		}
	}
	pr.end()
}
