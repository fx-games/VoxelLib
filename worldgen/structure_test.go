package worldgen

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

// tower starts in every chunk whose x and z are multiples of 3: a 40×40
// floor (crossing chunk borders) plus a seed-derived pillar, overlapping
// the next tower's floor.
type tower struct{ stone, wood voxel.StateID }

type towerAt struct {
	t    tower
	x, z int
	h    int
}

func (t tower) Reach() int { return 3 }
func (t tower) Plan(seed int64, p voxel.ChunkPos) []Instance {
	if p.X%3 != 0 || p.Z%3 != 0 {
		return nil
	}
	return []Instance{towerAt{t, p.X*16 + 5, p.Z*16 + 5, int(seed+int64(p.X*7+p.Z)) & 15}}
}
func (a towerAt) Build(r *Region) {
	for x := a.x; x < a.x+40; x++ {
		for z := a.z; z < a.z+40; z++ {
			id := a.t.stone
			if (x+z)%2 == 0 {
				id = a.t.wood
			}
			r.Set(x, 1, z, id)
		}
	}
	for y := 2; y < 2+a.h; y++ {
		r.Set(a.x, y, a.z, a.t.wood)
	}
}

func TestStructuresOrderIndependent(t *testing.T) {
	reg := voxel.NewRegistry()
	tw := tower{reg.Default("stone"), reg.Default("oak_planks")}
	gen := func(order []voxel.ChunkPos) map[voxel.ChunkPos]*voxel.Chunk {
		w := voxel.NewWorld(reg, 0, 64)
		out := map[voxel.ChunkPos]*voxel.Chunk{}
		for _, p := range order {
			c := w.NewChunk(p)
			PlaceStructures(99, w, c, tw)
			out[p] = c
		}
		return out
	}
	var fwd, rev []voxel.ChunkPos
	for z := -4; z < 4; z++ {
		for x := -4; x < 4; x++ {
			fwd = append(fwd, voxel.ChunkPos{X: x, Z: z})
		}
	}
	for i := len(fwd) - 1; i >= 0; i-- {
		rev = append(rev, fwd[i])
	}
	a, b := gen(fwd), gen(rev)
	n := 0
	for p, ca := range a {
		for y := 0; y < 64; y++ {
			for z := 0; z < 16; z++ {
				for x := 0; x < 16; x++ {
					if ca.Get(x, y, z) != b[p].Get(x, y, z) {
						t.Fatalf("%v %d,%d,%d differs", p, x, y, z)
					}
					if ca.Get(x, y, z) != voxel.Air {
						n++
					}
				}
			}
		}
	}
	// chunk 1,1 is only reached by the tower started in 0,0
	if a[voxel.ChunkPos{X: 1, Z: 1}].Get(0, 1, 0) == voxel.Air || n == 0 {
		t.Fatal("tower floor did not cross into the neighbour chunk")
	}
}

func TestRegister(t *testing.T) {
	Register("test:none", func(seed int64, b Blocks) Generator { return nil })
	if Lookup("test:none") == nil || Lookup("test:missing") != nil {
		t.Fatal("lookup")
	}
}
