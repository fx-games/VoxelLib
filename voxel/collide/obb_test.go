package collide

import (
	"math"
	"testing"
)

func cube(c vec, h float64) AABB {
	return AABB{c[0] - h, c[1] - h, c[2] - h, c[0] + h, c[1] + h, c[2] + h}
}

func rotY(a, s float64) [3][3]float64 {
	c, sn := math.Cos(a), math.Sin(a)
	return [3][3]float64{{c * s, 0, -sn * s}, {0, s, 0}, {sn * s, 0, c * s}}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func vnear(a, b vec) bool { v := sub(a, b); return math.Sqrt(dot(v, v)) < 1e-6 }

func ident() [3][3]float64 { return rotY(0, 1) }

func assertSweep(t *testing.T, got OBBSweep, wantT float64, wantN vec) {
	t.Helper()
	if !got.Hit {
		t.Fatalf("expected a collision, got %+v", got)
	}
	if math.Abs(got.T-wantT) > 1e-6 || !vnear(got.Normal, wantN) {
		t.Errorf("got T=%v normal=%v, want T=%v normal=%v", got.T, got.Normal, wantT, wantN)
	}
}

func TestSweepUnrotated(t *testing.T) {
	moving := cube(vec{-3, 0, 0}, 0.5)
	b := OBB{Local: cube(vec{}, 0.5), Axes: ident()}
	assertSweep(t, SweepAABBVsOBB(moving, b, vec{6, 0, 0}), 2.0/6, vec{-1, 0, 0})
}

// A box wide in x and thin in z blocks along z once turned 90 degrees.
func TestSweepRotated(t *testing.T) {
	moving := cube(vec{-3, 0, 0}, 0.05)
	wide := AABB{-1, -0.5, -0.25, 1, 0.5, 0.25}
	v := vec{6, 0, 0}
	assertSweep(t, SweepAABBVsOBB(moving, OBB{Local: wide, Axes: ident()}, v), (3-1.05)/6, vec{-1, 0, 0})
	assertSweep(t, SweepAABBVsOBB(moving, OBB{Local: wide, Axes: rotY(math.Pi/2, 1)}, v), (3-0.3)/6, vec{-1, 0, 0})
}

func TestSweepScaled(t *testing.T) {
	moving := cube(vec{-3, 0, 0}, 0.05)
	b := OBB{Local: cube(vec{}, 0.5), Axes: rotY(0, 2)}
	assertSweep(t, SweepAABBVsOBB(moving, b, vec{6, 0, 0}), (3-1.05)/6, vec{-1, 0, 0})
}

func TestSweepElongatedMover(t *testing.T) {
	moving := AABB{-3.5, -0.5, -0.1, -2.5, 0.5, 0.1}
	angle := 30 * math.Pi / 180
	b := OBB{Local: cube(vec{}, 0.5), Axes: rotY(angle, 1)}
	cos, sin := math.Cos(angle), math.Sin(angle)
	reach := cos*0.5 + sin*0.1
	dist := (3*cos - (0.5 + reach)) / cos
	assertSweep(t, SweepAABBVsOBB(moving, b, vec{6, 0, 0}), dist/6, vec{-cos, 0, sin})
}

func TestSweepMisses(t *testing.T) {
	moving := cube(vec{-3, 0, 0}, 0.5)
	b := OBB{Local: cube(vec{}, 0.5), Axes: ident()}
	for name, v := range map[string]vec{
		"moving away": {-6, 0, 0}, "too slow": {1, 0, 0}, "passing by": {0, 6, 0}, "still": {},
	} {
		if got := SweepAABBVsOBB(moving, b, v); got.Hit || got.T != 1 {
			t.Errorf("%s: expected no collision, got %+v", name, got)
		}
	}
}

func TestSweepAlreadyOverlapping(t *testing.T) {
	b := OBB{Local: cube(vec{}, 0.5), Axes: ident()}
	got := SweepAABBVsOBB(cube(vec{0.1, 0, 0}, 0.5), b, vec{6, 0, 0})
	if !got.Hit || got.T != 0 {
		t.Errorf("expected an immediate collision, got %+v", got)
	}
}

// Passing the corner of a rotated box clears it; an inflated box would catch.
func TestSweepClearsCorner(t *testing.T) {
	b := OBB{Local: cube(vec{}, 0.5), Axes: rotY(math.Pi/4, 1)}
	if got := SweepAABBVsOBB(cube(vec{-3, 0, 1.4}, 0.5), b, vec{6, 0, 0}); got.Hit {
		t.Errorf("caught on the corner it should have passed, got %+v", got)
	}
	if got := SweepAABBVsOBB(cube(vec{-3, 0, 1.1}, 0.5), b, vec{6, 0, 0}); !got.Hit {
		t.Errorf("missed the corner it should have clipped")
	}
}

// A turned, non-uniformly scaled, offset box hit head-on through each of its
// six faces, from 3 units out along that face's normal.
func TestRayOBBEachFace(t *testing.T) {
	// Rotate about Y then X, scales 2, 0.5, 3.
	ry, rx := rotY(0.6, 1), [3][3]float64{{1, 0, 0}, {0, math.Cos(0.4), math.Sin(0.4)}, {0, -math.Sin(0.4), math.Cos(0.4)}}
	apply := func(m [3][3]float64, v vec) vec { // columns of m are images of the axes
		return vec{m[0][0]*v[0] + m[1][0]*v[1] + m[2][0]*v[2], m[0][1]*v[0] + m[1][1]*v[1] + m[2][1]*v[2], m[0][2]*v[0] + m[1][2]*v[1] + m[2][2]*v[2]}
	}
	scale := vec{2, 0.5, 3}
	var axes [3][3]float64
	for i := 0; i < 3; i++ {
		e := vec{}
		e[i] = scale[i]
		axes[i] = apply(rx, apply(ry, e))
	}
	b := OBB{Local: AABB{-0.5, -1, -0.25, 0.5, 1, 0.25}, Axes: axes, Pos: vec{4, -2, 7}}
	lc, lh := b.Local.center(), b.Local.half()
	for i := 0; i < 3; i++ {
		for _, s := range []float64{-1, 1} {
			p := lc
			p[i] += s * lh[i]
			// world point on the face centre and the face's world normal
			var wp vec
			for k := 0; k < 3; k++ {
				for j := 0; j < 3; j++ {
					wp[j] += axes[k][j] * p[k]
				}
				wp[k] += b.Pos[k]
			}
			n := unit(axes[i])
			n = vec{n[0] * s, n[1] * s, n[2] * s}
			origin := vec{wp[0] + 3*n[0], wp[1] + 3*n[1], wp[2] + 3*n[2]}
			dist, normal, ok := IntersectRayOBB(origin, vec{-n[0] * 5, -n[1] * 5, -n[2] * 5}, b)
			if !ok || !near(dist, 3) || !vnear(normal, n) {
				t.Errorf("axis %d sign %v: ok=%v dist=%v normal=%v want 3, %v", i, s, ok, dist, normal, n)
			}
			// the same ray pointing away misses
			if _, _, ok := IntersectRayOBB(origin, vec{n[0], n[1], n[2]}, b); ok {
				t.Errorf("axis %d sign %v: ray pointing away hit", i, s)
			}
		}
	}
}

func TestRayAABBParallelAndInside(t *testing.T) {
	b := cube(vec{}, 1)
	if _, _, ok := RayAABB(vec{-3, 2, 0}, vec{1, 0, 0}, b); ok {
		t.Error("parallel ray outside slab hit")
	}
	if tt, n, ok := RayAABB(vec{-3, 1, 0}, vec{1, 0, 0}, b); !ok || tt != 2 || n != (vec{-1, 0, 0}) {
		t.Errorf("grazing parallel: %v %v %v", tt, n, ok)
	}
	if tt, _, ok := RayAABB(vec{}, vec{1, 0, 0}, b); !ok || tt != 0 {
		t.Errorf("inside: %v %v", tt, ok)
	}
}
