package voxel

// Section is a 16^3 cube of a chunk column.
type Section struct {
	Blocks *PalettedContainer
	// Light nibbles, 2048 bytes each in Anvil order (low nibble = even index).
	Sky   []byte
	Block []byte
	// nonAir counts non-air states; zero means the section can be skipped by
	// the mesher and the renderer entirely (the reference's hasOnlyAir()).
	nonAir int
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

func getNibble(a []byte, i int) uint8 {
	return (a[i>>1] >> ((uint(i) & 1) << 2)) & 15
}

func setNibble(a []byte, i int, v uint8) {
	sh := (uint(i) & 1) << 2
	a[i>>1] = a[i>>1]&^(15<<sh) | (v&15)<<sh
}

// Chunk is a 16 x height x 16 column.
type Chunk struct {
	Pos      ChunkPos
	Sections []*Section // index 0 = world MinY
	// Height is, per column (z<<4|x), one above the highest block that
	// filters sky light; everything at or above it sees the sky directly.
	Height [256]int32
	// Lit is set once the light engine has seeded this chunk.
	Lit bool
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
	minY   int
}

func newChunk(pos ChunkPos, minY, height int) *Chunk {
	c := &Chunk{Pos: pos, Sections: make([]*Section, height/16), minY: minY}
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
	old := s.Blocks.Set(SectionIndex(x, y, z), id)
	if old == Air && id != Air {
		s.nonAir++
	} else if old != Air && id == Air {
		s.nonAir--
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
}

// MinY returns the lowest block y of the column.
func (c *Chunk) MinY() int { return c.minY }
