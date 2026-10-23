package voxel

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// dynWorld is a 3x3 chunk world, y 0..63, filled by fill.
func dynWorld(fill func(x, y, z int) StateID) *World {
	reg := NewRegistry()
	w := NewWorld(reg, 0, 64)
	for cx := -1; cx <= 1; cx++ {
		for cz := -1; cz <= 1; cz++ {
			c := w.NewChunk(ChunkPos{cx, cz})
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					for y := 0; y < 64; y++ {
						if id := fill(cx*16+x, y, cz*16+z); id != Air {
							c.SetRaw(x, y, z, id)
						}
					}
				}
			}
			w.AddChunk(c)
		}
	}
	return w
}

// vanillaDyn iterates World.computeLightValue (block channel, 1.7.10, with
// the mod's emission: the dynamic level at a source block) to its fixpoint
// over the box, the light updateLightByType settles on.
func vanillaDyn(w *World, srcs []DynLight, lo, hi [3]int) map[BlockPos]uint8 {
	t := w.Reg.Table()
	emit := map[BlockPos]int{}
	for _, s := range srcs {
		p := BlockPos{s.X, s.Y, s.Z}
		emit[p] = max(emit[p], int(s.Level))
	}
	v := map[BlockPos]uint8{}
	for changed := true; changed; {
		changed = false
		for y := lo[1]; y <= hi[1]; y++ {
			for z := lo[2]; z <= hi[2]; z++ {
				for x := lo[0]; x <= hi[0]; x++ {
					p := BlockPos{x, y, z}
					l := emit[p]
					op := int(t[w.BlockUnlocked(x, y, z)].Opacity)
					if op >= 15 && l > 0 {
						op = 1
					}
					op = max(op, 1)
					if op >= 15 {
						l = 0
					} else if l < 14 {
						for f := Face(0); f < FaceCount; f++ {
							l = max(l, int(v[p.Offset(f)])-op)
						}
					}
					if uint8(l) != v[p] {
						v[p], changed = uint8(l), true
					}
				}
			}
		}
	}
	return v
}

func TestDynLightMatchesVanilla(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&BlockDef{Name: "test:water", Shape: ShapeCube, LightOpacity: 3})
	reg.Register(&BlockDef{Name: "test:leaves", Shape: ShapeCube, LightOpacity: 1})
	stone, water, leaves := reg.Default("stone"), reg.Default("test:water"), reg.Default("test:leaves")
	r := rand.New(rand.NewPCG(7, 8))
	ids := []StateID{Air, Air, Air, Air, stone, stone, water, leaves}
	w := dynWorld(func(x, y, z int) StateID { return ids[r.IntN(len(ids))] })
	w.Reg = reg
	srcs := []DynLight{{0, 32, 0, 15}, {3, 30, -2, 10}, {-5, 33, 4, 14}, {3, 30, -2, 12}}
	w.SetBlock(-5, 33, 4, stone) // a source inside an opaque block still shines
	w.SetDynLights(srcs)
	lo, hi := [3]int{-22, 12, -22}, [3]int{22, 52, 22}
	want := vanillaDyn(w, srcs, lo, hi)
	f := w.DynLights()
	n := 0
	for y := lo[1] + 1; y < hi[1]; y++ {
		for z := lo[2] + 1; z < hi[2]; z++ {
			for x := lo[0] + 1; x < hi[0]; x++ {
				var got uint8
				if a := f[SectionPos{x >> 4, y >> 4, z >> 4}]; a != nil {
					got = a[SectionIndex(x, y, z)]
				}
				if got != want[BlockPos{x, y, z}] {
					t.Fatalf("%d,%d,%d: got %d, vanilla %d", x, y, z, got, want[BlockPos{x, y, z}])
				}
				if got > 0 {
					n++
				}
			}
		}
	}
	if n < 100 {
		t.Fatalf("only %d lit blocks", n)
	}
}

func TestDynLightOpenAir(t *testing.T) {
	w := dynWorld(func(x, y, z int) StateID { return Air })
	w.SetDynLights([]DynLight{{0, 30, 0, 15}})
	for _, c := range []struct{ x, y, z, l int }{{0, 30, 0, 15}, {1, 30, 0, 14}, {3, 34, -2, 6}, {14, 30, 0, 1}, {15, 30, 0, 0}, {7, 30, 7, 1}} {
		if _, b := w.RenderLight(c.x, c.y, c.z); int(b) != c.l {
			t.Errorf("%d,%d,%d: %d, want 15 - manhattan = %d", c.x, c.y, c.z, b, c.l)
		}
	}
	if s, b := w.Light(1, 30, 0); s != 0 || b != 0 {
		t.Errorf("stored light changed: %d %d", s, b)
	}
}

func TestDynLightDirtySections(t *testing.T) {
	w := dynWorld(func(x, y, z int) StateID { return Air })
	sec := func(ps []SectionPos) []SectionPos {
		slices.SortFunc(ps, func(a, b SectionPos) int { return (a.X-b.X)*10000 + (a.Y-b.Y)*100 + a.Z - b.Z })
		return ps
	}
	// level 2 in the middle of a section: one section.
	if got := sec(w.SetDynLights([]DynLight{{8, 40, 8, 2}})); !slices.Equal(got, []SectionPos{{0, 2, 0}}) {
		t.Fatalf("got %v", got)
	}
	if got := w.SetDynLights([]DynLight{{8, 40, 8, 2}}); len(got) != 0 {
		t.Fatalf("unchanged sources dirtied %v", got)
	}
	// at x=15: the east neighbour's padded border holds it.
	got := sec(w.SetDynLights([]DynLight{{15, 40, 8, 1}}))
	if !slices.Equal(got, []SectionPos{{0, 2, 0}, {1, 2, 0}}) {
		t.Fatalf("got %v", got)
	}
	if got := w.SetDynLights(nil); len(got) != 2 || w.DynLights() == nil || len(w.DynLights()) != 0 {
		t.Fatalf("clearing: %v %v", got, w.DynLights())
	}
}

// One held torch in open air (the worst case: nothing stops the spread).
func BenchmarkDynLightTorch(b *testing.B) {
	w := dynWorld(func(x, y, z int) StateID { return Air })
	for i := 0; i < b.N; i++ {
		w.SetDynLights([]DynLight{{i & 1, 30, 0, 15}})
	}
}
