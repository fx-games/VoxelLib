package voxel

import (
	"sync/atomic"
	"unsafe"
)

// Section is a 16^3 cube of a chunk column.
type Section struct {
	Blocks *PalettedContainer
	// Light nibbles, 2048 bytes each in Anvil order (low nibble = even index).
	Sky   []byte
	Block []byte
	// nonAir counts non-air states; zero means the section can be skipped by
	// the mesher and the renderer entirely (the reference's hasOnlyAir()).
	nonAir int
	// ticking counts the states of the chunk's tickFlags and tickBits has
	// the bit SectionIndex set for each (nil while there are none).
	ticking  int32
	tickBits *[SectionVolume / 64]uint64
}

func NewSection() *Section {
	return &Section{Blocks: NewPalettedContainer(Air), Sky: make([]byte, 2048), Block: make([]byte, 2048)}
}

func (s *Section) IsEmpty() bool { return s == nil || s.nonAir == 0 }

// Recount recomputes the non-air counter (after bulk loading).
func (s *Section) Recount() {
	s.nonAir = 0
	if s.Blocks.Bits() == 0 {
		if s.Blocks.Get(0) != Air {
			s.nonAir = SectionVolume
		}
		return
	}
	var tmp [SectionVolume]StateID
	s.Blocks.Unpack(&tmp)
	for _, v := range tmp {
		if v != Air {
			s.nonAir++
		}
	}
}

// Light nibbles are read and written a 32-bit word at a time with atomic
// ops: a light pass writes them under the world's read lock while other
// readers hold it too (TryLightChunk). Writers never overlap (lightMu, or
// the write lock), so a set is a load and a store. The arrays come from
// make (word aligned); the nibble's place in its word assumes a
// little-endian machine.
func lightWord(a []byte, i int) *uint32 { return (*uint32)(unsafe.Pointer(&a[i>>3<<2])) }

func getNibble(a []byte, i int) uint8 {
	return uint8(atomic.LoadUint32(lightWord(a, i)) >> ((uint(i) & 7) << 2) & 15)
}

func setNibble(a []byte, i int, v uint8) {
	p, sh := lightWord(a, i), (uint(i)&7)<<2
	atomic.StoreUint32(p, atomic.LoadUint32(p)&^(15<<sh)|uint32(v&15)<<sh)
}

// Nibble is light nibble i of a section's Sky or Block array (0 for nil),
// safe to call during a light pass.
func Nibble(a []byte, i int) uint8 {
	if a == nil {
		return 0
	}
	return getNibble(a, i)
}

// clearNibbles zeroes a light array during a light pass.
func clearNibbles(a []byte) {
	for i := 0; i < len(a); i += 4 {
		atomic.StoreUint32((*uint32)(unsafe.Pointer(&a[i])), 0)
	}
}

// CopyNibbles copies a light array, safe during a light pass.
func CopyNibbles(a []byte) []byte {
	out := make([]byte, len(a))
	for i := 0; i < len(a); i += 4 {
		*(*uint32)(unsafe.Pointer(&out[i])) = atomic.LoadUint32((*uint32)(unsafe.Pointer(&a[i])))
	}
	return out
}

// Chunk is a 16 x height x 16 column.
type Chunk struct {
	Pos      ChunkPos
	Sections []*Section // index 0 = world MinY
	// Height is, per column (z<<4|x), one above the highest block that
	// filters sky light; everything at or above it sees the sky directly.
	Height [256]int32
	// Lit is set once the light engine has seeded this chunk (atomic: a
	// light pass sets it under the read lock).
	Lit atomic.Bool
	// Modified marks the chunk for saving.
	Modified bool
	// Edited has bit i set when World.SetBlock changed Sections[i]: what a
	// delta save stores (at most 64 sections).
	Edited uint64
	// Signs holds sign text by absolute block position (read-only: not saved).
	Signs map[BlockPos][4]string
	// Biomes holds legacy (pre-1.13) biome ids per column (z<<4|x), nil if
	// the chunk had none.
	Biomes []byte
	// version changes with a live chunk's blocks, light and height map
	// (World.SetBlock, light passes; not SetRaw while generating), so
	// readers can skip a chunk unchanged since they last read it. Unique
	// across chunks; atomic: light passes set it under the read lock, after
	// writing.
	version uint64
	minY    int
	// tickMask has bit i set when Sections[i] holds a state of tickFlags
	// (nil before the first Ticking); SetRaw keeps both up to date.
	tickMask  uint64
	tickFlags *TickFlags
	// tickNew is set when the chunk holds states past the end of tickFlags.
	tickNew bool
	// precip caches PrecipitationHeight per column (z<<4|x) as the height
	// - minY + 1, 0 unknown; SetRaw and SetSection forget it.
	precip atomic.Pointer[[256]atomic.Int32]
}

// TickFlags marks a set of states the chunks keep a count of, such as the
// randomly ticking ones (NewTickFlags).
type TickFlags struct {
	f    []bool
	line *TickFlags // the first table of a line of Grow
	root *TickFlags // the table of the line that last added marked states
}

// NewTickFlags makes a table of the states whose f[id] is true; f must
// not change afterwards (make a new table instead).
func NewTickFlags(f []bool) *TickFlags {
	t := &TickFlags{f: f}
	t.line, t.root = t, t
	return t
}

// Grow returns the table for f, which extends t's. The chunks' counts
// carry over, but for chunks holding added states when one of those is
// marked.
func (t *TickFlags) Grow(f []bool) *TickFlags {
	g := &TickFlags{f: f, line: t.line, root: t.root}
	for _, v := range f[len(t.f):] {
		if v {
			g.root = g
			break
		}
	}
	return g
}

