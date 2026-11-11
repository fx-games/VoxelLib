package nametag

import (
	"image"
	"testing"
)

// TestQuad: a 21 px wide tag (20 px of text) spans font x -11..10 and y
// -1..9, at 0.025 blocks per pixel, y up.
func TestQuad(t *testing.T) {
	m := quad(image.NewNRGBA(image.Rect(0, 0, 21, height)), -(20 / 2))
	lo, hi := m.Bounds()
	const eps = 1e-6
	want := [4]float32{-11 * pixel, -9 * pixel, 10 * pixel, 1 * pixel}
	got := [4]float32{lo[0], lo[1], hi[0], hi[1]}
	for i := range got {
		if d := got[i] - want[i]; d > eps || d < -eps {
			t.Fatalf("bounds %v, want %v", got, want)
		}
	}
}
