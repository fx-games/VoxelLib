package voxel

import (
	"sync"
	"sync/atomic"
)

// World is a thread-safe store of chunk columns.
//
// Readers that touch many blocks (the mesher) take RLock once and use the
// *Unlocked accessors, instead of paying a lock per block.
type World struct {
	Reg    *Registry
	MinY   int
	Height int

	mu sync.RWMutex
	// lightMu serialises light passes, which run under the read lock and
	// share lightQ/removeQ/cache. SetBlock relights under the write lock.
	lightMu sync.Mutex
	chunks  map[ChunkPos]*Chunk
	// dirty has its own lock so TakeDirty never waits behind a light pass.
	dirtyMu sync.Mutex
	dirty   map[SectionPos]struct{}

	lightQ  []lightNode
	removeQ []lightNode
	cache   *Chunk

	dyn atomic.Pointer[DynField] // dynamic lights (dynlight.go)
}

// NewWorld creates an empty world spanning [minY, minY+height). Minecraft
// 1.18+ overworld is -64, 384.
func NewWorld(reg *Registry, minY, height int) *World {
	if height%16 != 0 || minY%16 != 0 {
		panic("voxel: world minY and height must be multiples of 16")
	}
	return &World{Reg: reg, MinY: minY, Height: height,
		chunks: map[ChunkPos]*Chunk{}, dirty: map[SectionPos]struct{}{}}
}

func (w *World) MaxY() int { return w.MinY + w.Height }

// SectionCount is the number of sections in a column.
func (w *World) SectionCount() int { return w.Height / 16 }

// MinSection is the section y index of the lowest section.
func (w *World) MinSection() int { return w.MinY >> 4 }

func (w *World) RLock()   { w.mu.RLock() }
func (w *World) RUnlock() { w.mu.RUnlock() }
func (w *World) Lock()    { w.mu.Lock() }
func (w *World) Unlock()  { w.mu.Unlock() }

// NewChunk allocates an empty chunk shaped for this world (not inserted).
func (w *World) NewChunk(pos ChunkPos) *Chunk { return newChunk(pos, w.MinY, w.Height) }

// Chunk returns a loaded chunk or nil.
func (w *World) Chunk(pos ChunkPos) *Chunk {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.chunks[pos]
}

// ChunkUnlocked is Chunk for callers already holding a lock.
func (w *World) ChunkUnlocked(pos ChunkPos) *Chunk { return w.chunks[pos] }

// HasChunk reports whether a column is loaded.
func (w *World) HasChunk(pos ChunkPos) bool { return w.Chunk(pos) != nil }

// ChunkCount returns the number of loaded columns.
func (w *World) ChunkCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.chunks)
}

// Chunks returns a snapshot of loaded chunk positions.
func (w *World) Chunks() []ChunkPos {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]ChunkPos, 0, len(w.chunks))
	for p := range w.chunks {
		out = append(out, p)
	}
	return out
}

// AddChunk inserts a column. It is not lit yet: call LightChunk once its
// neighbours are present (the Streamer does this).
func (w *World) AddChunk(c *Chunk) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.chunks[c.Pos] = c
	w.cache = nil
}

// AddChunks inserts several columns under one lock.
func (w *World) AddChunks(cs []*Chunk) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range cs {
		w.chunks[c.Pos] = c
	}
	w.cache = nil
}

// RemoveChunks unloads several columns under one lock and returns those that
// were loaded.
func (w *World) RemoveChunks(ps []ChunkPos) []*Chunk {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []*Chunk
	for _, p := range ps {
		if c := w.chunks[p]; c != nil {
			out = append(out, c)
			delete(w.chunks, p)
		}
	}
	w.cache = nil
	return out
}

// RemoveChunk unloads a column and returns it (nil if not loaded).
func (w *World) RemoveChunk(pos ChunkPos) *Chunk {
	w.mu.Lock()
	defer w.mu.Unlock()
	c := w.chunks[pos]
	delete(w.chunks, pos)
	w.cache = nil
	return c
}

func (w *World) chunkAt(x, z int) *Chunk {
	cx, cz := x>>4, z>>4
	if c := w.cache; c != nil && c.Pos.X == cx && c.Pos.Z == cz {
		return c
	}
	c := w.chunks[ChunkPos{cx, cz}]
	if c != nil {
		w.cache = c
	}
	return c
}

// Block returns the state at a position (air when unloaded or out of range).
func (w *World) Block(x, y, z int) StateID {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.BlockUnlocked(x, y, z)
}

// BlockUnlocked is Block for callers holding a lock. It does not use the
// chunk cache so it is safe under a read lock from several goroutines.
func (w *World) BlockUnlocked(x, y, z int) StateID {
	c := w.chunks[ChunkPos{x >> 4, z >> 4}]
	if c == nil {
		return Air
	}
	return c.Get(x&15, y, z&15)
}

