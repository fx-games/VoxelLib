package render

import "math"

// Shadow is one entity's round blob shadow (the 1.7 reference's
// Render.renderShadow).
type Shadow struct {
	// X, Y, Z is the entity's interpolated position (Y its posY).
	X, Y, Z float64
	// Lift is the entity's getShadowSize (half its height for most; 0 for
	// boats, arrows and the like): the shadow's centre is at Y + Lift.
	Lift float64
	// Size is the renderer's shadowSize times the entity's render size
	// modifier (× 0.5 for a child).
	Size float64
	// Opaque is the renderer's shadowOpaque × (1 − d²/256), d² the camera's
	// squared distance to the entity's (uninterpolated) position.
	Opaque float64
}

// ShadowGround reports whether a shadow lands on the block below cell
// (x, y, z) (not air, a full cube, the cell's light value above 3) and the
// cell's brightness (World.getLightBrightness).
type ShadowGround func(x, y, z int) (bright float32, ok bool)

// ShadowQuads appends s's quads, camera-relative to cam, for a SkyDraw of
// the shadow texture (alpha blended, AlphaTest 0.1): one per cell at
// floor(X±Size) × floor(Y+Lift−Size)..floor(Y+Lift) × floor(Z±Size) whose
// ground holds, lying 1/64 above the block's top (the reference's offset
// against z-fighting), alpha (Opaque − (Y+Lift − y)/2)/2 × bright clamped to
// 1. Quads are clipped to the square X±Size, Z±Size, which is the
// reference's clamped texture with its transparent border.
func ShadowQuads(out []SkyVertex, cam *Camera, s Shadow, ground ShadowGround) []SkyVertex {
	cy := s.Y + s.Lift
	f := math.Floor
	for x := int(f(s.X - s.Size)); x <= int(f(s.X+s.Size)); x++ {
		for y := int(f(cy - s.Size)); y <= int(f(cy)); y++ {
			for z := int(f(s.Z - s.Size)); z <= int(f(s.Z+s.Size)); z++ {
				bright, ok := ground(x, y, z)
				if !ok {
					continue
				}
				a := (s.Opaque - (cy-float64(y))/2) * 0.5 * float64(bright)
				if a < 0 {
					continue
				}
				// ponytail: unit block bounds (renderAsNormalBlock cubes); per-block bounds if a full-cube block with others shows up
				x0, x1 := math.Max(float64(x), s.X-s.Size), math.Min(float64(x+1), s.X+s.Size)
				z0, z1 := math.Max(float64(z), s.Z-s.Size), math.Min(float64(z+1), s.Z+s.Size)
				if x0 >= x1 || z0 >= z1 {
					continue
				}
				py := float32(float64(y) + 0.015625 - cam.Y)
				col := [4]float32{1, 1, 1, float32(math.Min(a, 1))}
				u := func(v, c float64) float32 { return float32((c-v)/2/s.Size + 0.5) }
				for _, c := range [4][2]float64{{x0, z0}, {x0, z1}, {x1, z1}, {x1, z0}} {
					out = append(out, SkyVertex{
						Pos:   [3]float32{float32(c[0] - cam.X), py, float32(c[1] - cam.Z)},
						UV:    [2]float32{u(c[0], s.X), u(c[1], s.Z)},
						Color: col,
					})
				}
			}
		}
	}
	return out
}

// DrawSkyDraws draws draws now, in the current frame's pass (e.g. entity
// shadows after the models).
func (r *Renderer) DrawSkyDraws(cam *Camera, draws []SkyDraw) { r.drawSky(cam, draws) }
