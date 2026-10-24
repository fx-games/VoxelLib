package render

import (
	"math"
	"slices"

	"github.com/go-gl/mathgl/mgl32"
)

// casterMargin (blocks) grows a caster's shadow-view box for the filter
// taps around a receiver's shadow position (at most a few texels, under
// 0.4 blocks at the map's edge) and the per-vertex distortion.
const casterMargin = 2

// casterHull is the shadow-view xy outline (counter-clockwise) of the
// camera frustum up to the depth of the shadow box's diagonal: every pixel
// that samples the shadow maps lies in it, so terrain whose shadow-view box
// misses it writes only texels no pixel reads.
func casterHull(u *packUniforms) [][2]float32 {
	d := float32(math.Sqrt(8*packShadowDistance*packShadowDistance + packShadowFar*packShadowFar))
	toShadow := u.ShadowMV.Mul4(u.ModelViewInv)
	eye := toShadow.Col(3)
	pts := [][2]float32{{eye[0], eye[1]}}
	for _, c := range [4][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
		p := u.ProjInv.Mul4x1(mgl32.Vec4{c[0], c[1], 1, 1})
		v := p.Vec3().Mul(d / -p[2]) // on the corner ray at view depth d
		s := toShadow.Mul4x1(v.Vec4(1))
		pts = append(pts, [2]float32{s[0], s[1]})
	}
	return convexHull(pts)
}

// convexHull is Andrew's monotone chain, counter-clockwise.
func convexHull(p [][2]float32) [][2]float32 {
	slices.SortFunc(p, func(a, b [2]float32) int {
		if a[0] != b[0] {
			if a[0] < b[0] {
				return -1
			}
			return 1
		}
		if a[1] < b[1] {
			return -1
		}
		if a[1] > b[1] {
			return 1
		}
		return 0
	})
	cross := func(o, a, b [2]float32) float32 { return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0]) }
	h := make([][2]float32, 0, 2*len(p))
	for pass := 0; pass < 2; pass++ {
		start := len(h)
		for _, q := range p {
			for len(h) >= start+2 && cross(h[len(h)-2], h[len(h)-1], q) <= 0 {
				h = h[:len(h)-1]
			}
			h = append(h, q)
		}
		h = h[:len(h)-1]
		slices.Reverse(p)
	}
	return h
}

// overHull: the box [lo, hi] meets the convex counter-clockwise polygon h.
func overHull(h [][2]float32, lo, hi [2]float32) bool {
	if len(h) < 3 {
		return true
	}
	for i, a := range h {
		b := h[(i+1)%len(h)]
		n := [2]float32{b[1] - a[1], a[0] - b[0]} // outward
		c := lo
		if n[0] < 0 {
			c[0] = hi[0]
		}
		if n[1] < 0 {
			c[1] = hi[1]
		}
		if n[0]*(c[0]-a[0])+n[1]*(c[1]-a[1]) > 0 {
			return false
		}
	}
	for k := range 2 { // the box's own axes
		lo1, hi1 := h[0][k], h[0][k]
		for _, a := range h {
			lo1, hi1 = min(lo1, a[k]), max(hi1, a[k])
		}
		if hi1 < lo[k] || lo1 > hi[k] {
			return false
		}
	}
	return true
}

// casts: the section box at camera-relative (x, y, z) can shade a pixel.
func (pk *packGPU) casts(x, y, z float32) bool {
	m := pk.u.ShadowMV
	cx, cy, cz := x+8, y+8, z+8
	sx := m[0]*cx + m[4]*cy + m[8]*cz + m[12]
	sy := m[1]*cx + m[5]*cy + m[9]*cz + m[13]
	ex := 8*(abs32(m[0])+abs32(m[4])+abs32(m[8])) + casterMargin
	ey := 8*(abs32(m[1])+abs32(m[5])+abs32(m[9])) + casterMargin
	return overHull(pk.hull, [2]float32{sx - ex, sy - ey}, [2]float32{sx + ex, sy + ey})
}

func abs32(v float32) float32 { return float32(math.Abs(float64(v))) }
