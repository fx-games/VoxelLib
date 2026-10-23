package voxel

import (
	"math/rand"
	"testing"
)

func TestPalettedContainer(t *testing.T) {
	p := NewPalettedContainer(Air)
	ref := make([]StateID, SectionVolume)
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 20000; n++ {
		i := r.Intn(SectionVolume)
		v := StateID(r.Intn(300)) // forces indirect -> direct transition
		if n < 10000 {
			v %= 9
		}
		p.Set(i, v)
		ref[i] = v
	}
	for i, v := range ref {
		if p.Get(i) != v {
			t.Fatalf("idx %d: got %d want %d", i, p.Get(i), v)
		}
	}
	var buf [SectionVolume]StateID
	p.Unpack(&buf)
	q := &PalettedContainer{}
	q.Pack(&buf)
	for i, v := range ref {
		if q.Get(i) != v {
			t.Fatalf("pack idx %d", i)
		}
	}
}

func flatWorld(t *testing.T) (*World, StateID) { return flatWorldSky(t, false) }

func flatWorldSky(t *testing.T, noSky bool) (*World, StateID) {
	reg := NewRegistry()
	w := NewWorld(reg, -64, 384)
	w.NoSky.Store(noSky)
	stone := reg.Default("stone")
	for cx := -2; cx <= 2; cx++ {
		for cz := -2; cz <= 2; cz++ {
			c := w.NewChunk(ChunkPos{cx, cz})
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					for y := -64; y < 64; y++ {
						c.SetRaw(x, y, z, stone)
					}
				}
			}
			c.RecomputeHeights(reg.Table())
			w.AddChunk(c)
		}
	}
	for cx := -1; cx <= 1; cx++ {
		for cz := -1; cz <= 1; cz++ {
			if !w.TryLightChunk(ChunkPos{cx, cz}) {
				t.Fatal("light failed")
			}
		}
	}
	return w, stone
}

func TestLight(t *testing.T) {
	w, stone := flatWorld(t)
	if s, _ := w.Light(0, 64, 0); s != 15 {
		t.Fatalf("surface sky %d", s)
	}
	if s, _ := w.Light(0, 60, 0); s != 0 {
		t.Fatalf("underground sky %d", s)
	}
	// dig a 1x1 shaft 5 deep, then a side tunnel
	for y := 63; y >= 59; y-- {
		w.SetBlock(3, y, 3, Air)
	}
	if s, _ := w.Light(3, 59, 3); s != 15 {
		t.Fatalf("shaft bottom sky %d", s)
	}
	w.SetBlock(4, 59, 3, Air)
	if s, _ := w.Light(4, 59, 3); s != 14 {
		t.Fatalf("tunnel sky %d", s)
	}
	// cap the shaft: everything goes dark
	w.SetBlock(3, 63, 3, stone)
	if s, _ := w.Light(4, 59, 3); s != 0 {
		t.Fatalf("capped tunnel sky %d", s)
	}
	// torch light
	glow := w.Reg.Default("glowstone")
	w.SetBlock(3, 61, 3, glow)
	if _, b := w.Light(3, 60, 3); b != 14 {
		t.Fatalf("block light %d", b)
	}
	if _, b := w.Light(4, 59, 3); b != 12 {
		t.Fatalf("block light tunnel %d", b)
	}
	w.SetBlock(3, 61, 3, Air)
	if _, b := w.Light(4, 59, 3); b != 0 {
		t.Fatalf("block light after removal %d", b)
	}
}

func TestNoSky(t *testing.T) {
	w, _ := flatWorldSky(t, true)
	for _, y := range []int{64, 200, 400} {
		if s, _ := w.Light(0, y, 0); s != 0 {
			t.Fatalf("sky at %d: %d", y, s)
		}
	}
	w.SetBlock(3, 63, 3, Air)
	w.SetBlock(3, 319, 3, w.Reg.Default("glowstone"))
	w.SetBlock(3, 319, 3, Air)
	if s, b := w.Light(3, 63, 3); s != 0 || b != 0 {
		t.Fatalf("dug: sky %d block %d", s, b)
	}
}

func TestLightBlockIsInvisibleAndEmits(t *testing.T) {
	r := NewRegistry()
	i := r.Info(r.Parse("light[level=15]"))
	if i.Shape != ShapeNone || i.Collides || i.Opaque || i.Emission != 15 {
		t.Fatalf("light: shape %v collides %v opaque %v emission %d", i.Shape, i.Collides, i.Opaque, i.Emission)
	}
}
