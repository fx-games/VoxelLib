package anvil

import (
	"testing"
	"time"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/worldgen/overworld"
)

func TestRoundTrip(t *testing.T) {
	reg := voxel.NewRegistry()
	w := voxel.NewWorld(reg, -64, 384)
	g := overworld.New(reg, 42)
	start := time.Now()
	var chunks []*voxel.Chunk
	for i := 0; i < 16; i++ {
		c := w.NewChunk(voxel.ChunkPos{X: i - 8, Z: 3})
		g.Generate(w, c)
		chunks = append(chunks, c)
	}
	t.Logf("generate: %v per chunk", time.Since(start)/16)
	st := OpenStore(t.TempDir(), true)
	for _, c := range chunks {
		if err := st.Write(c.Pos.X, c.Pos.Z, EncodeChunk(w, c)); err != nil {
			t.Fatal(err)
		}
	}
	st.Close()
	st = OpenStore(st.Dir, false)
	defer st.Close()
	for _, c := range chunks {
		nbt, err := st.Read(c.Pos.X, c.Pos.Z)
		if err != nil {
			t.Fatal(err)
		}
		d, err := DecodeChunk(w, nbt)
		if err != nil {
			t.Fatal(err)
		}
		for y := -64; y < 320; y++ {
			for z := 0; z < 16; z++ {
				for x := 0; x < 16; x++ {
					if a, b := c.Get(x, y, z), d.Get(x, y, z); a != b {
						t.Fatalf("%v %d,%d,%d: %s vs %s", c.Pos, x, y, z, reg.StateString(a), reg.StateString(b))
					}
				}
			}
		}
	}
}
