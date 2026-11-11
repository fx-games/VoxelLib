package main

// Scope lenses: TACZ's BedrockAttachmentModel.renderScope/renderSight on
// the hand pass stencil. A scope shows its reticle (division) through each
// ocular inside a disc that grows with aiming, the gun hidden over it;
// a sight shows its reticle through its oculars, always on top.

import (
	"image"
	"image/color"
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/tacz"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/voxel/render"
)

const (
	// lensView127: the gun and hands draw where 127 > stencil, i.e. not
	// inside an ocular's view disc (whose inverted value is above 127).
	lensView127 = 127
	// The view disc (renderOcularAndDivision): radius lensDiscRadius·aim
	// on a plane lensDiscDepth away, centred on the ocular.
	lensDiscRadius = 80
	lensDiscDepth  = 90
	lensDiscSides  = 90
)

// lensView is a scope's parts uploaded (handles; -1 for absent parts).
type lensView struct {
	scope, sight bool
	ring, body   int
	oculars      []lensOcular
	divisions    []int
	disc         int
}

type lensOcular struct {
	handle, bone int
	scope        bool
}

func (g *game) lensView(l *tacz.Lens) (*lensView, error) {
	add := func(m *model.Model) (int, error) {
		if m == nil {
			return -1, nil
		}
		return g.Models.Add(m)
	}
	v := &lensView{scope: l.Scope, sight: l.Sight}
	var err error
	if v.ring, err = add(l.Ring); err != nil {
		return nil, err
	}
	if v.body, err = add(l.Body); err != nil {
		return nil, err
	}
	for _, o := range l.Oculars {
		h, err := add(o.Part)
		if err != nil {
			return nil, err
		}
		v.oculars = append(v.oculars, lensOcular{h, o.Bone, o.Scope})
	}
	for _, d := range l.Divisions {
		h, err := add(d)
		if err != nil {
			return nil, err
		}
		v.divisions = append(v.divisions, h)
	}
	if g.lensDisc < 0 {
		if g.lensDisc, err = g.Models.Add(discModel()); err != nil {
			return nil, err
		}
	}
	v.disc = g.lensDisc
	return v, nil
}

// discModel is a unit disc facing +Z (a fan of lensDiscSides triangles).
func discModel() *model.Model {
	white := image.NewRGBA(image.Rect(0, 0, 1, 1))
	white.Set(0, 0, color.White)
	m := &model.Model{Name: "lens disc", Textures: []image.Image{white}, Clips: map[string]*model.Clip{},
		Bones: []model.Bone{{Name: "disc", Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}}}
	me := model.Mesh{Verts: []model.Vertex{{Normal: mgl32.Vec3{0, 0, 1}, Weights: [4]float32{1}}}}
	for j := 0; j <= lensDiscSides; j++ {
		a := float64(j) * 2 * math.Pi / lensDiscSides
		me.Verts = append(me.Verts, model.Vertex{Pos: mgl32.Vec3{float32(math.Cos(a)), float32(math.Sin(a)), 0},
			Normal: mgl32.Vec3{0, 0, 1}, Weights: [4]float32{1}})
		if j > 0 {
			me.Indices = append(me.Indices, 0, uint32(j), uint32(j+1), 0, uint32(j+1), uint32(j)) // both faces
		}
	}
	m.Meshes = []model.Mesh{me}
	return m
}

// lensInstances are the lens draws before the gun's, in TACZ's order.
func (g *game) lensInstances(v *gunView, view mgl32.Mat4, aim float32, sky, blk uint8) []render.ModelInstance {
	l := v.lens
	if l == nil {
		return nil
	}
	var ins []render.ModelInstance
	draw := func(h int, st render.HandStencil, skin []mgl32.Mat4) {
		if h >= 0 {
			ins = append(ins, render.ModelInstance{Model: h, Skin: skin, Sky: sky, Block: blk, Stencil: st})
		}
	}
	part := func(h int, st render.HandStencil) { draw(h, st, v.skin) }
	test := func(c render.StencilCompare, ref int) render.HandStencil {
		return render.HandStencil{Test: true, Compare: c, Ref: uint8(ref)}
	}
	// renderOcularStencil: the oculars of one kind mark their own number.
	marks := func(scope bool) {
		for i := len(l.oculars) - 1; i >= 0; i-- {
			if l.oculars[i].scope == scope {
				part(l.oculars[i].handle, render.HandStencil{Test: true, Compare: render.StencilGreater, Ref: uint8(i + 1),
					Pass: render.StencilReplace, NoColor: true, NoDepthWrite: true})
			}
		}
	}
	plain := render.HandStencil{Test: true} // in the stencil pass, always drawn
	switch {
	case l.scope && l.sight: // renderBoth
		part(l.ring, plain)
		marks(true)
		part(l.body, test(render.StencilEqual, 0))
		marks(false)
		g.lensViews(v, view, aim, true, draw, part, test)
	case l.scope: // renderScope
		part(l.ring, plain)
		marks(false)
		part(l.body, test(render.StencilEqual, 0))
		g.lensViews(v, view, aim, false, draw, part, test)
	case l.sight: // renderSight
		marks(false)
		for i, d := range l.divisions {
			st := test(render.StencilEqual, i+1)
			st.DepthAlways = true
			part(d, st)
		}
		part(l.body, plain)
		part(l.ring, plain)
	default:
		part(l.body, plain)
		part(l.ring, plain)
	}
	return ins
}

// lensViews is renderOcularAndDivision: each ocular (selective: only the
// ocular_scope ones) inverts its mark inside the view disc, is masked black
// outside it and shows the magnified scope view (App.ScopeFOV) and its
// reticle inside; the others show their reticle on the mark.
func (g *game) lensViews(v *gunView, view mgl32.Mat4, aim float32, selective bool,
	draw func(int, render.HandStencil, []mgl32.Mat4), part func(int, render.HandStencil),
	test func(render.StencilCompare, int) render.HandStencil) {
	l := v.lens
	for i, o := range l.oculars {
		if selective && !o.scope || aim <= 0 {
			continue
		}
		c := view.Mul4(v.skin[o.bone]).Mul4(v.m.Bones[o.bone].InvBind.Inv()).Col(3).Vec3()
		// The disc's angular size, at the ocular's depth (inside the
		// hand projection's depth range).
		r := float32(lensDiscRadius) / lensDiscDepth * float32(math.Abs(float64(c.Z()))) * aim
		disc := view.Inv().Mul4(mgl32.Translate3D(c.X(), c.Y(), c.Z())).Mul4(mgl32.Scale3D(r, r, 1))
		draw(l.disc, render.HandStencil{Test: true, Compare: render.StencilEqual, Ref: uint8(i + 1),
			Pass: render.StencilInvert, NoColor: true, NoDepthWrite: true}, []mgl32.Mat4{disc})
	}
	for i := 0; i < len(l.oculars) && i < len(l.divisions); i++ {
		if selective && !l.oculars[i].scope {
			part(l.divisions[i], test(render.StencilEqual, i+1))
			continue
		}
		part(l.oculars[i].handle, test(render.StencilEqual, i+1)) // the black mask
		inside := test(render.StencilEqual, ^(i+1)&0xFF)
		inside.ScopeView = true
		part(l.oculars[i].handle, inside) // the magnified world in the disc
		part(l.divisions[i], test(render.StencilEqual, ^(i+1)&0xFF))
	}
}
