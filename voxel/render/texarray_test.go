package render

import (
	"math"
	"testing"
)

// TestTexArrayTiles: tiles of mixed sizes never overlap, each size fills
// its own layers, and the code decodes (as wgslBlockTex does) to the slot.
func TestTexArrayTiles(t *testing.T) {
	ta := textureArray{layerSize: 128, open: map[int]openLayer{}}
	type px struct{ layer, x, y int }
	owner := map[px]int{}
	for n, s := range []int{16, 64, 16, 128, 32, 16, 64, 64, 64, 64, 16} {
		for k := 0; k < 70; k++ {
			sl := ta.tile(s, nil)
			c := sl.code()
			g, i := int(c>>25&7), int(c>>11&16383)
			if int(c&2047) != sl.layer || g != sl.grid || i != sl.tile || ta.layerSize>>g != s {
				t.Fatalf("size %d: code %#x decodes to layer %d grid %d tile %d, slot %+v", s, c, c&2047, g, i, sl)
			}
			x, y := sl.tileXY()
			for dy := 0; dy < s; dy += 16 {
				for dx := 0; dx < s; dx += 16 {
					p := px{sl.layer, x*s + dx, y*s + dy}
					if p.x >= ta.layerSize || p.y >= ta.layerSize {
						t.Fatalf("size %d tile %d outside the layer", s, sl.tile)
					}
					if o, ok := owner[p]; ok {
						t.Fatalf("size %d tile %d overlaps tile of run %d", s, sl.tile, o)
					}
					owner[p] = n
				}
			}
		}
	}
}

// TestBlockTexDecode mirrors wgslBlockTex on the CPU: nearest sampling at
// the tile's fract(uv) hits the texel a whole repeat-wrapped texture would,
// at every mip level.
func TestBlockTexDecode(t *testing.T) {
	const size = 16
	for grid := 0; grid <= 3; grid++ {
		n := float32(uint32(1) << grid)
		for _, x := range []int{0, 1, 1<<grid - 1} {
			for _, u := range []float32{0, 0.03, 0.5, 0.999, 1, 1.0625, 3.97, 7.5, 31.99, -0.25, -1e-8, 2 - 1e-7} {
				fr := min(u-float32(math.Floor(float64(u))), 1-1.0/4096) // wgslBlockTex's fract
				p := (fr + float32(x)) / n
				for lv := 0; lv <= mipLevels; lv++ {
					s := size >> lv
					got := int(math.Floor(float64(p * n * float32(s))))
					want := x*s + int(math.Floor(float64(u)*float64(s)))&(s-1)
					if got != want {
						t.Errorf("grid %d tile x %d u %v level %d: texel %d, want %d", grid, x, u, lv, got, want)
					}
				}
			}
		}
	}
}
