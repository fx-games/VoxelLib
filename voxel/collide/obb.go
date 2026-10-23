package collide

import "math"

// OBB is a box that has been turned and scaled: Local, an axis aligned box in
// the object's own space, carried into the world by Axes and Pos. Axes[i] is
// where local axis i points in the world, scaled by that axis' scale (the
// columns of the object's transform); Pos is where the local origin lands.
// Axes must be orthogonal (rotation and scale, no shear).
type OBB struct {
	Local AABB
	Axes  [3][3]float64
	Pos   [3]float64
}

type vec = [3]float64

func dot(a, b vec) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func sub(a, b vec) vec     { return vec{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func cross(a, b vec) vec {
	return vec{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func unit(a vec) vec {
	l := math.Sqrt(dot(a, a))
	return vec{a[0] / l, a[1] / l, a[2] / l}
}

func (b AABB) center() vec {
	return vec{(b.MinX + b.MaxX) / 2, (b.MinY + b.MaxY) / 2, (b.MinZ + b.MaxZ) / 2}
}

func (b AABB) half() vec {
	return vec{(b.MaxX - b.MinX) / 2, (b.MaxY - b.MinY) / 2, (b.MaxZ - b.MinZ) / 2}
}

// RayAABB returns the distance along the ray (direction need not be unit; the
// distance is in multiples of it) at which it enters b, and the outward normal
// of the face it entered through. A ray starting inside returns 0 and a zero
// normal. Rays parallel to a slab are handled, not divided by.
func RayAABB(o, d [3]float64, b AABB) (t float64, normal [3]float64, ok bool) {
	lo, hi := vec{b.MinX, b.MinY, b.MinZ}, vec{b.MaxX, b.MaxY, b.MaxZ}
	near, far := math.Inf(-1), math.Inf(1)
	axis, sign := -1, 0.0
	for i := 0; i < 3; i++ {
		if d[i] == 0 {
			if o[i] < lo[i] || o[i] > hi[i] {
				return 0, normal, false
			}
			continue
		}
		t0, t1 := (lo[i]-o[i])/d[i], (hi[i]-o[i])/d[i]
		s := -1.0 // entering through the low face: normal points -axis
		if t0 > t1 {
			t0, t1, s = t1, t0, 1
		}
		if t0 > near {
			near, axis, sign = t0, i, s
		}
		far = math.Min(far, t1)
	}
	if near > far || far < 0 {
		return 0, normal, false
	}
	if near < 0 || axis < 0 {
		return 0, normal, true
	}
	normal[axis] = sign
	return near, normal, true
}

// IntersectRayOBB intersects a world space ray with an oriented box. dir need
// not be unit; the returned distance is in world units along it, even when the
// box is scaled. normal is the world space outward normal of the face hit
// (zero when the ray starts inside, distance 0).
func IntersectRayOBB(origin, dir [3]float64, b OBB) (dist float64, normal [3]float64, ok bool) {
	dl := math.Sqrt(dot(dir, dir))
	if dl == 0 {
		return 0, normal, false
	}
	// With orthogonal axes, local coordinate i of a point is
	// dot(axis_i, p-pos)/|axis_i|^2, and a ray parameter t means the same
	// thing in both spaces.
	var lo, ld vec
	rel := sub(origin, b.Pos)
	var s2 vec
	for i := 0; i < 3; i++ {
		s2[i] = dot(b.Axes[i], b.Axes[i])
		if s2[i] == 0 {
			return 0, normal, false // flattened to nothing
		}
		lo[i], ld[i] = dot(b.Axes[i], rel)/s2[i], dot(b.Axes[i], dir)/s2[i]
	}
	t, ln, hit := RayAABB(lo, ld, b.Local)
	if !hit {
		return 0, normal, false
	}
	// A local normal n maps to the world via the inverse transpose: the axis
	// direction (axis/|axis|) for an orthogonal transform.
	for i := 0; i < 3; i++ {
		if ln[i] != 0 {
			u := unit(b.Axes[i])
			normal = vec{u[0] * ln[i], u[1] * ln[i], u[2] * ln[i]}
		}
	}
	return t * dl, normal, true
}

// OBBSweep is the result of SweepAABBVsOBB.
type OBBSweep struct {
	// T is when the moving box first touches, as a fraction of its movement;
	// 1 when it does not.
	T float64
	// Normal points out of the surface touched, towards the mover.
	Normal [3]float64
	// Hit reports T < 1 (or an overlap at the start, T = 0).
	Hit bool
}

// SweepAABBVsOBB sweeps a world space box along vel against an oriented box and
// reports when it first touches. This is the separating axis theorem run over
// time: two convex boxes are apart exactly when their shadows on some direction
// do not overlap, so each of the fifteen candidate directions (three faces of
// each box and the nine cross products of one edge of each) gives a window of
// time in which the shadows overlap, and the boxes touch when the last window
// opens. Boxes that already overlap report T = 0 and the shortest way out.
func SweepAABBVsOBB(a AABB, b OBB, vel [3]float64) OBBSweep {
	centerA, halfA := a.center(), a.half()
	lc, lh := b.Local.center(), b.Local.half()
	var centerB, halfB vec
	var axesB [3]vec
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			centerB[j] += b.Axes[i][j] * lc[i]
		}
	}
	for j := range centerB {
		centerB[j] += b.Pos[j]
	}
	for i := 0; i < 3; i++ {
		scale := math.Sqrt(dot(b.Axes[i], b.Axes[i]))
		if scale == 0 {
			continue // a flattened box has no reach along this one
		}
		axesB[i] = unit(b.Axes[i])
		halfB[i] = scale * lh[i]
	}

	offset := sub(centerA, centerB)
	earliest, latest := math.Inf(-1), math.Inf(1)
	shallowest := math.Inf(1)
	var normalAtImpact, wayOut vec
	apart := false

	consider := func(d vec) {
		if apart || math.Sqrt(dot(d, d)) < 1e-6 {
			return // parallel axes cross to nothing and rule nothing out
		}
		d = unit(d)
		reach := math.Abs(d[0])*halfA[0] + math.Abs(d[1])*halfA[1] + math.Abs(d[2])*halfA[2]
		for i := 0; i < 3; i++ {
			reach += math.Abs(dot(d, axesB[i])) * halfB[i]
		}
		gap, closing := dot(d, offset), dot(d, vel)

		if depth := reach - math.Abs(gap); depth < shallowest {
			s := away(gap)
			shallowest, wayOut = depth, vec{d[0] * s, d[1] * s, d[2] * s}
		}
		if closing == 0 {
			apart = math.Abs(gap) > reach // never overlaps on this one, so never at all
			return
		}
		opens, shuts := (-reach-gap)/closing, (reach-gap)/closing
		if opens > shuts {
			opens, shuts = shuts, opens
		}
		if opens > earliest {
			earliest = opens
			s := away(gap + closing*opens)
			normalAtImpact = vec{d[0] * s, d[1] * s, d[2] * s}
		}
		latest = math.Min(latest, shuts)
		apart = earliest > latest
	}

	world := [3]vec{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	for i := 0; i < 3; i++ {
		consider(world[i])
	}
	for i := 0; i < 3; i++ {
		consider(axesB[i])
	}
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			consider(cross(world[i], axesB[j]))
		}
	}

	// Apart, touching only after the move, or only before it began (moving away).
	if apart || earliest > 1 || latest < 0 {
		return OBBSweep{T: 1}
	}
	if earliest <= 0 {
		return OBBSweep{Normal: wayOut, Hit: true}
	}
	return OBBSweep{T: earliest, Normal: normalAtImpact, Hit: true}
}

// away is the sign turning a direction into one pointing out of the oriented
// box, given which side of it the mover is on.
func away(side float64) float64 {
	if side < 0 {
		return -1
	}
	return 1
}
