package minimap

import (
	"math"
	"testing"
)

// The expected values come from the reference formulas run verbatim on a
// JVM (MapPixel slope light and colours, MinimapWriter.getBlockBrightness,
// World.getSunBrightnessFactor with the table cosine).

func TestSlopeLight(t *testing.T) {
	shadow := [3]float32{0.518, 0.678, 1}
	white := [3]float32{1, 1, 1}
	for _, c := range []struct {
		v, d                int
		plain, glow, sr, sg uint32
	}{
		{0, 0, 0x3f95f086, 0x3f941cfa, 0x3f8999b0, 0x3f8db244},
		{1, 1, 0x3faeeef0, 0x3f9c71c7, 0x3fa2981a, 0x3fa6b0ad},
		{1, 0, 0x3f9d7b64, 0x3f96a099, 0x3f91248e, 0x3f953d21},
		{2, 1, 0x3f9d7b64, 0x3f96a099, 0x3f91248e, 0x3f953d21},
		{-1, 0, 0x3f333333, 0x3f800000, 0x3f1a8588, 0x3f22b6ae},
		{-1, -1, 0x3f333333, 0x3f800000, 0x3f1a8588, 0x3f22b6ae},
		{0, 3, 0x3f607465, 0x3f878ade, 0x3f47c6ba, 0x3f4ff7e0},
		{3, -2, 0x3f7e9fdb, 0x3f8c921c, 0x3f65f230, 0x3f6e2356},
		{5, 5, 0x3f9d7b64, 0x3f96a099, 0x3f91248e, 0x3f953d21},
	} {
		p, g, s := slopeLight(c.v, c.d, false, white), slopeLight(c.v, c.d, true, white), slopeLight(c.v, c.d, false, shadow)
		got := []uint32{math.Float32bits(p[0]), math.Float32bits(g[0]), math.Float32bits(s[0]), math.Float32bits(s[1])}
		want := []uint32{c.plain, c.glow, c.sr, c.sg}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("slope %d,%d value %d: %08x, want %08x", c.v, c.d, i, got[i], want[i])
			}
		}
	}
}

type fakeSurface map[[2]int]*surfPixel

func (f fakeSurface) pixel(wx, wz int) *surfPixel { return f[[2]int{wx, wz}] }

func TestSurfaceColour(t *testing.T) {
	src := &Source{Blocks: []Block{
		{Invisible: true},
		{Colour: 0x7fb238},
		{Colour: 0xdbd3a0},
		{Colour: 0x3f76e4, Overlay: true, Transparency: 0.66, Opacity: 3},
	}}
	white := [3]float32{1, 1, 1}
	s := fakeSurface{
		{5, 4}: {height: 63}, {4, 4}: {height: 64},
		{5, 5}: {state: 1, height: 64},
	}
	if rgb, a := src.surfaceColour(s, 5, 5, s[[2]int{5, 5}], white); rgb != 156<<16|218<<8|68 || a != 0 {
		t.Errorf("land: %06x %d", rgb, a)
	}
	s = fakeSurface{
		{5, 4}: {height: 60}, {4, 4}: {height: 60},
		{5, 5}: {state: 2, height: 60, layers: []layer{{state: 3, opacity: 3}}},
	}
	if rgb, _ := src.surfaceColour(s, 5, 5, s[[2]int{5, 5}], white); rgb != 113<<16|147<<8|203 {
		t.Errorf("water: %06x", rgb)
	}
	s[[2]int{5, 5}].light = 7
	s[[2]int{5, 5}].layers[0].light = 7
	if _, a := src.surfaceColour(s, 5, 5, s[[2]int{5, 5}], white); a != 170 {
		t.Errorf("alpha light 7: %d", a)
	}
}

func TestSunAndCaveBrightness(t *testing.T) {
	for _, c := range []struct {
		a         float32
		dry, rain uint32
	}{
		{0, 0x3f800000, 0x3f472000}, {0.25, 0x3f000000, 0x3ec72000}, {0.3, 0, 0}, {0.5, 0, 0}, {0.77, 0x3f4022b4, 0x3f1572fe},
	} {
		if g := math.Float32bits(SunBrightness(c.a, 0, 0)); g != c.dry {
			t.Errorf("sun %v: %08x, want %08x", c.a, g, c.dry)
		}
		if g := math.Float32bits(SunBrightness(c.a, 0.5, 0.25)); g != c.rain {
			t.Errorf("sun %v rain: %08x, want %08x", c.a, g, c.rain)
		}
	}
	for l, w := range []uint32{0x3f600000, 0x3f400000, 0x3f200000, 0x3f000000, 0x3f000000} {
		if g := math.Float32bits(caveBrightness(5, 9, 12, l, 3)); g != w {
			t.Errorf("cave level %d: %08x, want %08x", l, g, w)
		}
	}
	if g := math.Float32bits(caveBrightness(1, 9, 12, 2, 3)); g != 0x3f600000 {
		t.Errorf("cave one level: %08x", g)
	}
	if g := shade(0x808080, 0, daylight(0)); g != 0x303030 {
		t.Errorf("shade night: %06x", g)
	}
}

// edge is translatePosition: a far point lands on the square's edge,
// rounded, minus one.
func TestEdge(t *testing.T) {
	if x, y := edge(100, 1, 0, 0, -10000, 1); x != -1 || y != -101 {
		t.Errorf("north, locked: %v,%v", x, y)
	}
	// Facing south (ps -1, pc 0): north is down.
	if x, y := edge(100, -1, 0, 0, -10000, 1); x != -1 || y != 99 {
		t.Errorf("north, facing south: %v,%v", x, y)
	}
	// A near point keeps its offset: 3 blocks east at zoom 2.
	if x, y := edge(100, 1, 0, 3, 0, 2); x != 5 || y != -1 {
		t.Errorf("near: %v,%v", x, y)
	}
}
