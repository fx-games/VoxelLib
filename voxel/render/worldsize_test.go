package render

import "testing"

// TestWorldSize: a smaller world size gets its own target; unset, equal or
// larger sizes render the world at the framebuffer's.
func TestWorldSize(t *testing.T) {
	defer SetWorldSize(0, 0)
	for _, c := range []struct {
		sw, sh, w, h, ww, wh int
		own                  bool
	}{
		{0, 0, 2560, 1440, 2560, 1440, false},
		{1280, 720, 2560, 1440, 1280, 720, true},
		{2560, 1440, 2560, 1440, 2560, 1440, false},
		{3000, 720, 2560, 1440, 2560, 1440, false},
	} {
		SetWorldSize(c.sw, c.sh)
		if ww, wh, own := worldSize(c.w, c.h); ww != c.ww || wh != c.wh || own != c.own {
			t.Errorf("%+v: %d %d %v", c, ww, wh, own)
		}
	}
}