func (t *TickFlags) has(id StateID) bool { return int(id) < len(t.f) && t.f[id] }

// counts reports whether id is marked or past the end of t (and so has to
// be looked at in a count).
func (t *TickFlags) counts(id StateID) bool { return int(id) >= len(t.f) || t.f[id] }

// Ticking returns the sections holding a state of t as a mask, bit i for
// Sections[i] (the reference's ExtendedBlockStorage.getNeedsRandomTick,
// tickRefCount > 0; the first 64 sections). The counts are made on the
// first call with t and kept by SetRaw after that. Call it under the
// world's read lock, from one goroutine.
func (c *Chunk) Ticking(t *TickFlags) uint64 {
	if c.tickFlags == t {
		return c.tickMask
	}
	if o := c.tickFlags; o != nil && o.line == t.line && (o.root == t.root || !c.tickNew) {
		c.tickFlags = t
		return c.tickMask
	}
	c.tickFlags, c.tickMask, c.tickNew = t, 0, false
	var tmp [SectionVolume]StateID
	for i, s := range c.Sections {
		if s == nil {
			continue
		}
		s.ticking, s.tickBits = 0, nil
		if s.nonAir == 0 {
			// all air
		} else if s.Blocks.Bits() == 0 {
			v := s.Blocks.Get(0)
			c.tickNew = c.tickNew || int(v) >= len(t.f)
			if t.has(v) {
				s.ticking = SectionVolume
				s.tickBits = new([SectionVolume / 64]uint64)
				for j := range s.tickBits {
					s.tickBits[j] = ^uint64(0)
				}
			}
		} else if s.Blocks.MayContain(t.counts) {
			s.Blocks.Unpack(&tmp)
			for j, v := range tmp {
				c.tickNew = c.tickNew || int(v) >= len(t.f)
				if t.has(v) {
					if s.tickBits == nil {
						s.tickBits = new([SectionVolume / 64]uint64)
					}
					s.ticking++
					s.tickBits[j>>6] |= 1 << uint(j&63)
				}
			}
		}
		if s.ticking > 0 && i < 64 {
			c.tickMask |= 1 << uint(i)
		}
	}
	return c.tickMask
}

// SetSection replaces Sections[i] (the next Ticking counts afresh).
func (c *Chunk) SetSection(i int, s *Section) {
	c.Sections[i], c.tickFlags = s, nil
	c.precip.Store(nil)
}

// TickingAt reports whether the state at SectionIndex i of a section
// in the last Ticking's mask is one of its states.
func (s *Section) TickingAt(i int) bool {
	return s.tickBits != nil && s.tickBits[i>>6]&(1<<uint(i&63)) != 0
}

var versions atomic.Uint64

// Changed gives the chunk a new version after a write.
func (c *Chunk) Changed() { atomic.StoreUint64(&c.version, versions.Add(1)) }

// Version identifies the chunk's contents; read it before reading them.
func (c *Chunk) Version() uint64 { return atomic.LoadUint64(&c.version) }

func newChunk(pos ChunkPos, minY, height int) *Chunk {
	c := &Chunk{Pos: pos, Sections: make([]*Section, height/16), minY: minY, version: versions.Add(1)}
	for i := range c.Sections {
		c.Sections[i] = NewSection()
	}
	for i := range c.Height {
		c.Height[i] = int32(minY)
	}
	return c
}

// Section returns the section containing absolute block y, or nil.
func (c *Chunk) Section(y int) *Section {
	i := (y - c.minY) >> 4
	if i < 0 || i >= len(c.Sections) {
		return nil
	}
	return c.Sections[i]
}

// Get returns the state at chunk-local x,z and absolute y.
func (c *Chunk) Get(x, y, z int) StateID {
	s := c.Section(y)
	if s == nil {
		return Air
	}
	return s.Blocks.Get(SectionIndex(x, y, z))
}

// setRaw stores a state without light/heightmap upkeep (generation, loading).
func (c *Chunk) SetRaw(x, y, z int, id StateID) {
	s := c.Section(y)
	if s == nil {
		return
	}
	j := SectionIndex(x, y, z)
	old := s.Blocks.Set(j, id)
	if p := c.precip.Load(); p != nil {
		p[(z&15)<<4|x&15].Store(0)
	}
	if old == Air && id != Air {
		s.nonAir++
	} else if old != Air && id == Air {
		s.nonAir--
	}
	if t := c.tickFlags; t != nil {
		c.tickNew = c.tickNew || int(id) >= len(t.f)
		if t.has(old) {
			s.ticking--
			s.tickBits[j>>6] &^= 1 << uint(j&63)
		}
		if t.has(id) {
			if s.tickBits == nil {
				s.tickBits = new([SectionVolume / 64]uint64)
			}
			s.ticking++
			s.tickBits[j>>6] |= 1 << uint(j&63)
		}
		if i := uint(y-c.minY) >> 4; i < 64 {
			if s.ticking > 0 {
				c.tickMask |= 1 << i
			} else {
				c.tickMask &^= 1 << i
			}
		}
	}
}

// RecomputeHeights rebuilds the sky heightmap from blocks.
func (c *Chunk) RecomputeHeights(t []StateInfo) {
	top := c.minY + len(c.Sections)*16
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			h := c.minY
			for y := top - 1; y >= c.minY; y-- {
				s := c.Section(y)
				if s.IsEmpty() {
					y = (y &^ 15) // skip to section bottom
					continue
				}
				if t[s.Blocks.Get(SectionIndex(x, y, z))].Opacity > 0 {
					h = y + 1
					break
				}
			}
			c.Height[z<<4|x] = int32(h)
		}
	}
	c.Changed()
}

// MinY returns the lowest block y of the column.
func (c *Chunk) MinY() int { return c.minY }
