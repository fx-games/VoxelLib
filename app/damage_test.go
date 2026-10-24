package app

import (
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

// TestCracks: RenderGlobal.destroyBlockInWorldPartially per breaker, and
// updateClouds dropping a crack 400 ticks after it was last set.
func TestCracks(t *testing.T) {
	a := &App{}
	b := voxel.BlockPos{X: 1, Y: 2, Z: 3}
	a.DestroyBlockPartially(7, b, 3)
	a.DestroyBlockPartially(8, b, 9)
	a.DestroyBlockPartially(9, b, 10) // out of range: none
	if len(a.cracks) != 2 || a.cracks[7].stage != 3 {
		t.Fatalf("cracks %v", a.cracks)
	}
	a.DestroyBlockPartially(8, b, -1)
	if a.cracks[8] != nil {
		t.Error("-1 kept the crack")
	}
	for a.Tick < 419 {
		a.Tick++
		a.expireCracks()
	}
	if a.cracks[7] == nil {
		t.Error("expired before 420 (400-0 > 400 is false)")
	}
	a.Tick++
	a.expireCracks()
	if a.cracks[7] != nil {
		t.Error("not expired at 420")
	}
}
