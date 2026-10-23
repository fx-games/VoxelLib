package engine

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"os"
	"path/filepath"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/anvil"
)

// A save slot (Config.SaveDir) of a Map world holds
//
//	game.dat        the game's state blob (SaveGame)
//	dim<ID>/region  per dimension, region files whose chunks hold only the
//	                sections the player edited (1.18-style NBT, block keys)
//	save.journal    only while a save is being applied (see below)
//
// Loading a chunk = the map's (or generated) chunk with those sections
// laid over it.
//
// Saves are all-or-nothing through a journal. Between saves, edited chunks
// that stream out are staged in memory (dimSrc.unsaved), never written to
// the slot, so the regions always match game.dat. SaveGame writes the game
// blob and every staged chunk into one file, fsyncs it and renames it to
// save.journal: that rename is the commit. Only then are the chunks written
// into the region files (fsynced), game.dat replaced (temp + fsync +
// rename) and the journal removed. A crash before the rename leaves the
// previous save untouched; a crash after it is finished on the next open
// by replaying the journal, which is idempotent. A journal was chosen over
// copying the slot into a new directory because a save then costs only the
// changed chunks, and the region files keep their layout.
//
// A slot with neither game.dat nor a journal holds no save (e.g. NewGame
// was interrupted): its region files are ignored, and the first save
// clears them.

const (
	gameFile    = "game.dat"
	journalFile = "save.journal"
)

func slotDim(dir string, dim int) string { return filepath.Join(dir, fmt.Sprintf("dim%d", dim)) }

// saveFault, when set by a test, fails a save at a step ("journal" before
// the commit rename, "chunk" after the first region write, "game" before
// game.dat's rename) to simulate a crash there.
var saveFault func(step string) error

// stageHook, when set by a test, runs as a chunk starts being staged.
var stageHook func(voxel.ChunkPos)

func fault(step string) error {
	if saveFault != nil {
		return saveFault(step)
	}
	return nil
}

// openSlot finishes a save a crash interrupted and opens the slot's
// regions of dimension dim. Read-only, it writes nothing and returns the
// pending journal instead, for the engine to lay over what it reads
// (Engine.journal). fresh reports that the slot's regions hold no save.
func openSlot(dir string, dim int, write bool) (st *anvil.Store, fresh bool, pending *journal, err error) {
	j, err := readJournal(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		j = nil
	case err != nil:
		return nil, false, nil, err
	case write:
		if err := applyJournal(dir, j, nil); err != nil {
			return nil, false, nil, fmt.Errorf("engine: finishing interrupted save: %w", err)
		}
		j = nil
	}
	fresh = !HasSave(dir) || (j != nil && j.reset)
	return anvil.OpenStore(slotDim(dir, dim), write), fresh, j, nil
}

// journal is one committed save: the game blob and the staged chunks by
// dimension. reset clears every dimension's regions first (the first save
// of a fresh slot).
type journal struct {
	reset  bool
	game   []byte
	chunks map[int]map[voxel.ChunkPos]anvil.Compound
}

func writeJournal(dir string, j *journal) error {
	list := anvil.List{Type: anvil.TagCompound}
	for dim, cs := range j.chunks {
		for p, c := range cs {
			list.Items = append(list.Items, anvil.Compound{"dim": int32(dim), "x": int32(p.X), "z": int32(p.Z), "chunk": c})
		}
	}
	reset := int8(0)
	if j.reset {
		reset = 1
	}
	var buf bytes.Buffer
	if err := anvil.WriteNBT(&buf, "", anvil.Compound{
		"reset": reset, "game": append([]byte{}, j.game...), "chunks": list,
	}); err != nil {
		return err
	}
	return writeFileSync(filepath.Join(dir, journalFile), buf.Bytes(), "journal")
}

func readJournal(dir string) (*journal, error) {
	data, err := os.ReadFile(filepath.Join(dir, journalFile))
	if err != nil {
		return nil, err
	}
	c, _, err := anvil.ReadNBT(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("engine: %s: %w", journalFile, err)
	}
	j := &journal{reset: c.Int("reset") != 0, game: c.Bytes("game"), chunks: map[int]map[voxel.ChunkPos]anvil.Compound{}}
	for _, it := range c.List("chunks").Items {
		e, _ := it.(anvil.Compound)
		dim := c.Int("dim") // journals of one dimension had it here
		if e.Has("dim") {
			dim = e.Int("dim")
		}
		if j.chunks[dim] == nil {
			j.chunks[dim] = map[voxel.ChunkPos]anvil.Compound{}
		}
		j.chunks[dim][voxel.ChunkPos{X: e.Int("x"), Z: e.Int("z")}] = e.Compound("chunk")
	}
	return j, nil
}

