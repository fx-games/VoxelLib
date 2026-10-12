// Package model is a format-independent animated model: a bone hierarchy,
// skinned triangle meshes, textures and animation clips. Importers
// (model/gltf, importer/roblox, importer/mcmodel) produce it, an Animator
// poses it, and render.ModelRenderer draws it. It is pure Go (no GL).
//
// Conventions: Y up, the model faces +Z, one unit is one block (metre).
package model

import (
	"image"
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// Bone is one joint. Its local transform is T * R * S relative to its
// parent (or the model origin for roots).
type Bone struct {
	Name   string
	Parent int // -1 for roots; parents always precede children
	T      mgl32.Vec3
	R      mgl32.Quat
	S      mgl32.Vec3
	// Euler bones (Bedrock/Blockbench) rotate by EulerZYX(Euler) instead of
	// R; additive rotation channels add to the angles (degrees).
	UseEuler bool
	Euler    mgl32.Vec3
	// InvBind maps model space at bind time to bone space.
	InvBind mgl32.Mat4
}

// Vertex is a skinned vertex in bind-pose model space.
type Vertex struct {
	Pos     mgl32.Vec3
	Normal  mgl32.Vec3
	UV      mgl32.Vec2
	Joints  [4]uint8
	Weights [4]float32
}

// Mesh is an indexed triangle list drawn with one texture.
type Mesh struct {
	Verts   []Vertex
	Indices []uint32
	Texture int // index into Model.Textures, -1 for none
}

// Model is an animated model.
type Model struct {
	Name     string
	Bones    []Bone
	Meshes   []Mesh
	Textures []image.Image
	Clips    map[string]*Clip
	// Tags carries importer hints, e.g. "flying" for winged creatures.
	Tags map[string]bool
	// root is a transform baked in by Transform, applied above every bone.
	root mgl32.Mat4
}

// Bone returns the index of a named bone, or -1.
func (m *Model) Bone(name string) int {
	for i, b := range m.Bones {
		if b.Name == name {
			return i
		}
	}
	return -1
}

// ClipNames returns the clip names in sorted order.
func (m *Model) ClipNames() []string {
	out := make([]string, 0, len(m.Clips))
	for n := range m.Clips {
		out = append(out, n)
	}
	sortStrings(out)
	return out
}

// Bounds returns the bind-pose bounding box.
func (m *Model) Bounds() (min, max mgl32.Vec3) {
	inf := float32(math.Inf(1))
	min, max = mgl32.Vec3{inf, inf, inf}, mgl32.Vec3{-inf, -inf, -inf}
	for _, me := range m.Meshes {
		for _, v := range me.Verts {
			for k := 0; k < 3; k++ {
				min[k] = float32(math.Min(float64(min[k]), float64(v.Pos[k])))
				max[k] = float32(math.Max(float64(max[k]), float64(v.Pos[k])))
			}
		}
	}
	return
}

// Transform bakes a transform into the bind pose (vertices, root bones and
// inverse bind matrices), e.g. to rescale or turn an imported model.
func (m *Model) Transform(t mgl32.Mat4) {
	nt := t.Mat3().Inv().Transpose()
	for i := range m.Meshes {
		for j := range m.Meshes[i].Verts {
			v := &m.Meshes[i].Verts[j]
			v.Pos = mgl32.TransformCoordinate(v.Pos, t)
			v.Normal = nt.Mul3x1(v.Normal).Normalize()
		}
	}
	inv := t.Inv()
	for i := range m.Bones {
		m.Bones[i].InvBind = m.Bones[i].InvBind.Mul4(inv)
	}
	m.root = t.Mul4(m.root0())
}

func (m *Model) root0() mgl32.Mat4 {
	if m.root == (mgl32.Mat4{}) {
		return mgl32.Ident4()
	}
	return m.root
}

// EulerZYX builds a rotation from angles in degrees applied X, then Y, then
// Z (the matrix Rz*Ry*Rx), as Blockbench and Bedrock do.
func EulerZYX(deg mgl32.Vec3) mgl32.Quat {
	r := func(a float32) float32 { return a * math.Pi / 180 }
	qx := mgl32.QuatRotate(r(deg[0]), mgl32.Vec3{1, 0, 0})
	qy := mgl32.QuatRotate(r(deg[1]), mgl32.Vec3{0, 1, 0})
	qz := mgl32.QuatRotate(r(deg[2]), mgl32.Vec3{0, 0, 1})
	return qz.Mul(qy).Mul(qx)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// Subtree cuts the named bone and everything under it out of m as a new,
// static single-bone model, e.g. a weapon a character holds. Vertices are
// posed as at rest and expressed in the named bone's own frame (origin at
// its pivot), so callers can place the part freely. Textures are shared.
// It returns nil if the bone doesn't exist or has no geometry.
func (m *Model) Subtree(name string) *Model {
	top := m.Bone(name)
	if top < 0 {
		return nil
	}
	under := func(b int) bool {
		for ; b >= 0; b = m.Bones[b].Parent {
			if b == top {
				return true
			}
		}
		return false
	}
	world := m.BindWorld()
	root := m.root0()
	toLocal := root.Mul4(world[top]).Inv()
	out := &Model{Name: m.Name + "/" + name, Textures: m.Textures, Clips: map[string]*Clip{}, Tags: map[string]bool{},
		Bones: []Bone{{Name: name, Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}}}
	for _, me := range m.Meshes {
		var nm Mesh
		nm.Texture = me.Texture
		remap := map[uint32]uint32{}
		for i := 0; i+2 < len(me.Indices); i += 3 {
			tri := me.Indices[i : i+3]
			if !under(int(me.Verts[tri[0]].Joints[0])) {
				continue
			}
			for _, vi := range tri {
				ni, ok := remap[vi]
				if !ok {
					v := me.Verts[vi]
					xf := toLocal.Mul4(root).Mul4(world[v.Joints[0]]).Mul4(m.Bones[v.Joints[0]].InvBind)
					v.Pos = mgl32.TransformCoordinate(v.Pos, xf)
					v.Normal = xf.Mat3().Mul3x1(v.Normal).Normalize()
					v.Joints, v.Weights = [4]uint8{}, [4]float32{1}
					ni = uint32(len(nm.Verts))
					remap[vi] = ni
					nm.Verts = append(nm.Verts, v)
				}
				nm.Indices = append(nm.Indices, ni)
			}
		}
		if len(nm.Indices) > 0 {
			out.Meshes = append(out.Meshes, nm)
		}
	}
	if len(out.Meshes) == 0 {
		return nil
	}
	return out
}
