package render

import (
	"math"
	"testing"
)

// TestTexArrayGrid checks the layer allocation past the GPU's array layer
// limit: Hobbiton needs 3236 physical layers on Metal's 2048.
func TestTexArrayGrid(t *testing.T) {
	for _, c := range []struct{ layers, max, grid int }{
		{64, 2048, 0}, {2048, 2048, 0}, {2049, 2048, 1}, {3236, 2048, 1}, {8192, 2048, 1}, {8193, 2048, 2}, {maxLayerCode + maxLayerCode/4 + 16, 2048, 2},
	} {
		if g := gridShift(c.layers, c.max); g != c.grid {
			t.Errorf("gridShift(%d, %d) = %d, want %d", c.layers, c.max, g, c.grid)
		}
	}
	// every physical layer gets its own tile within the array layers
	for grid := 0; grid <= 2; grid++ {
		ta := textureArray{capLayers: 3236, grid: grid}
		seen := map[[3]int]bool{}
		for i := 0; i < ta.capLayers; i++ {
			l, x, y := tileOf(i, grid)
			if l >= ta.arrayLayers() || x >= 1<<grid || y >= 1<<grid || seen[[3]int{l, x, y}] {
				t.Fatalf("grid %d: layer %d -> tile %d,%d,%d (array layers %d)", grid, i, l, x, y, ta.arrayLayers())
			}
			seen[[3]int{l, x, y}] = true
		}
	}
}

// TestBlockTexDecode mirrors wgslBlockTex on the CPU: the code decodes to
// tileOf's tile, and nearest sampling at the tile's fract(uv) hits the
// texel a whole repeat-wrapped layer would, at every mip level.
func TestBlockTexDecode(t *testing.T) {
	const size = 16
	for grid := 0; grid <= 2; grid++ {
		for _, i := range []int{0, 1, 5, 2047, 2048, 3235} {
			code := uint32(layerCode(i, grid))
			g := code >> 14
			idx := code & 16383
			m := uint32(1)<<g - 1
			l, x, y := tileOf(i, grid)
			if int(g) != grid || int(idx>>(2*g)) != l || int(idx&m) != x || int((idx>>g)&m) != y {
				t.Fatalf("grid %d layer %d: code %#x decodes differently from tile %d,%d,%d", grid, i, code, l, x, y)
			}
			if grid == 0 && code != uint32(i) {
				t.Fatalf("grid 0 code %d != layer %d", code, i)
			}
			n := float32(uint32(1) << g)
			for _, u := range []float32{0, 0.03, 0.5, 0.999, 1, 1.0625, 3.97, 7.5, 31.99, -0.25, -1e-8, 2 - 1e-7} {
				fr := min(u-float32(math.Floor(float64(u))), 1-1.0/4096) // wgslBlockTex's fract
				p := u
				if g > 0 {
					p = (fr + float32(x)) / n
				}
				for lv := 0; lv <= 4; lv++ {
					s := size >> lv
					got := int(math.Floor(float64(p * n * float32(s))))
					want := x*s + int(math.Floor(float64(u)*float64(s)))&(s-1)
					if grid == 0 {
						got &= s - 1 // repeat
					}
					if got != want {
						t.Errorf("grid %d tile x %d u %v level %d: texel %d, want %d", grid, x, u, lv, got, want)
					}
				}
			}
		}
	}
}