// applyJournal writes a committed journal into the slot and removes it.
// stores are the open stores by dimension; the others are opened here.
// Safe to repeat.
func applyJournal(dir string, j *journal, stores map[int]*anvil.Store) error {
	if j.reset {
		for _, st := range stores {
			st.Close() // reopens its files on demand
		}
		if err := removeDims(dir); err != nil {
			return err
		}
		for _, st := range stores {
			os.MkdirAll(st.Dir, 0o755)
		}
	}
	// The first chunk goes alone so a test can crash between chunks; the
	// rest share one fsync per region file.
	first := true
	for dim, cs := range j.chunks {
		st := stores[dim]
		if st == nil {
			st = anvil.OpenStore(slotDim(dir, dim), true)
			defer st.Close()
		}
		rest := make(map[[2]int]anvil.Compound, len(cs))
		for p, c := range cs {
			if !first {
				rest[[2]int{p.X, p.Z}] = c
				continue
			}
			if err := st.Write(p.X, p.Z, c); err != nil {
				return err
			}
			if err := fault("chunk"); err != nil {
				return err
			}
			first = false
		}
		if err := st.WriteAll(rest); err != nil {
			return err
		}
		if err := st.Sync(); err != nil {
			return err
		}
	}
	if err := writeFileSync(filepath.Join(dir, gameFile), j.game, "game"); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, journalFile)); err != nil {
		return err
	}
	syncDir(dir)
	return nil
}

// writeFileSync replaces file with data atomically and durably: temp file,
// fsync, rename, fsync of the directory.
func writeFileSync(file string, data []byte, step string) error {
	tmp := file + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = fault(step)
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, file); err != nil {
		return err
	}
	syncDir(filepath.Dir(file))
	return nil
}

// syncDir makes renames in dir durable. Best effort: not every system
// can fsync a directory.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}

func removeDims(dir string) error {
	dims, _ := filepath.Glob(filepath.Join(dir, "dim*"))
	for _, d := range dims {
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}
	return nil
}

// produceMap makes chunk p of dimension s of a Map world: map regions,
// else the generator, then the slot's edited sections on top.
func (e *Engine) produceMap(s *dimSrc, p voxel.ChunkPos) *voxel.Chunk {
	c, level, err := e.cfg.Map.ReadChunk(e.World, s.id, p)
	switch {
	case err == nil:
		if e.cfg.OnMapChunk != nil {
			e.cfg.OnMapChunk(p, level)
		}
	default:
		if !errors.Is(err, anvil.ErrNoChunk) {
			log.Printf("engine: map chunk %v: %v", p, err)
		}
		c = e.World.NewChunk(p)
		if s.gen != nil {
			s.gen.Generate(e.World, c)
		}
	}
	if s.store != nil {
		e.applyDelta(s, c)
	}
	return c
}

// applyDelta lays the slot's edited sections of c (of dimension s) over it.
func (e *Engine) applyDelta(s *dimSrc, c *voxel.Chunk) {
	e.unsavedMu.Lock()
	staged := s.unsaved[c.Pos]
	e.unsavedMu.Unlock()
	var nbt anvil.Compound
	var err error
	var committed anvil.Compound
	if j := e.journal; j != nil {
		committed = j.chunks[s.id][c.Pos]
	}
	switch {
	case staged != nil:
		nbt = *staged
	case committed != nil:
		nbt = committed
	case e.fresh.Load():
		return
	default:
		nbt, err = s.store.Read(c.Pos.X, c.Pos.Z)
	}
	if err != nil {
		if !errors.Is(err, anvil.ErrNoChunk) {
			log.Printf("engine: save %v: %v", c.Pos, err)
		}
		return
	}
	d, err := anvil.DecodeChunk(e.World, nbt)
	if err != nil {
		log.Printf("engine: save %v: %v", c.Pos, err)
		return
	}
	for _, it := range nbt.List("sections").Items {
		sc, _ := it.(anvil.Compound)
		i := sc.Int("Y") - e.World.MinSection()
		if i >= 0 && i < len(c.Sections) && i < 64 {
			c.Sections[i] = d.Sections[i]
			c.Edited |= 1 << uint(i)
		}
	}
	c.Lit = false
	c.RecomputeHeights(e.World.Reg.Table())
}

// encodeDelta encodes only the edited sections of c (nil: none). Caller
// holds the world read lock.
func encodeDelta(w *voxel.World, c *voxel.Chunk) anvil.Compound {
	if c.Edited == 0 {
		return nil
	}
	root := anvil.EncodeChunk(w, c)
	all := root.List("sections")
	kept := anvil.List{Type: anvil.TagCompound}
	for i, it := range all.Items {
		if c.Edited&(1<<uint(i)) != 0 {
			kept.Items = append(kept.Items, it)
		}
	}
	root["sections"] = kept
	return root
}

