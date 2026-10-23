package engine

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/memmaker/voxellib/resloc"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/anvil"
	"github.com/memmaker/voxellib/voxel/voxelmap"
	"github.com/memmaker/voxellib/voxel/voxelmap/maptest"
	"github.com/memmaker/voxellib/worldgen/flat"
)

// mapEngine is an Engine without renderer or workers: enough for chunk
// composition and saving. It opens the slot like New does.
func mapEngine(t *testing.T, m *voxelmap.Map, reg *voxel.Registry, slot string, hook func(voxel.ChunkPos, anvil.Compound)) *Engine {
	t.Helper()
	return slotEngine(t, m, reg, slot, hook, false)
}

func slotEngine(t *testing.T, m *voxelmap.Map, reg *voxel.Registry, slot string, hook func(voxel.ChunkPos, anvil.Compound), readOnly bool) *Engine {
	t.Helper()
	st, fresh, j, err := openSlot(slot, 0, !readOnly)
	if err != nil {
		t.Fatal(err)
	}
	w := voxel.NewWorld(reg, voxelmap.MinY, voxelmap.Height)
	gen := &flat.Generator{Layers: []flat.Layer{{Block: m.State(resloc.Default + ":stone#0"), Count: 2}}}
	e := &Engine{World: w, delta: true, journal: j, cfg: Config{
		Map: m, SaveDir: slot, OnMapChunk: hook, ReadOnly: readOnly, Generator: gen,
	}}
	src := &dimSrc{id: 0, gen: gen, store: st}
	e.src.Store(src)
	e.worldSrc, e.dims = src, map[int]*dimSrc{0: src}
	e.fresh.Store(fresh)
	return e
}

