package voxel

// Light engine: two 4-bit channels per block like Minecraft.
//
//   - Sky light starts at 15 above the heightmap and travels straight down
//     without loss through blocks of opacity 0; every other step costs
//     max(1, opacity).
//   - Block light starts at a block's emission and loses max(1, opacity) per step.
//
// Initial lighting fills the open sky directly (no BFS through air above the
// terrain) and only flood-fills from the heightmap "edges", which is what keeps
// it cheap. Edits use the classic two-queue removal / re-propagation update.

type lightNode struct {
	x, y, z int32
	l       uint8
}

func (w *World) lightAt(x, y, z int, sky bool) (uint8, bool) {
	if y >= w.MaxY() {
		if sky {
			return 15, true
		}
		return 0, true
	}
	if y < w.MinY {
		return 0, false
	}
	c := w.chunkAt(x, z)
	if c == nil {
		return 0, false
	}
	s := c.Sections[(y-w.MinY)>>4]
	i := SectionIndex(x, y, z)
	if sky {
		return getNibble(s.Sky, i), true
	}
	return getNibble(s.Block, i), true
}

func (w *World) setLight(x, y, z int, sky bool, v uint8) {
	c := w.chunkAt(x, z)
	if c == nil || y < w.MinY || y >= w.MaxY() {
		return
	}
	s := c.Sections[(y-w.MinY)>>4]
	i := SectionIndex(x, y, z)
	if sky {
		setNibble(s.Sky, i, v)
	} else {
		setNibble(s.Block, i, v)
	}
	if c.Lit {
		w.markLightDirty(x, y, z)
	}
}

// markLightDirty flags the section of a changed light value plus the
// neighbours that sample it for smooth lighting when it is on their border.
func (w *World) markLightDirty(x, y, z int) {
	w.markDirty(SectionPos{x >> 4, y >> 4, z >> 4})
	lx, ly, lz := x&15, y&15, z&15
	if lx == 0 {
		w.markDirty(SectionPos{(x - 1) >> 4, y >> 4, z >> 4})
	} else if lx == 15 {
		w.markDirty(SectionPos{(x + 1) >> 4, y >> 4, z >> 4})
	}
	if ly == 0 {
		w.markDirty(SectionPos{x >> 4, (y - 1) >> 4, z >> 4})
	} else if ly == 15 {
		w.markDirty(SectionPos{x >> 4, (y + 1) >> 4, z >> 4})
	}
	if lz == 0 {
		w.markDirty(SectionPos{x >> 4, y >> 4, (z - 1) >> 4})
	} else if lz == 15 {
		w.markDirty(SectionPos{x >> 4, y >> 4, (z + 1) >> 4})
	}
}

func (w *World) stateAt(x, y, z int) StateID {
	c := w.chunkAt(x, z)
	if c == nil {
		return Air
	}
	return c.Get(x&15, y, z&15)
}

// propagate runs the add queue for one channel.
func (w *World) propagate(sky bool, t []StateInfo) {
	q := w.lightQ
	for len(q) > 0 {
		n := q[len(q)-1]
		q = q[:len(q)-1]
		x, y, z := int(n.x), int(n.y), int(n.z)
		lvl, ok := w.lightAt(x, y, z, sky)
		if !ok || lvl <= 1 {
			continue
		}
		for f := Face(0); f < FaceCount; f++ {
			d := faceNormals[f]
			nx, ny, nz := x+d.X, y+d.Y, z+d.Z
			if ny < w.MinY || ny >= w.MaxY() {
				continue
			}
			cur, ok := w.lightAt(nx, ny, nz, sky)
			if !ok || cur >= lvl {
				continue
			}
			op := t[w.stateAt(nx, ny, nz)].Opacity
			var nl uint8
			if sky && f == FaceDown && lvl == 15 && op == 0 {
				nl = 15
			} else {
				loss := op
				if loss < 1 {
					loss = 1
				}
				if loss >= lvl {
					continue
				}
				nl = lvl - loss
			}
			if nl > cur {
				w.setLight(nx, ny, nz, sky, nl)
				q = append(q, lightNode{int32(nx), int32(ny), int32(nz), nl})
			}
		}
	}
	w.lightQ = q[:0]
}

// unpropagate runs the removal queue, collecting re-light seeds into lightQ.
func (w *World) unpropagate(sky bool, t []StateInfo) {
	q := w.removeQ
	for len(q) > 0 {
		n := q[len(q)-1]
		q = q[:len(q)-1]
		x, y, z := int(n.x), int(n.y), int(n.z)
		for f := Face(0); f < FaceCount; f++ {
			d := faceNormals[f]
			nx, ny, nz := x+d.X, y+d.Y, z+d.Z
			if ny < w.MinY || ny >= w.MaxY() {
				continue
			}
			nl, ok := w.lightAt(nx, ny, nz, sky)
			if !ok || nl == 0 {
				continue
			}
			if nl < n.l || (sky && f == FaceDown && n.l == 15 && nl == 15) {
				w.setLight(nx, ny, nz, sky, 0)
				q = append(q, lightNode{int32(nx), int32(ny), int32(nz), nl})
				if !sky {
					if e := t[w.stateAt(nx, ny, nz)].Emission; e > 0 {
						w.setLight(nx, ny, nz, false, e)
						w.lightQ = append(w.lightQ, lightNode{int32(nx), int32(ny), int32(nz), e})
					}
				}
			} else {
				w.lightQ = append(w.lightQ, lightNode{int32(nx), int32(ny), int32(nz), nl})
			}
		}
	}
	w.removeQ = q[:0]
}

