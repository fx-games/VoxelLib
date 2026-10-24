package voxel

import "testing"

func TestChunkTicking(t *testing.T) {
	c := newChunk(ChunkPos{}, 0, 64)
	c.SetRaw(1, 2, 3, 5)
	c.SetRaw(4, 20, 4, 6)
	f := NewTickFlags([]bool{5: true, 6: false})
	if m := c.Ticking(f); m != 1 || !c.Sections[0].TickingAt(SectionIndex(1, 2, 3)) || c.Sections[0].TickingAt(0) {
		t.Fatalf("mask %b", m)
	}
	c.SetRaw(4, 40, 4, 5) // section 2 starts ticking
	c.SetRaw(1, 2, 3, 6)  // section 0 stops
	if m := c.Ticking(f); m != 4 || c.Sections[0].TickingAt(SectionIndex(1, 2, 3)) {
		t.Fatalf("after SetRaw mask %b", m)
	}
	g := f.Grow([]bool{5: true, 6: false, 7: false}) // counts carry over
	c.SetRaw(4, 20, 4, 7)
	if m := c.Ticking(g); m != 4 {
		t.Fatalf("grown mask %b", m)
	}
	h := g.Grow([]bool{5: true, 6: false, 7: false, 8: true})
	c.SetRaw(0, 0, 0, 8) // was unmarked when set under g
	if m := c.Ticking(h); m != 5 {
		t.Fatalf("recounted mask %b", m)
	}
	c.SetSection(2, NewSection())
	if m := c.Ticking(h); m != 1 {
		t.Fatalf("after SetSection mask %b", m)
	}
}
