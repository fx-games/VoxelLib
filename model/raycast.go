package model

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// RayHit is where a ray first meets a posed model.
type RayHit struct {
	T    float32    // distance along the ray, in units of dir's length
	Pos  mgl32.Vec3 // hit point in model space
	Bone int        // bone with the most weight on the hit triangle
}

// Pose is a model's vertices deformed by one set of skin matrices, with a
// bounding box per mesh, ready for repeated raycasts.
type Pose struct {
	Pos    [][]mgl32.Vec3 // per mesh, per vertex
	lo, hi []mgl32.Vec3   // per mesh bounds
}

// Pose deforms the vertices by skin, the matrices an Animator's Skin returns
// (nil = bind pose), exactly as the model shader does. dst is reused.
func (m *Model) Pose(skin []mgl32.Mat4, dst *Pose) *Pose {
	if dst == nil {
		dst = &Pose{}
	}
	n := len(m.Meshes)
	for len(dst.Pos) < n {
		dst.Pos = append(dst.Pos, nil)
	}
	dst.Pos, dst.lo, dst.hi = dst.Pos[:n], append(dst.lo[:0], make([]mgl32.Vec3, n)...), append(dst.hi[:0], make([]mgl32.Vec3, n)...)
	for i, me := range m.Meshes {
		pos := dst.Pos[i][:0]
		inf := float32(math.Inf(1))
		lo, hi := mgl32.Vec3{inf, inf, inf}, mgl32.Vec3{-inf, -inf, -inf}
		for _, v := range me.Verts {
			p := skinPos(skin, v)
			for k := 0; k < 3; k++ {
				lo[k], hi[k] = min(lo[k], p[k]), max(hi[k], p[k])
			}
			pos = append(pos, p)
		}
		dst.Pos[i], dst.lo[i], dst.hi[i] = pos, lo, hi
	}
	return dst
}

// Raycast intersects a ray (model space) with the model posed by skin and
// returns the nearest hit with 0 <= T <= maxT. To cast often against one
// pose, build it once with Pose and use RaycastPose.
func (m *Model) Raycast(skin []mgl32.Mat4, orig, dir mgl32.Vec3, maxT float32) (RayHit, bool) {
	return m.RaycastPose(m.Pose(skin, nil), orig, dir, maxT)
}

// RaycastPose intersects a ray with a posed model: meshes whose bounds the
// ray misses are skipped, then every triangle of the rest is tested.
func (m *Model) RaycastPose(p *Pose, orig, dir mgl32.Vec3, maxT float32) (RayHit, bool) {
	best := RayHit{T: maxT, Bone: -1}
	hit := false
	for mi, me := range m.Meshes {
		if !rayBox(orig, dir, best.T, p.lo[mi], p.hi[mi]) {
			continue
		}
		pos := p.Pos[mi]
		for i := 0; i+2 < len(me.Indices); i += 3 {
			a, b, c := pos[me.Indices[i]], pos[me.Indices[i+1]], pos[me.Indices[i+2]]
			if t, ok := rayTri(orig, dir, a, b, c); ok && t <= best.T {
				v := me.Verts[me.Indices[i]]
				j := 0
				for k := 1; k < 4; k++ {
					if v.Weights[k] > v.Weights[j] {
						j = k
					}
				}
				best = RayHit{T: t, Pos: orig.Add(dir.Mul(t)), Bone: int(v.Joints[j])}
				hit = true
			}
		}
	}
	return best, hit
}

// skinPos deforms a vertex like the model shader: the weighted sum of its
// joints' skin matrices.
func skinPos(skin []mgl32.Mat4, v Vertex) mgl32.Vec3 {
	if len(skin) == 0 {
		return v.Pos
	}
	var s mgl32.Mat4
	for k := 0; k < 4; k++ {
		if w := v.Weights[k]; w != 0 && int(v.Joints[k]) < len(skin) {
			s = s.Add(skin[v.Joints[k]].Mul(w))
		}
	}
	return mgl32.TransformCoordinate(v.Pos, s)
}

// rayBox reports whether the ray meets the box within 0..maxT (slabs).
func rayBox(o, d mgl32.Vec3, maxT float32, lo, hi mgl32.Vec3) bool {
	t0, t1 := float32(0), maxT
	for k := 0; k < 3; k++ {
		if d[k] == 0 {
			if o[k] < lo[k] || o[k] > hi[k] {
				return false
			}
			continue
		}
		a, b := (lo[k]-o[k])/d[k], (hi[k]-o[k])/d[k]
		if a > b {
			a, b = b, a
		}
		t0, t1 = max(t0, a), min(t1, b)
		if t0 > t1 {
			return false
		}
	}
	return true
}

// rayTri is Möller–Trumbore, two-sided.
func rayTri(o, d, a, b, c mgl32.Vec3) (float32, bool) {
	e1, e2 := b.Sub(a), c.Sub(a)
	p := d.Cross(e2)
	det := e1.Dot(p)
	if math.Abs(float64(det)) < 1e-12 {
		return 0, false
	}
	inv := 1 / det
	s := o.Sub(a)
	u := s.Dot(p) * inv
	if u < 0 || u > 1 {
		return 0, false
	}
	q := s.Cross(e1)
	v := d.Dot(q) * inv
	if v < 0 || u+v > 1 {
		return 0, false
	}
	t := e2.Dot(q) * inv
	return t, t >= 0
}