// relight updates both channels after the block at x,y,z changed.
func (w *World) relight(x, y, z int, oldInfo, newInfo StateInfo, t []StateInfo) {
	for _, sky := range []bool{true, false} {
		lvl, _ := w.lightAt(x, y, z, sky)
		w.setLight(x, y, z, sky, 0)
		w.removeQ = append(w.removeQ[:0], lightNode{int32(x), int32(y), int32(z), lvl})
		w.lightQ = w.lightQ[:0]
		w.unpropagate(sky, t)
		if !sky && newInfo.Emission > 0 {
			w.setLight(x, y, z, false, newInfo.Emission)
			w.lightQ = append(w.lightQ, lightNode{int32(x), int32(y), int32(z), newInfo.Emission})
		}
		if newInfo.Opacity < 15 {
			if sky && y == w.MaxY()-1 && newInfo.Opacity == 0 {
				w.setLight(x, y, z, true, 15)
				w.lightQ = append(w.lightQ, lightNode{int32(x), int32(y), int32(z), 15})
			}
			// light flows back in from every neighbour
			for f := Face(0); f < FaceCount; f++ {
				d := faceNormals[f]
				if l, ok := w.lightAt(x+d.X, y+d.Y, z+d.Z, sky); ok && l > 0 && y+d.Y < w.MaxY() {
					w.lightQ = append(w.lightQ, lightNode{int32(x + d.X), int32(y + d.Y), int32(z + d.Z), l})
				}
			}
		}
		w.propagate(sky, t)
	}
}

// TryLightChunk lights a column if it and its 8 neighbours are loaded. It
// returns true when the column is (now) lit.
//
// A light pass takes milliseconds, so it holds only the read lock (plus
// lightMu, so passes don't overlap). It writes nothing but light nibbles,
// the Lit flag and the chunk lookup cache: block readers (physics,
// raycasts) never wait for it. Mesh snapshots taken meanwhile may see partial
// light; the sections are re-dirtied when the pass ends, so they are remeshed.
func (w *World) TryLightChunk(p ChunkPos) bool {
	w.lightMu.Lock()
	defer w.lightMu.Unlock()
	w.mu.RLock()
	defer w.mu.RUnlock()
	c := w.chunks[p]
	if c == nil {
		return false
	}
	if c.Lit {
		return true
	}
	if !w.neighbours8Loaded(p) {
		return false
	}
	w.lightChunk(c)
	return true
}

// MarkLit flags a column whose stored light (e.g. from an Anvil file) is
// trusted as-is.
func (w *World) MarkLit(p ChunkPos) {
	w.mu.Lock()
	if c := w.chunks[p]; c != nil {
		c.Lit = true
	}
	w.mu.Unlock()
}

func (w *World) lightChunk(c *Chunk) {
	t := w.Reg.Table()
	bx, bz := c.Pos.X<<4, c.Pos.Z<<4
	top := w.MaxY()

	// Sky: everything at/above the column height sees the sky.
	for i := range c.Sections {
		s := c.Sections[i]
		clear(s.Sky)
		clear(s.Block)
	}
	maxH := w.MinY
	for i, h := range c.Height {
		if int(h) > maxH {
			maxH = int(h)
		}
		x, z := i&15, i>>4
		for y := int(h); y < top; y++ {
			s := c.Sections[(y-w.MinY)>>4]
			setNibble(s.Sky, SectionIndex(x, y, z), 15)
		}
	}

	heightAt := func(x, z int) int {
		cc := w.chunkAt(x, z)
		if cc == nil {
			return top
		}
		return int(cc.Height[(z&15)<<4|(x&15)])
	}
	q := w.lightQ[:0]
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			wx, wz := bx+x, bz+z
			h := int(c.Height[z<<4|x])
			m := h
			for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				if nh := heightAt(wx+d[0], wz+d[1]); nh > m {
					m = nh
				}
			}
			if h >= top {
				continue
			}
			if m >= top {
				m = top - 1
			}
			for y := h; y <= m; y++ {
				q = append(q, lightNode{int32(wx), int32(y), int32(wz), 15})
			}
		}
	}
	// Light already in lit neighbours flows across the border.
	border := func(sky bool) {
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := w.chunks[c.Pos.Add(d[0], d[1])]
			if n == nil || !n.Lit {
				continue
			}
			for k := 0; k < 16; k++ {
				var x, z int
				switch {
				case d[0] == 1:
					x, z = bx+16, bz+k
				case d[0] == -1:
					x, z = bx-1, bz+k
				case d[1] == 1:
					x, z = bx+k, bz+16
				default:
					x, z = bx+k, bz-1
				}
				for y := w.MinY; y < top; y++ {
					if l, _ := w.lightAt(x, y, z, sky); l > 1 {
						q = append(q, lightNode{int32(x), int32(y), int32(z), l})
					}
				}
			}
		}
	}
	border(true)
	w.lightQ = q
	w.propagate(true, t)

	// Block light: seed emitters.
	q = w.lightQ[:0]
	var buf [SectionVolume]StateID
	for si, s := range c.Sections {
		if s.IsEmpty() {
			continue
		}
		s.Blocks.Unpack(&buf)
		for i, id := range buf {
			if e := t[id].Emission; e > 0 {
				x, y, z := i&15, i>>8, (i>>4)&15
				wy := w.MinY + si*16 + y
				setNibble(s.Block, i, e)
				q = append(q, lightNode{int32(bx + x), int32(wy), int32(bz + z), e})
			}
		}
	}
	border(false)
	w.lightQ = q
	w.propagate(false, t)

	c.Lit = true
	// Mark this column's and its neighbours' sections for meshing.
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			for sy := 0; sy < len(c.Sections); sy++ {
				w.markDirty(SectionPos{c.Pos.X + dx, w.MinSection() + sy, c.Pos.Z + dz})
			}
		}
	}
}
