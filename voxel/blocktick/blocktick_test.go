package blocktick

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

func world(t *testing.T) (*voxel.World, voxel.StateID, voxel.StateID) {
	reg := voxel.NewRegistry()
	reg.Register(&voxel.BlockDef{Name: "test:a"})
	reg.Register(&voxel.BlockDef{Name: "test:b"})
	w := voxel.NewWorld(reg, 0, 32)
	w.AddChunk(w.NewChunk(voxel.ChunkPos{}))
	return w, reg.Default("test:a"), reg.Default("test:b")
}

// Order is time, then priority, then scheduling order; a pending tick of
// the same block at a position is not added twice; at most MaxPerTick run.
func TestScheduledOrder(t *testing.T) {
	w, a, b := world(t)
	tk := New(w, func(id voxel.StateID) int32 { return int32(id) })
	for x := 0; x < 4; x++ {
		w.SetBlock(x, 1, 0, a)
	}
	w.SetBlock(5, 1, 0, b)
	var got []int
	tk.OnScheduled = func(x, y, z int, id voxel.StateID) { got = append(got, x) }
	tk.Schedule(voxel.BlockPos{X: 0, Y: 1}, int32(a), 5, 0, 0)
	tk.Schedule(voxel.BlockPos{X: 1, Y: 1}, int32(a), 2, 0, 0)
	tk.Schedule(voxel.BlockPos{X: 2, Y: 1}, int32(a), 5, -1, 0)
	tk.Schedule(voxel.BlockPos{X: 3, Y: 1}, int32(a), 5, 0, 0)
	tk.Schedule(voxel.BlockPos{X: 3, Y: 1}, int32(a), 1, 0, 0)  // already pending
	tk.Schedule(voxel.BlockPos{X: 5, Y: 1}, int32(a), 1, 0, 0)  // b there: dropped
	tk.Schedule(voxel.BlockPos{X: 99, Y: 1}, int32(a), 1, 0, 0) // not loaded
	for now := int64(0); now <= 5; now++ {
		tk.RunScheduled(now)
	}
	if want := []int{1, 2, 0, 3}; len(got) != 4 || got[0] != 1 || got[1] != 2 || got[2] != 0 || got[3] != 3 {
		t.Fatalf("order %v, want %v", got, want)
	}
	for i := 0; i < MaxPerTick+5; i++ {
		tk.Schedule(voxel.BlockPos{X: i % 16, Y: 2 + i/256, Z: i / 16 % 16}, int32(a), 0, 0, 10)
	}
	tk.RunScheduled(10)
	if tk.Len() != 5 {
		t.Fatalf("left %d, want 5", tk.Len())
	}
}

// Three LCG picks per section holding a ticking state, as the reference.
func TestRandomPicks(t *testing.T) {
	w, a, _ := world(t)
	w.SetBlock(0, 0, 0, a) // section 0 only
	tk := New(w, func(id voxel.StateID) int32 { return int32(id) })
	tk.Random = make([]bool, int(a)+1)
	tk.Random[a] = true
	n := 0
	tk.OnRandom = func(x, y, z int, id voxel.StateID) { n++ }
	lcg := int32(12345)
	tk.LCG = lcg
	hits := 0
	for range 4096 {
		tk.RunRandom([]voxel.ChunkPos{{}}, 0)
		for range 3 {
			lcg = lcg*3 + 1013904223
			r := lcg >> 2
			if r&15 == 0 && r>>8&15 == 0 && r>>16&15 == 0 {
				hits++
			}
		}
	}
	if n != hits || tk.LCG != lcg {
		t.Fatalf("ticks %d, want %d", n, hits)
	}
}

// A due tick counts as scheduled this tick until it has run.
func TestScheduledThisTick(t *testing.T) {
	w, a, _ := world(t)
	tk := New(w, func(id voxel.StateID) int32 { return int32(id) })
	p0, p1 := voxel.BlockPos{X: 0, Y: 1}, voxel.BlockPos{X: 1, Y: 1}
	w.SetBlock(0, 1, 0, a)
	w.SetBlock(1, 1, 0, a)
	tk.Schedule(p0, int32(a), 1, 0, 0)
	tk.Schedule(p1, int32(a), 1, 0, 0)
	var seen []bool
	tk.OnScheduled = func(x, y, z int, id voxel.StateID) {
		seen = append(seen, tk.ScheduledThisTick(p0, int32(a)), tk.ScheduledThisTick(p1, int32(a)))
	}
	tk.RunScheduled(1)
	if len(seen) != 4 || seen[0] || !seen[1] || seen[2] || seen[3] {
		t.Fatalf("seen %v", seen)
	}
	if tk.ScheduledThisTick(p1, int32(a)) {
		t.Fatal("still scheduled after the run")
	}
}
