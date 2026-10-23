package render

import (
	"image"
	"image/draw"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel/mesh"
)

// BlockMesh is one block's geometry (mesh.Block, Mesher.BuildCell) on the
// GPU, drawn on its own: a block as an item (DrawBlockItems,
// DrawBlockHand) or the breaking cracks (DrawBlockDamage). Its vertices
// are section local, in blocks.
type BlockMesh struct {
	gpu   gpuBlockMesh
	quads int
}

// NewBlockMesh uploads a mesh's quads, translucent ones included; nil when
// it has none. dedupe drops quads at the same corners as an earlier one
// (coplanar overlays, as a grass block's tinted sides, that Minecraft does
// not draw with an override texture).
func NewBlockMesh(m *mesh.Mesh, dedupe bool) *BlockMesh {
	data := append(append([]uint32(nil), m.Data...), m.Trans...)
	if dedupe {
		data = dedupeQuads(data)
	}
	if len(data) == 0 {
		return nil
	}
	b := &BlockMesh{quads: len(data) / (4 * mesh.VertexWords)}
	b.upload(data)
	return b
}

// dedupeQuads keeps the first quad of each set of corner positions.
func dedupeQuads(data []uint32) []uint32 {
	const qw = 4 * mesh.VertexWords
	type key [8]uint32
	seen := map[key]bool{}
	out := data[:0]
	for q := 0; q+qw <= len(data); q += qw {
		var k key
		for v := range 4 {
			w := data[q+v*mesh.VertexWords:]
			k[2*v], k[2*v+1] = w[0]&(1<<30-1), w[1]>>26 // position bits only (mesh.VertexWords)
		}
		// the same corners in any order and winding
		for i := 0; i < 4; i++ {
			for j := i + 1; j < 4; j++ {
				if k[2*j] < k[2*i] || k[2*j] == k[2*i] && k[2*j+1] < k[2*i+1] {
					k[2*i], k[2*j], k[2*i+1], k[2*j+1] = k[2*j], k[2*i], k[2*j+1], k[2*i+1]
				}
			}
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, data[q:q+qw]...)
	}
	return out
}

// Quads is the number of quads drawn.
func (b *BlockMesh) Quads() int { return b.quads }

// itemLights are RenderHelper's GL_LIGHT0 and GL_LIGHT1 directions
// (field_82884_b, field_82885_c), fixed in the world.
var itemLights = [2]mgl32.Vec3{mgl32.Vec3{0.2, 1, -0.7}.Normalize(), mgl32.Vec3{-0.2, 1, 0.7}.Normalize()}

// BlockItem is a BlockMesh placed in the world: M (in blocks, about X,Y,Z)
// maps the mesh's coordinates to the world, e.g. RenderItem's dropped
// block transform.
type BlockItem struct {
	Mesh       *BlockMesh
	X, Y, Z    float64
	M          mgl32.Mat4
	Sky, Block uint8 // light levels 0..15 at the item
}

// DrawBlockItems draws blocks as items in the world with Minecraft's
// item lighting (RenderHelper.enableStandardItemLighting: ambient 0.4 and
// two diffuse 0.6 lights fixed in the world) times the light colour, and
// the terrain fog. Call after Renderer.Render with the same camera.
func (r *Renderer) DrawBlockItems(cam *Camera, items []BlockItem) {
	if len(items) == 0 {
		return
	}
	vp := cam.ViewProj()
	fog := r.CurrentFog()
	for _, it := range items {
		model := mgl32.Translate3D(float32(it.X-cam.X), float32(it.Y-cam.Y), float32(it.Z-cam.Z)).Mul4(it.M)
		r.drawBlock(it.Mesh, vp.Mul4(model), model, r.LightColor(it.Sky, it.Block), lightCode(it.Sky, it.Block), fog, itemLights[0], itemLights[1], -1)
	}
}

// DrawBlockHand draws a block held in first person, over the world (depth
// is cleared, see BeginHands) with the fixed 70° projection of
// ItemRenderer.DrawHand. model maps the mesh into view space; lights turns
// the item lights into view space (renderItemInFirstPerson enables them
// after glRotatef(pitch, 1, 0, 0) and glRotatef(yaw, 0, 1, 0)).
func (r *Renderer) DrawBlockHand(b *BlockMesh, aspect float32, model, lights mgl32.Mat4, sky, block uint8) {
	if b == nil {
		return
	}
	clearHandDepth()
	proj := mgl32.Perspective(mgl32.DegToRad(70), aspect, 0.05, 10)
	l0 := lights.Mul4x1(itemLights[0].Vec4(0)).Vec3()
	l1 := lights.Mul4x1(itemLights[1].Vec4(0)).Vec3()
	r.drawBlock(b, proj.Mul4(model), model, r.LightColor(sky, block), lightCode(sky, block), Fog{Start: 1e6, End: 2e6}, l0, l1, -1)
}

// SetDamageTextures sets the breaking crack textures, destroy_stage_0..9
// (RenderGlobal.destroyBlockIcons); they share the largest one's size.
func (r *Renderer) SetDamageTextures(imgs []image.Image) {
	size := 1
	for _, img := range imgs {
		size = max(size, img.Bounds().Dx(), img.Bounds().Dy())
	}
	layers := make([][]byte, len(imgs))
	for i, img := range imgs {
		l := image.NewNRGBA(image.Rect(0, 0, size, size))
		draw.Draw(l, l.Bounds(), img, img.Bounds().Min, draw.Src)
		layers[i] = l.Pix
	}
	if len(layers) > 0 {
		r.uploadDamage(size, layers)
	}
}

// DrawBlockDamage draws the cracks of a block being broken over b, the
// block's own mesh (Mesher.BuildCell of its section at origin, the
// section's world position in blocks): RenderGlobal.drawBlockDamageTexture,
// the texture destroy_stage_<stage> at alpha 0.5 blended 2*src*dst, alpha
// test > 0.1, polygon offset -3,-3, depth written, fogged, unlit. The
// polygon offset is a depth bias plus a pull towards the eye of 0.2 % of
// the distance (blockWGSL damagePull): at the 0.05 near plane a float depth
// buffer and the terrain's own rounding need about 0.04 % at 6 blocks.
func (r *Renderer) DrawBlockDamage(cam *Camera, b *BlockMesh, origin [3]int, stage int) {
	if stage < 0 {
		return
	}
	model := mgl32.Translate3D(float32(float64(origin[0])-cam.X), float32(float64(origin[1])-cam.Y), float32(float64(origin[2])-cam.Z))
	r.drawBlock(b, cam.ViewProj(), model, mgl32.Vec3{1, 1, 1}, 0, r.CurrentFog(), mgl32.Vec3{}, mgl32.Vec3{}, stage)
}
