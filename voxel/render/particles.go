package render

import (
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

const particleStride = 10 // rel pos 3, size 1, rgba 4, light 2

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
	for _, additive := range []bool{false, true} {
		pr.buf = pr.buf[:0]
		for i := range ps {
			q := &ps[i]
			if q.Additive != additive {
				continue
			}
			x, y, z, size, col := q.Lerp(alpha)
			var sky, blk uint8 = 15, 15
			if !additive && light != nil {
				sky, blk = light(x, y, z)
			}
			pr.buf = append(pr.buf, float32(x-cam.X), float32(y-cam.Y), float32(z-cam.Z), size,
				col[0], col[1], col[2], col[3], float32(sky)/15, float32(blk)/15)
		}
		if len(pr.buf) > 0 {
			pr.drawPass(&pf, additive, pr.buf)
		}
	}
	pr.end()
}