func TestMapCompositionAndDeltaSave(t *testing.T) {
	dir, slot := t.TempDir(), t.TempDir()
	if err := maptest.Write(dir); err != nil {
		t.Fatal(err)
	}
	reg := voxel.NewRegistry()
	m, err := voxelmap.Open(dir, reg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	stone, lamp := m.State(resloc.Default+":stone#0"), m.State("test:lamp#2")
	var hooked atomic.Int32
	hook := func(p voxel.ChunkPos, level anvil.Compound) {
		if p == (voxel.ChunkPos{}) && len(level.List("Entities").Items) == 1 {
			hooked.Add(1)
		}
	}

	// map chunk 0,0 and generated chunk 1,0
	e := mapEngine(t, m, reg, slot, hook)
	c0, c1 := e.produce(voxel.ChunkPos{}), e.produce(voxel.ChunkPos{X: 1})
	if c0.Get(1, 4, 1) != lamp || c0.Get(0, 3, 0) != stone || c1.Get(0, 1, 0) != stone || c1.Get(0, 2, 0) != voxel.Air {
		t.Fatal("composition: map chunk or generated chunk wrong")
	}
	if hooked.Load() != 1 {
		t.Fatal("OnMapChunk not called with the map chunk's NBT")
	}
	e.World.AddChunks([]*voxel.Chunk{c0, c1})
	e.World.SetBlock(5, 40, 5, lamp)     // map chunk, section 2
	e.World.SetBlock(1, 4, 1, voxel.Air) // map chunk, section 0
	e.World.SetBlock(20, 1, 3, lamp)     // generated chunk
	if err := e.SaveGame([]byte("quest=3")); err != nil {
		t.Fatal(err)
	}
	e.closeStores()

	// load = map + delta
	e = mapEngine(t, m, reg, slot, hook)
	c0, c1 = e.produce(voxel.ChunkPos{}), e.produce(voxel.ChunkPos{X: 1})
	if c0.Get(5, 40, 5) != lamp || c0.Get(1, 4, 1) != voxel.Air || c0.Get(2, 4, 2) != m.State("test:plant#0") || c1.Get(4, 1, 3) != lamp {
		t.Fatal("delta not applied")
	}
	if c0.Edited != 1|1<<2 || c0.Signs == nil || c0.Biomes == nil {
		t.Fatalf("edited %b, signs %v", c0.Edited, c0.Signs)
	}
	if st, err := e.GameState(); err != nil || !bytes.Equal(st, []byte("quest=3")) || !HasSave(slot) {
		t.Fatalf("game state %q %v", st, err)
	}
	// only edited sections are stored
	nbt, err := e.src.Load().store.Read(0, 0)
	if err != nil || len(nbt.List("sections").Items) != 2 {
		t.Fatalf("delta chunk: %v sections, %v", len(nbt.List("sections").Items), err)
	}
	e.closeStores()

	// new game = the map again
	if err := NewGame(slot); err != nil {
		t.Fatal(err)
	}
	e = mapEngine(t, m, reg, slot, hook)
	defer e.closeStores()
	if c0 = e.produce(voxel.ChunkPos{}); c0.Get(1, 4, 1) != lamp || c0.Get(5, 40, 5) != voxel.Air || HasSave(slot) {
		t.Fatal("NewGame did not reset the slot")
	}
	if st, _ := e.GameState(); st != nil {
		t.Fatal("NewGame kept the game state")
	}
}

// TestInterruptedSave crashes a save at each step: before the journal's
// commit the previous save must load intact, after it the next open must
// finish the new save. Never a mix.
func TestInterruptedSave(t *testing.T) {
	dir := t.TempDir()
	if err := maptest.Write(dir); err != nil {
		t.Fatal(err)
	}
	reg := voxel.NewRegistry()
	m, err := voxelmap.Open(dir, reg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	lamp := m.State("test:lamp#2")
	load := func(e *Engine) {
		e.World.AddChunks([]*voxel.Chunk{e.produce(voxel.ChunkPos{}), e.produce(voxel.ChunkPos{X: 1})})
	}
	for _, step := range []string{"journal", "chunk", "game"} {
		slot := t.TempDir()
		e := mapEngine(t, m, reg, slot, nil)
		load(e)
		e.World.SetBlock(5, 40, 5, lamp)
		if err := e.SaveGame([]byte("v1")); err != nil {
			t.Fatal(err)
		}
		e.World.SetBlock(5, 41, 5, lamp) // not saved: lost in the crash
		e.closeStores()

		e = mapEngine(t, m, reg, slot, nil)
		load(e)
		e.World.SetBlock(6, 40, 6, lamp)
		e.World.SetBlock(20, 1, 3, lamp)
		saveFault = func(s string) error {
			if s == step {
				return errors.New("crash")
			}
			return nil
		}
		err := e.SaveGame([]byte("v2"))
		saveFault = nil
		if err == nil {
			t.Fatalf("%s: injected failure not hit", step)
		}
		e.closeStores() // the process dies here

		// A read-only engine sees the committed state without writing.
		before := slotFiles(t, slot)
		e = slotEngine(t, m, reg, slot, nil, true)
		load(e)
		st, err := e.GameState()
		if v2 := step != "journal"; err != nil || (string(st) == "v2") != v2 ||
			(e.World.Block(6, 40, 6) == lamp) != v2 || (e.World.Block(20, 1, 3) == lamp) != v2 || e.World.Block(5, 40, 5) != lamp {
			t.Fatalf("read-only after crash at %s: state %q %v", step, st, err)
		}
		e.closeStores()
		if after := slotFiles(t, slot); after != before {
			t.Fatalf("read-only engine changed the slot:\n%s\n%s", before, after)
		}

		e = mapEngine(t, m, reg, slot, nil)
		load(e)
		st, err = e.GameState()
		if err != nil {
			t.Fatal(err)
		}
		v2 := step != "journal"
		w := e.World
		if !HasSave(slot) || w.Block(5, 40, 5) != lamp || w.Block(5, 41, 5) == lamp ||
			(w.Block(6, 40, 6) == lamp) != v2 || (w.Block(20, 1, 3) == lamp) != v2 || (string(st) == "v2") != v2 {
			t.Fatalf("crash at %s: state %q, blocks %v %v %v %v", step, st,
				w.Block(5, 40, 5) == lamp, w.Block(5, 41, 5) == lamp, w.Block(6, 40, 6) == lamp, w.Block(20, 1, 3) == lamp)
		}
		if _, err := os.Stat(filepath.Join(slot, journalFile)); err == nil {
			t.Fatalf("crash at %s: journal left after recovery", step)
		}
		e.closeStores()
	}
}

// TestSaveWhileUnloading saves while the light worker is between taking an
// edited chunk out of the world and staging it: the save must wait for it.
func TestSaveWhileUnloading(t *testing.T) {
	dir, slot := t.TempDir(), t.TempDir()
	if err := maptest.Write(dir); err != nil {
		t.Fatal(err)
	}
	reg := voxel.NewRegistry()
	m, err := voxelmap.Open(dir, reg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	lamp := m.State("test:lamp#2")
	e := mapEngine(t, m, reg, slot, nil)
	e.World.AddChunks([]*voxel.Chunk{e.produce(voxel.ChunkPos{}), e.produce(voxel.ChunkPos{X: 1})})
	e.World.SetBlock(20, 1, 3, lamp)
	e.worldOps, e.litRes, e.quit = make(chan worldOp), make(chan voxel.ChunkPos, 16), make(chan struct{})
	e.wg.Add(1)
	go e.lightWorker()

	inside, release := make(chan struct{}), make(chan struct{})
	stageHook = func(p voxel.ChunkPos) {
		if p == (voxel.ChunkPos{X: 1}) && inside != nil {
			close(inside)
			inside = nil
			<-release
		}
	}
	defer func() { stageHook = nil }()
	wait := inside
	e.worldOps <- worldOp{remove: []voxel.ChunkPos{{X: 1}}}
	<-wait // chunk 1 is out of the world, not yet staged
	done := make(chan error)
	go func() { done <- e.SaveGame([]byte("s")) }()
	select {
	case err := <-done:
		t.Fatalf("SaveGame finished while a chunk was in flight (%v)", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	close(e.quit)
	e.wg.Wait()
	e.closeStores()

	e = mapEngine(t, m, reg, slot, nil)
	defer e.closeStores()
	if c := e.produce(voxel.ChunkPos{X: 1}); c.Get(4, 1, 3) != lamp {
		t.Fatal("the in-flight chunk's edit is missing from the save")
	}
}

// TestDimensions edits two dimensions, switching between them through the
// light worker, and saves both in one SaveGame.
func TestDimensions(t *testing.T) {
	dir, slot := t.TempDir(), t.TempDir()
	if err := maptest.Write(dir); err != nil {
		t.Fatal(err)
	}
	mj := filepath.Join(dir, "map.json")
	b, _ := os.ReadFile(mj)
	b = bytes.Replace(b, []byte(`"generator": "test:flat"}]`), []byte(`"generator": "test:flat"}, {"id": 7, "name": "Under", "regions": "regions/7"}]`), 1)
	os.WriteFile(mj, b, 0o644)
	reg := voxel.NewRegistry()
	m, err := voxelmap.Open(dir, reg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	lamp := m.State("test:lamp#2")
	under := &flat.Generator{Layers: []flat.Layer{{Block: lamp, Count: 1}}}
	e := mapEngine(t, m, reg, slot, nil)
	e.cfg.Skyless = map[int]bool{7: true}
	e.worldOps, e.litRes, e.quit = make(chan worldOp), make(chan voxel.ChunkPos, 16), make(chan struct{})
	e.wg.Add(1)
	go e.lightWorker()
	e.World.AddChunks([]*voxel.Chunk{e.produce(voxel.ChunkPos{X: 1})})
	e.World.SetBlock(20, 1, 3, lamp)
	s7, err := e.useDim(7, under)
	if err != nil {
		t.Fatal(err)
	}
	if c := e.produce(voxel.ChunkPos{X: 1}); c.Get(4, 0, 3) != lamp || e.Dimension() != 7 {
		t.Fatal("dimension 7 not generated by its generator")
	}
	e.worldOps <- worldOp{remove: []voxel.ChunkPos{{X: 1}}, to: s7}
	e.worldOps <- worldOp{add: []*voxel.Chunk{e.produce(voxel.ChunkPos{X: 1})}}
	e.worldOps <- worldOp{} // the add is done
	if !e.World.NoSky.Load() {
		t.Fatal("the world kept the sky of dimension 0")
	}
	e.World.SetBlock(21, 5, 3, lamp)
	if err := e.SaveGame([]byte("s")); err != nil {
		t.Fatal(err)
	}
	close(e.quit)
	e.wg.Wait()
	e.closeStores()

	e = mapEngine(t, m, reg, slot, nil)
	defer e.closeStores()
	if c := e.produce(voxel.ChunkPos{X: 1}); c.Get(4, 1, 3) != lamp || c.Get(5, 5, 3) == lamp {
		t.Fatal("dimension 0's edit lost or mixed up")
	}
	if _, err := e.useDim(7, under); err != nil {
		t.Fatal(err)
	}
	if c := e.produce(voxel.ChunkPos{X: 1}); c.Get(5, 5, 3) != lamp || c.Get(4, 1, 3) == lamp {
		t.Fatal("dimension 7's edit lost or mixed up")
	}
}

// slotFiles lists a slot's files with sizes and modification times.
func slotFiles(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		fi, err := d.Info()
		if err == nil {
			fmt.Fprintf(&b, "%s %d %d\n", p, fi.Size(), fi.ModTime().UnixNano())
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
