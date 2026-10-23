// Package extrude turns a 2D sprite into a 3D mesh the way the reference draws
// held and dropped items: the sprite becomes a thin slab one texel thick.
// The front and back are single quads textured with the whole sprite (the
// renderer discards transparent texels), and every opaque texel that borders
// a transparent one or the image edge gets a side face, coloured by
// sampling that texel's centre.
//
// It is pure Go with no GL: the output is a flat triangle list any renderer
// can upload.
package extrude

import "image"

// Vertex is one mesh vertex. Positions span [-0.5, 0.5] in X and Y, with
// the slab centred on Z = 0; UV is in [0,1] with V pointing down the image.
type Vertex struct {
	Pos    [3]float32
	UV     [2]float32
	Normal [3]float32
}

// Mesh is a triangle list.
type Mesh struct {
	Verts []Vertex
}

// AlphaCut is the alpha below which a texel counts as transparent.
const AlphaCut = 128

// Extrude builds the slab for img. Non-square images are fitted into the
// unit square keeping their aspect ratio.
func Extrude(img image.Image) *Mesh {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return &Mesh{}
	}
	px := float32(1) / float32(max(w, h)) // one texel in model units
	x0, y0 := -float32(w)*px/2, float32(h)*px/2
	t := px / 2 // half thickness
	opaque := func(x, y int) bool {
		if x < 0 || y < 0 || x >= w || y >= h {
			return false
		}
		_, _, _, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
		return a>>8 >= AlphaCut
	}
	m := &Mesh{}
	// corner position of texel grid point (gx, gy)
	pos := func(gx, gy int, z float32) [3]float32 {
		return [3]float32{x0 + float32(gx)*px, y0 - float32(gy)*px, z}
	}
	uvAt := func(gx, gy int) [2]float32 { return [2]float32{float32(gx) / float32(w), float32(gy) / float32(h)} }

	// front (+Z) and back (-Z)
	m.quad([4][3]float32{pos(0, h, t), pos(w, h, t), pos(w, 0, t), pos(0, 0, t)},
		[4][2]float32{uvAt(0, h), uvAt(w, h), uvAt(w, 0), uvAt(0, 0)}, [3]float32{0, 0, 1})
	m.quad([4][3]float32{pos(w, h, -t), pos(0, h, -t), pos(0, 0, -t), pos(w, 0, -t)},
		[4][2]float32{uvAt(w, h), uvAt(0, h), uvAt(0, 0), uvAt(w, 0)}, [3]float32{0, 0, -1})

	// sides
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !opaque(x, y) {
				continue
			}
			c := [2]float32{(float32(x) + 0.5) / float32(w), (float32(y) + 0.5) / float32(h)}
			uv := [4][2]float32{c, c, c, c}
			if !opaque(x, y-1) { // top edge
				m.quad([4][3]float32{pos(x, y, t), pos(x+1, y, t), pos(x+1, y, -t), pos(x, y, -t)}, uv, [3]float32{0, 1, 0})
			}
			if !opaque(x, y+1) { // bottom edge
				m.quad([4][3]float32{pos(x, y+1, -t), pos(x+1, y+1, -t), pos(x+1, y+1, t), pos(x, y+1, t)}, uv, [3]float32{0, -1, 0})
			}
			if !opaque(x-1, y) { // left edge
				m.quad([4][3]float32{pos(x, y+1, -t), pos(x, y+1, t), pos(x, y, t), pos(x, y, -t)}, uv, [3]float32{-1, 0, 0})
			}
			if !opaque(x+1, y) { // right edge
				m.quad([4][3]float32{pos(x+1, y+1, t), pos(x+1, y+1, -t), pos(x+1, y, -t), pos(x+1, y, t)}, uv, [3]float32{1, 0, 0})
			}
		}
	}
	return m
}

// quad appends two counter-clockwise triangles (seen from the normal side).
func (m *Mesh) quad(p [4][3]float32, uv [4][2]float32, n [3]float32) {
	for _, i := range [6]int{0, 1, 2, 0, 2, 3} {
		m.Verts = append(m.Verts, Vertex{p[i], uv[i], n})
	}
}