// Light returns sky and block light at a position.
func (w *World) Light(x, y, z int) (sky, block uint8) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if y >= w.MaxY() {
		return 15, 0
	}
	c := w.chunks[ChunkPos{x >> 4, z >> 4}]
	if c == nil {
		return 15, 0
	}
	s := c.Section(y)
	if s == nil {
		return 0, 0
	}
	i := SectionIndex(x, y, z)
	return getNibble(s.Sky, i), getNibble(s.Block, i)
}

// SetBlock changes a block, keeping heightmap and light up to date and marking
// every section whose mesh may change (including neighbours that share the
// face, AO or smooth-light samples). Returns false if the chunk isn't loaded.
func (w *World) SetBlock(x, y, z int, id StateID) bool {
	if y < w.MinY || y >= w.MaxY() {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	c := w.chunkAt(x, z)
	if c == nil {
		return false
	}
	lx, lz := x&15, z&15
	old := c.Get(lx, y, lz)
	if old == id {
		return true
	}
	c.SetRaw(lx, y, lz, id)
	c.Modified = true
	c.Edited |= 1 << uint((y-w.MinY)>>4)
	t := w.Reg.Table()

	// heightmap
	hi := lz<<4 | lx
	h := int(c.Height[hi])
	if t[id].Opacity > 0 && y >= h {
		c.Height[hi] = int32(y + 1)
	} else if t[id].Opacity == 0 && y == h-1 {
		ny := w.MinY
		for yy := y - 1; yy >= w.MinY; yy-- {
			if t[c.Get(lx, yy, lz)].Opacity > 0 {
				ny = yy + 1
				break
			}
		}
		c.Height[hi] = int32(ny)
	}

	// Any section within one block of the change may need a remesh.
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for dz := -1; dz <= 1; dz++ {
				w.markDirty(SectionPos{(x + dx) >> 4, (y + dy) >> 4, (z + dz) >> 4})
			}
		}
	}

	if c.Lit {
		w.relight(x, y, z, t[old], t[id], t)
	}
	return true
}

// markDirty records a section for remeshing; caller holds the write lock.
func (w *World) markDirty(p SectionPos) {
	w.dirtyMu.Lock()
	w.dirty[p] = struct{}{}
	w.dirtyMu.Unlock()
}

// MarkDirty flags a section for remeshing.
func (w *World) MarkDirty(p SectionPos) {
	w.mu.Lock()
	w.markDirty(p)
	w.mu.Unlock()
}

// TakeDirty returns and clears the set of sections that need remeshing.
func (w *World) TakeDirty() []SectionPos {
	w.dirtyMu.Lock()
	defer w.dirtyMu.Unlock()
	if len(w.dirty) == 0 {
		return nil
	}
	out := make([]SectionPos, 0, len(w.dirty))
	for p := range w.dirty {
		out = append(out, p)
	}
	clear(w.dirty)
	return out
}

// Neighbours8Loaded reports whether a column and all 8 around it exist.
func (w *World) neighbours8Loaded(p ChunkPos) bool {
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if w.chunks[p.Add(dx, dz)] == nil {
				return false
			}
		}
	}
	return true
}

// Renderable reports whether a column is lit and has lit neighbours on all
// sides, so its mesh (faces, AO, smooth light) won't change when they arrive.
func (w *World) Renderable(p ChunkPos) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.RenderableUnlocked(p)
}

func (w *World) RenderableUnlocked(p ChunkPos) bool {
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			c := w.chunks[p.Add(dx, dz)]
			if c == nil || !c.Lit {
				return false
			}
		}
	}
	return true
}

// SolidAt reports whether the block at a position has a collision box.
func (w *World) SolidAt(x, y, z int) bool {
	return w.Reg.Info(w.Block(x, y, z)).Collides
}

// PrecipitationHeight is Minecraft's World.getPrecipitationHeight: one
// above the highest block that collides or is a liquid (rain and snow stop
// there), or MinY for an empty or unloaded column.
func (w *World) PrecipitationHeight(x, z int) int {
	t := w.Reg.Table()
	w.mu.RLock()
	defer w.mu.RUnlock()
	c := w.chunks[ChunkPos{x >> 4, z >> 4}]
	if c == nil {
		return w.MinY
	}
	for y := w.MaxY() - 1; y >= w.MinY; y-- {
		s := c.Section(y)
		if s.IsEmpty() {
			y &^= 15 // skip to the section bottom
			continue
		}
		if i := &t[s.Blocks.Get(SectionIndex(x&15, y, z&15))]; i.Collides || i.Shape == ShapeLiquid {
			return y + 1
		}
	}
	return w.MinY
}
