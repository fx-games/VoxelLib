// Package mcjava imports Minecraft Java Edition 1.7.10 entity models
// (net.minecraft.client.model.ModelBase subclasses) and poses them the way
// the game does.
//
// A Geometry is what a ModelBase constructor builds: ModelRenderer parts
// with rotation points, rotation angles, child parts and addBox cuboids
// (with texture offset, mirror flag, inflation and the part's texture size
// at the time of the call). A converter that reads model classes writes it
// as JSON; Build turns it into a model.Model that renders exactly like
// ModelBox/TexturedQuad (same vertices, quad order, UVs, mirror handling
// and normals).
//
// Minecraft animates these models in code: setRotationAngles writes each
// part's rotation point and angles every frame, render() wraps parts in GL
// transforms, and the entity renderer adds preRenderCallback transforms. A
// Pose carries exactly that state; Skin turns it into skinning matrices
// for render.ModelInstance.
//
// Coordinates: Minecraft model space is in pixels (1/16 block), Y down,
// facing -Z. The entity renderer draws it with
// glScalef(-1,-1,1) · preRenderCallback · glTranslatef(0, -24/16-1/128, 0)
// · scale 1/16, after turning by 180-yaw. The resulting model.Model is in
// model package conventions (blocks, Y up, facing +Z, origin at the
// entity's feet): engine = Ry(180) · corpse · flip · root · lift · px.
package mcjava

