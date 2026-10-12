package anvil

import (
	"bytes"
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

// Light voxellib computed survives a save/load and marks the chunk lit;
// chunks saved unlit still load unlit (relit by the world).
func TestLightRoundTrip(t *testing.T) {
	reg := voxel.NewRegistry()
	w := voxel.NewWorld(reg, -64, 384)
	c := w.NewChunk(voxel.ChunkPos{})
	c.Sections[5].Sky[100], c.Sections[5].Block[7] = 0xAB, 0x3C
	for _, lit := range []bool{true, false} {
		c.Lit = lit
		d, err := DecodeChunk(w, EncodeChunk(w, c))
		if err != nil {
			t.Fatal(err)
		}
		if d.Lit != lit {
			t.Fatalf("Lit = %v, want %v", d.Lit, lit)
		}
		if lit && (!bytes.Equal(d.Sections[5].Sky, c.Sections[5].Sky) || !bytes.Equal(d.Sections[5].Block, c.Sections[5].Block)) {
			t.Fatal("light changed in the round trip")
		}
	}
}
