package minimap

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

// slopeWorld is terrain whose height changes at every column, so every
// edge pixel's slope reads its neighbour chunk.
func slopeWorld(cs ...voxel.ChunkPos) (*voxel.World, *Map) {
	w := voxel.NewWorld(nil, 0, 256)
	for _, p := range cs {
		c := w.NewChunk(p)
		for z := 0; z < 16; z++ {
			for x := 0; x < 16; x++ {
				wx, wz := p.X<<4|x, p.Z<<4|z
				h := 60 + (wx*7+wz*13)&7
				for y := 0; y <= h; y++ {
					c.SetRaw(x, y, z, 1)
				}
				c.Height[z<<4|x] = int32(h + 1)
			}
		}
		w.AddChunk(c)
	}
	m := &Map{Cfg: Config{Zooms: []float64{1}, Slopes: 2, Shadow: [3]float32{1, 1, 1}},
		Src: &Source{Blocks: []Block{{Invisible: true}, {Colour: 0x7fb238}}, Biome: func(x, z int) int { return 0 }}}
	m.resize(55, 2)
	return w, m
}

func writeAll(w *voxel.World, m *Map, order []voxel.ChunkPos) {
	for _, p := range order {
		m.write(w, w.ChunkUnlocked(p), 64, 1)
	}
}

// edgeLit is the colours of chunk p's north row and west column.
func edgeLit(m *Map, p voxel.ChunkPos) []uint32 {
	var out []uint32
	for i := 0; i < 16; i++ {
		out = append(out, m.lit[m.idx(p.X<<4|i, p.Z<<4)], m.lit[m.idx(p.X<<4, p.Z<<4|i)])
	}
	return out
}

// TestEdgeSlopesLoadOrder: MapWriter's cached slopes with the edge
// invalidation give the same edge pixels whichever chunk comes first;
// across a region border the reference does not invalidate, so the edge
// keeps the next pixel's slopes until the chunk is written again.
func TestEdgeSlopesLoadOrder(t *testing.T) {
	for _, c := range []struct {
		p      voxel.ChunkPos
		region bool
	}{{voxel.ChunkPos{X: 3, Z: 5}, false}, {voxel.ChunkPos{X: 32, Z: 5}, true}, {voxel.ChunkPos{X: -5, Z: 3}, false}, {voxel.ChunkPos{X: -1, Z: 0}, true}} {
		p := c.p
		nb := []voxel.ChunkPos{{X: p.X, Z: p.Z - 1}, {X: p.X - 1, Z: p.Z}, {X: p.X - 1, Z: p.Z - 1}}
		w, first := slopeWorld(append(nb, p)...)
		writeAll(w, first, append(nb, p))
		want := edgeLit(first, p)
		// The slopes north are the true height steps.
		px := &first.surf[first.idx(p.X<<4|5, p.Z<<4)]
		if h := heightAt(first, p.X<<4|5, p.Z<<4-1); int(px.vs) != int(px.height)-h {
			t.Errorf("%v: vertical slope %d, want %d", p, px.vs, int(px.height)-h)
		}

		_, last := slopeWorld()
		writeAll(w, last, append([]voxel.ChunkPos{p}, nb...))
		got := edgeLit(last, p)
		same := true
		for i := range want {
			same = same && got[i] == want[i]
		}
		if same == c.region {
			t.Errorf("%v: edge equal %v after the neighbours came last (region border %v)", p, same, c.region)
		}
		if c.region {
			writeAll(w, last, []voxel.ChunkPos{p})
			got = edgeLit(last, p)
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("%v: edge pixel %d %06x after the rewrite, want %06x", p, i, got[i], want[i])
				}
			}
		}
	}
}

// TestOrigin: the map square's corner is (int)(x * (scale / minimapScale))
// + 9 units, unit (minimapScale) screen px each, so at a GUI scale the unit
// does not divide it lands between GUI pixels; the screen-px numbers are
// the reference's for these positions.
func TestOrigin(t *testing.T) {
	for _, c := range []struct {
		x, gui int
		unit   float64
		px     int // screen px of the corner
	}{
		{0, 2, 3, 27}, // 13.5 GUI px (a rounded scissor gave 14 = 28 px)
		{5, 2, 3, 36}, // 5 * 0.6666667f = 3.3333335 -> 3
		{3, 3, 2, 26}, // 3 * 1.5f = 4.5 -> 4: 8.67 GUI px
		{7, 3, 4, 56}, // 7 * 0.75f = 5.25 -> 5
		{0, 2, 2, 18},
	} {
		cfg := Config{X: c.x, Y: c.x}
		ox, oy := cfg.origin(c.gui, c.unit)
		if ox != oy || float64(ox)*c.unit != float64(c.px) {
			t.Errorf("x %d gui %d unit %v: origin %d,%d units, want %d screen px", c.x, c.gui, c.unit, ox, oy, c.px)
		}
		// The quad edge in GUI px times the scale is that screen px exactly.
		if g := float32(float64(ox) * (c.unit / float64(c.gui))); g*float32(c.gui) != float32(c.px) {
			t.Errorf("x %d gui %d unit %v: GUI edge %v -> %v screen px", c.x, c.gui, c.unit, g, g*float32(c.gui))
		}
	}
}
