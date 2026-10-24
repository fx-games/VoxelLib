package ui

import "testing"

func TestSoftOrReverse(t *testing.T) {
	s := NewSoftCanvas(2, 1, 1)
	copy(s.Img.Pix, []byte{10, 200, 30, 255, 0, 0, 0, 0})
	s.SetOrReverse(true)
	s.Quad(Quad{X0: 0, Y0: 0, X1: 2, Y1: 1, Top: 0xFF0000FF, Bottom: 0xFF0000FF})
	s.SetOrReverse(false)
	if got := s.Img.Pix; got[0] != 245 || got[1] != 55 || got[2] != 255 || got[3] != 255 || got[4] != 255 || got[6] != 255 {
		t.Fatal(got)
	}
}