import (
	"fmt"
	"image"
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

// Box is one ModelRenderer.addBox call (a ModelBox).
type Box struct {
	Pos     [3]float32 `json:"pos"`     // lower corner in part space, pixels
	Size    [3]int     `json:"size"`    // width, height, depth (ints, as addBox takes them)
	Inflate float32    `json:"inflate"` // the scale factor argument (grows every side)
	UV      [2]int     `json:"uv"`      // texture offset at the time of the call
	Mirror  bool       `json:"mirror"`  // the part's mirror flag at the time of the call
	TexSize [2]float32 `json:"tex"`     // the part's textureWidth/Height at the time of the call
	// Plane makes this a single quad instead of a cuboid: "left" (the x+
	// side), "top" (y-) or "back" (z-), as CustomNPC's ModelPlane builds
	// it (same corners, UV rectangle at UV of the plane's two sizes).
	Plane string `json:"plane,omitempty"`
	// Item2D replaces the cuboid with CustomNPC's Model2DRenderer: a
	// texture rectangle extruded like a held item (ItemRenderer.
	// renderItemIn2D: front, back and one side strip per texel row and
	// column; transparent texels are cut by the alpha test, so the shape
	// follows the skin without depending on it).
	Item2D *Item2D `json:"item2d,omitempty"`
}

// Item2D is a Model2DRenderer display list.
type Item2D struct {
	UV    [4]float32  `json:"uv"`    // u1, v1, u2, v2 (0..1)
	W     int         `json:"w"`     // texel columns
	H     int         `json:"h"`     // texel rows
	Depth float32     `json:"depth"` // renderItemIn2D's thickness argument (the render scale)
	M     [16]float32 `json:"m"`     // the list's GL transform, in blocks (column-major)
}

// Part is one ModelRenderer.
type Part struct {
	Name   string     `json:"name"`
	Parent int        `json:"parent"` // index of the part it was added to with addChild, -1 for none
	Pivot  [3]float32 `json:"pivot"`  // rotationPointX/Y/Z, pixels
	Rot    [3]float32 `json:"rot"`    // rotateAngleX/Y/Z, radians
	Offset [3]float32 `json:"offset,omitempty"`
	Hidden bool       `json:"hidden,omitempty"` // isHidden or !showModel after construction
	// Same is 1 + the index of the part this one is a copy of: one
	// ModelRenderer added as a child to several parents is drawn under
	// each, in the original's pose (see PartPose.Own). 0 for an ordinary part.
	Same  int   `json:"same,omitempty"`
	Boxes []Box `json:"boxes"`
}

// Geometry is a constructed ModelBase.
type Geometry struct {
	Name    string `json:"name"`
	TexSize [2]int `json:"tex"` // ModelBase textureWidth/Height
	Parts   []Part `json:"parts"`
	// Render lists the root parts the model's render() draws, in order.
	// Parts outside it (and their children) are never drawn, e.g.
	// ModelBiped's ears and cloak.
	Render []int `json:"render"`
}

// Part returns the index of a named part, or -1.
func (g *Geometry) Part(name string) int {
	for i, p := range g.Parts {
		if p.Name == name {
			return i
		}
	}
	return -1
}

// BoxCount returns the number of cuboids.
func (g *Geometry) BoxCount() int {
	n := 0
	for _, p := range g.Parts {
		n += len(p.Boxes)
	}
	return n
}

// Lift is the entity renderer's glTranslatef(0, -24*0.0625-0.0078125, 0).
const Lift = 24*0.0625 + 0.0078125

// flip is glScalef(-1,-1,1) followed by the renderer's 180° turn, which
// together map Minecraft's entity frame to model package conventions.
var flip = mgl32.Scale3D(1, -1, -1)

// Validate checks part references.
func (g *Geometry) Validate() error {
	for i, p := range g.Parts {
		if p.Parent >= i || p.Parent < -1 {
			return fmt.Errorf("mcjava: %s: part %d (%s) has parent %d; parents must precede children", g.Name, i, p.Name, p.Parent)
		}
	}
	for i, p := range g.Parts {
		if p.Same < 0 || p.Same > i {
			return fmt.Errorf("mcjava: %s: part %d copies part %d, which must precede it", g.Name, i, p.Same-1)
		}
	}
	for _, r := range g.Render {
		if r < 0 || r >= len(g.Parts) {
			return fmt.Errorf("mcjava: %s: render part %d out of range", g.Name, r)
		}
	}
	if len(g.Parts) > model.MaxBones {
		return fmt.Errorf("mcjava: %s: %d parts (max %d)", g.Name, len(g.Parts), model.MaxBones)
	}
	return nil
}

// Build turns a geometry into a model with one bone per part (bone i is
// part i, named like it) and one mesh drawn with tex. The bind pose is the
// constructed pose (RestPose). Parts not reachable from Render get no
// triangles.
func Build(g *Geometry, tex image.Image) (*model.Model, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}
	m := &model.Model{Name: g.Name, Clips: map[string]*model.Clip{}, Tags: map[string]bool{}}
	if tex == nil {
		tex = image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	m.Textures = []image.Image{tex}
	scale := float32(1.0 / 16)
	for _, p := range g.Parts {
		b := model.Bone{Name: p.Name, Parent: p.Parent, R: eulerMC(p.Rot), S: mgl32.Vec3{1, 1, 1}}
		t := mgl32.Vec3{p.Pivot[0] + 16*p.Offset[0], p.Pivot[1] + 16*p.Offset[1], p.Pivot[2] + 16*p.Offset[2]}
		if p.Parent < 0 {
			// flip · lift · scale(1/16) · T(pivot) · R = T(flip(lift + pivot/16)) · (flip R) · S(1/16)
			t = flip.Mul4x1(t.Mul(scale).Sub(mgl32.Vec3{0, Lift, 0}).Vec4(1)).Vec3()
			b.R = mgl32.Mat4ToQuat(flip).Mul(b.R)
			b.S = mgl32.Vec3{scale, scale, scale}
		}
		b.T = t
		m.Bones = append(m.Bones, b)
	}
	world := m.BindWorld()
	for i := range m.Bones {
		m.Bones[i].InvBind = world[i].Inv()
	}
	drawn := g.drawn()
	me := model.Mesh{Texture: 0}
	for pi, p := range g.Parts {
		if !drawn[pi] {
			continue
		}
		w := world[pi]
		nm := w.Mat3()
		for _, b := range p.Boxes {
			for _, q := range boxQuads(b) {
				n := q.normal()
				if n == (mgl32.Vec3{}) && degenerate(q) {
					continue // a face of a zero-size side: draws nothing
				}
				en := nm.Mul3x1(n)
				if l := en.Len(); l > 0 {
					en = en.Mul(1 / l)
				}
				base := uint32(len(me.Verts))
				for k := 0; k < 4; k++ {
					v := model.Vertex{Pos: mgl32.TransformCoordinate(q.pos[k], w), Normal: en, UV: q.uv[k]}
					v.Joints[0], v.Weights[0] = uint16(pi), 1
					me.Verts = append(me.Verts, v)
				}
				// GL_QUADS draws v0 v1 v2 and v0 v2 v3.
				me.Indices = append(me.Indices, base, base+1, base+2, base, base+2, base+3)
			}
		}
	}
	m.Meshes = []model.Mesh{me}
	return m, nil
}

// drawn marks the parts render() draws: the Render roots and their
// children.
func (g *Geometry) drawn() []bool {
	d := make([]bool, len(g.Parts))
	for _, r := range g.Render {
		d[r] = true
	}
	for i, p := range g.Parts {
		if p.Parent >= 0 && d[p.Parent] {
			d[i] = true
		}
	}
	return d
}

// quad is a TexturedQuad: 4 vertices in part space (pixels) and their UVs.
type quad struct {
	pos [4]mgl32.Vec3
	uv  [4]mgl32.Vec2
}

// normal is TexturedQuad.draw's normal: (v2-v1) × (v0-v1), normalized
// (Vec3.subtract is reversed in 1.7.10), zero if shorter than 1e-4.
func (q quad) normal() mgl32.Vec3 {
	a := q.pos[0].Sub(q.pos[1])
	b := q.pos[2].Sub(q.pos[1])
	c := b.Cross(a)
	l := float32(math.Sqrt(float64(c.Dot(c))))
	if l < 1e-4 {
		return mgl32.Vec3{}
	}
	return c.Mul(1 / l)
}

func degenerate(q quad) bool {
	a, b := q.pos[1].Sub(q.pos[0]), q.pos[3].Sub(q.pos[0])
	return a.Cross(b).Len() == 0
}

// boxQuads replicates ModelBox's constructor (or ModelPlane's for a
// plane): the six TexturedQuads in
// order (east, west, up, down, north, south as Minecraft names x+, x-, y-
// (top in model space), y+, z-, z+), with mirror swapping x and reversing
// each quad's vertex order.
func boxQuads(b Box) []quad {
	if b.Item2D != nil {
		return item2DQuads(b.Item2D)
	}
	x, y, z := b.Pos[0], b.Pos[1], b.Pos[2]
	w, h, d := b.Size[0], b.Size[1], b.Size[2]
	f4 := x + float32(w)
	f5 := y + float32(h)
	f6 := z + float32(d)
	x -= b.Inflate
	y -= b.Inflate
	z -= b.Inflate
	f4 += b.Inflate
	f5 += b.Inflate
	f6 += b.Inflate
	if b.Mirror {
		x, f4 = f4, x
	}
	v7 := mgl32.Vec3{x, y, z}
	v0 := mgl32.Vec3{f4, y, z}
	v1 := mgl32.Vec3{f4, f5, z}
	v2 := mgl32.Vec3{x, f5, z}
	v3 := mgl32.Vec3{x, y, f6}
	v4 := mgl32.Vec3{f4, y, f6}
	v5 := mgl32.Vec3{f4, f5, f6}
	v6 := mgl32.Vec3{x, f5, f6}
	u, v := b.UV[0], b.UV[1]
	tw, th := b.TexSize[0], b.TexSize[1]
	mk := func(p [4]mgl32.Vec3, u1, v1, u2, v2 int) quad {
		q := quad{pos: p}
		// TexturedQuad: v0 (u2,v1), v1 (u1,v1), v2 (u1,v2), v3 (u2,v2).
		q.uv[0] = mgl32.Vec2{float32(u2) / tw, float32(v1) / th}
		q.uv[1] = mgl32.Vec2{float32(u1) / tw, float32(v1) / th}
		q.uv[2] = mgl32.Vec2{float32(u1) / tw, float32(v2) / th}
		q.uv[3] = mgl32.Vec2{float32(u2) / tw, float32(v2) / th}
		if b.Mirror { // flipFace
			q.pos[0], q.pos[1], q.pos[2], q.pos[3] = q.pos[3], q.pos[2], q.pos[1], q.pos[0]
			q.uv[0], q.uv[1], q.uv[2], q.uv[3] = q.uv[3], q.uv[2], q.uv[1], q.uv[0]
		}
		return q
	}
	switch b.Plane {
	case "left":
		return []quad{mk([4]mgl32.Vec3{v4, v0, v1, v5}, u, v, u+d, v+h)}
	case "top":
		return []quad{mk([4]mgl32.Vec3{v4, v3, v7, v0}, u, v, u+w, v+d)}
	case "back":
		return []quad{mk([4]mgl32.Vec3{v0, v7, v2, v1}, u, v, u+w, v+h)}
	}
	return []quad{
		mk([4]mgl32.Vec3{v4, v0, v1, v5}, u+d+w, v+d, u+d+w+d, v+d+h),
		mk([4]mgl32.Vec3{v7, v3, v6, v2}, u, v+d, u+d, v+d+h),
		mk([4]mgl32.Vec3{v4, v3, v7, v0}, u+d, v, u+d+w, v+d),
		mk([4]mgl32.Vec3{v1, v2, v6, v5}, u+d+w, v+d, u+d+w+w, v),
		mk([4]mgl32.Vec3{v0, v7, v2, v1}, u+d, v+d, u+d+w, v+d+h),
		mk([4]mgl32.Vec3{v3, v4, v5, v6}, u+d+w+d, v+d, u+d+w+d+w, v+d+h),
	}
}

// eulerMC is ModelRenderer's rotation: glRotatef Z, then Y, then X, i.e.
// the matrix Rz·Ry·Rx, angles in radians.
func eulerMC(r [3]float32) mgl32.Quat {
	qx := mgl32.QuatRotate(r[0], mgl32.Vec3{1, 0, 0})
	qy := mgl32.QuatRotate(r[1], mgl32.Vec3{0, 1, 0})
	qz := mgl32.QuatRotate(r[2], mgl32.Vec3{0, 0, 1})
	return qz.Mul(qy).Mul(qx)
}

// Copy appends a copy of part i and its children (Part.Same set, so they
// share the originals' pose unless PartPose.Own), for render() code that
// draws a part more than once. The copy hangs under parent (-1: a root,
// which is appended to Render). It returns the new index of part i.
func (g *Geometry) Copy(i, parent int) int {
	idx := map[int]int{}
	for j, n := i, len(g.Parts); j < n; j++ {
		p := g.Parts[j]
		np, ok := idx[p.Parent]
		if j == i {
			np, ok = parent, true
		}
		if !ok {
			continue
		}
		c := p
		c.Parent = np
		c.Same = j + 1
		if p.Same > 0 {
			c.Same = p.Same
		}
		idx[j] = len(g.Parts)
		g.Parts = append(g.Parts, c)
	}
	if parent < 0 {
		g.Render = append(g.Render, idx[i])
	}
	return idx[i]
}

// item2DQuads is ItemRenderer.renderItemIn2D under the list's transform,
// in pixels.
func item2DQuads(it *Item2D) []quad {
	u1, v1, u2, v2 := it.UV[0], it.UV[1], it.UV[2], it.UV[3]
	w, h, t := it.W, it.H, it.Depth
	m := mgl32.Mat4(it.M)
	px := func(x, y, z, u, v float32) (mgl32.Vec3, mgl32.Vec2) {
		return mgl32.TransformCoordinate(mgl32.Vec3{x, y, z}, m).Mul(16), mgl32.Vec2{u, v}
	}
	var out []quad
	add := func(vs [4][5]float32) {
		var q quad
		for i, v := range vs {
			q.pos[i], q.uv[i] = px(v[0], v[1], v[2], v[3], v[4])
		}
		out = append(out, q)
	}
	add([4][5]float32{{0, 0, 0, u1, v2}, {1, 0, 0, u2, v2}, {1, 1, 0, u2, v1}, {0, 1, 0, u1, v1}})
	add([4][5]float32{{0, 1, -t, u1, v1}, {1, 1, -t, u2, v1}, {1, 0, -t, u2, v2}, {0, 0, -t, u1, v2}})
	f5 := 0.5 * (u1 - u2) / float32(w)
	f6 := 0.5 * (v2 - v1) / float32(h)
	for k := 0; k < w; k++ {
		f7 := float32(k) / float32(w)
		f8 := u1 + (u2-u1)*f7 - f5
		add([4][5]float32{{f7, 0, -t, f8, v2}, {f7, 0, 0, f8, v2}, {f7, 1, 0, f8, v1}, {f7, 1, -t, f8, v1}})
	}
	for k := 0; k < w; k++ {
		f7 := float32(k) / float32(w)
		f8 := u1 + (u2-u1)*f7 - f5
		f9 := f7 + 1/float32(w)
		add([4][5]float32{{f9, 1, -t, f8, v1}, {f9, 1, 0, f8, v1}, {f9, 0, 0, f8, v2}, {f9, 0, -t, f8, v2}})
	}
	for k := 0; k < h; k++ {
		f7 := float32(k) / float32(h)
		f8 := v2 + (v1-v2)*f7 - f6
		f9 := f7 + 1/float32(h)
		add([4][5]float32{{0, f9, 0, u1, f8}, {1, f9, 0, u2, f8}, {1, f9, -t, u2, f8}, {0, f9, -t, u1, f8}})
	}
	for k := 0; k < h; k++ {
		f7 := float32(k) / float32(h)
		f8 := v2 + (v1-v2)*f7 - f6
		add([4][5]float32{{1, f7, 0, u2, f8}, {0, f7, 0, u1, f8}, {0, f7, -t, u1, f8}, {1, f7, -t, u2, f8}})
	}
	return out
}
