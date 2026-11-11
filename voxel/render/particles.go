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

const particleStride = 15 // rel pos 3, size 1, rgba 4, sprite 1, sheet flag 1, uv rect 4, roll 1

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
// like the reference's particles.png.
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
	pr.draw(&pf, [3]float64{cam.X, cam.Y, cam.Z}, ps, alpha, light)
}

// DrawHand draws particles in hand space like ModelRenderer.DrawHand
// (view places them, the hand projection, no fog), after the hand models
// so they share its depth (a gun's muzzle flash). light is the light at
// the player.
func (pr *ParticleRenderer) DrawHand(aspect float32, view mgl32.Mat4, ps []particles.Particle, sky, block uint8) {
	if len(ps) == 0 {
		return
	}
	inv := view.Inv()
	pf := particleFrame{vp: pr.r.handProj(aspect).Mul4(view), right: inv.Col(0).Vec3().Normalize(), up: inv.Col(1).Vec3().Normalize(),
		fog: Fog{Start: 1e6, End: 2e6}}
	pr.draw(&pf, [3]float64{}, ps, 1, func(x, y, z float64) (uint8, uint8) { return sky, block })
}

func (pr *ParticleRenderer) draw(pf *particleFrame, origin [3]float64, ps []particles.Particle, alpha float64, light func(x, y, z float64) (sky, block uint8)) {
	pr.begin(pf)
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
				uv := q.Rect()
				lc := mgl32.Vec3{1, 1, 1}
				flag := float32(0)
				if sheet > 0 {
					flag = 1
				}
				if pack := pr.r.Opt.Pack != nil; pack && light != nil {
					sky, block := light(x, y, z)
					flag += 2 * lightCode(sky, max(block, q.Emit)) // the pack lights them (packParticleWGSL); Emit is the particle's own light
				} else if pack {
					flag += 2 * 15
				} else if (!additive || q.Lit) && light != nil {
					sky, block := light(x, y, z)
					lc = pr.r.LightColor(sky, max(block, q.Emit))
				}
				pr.buf = append(pr.buf, float32(x-origin[0]), float32(y-origin[1]), float32(z-origin[2]), size,
					col[0]*lc[0], col[1]*lc[1], col[2]*lc[2], col[3], float32(q.Sprite), flag, uv[0], uv[1], uv[2], uv[3], q.Roll)
			}
			if len(pr.buf) > 0 {
				pr.drawPass(pf, additive, sheet, pr.buf)
			}
		}
	}
	pr.end()
}