// stage keeps the edited sections of c (a Map world's chunk of dimension
// s) until the next SaveGame. Caller holds stageMu.
func (e *Engine) stage(s *dimSrc, c *voxel.Chunk) {
	if stageHook != nil {
		stageHook(c.Pos)
	}
	e.World.RLock()
	nbt := encodeDelta(e.World, c)
	e.World.RUnlock()
	if nbt == nil {
		return
	}
	e.unsavedMu.Lock()
	if s.unsaved == nil {
		s.unsaved = map[voxel.ChunkPos]*anvil.Compound{}
	}
	s.unsaved[c.Pos] = &nbt
	e.unsavedMu.Unlock()
}

// SaveGame saves the game: every chunk changed so far and the game's state
// blob (anything the game wants back on load: entities, inventory, quests),
// all or nothing (see the top of save.go). Changes not saved by a SaveGame
// are lost on Close. It does nothing without Config.SaveDir.
func (e *Engine) SaveGame(state []byte) error {
	if e.cfg.SaveDir == "" || e.cfg.ReadOnly {
		return nil
	}
	e.slotMu.Lock()
	defer e.slotMu.Unlock()
	// Every edited chunk is in the world or in unsaved: the light worker
	// unloads and stages under stageMu, so none is between the two now.
	e.stageMu.Lock()
	for _, p := range e.World.Chunks() {
		if c := e.World.Chunk(p); c != nil && c.Modified {
			e.stage(e.worldSrc, c)
		}
	}
	e.unsavedMu.Lock()
	snap := map[*dimSrc]map[voxel.ChunkPos]*anvil.Compound{}
	stores := map[int]*anvil.Store{}
	for _, s := range e.dims {
		snap[s] = maps.Clone(s.unsaved)
		if s.store != nil {
			stores[s.id] = s.store
		}
	}
	e.unsavedMu.Unlock()
	e.stageMu.Unlock()
	if err := os.MkdirAll(e.cfg.SaveDir, 0o755); err != nil {
		return err
	}
	j := &journal{reset: e.fresh.Load(), game: state, chunks: map[int]map[voxel.ChunkPos]anvil.Compound{}}
	for s, cs := range snap {
		if len(cs) == 0 {
			continue
		}
		j.chunks[s.id] = make(map[voxel.ChunkPos]anvil.Compound, len(cs))
		for p, c := range cs {
			j.chunks[s.id][p] = *c
		}
	}
	if err := writeJournal(e.cfg.SaveDir, j); err != nil {
		os.Remove(filepath.Join(e.cfg.SaveDir, journalFile+".tmp"))
		return err
	}
	if err := applyJournal(e.cfg.SaveDir, j, stores); err != nil {
		return err // committed: the next open finishes it
	}
	e.fresh.Store(false)
	e.unsavedMu.Lock()
	for s, cs := range snap {
		for p, c := range cs {
			if s.unsaved[p] == c { // not staged again meanwhile
				delete(s.unsaved, p)
			}
		}
	}
	e.unsavedMu.Unlock()
	return nil
}

// GameState returns the blob of the slot's last SaveGame, or nil for a
// new game.
func (e *Engine) GameState() ([]byte, error) {
	if e.cfg.SaveDir == "" {
		return nil, nil
	}
	if e.journal != nil { // read-only over an unfinished save: its blob
		return append([]byte{}, e.journal.game...), nil
	}
	data, err := os.ReadFile(filepath.Join(e.cfg.SaveDir, gameFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// HasSave reports whether a save slot holds a saved game.
func HasSave(saveDir string) bool {
	for _, f := range []string{gameFile, journalFile} {
		if _, err := os.Stat(filepath.Join(saveDir, f)); err == nil {
			return true
		}
	}
	return false
}

// NewGame empties a save slot, so the next Engine on it starts from the
// map alone. Call it while no Engine uses the slot. Removing game.dat is
// the step that drops the save; region files left by a crash after it are
// ignored (see the top of save.go).
func NewGame(saveDir string) error {
	// game.dat first: while only a journal is left, it is a whole save
	for _, f := range []string{gameFile, journalFile} {
		if err := os.Remove(filepath.Join(saveDir, f)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	syncDir(saveDir)
	return removeDims(saveDir)
}

// closeStores closes every dimension's store.
func (e *Engine) closeStores() {
	e.unsavedMu.Lock()
	defer e.unsavedMu.Unlock()
	for _, s := range e.dims {
		if s.store != nil {
			s.store.Close()
		}
	}
}
