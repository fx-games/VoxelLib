package anvil

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
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

// TestStoreBoundsOpenFiles opens more regions than maxOpen from concurrent
// writers and readers: the store keeps at most maxOpen files open and every
// chunk still reads back.
func TestStoreBoundsOpenFiles(t *testing.T) {
	const n = maxOpen * 3
	st := OpenStore(t.TempDir(), true)
	defer st.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := g; i < n; i += 8 {
				if err := st.Write(i*32, -i*32, Compound{"i": int32(i)}); err != nil {
					errs <- err
					return
				}
				if c, err := st.Read((i/2)*32, -(i/2)*32); err != nil && !errors.Is(err, ErrNoChunk) {
					errs <- err
					return
				} else if err == nil && c.Int("i") != i/2 {
					errs <- fmt.Errorf("region %d holds %d", i/2, c.Int("i"))
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if len(st.regions) > maxOpen {
		t.Fatalf("%d regions open, limit %d", len(st.regions), maxOpen)
	}
	for i := 0; i < n; i++ {
		c, err := st.Read(i*32, -i*32)
		if err != nil || c.Int("i") != i {
			t.Fatalf("region %d: %v %v", i, c, err)
		}
		if len(st.regions) > maxOpen {
			t.Fatalf("%d regions open, limit %d", len(st.regions), maxOpen)
		}
	}
	if err := st.Sync(); err != nil {
		t.Fatal(err)
	}
}

// TestWriteChunkCrashSafe crashes a rewrite of a chunk after its new data
// is written but before its header entry: the region still holds the old
// chunk intact, and later writes don't reuse its sectors early.
func TestWriteChunkCrashSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "r.0.0.mca")
	r, err := OpenRegion(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.WriteChunk(1, 2, Compound{"v": int32(1)}); err != nil {
		t.Fatal(err)
	}
	writeFault = func(string) error { return errors.New("crash") }
	err = r.WriteChunk(1, 2, Compound{"v": int32(2)})
	writeFault = nil
	if err == nil {
		t.Fatal("fault not hit")
	}
	r.Close() // the process dies

	r, err = OpenRegion(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if c, err := r.ReadChunk(1, 2); err != nil || c.Int("v") != 1 {
		t.Fatalf("after crash: %v %v, want the old chunk", c, err)
	}
	// rewrite twice, then another chunk: every chunk keeps its own data
	start := time.Now()
	for v := 2; v <= 3; v++ {
		if err := r.WriteChunk(1, 2, Compound{"v": int32(v)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.WriteChunk(3, 3, Compound{"v": int32(9)}); err != nil {
		t.Fatal(err)
	}
	t.Logf("%v per chunk write", time.Since(start)/3)
	a, _ := r.ReadChunk(1, 2)
	b, _ := r.ReadChunk(3, 3)
	if a.Int("v") != 3 || b.Int("v") != 9 {
		t.Fatalf("chunks %v %v", a, b)
	}
}

// TestWriteAll: a batch spanning two regions reads back, and rewriting it
// replaces every chunk.
func TestWriteAll(t *testing.T) {
	s := OpenStore(t.TempDir(), true)
	defer s.Close()
	for round := int32(0); round < 2; round++ {
		cs := map[[2]int]Compound{}
		for _, p := range [][2]int{{0, 0}, {1, 0}, {31, 31}, {32, 0}, {-1, -5}} {
			cs[p] = Compound{"v": round*100 + int32(p[0])}
		}
		if err := s.WriteAll(cs); err != nil {
			t.Fatal(err)
		}
		for p, c := range cs {
			got, err := s.Read(p[0], p[1])
			if err != nil || got.Int("v") != c.Int("v") {
				t.Fatalf("round %d chunk %v: %v %v, want %v", round, p, got.Int("v"), err, c.Int("v"))
			}
		}
	}
}
