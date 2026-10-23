package render

import "math"

// QuadRot is Quad turned by deg degrees around px, py (glRotatef about z
// in GUI space: clockwise on the y-down screen).
func (b *GUIBatch) QuadRot(t *GUITexture, x0, y0, x1, y1, u0, v0, u1, v1, skew float32, top, bottom uint32, deg, px, py float32) {
	if t == nil {
		t = whiteTexture()
	}
	b.state(t, b.cur.scissor, b.cur.invert)
	ct, cb := argbToRGBA(top), argbToRGBA(bottom)
	sn, cs := math.Sincos(float64(deg) * math.Pi / 180)
	s, c := float32(sn), float32(cs)
	r := func(x, y float32) (float32, float32) {
		dx, dy := x-px, y-py
		return px + dx*c - dy*s, py + dx*s + dy*c
	}
	ax, ay := r(x0+skew, y0) // top left
	bx, by := r(x0-skew, y1) // bottom left
	cx, cy := r(x1+skew, y0) // top right
	dx, dy := r(x1-skew, y1) // bottom right
	b.verts = append(b.verts,
		ax, ay, u0, v0, ct, bx, by, u0, v1, cb, cx, cy, u1, v0, ct,
		cx, cy, u1, v0, ct, bx, by, u0, v1, cb, dx, dy, u1, v1, cb)
}
