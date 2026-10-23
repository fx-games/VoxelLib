package voxel

// Dynamic lights (AtomicStryker's Dynamic Lights): a light source attached
// to an entity makes the block it stands in emit Level, as if that block
// were a light-emitting block. The mod patches World.computeLightValue's
// emission and relights the old and new block with updateLightByType, so the
// light spreads exactly like block light: each step loses max(1, opacity)
// of the block entered. Here the spread is kept apart from the stored light
// (an overlay); readers that draw take max(stored block light, overlay):
// the snapshot the mesher reads and RenderLight.

// DynLight is one dynamic light source: the block X, Y, Z emits Level.
type DynLight struct {
	X, Y, Z int
	Level   uint8
}

// DynField is the block light the dynamic lights spread, per section
// (SectionIndex order). Sections without any are absent. Never modified
// once published.
type DynField map[SectionPos]*[SectionVolume]uint8

// spread computes the field of srcs: a BFS over the blocks like propagate
// (the cell entered loses max(1, opacity)). Caller holds the read lock.
func (w *World) spread(srcs []DynLight) DynField {
	f := DynField{}
	if len(srcs) == 0 {
		return f
	}
	t := w.Reg.Table()
	var cp ChunkPos
	var cc *Chunk
	state := func(x, y, z int) StateID {
		if p := (ChunkPos{x >> 4, z >> 4}); cc == nil || p != cp {
			cp, cc = p, w.chunks[p]
		}
		if cc == nil {
			return Air
		}
		return cc.Get(x&15, y, z&15)
	}
	var sp SectionPos
	var sa *[SectionVolume]uint8
	cell := func(x, y, z int) *uint8 {
		if p := (SectionPos{x >> 4, y >> 4, z >> 4}); sa == nil || p != sp {
			sp, sa = p, f[p]
			if sa == nil {
				sa = new([SectionVolume]uint8)
				f[p] = sa
			}
		}
		return &sa[SectionIndex(x, y, z)]
	}
	var q []lightNode
	for _, s := range srcs {
		if s.Level == 0 || s.Y < w.MinY || s.Y >= w.MaxY() {
			continue
		}
		if c := cell(s.X, s.Y, s.Z); *c < s.Level {
			*c = s.Level
			q = append(q, lightNode{int32(s.X), int32(s.Y), int32(s.Z), s.Level})
		}
	}
	for len(q) > 0 {
		n := q[len(q)-1]
		q = q[:len(q)-1]
		x, y, z := int(n.x), int(n.y), int(n.z)
		if *cell(x, y, z) != n.l { // raised since queued: that entry spreads
			continue
		}
		for fc := Face(0); fc < FaceCount; fc++ {
			d := faceNormals[fc]
			nx, ny, nz := x+d.X, y+d.Y, z+d.Z
			if ny < w.MinY || ny >= w.MaxY() {
				continue
			}
			loss := max(t[state(nx, ny, nz)].Opacity, 1)
			if loss >= n.l {
				continue
			}
			nl := n.l - loss
			if c := cell(nx, ny, nz); *c < nl {
				*c = nl
				q = append(q, lightNode{int32(nx), int32(ny), int32(nz), nl})
			}
		}
	}
	return f
}

// SetDynLights replaces the dynamic light sources and returns the sections
// whose mesh changes: every section whose padded 18³ snapshot holds a block
// whose dynamic light changed. It takes the read lock.
func (w *World) SetDynLights(srcs []DynLight) []SectionPos {
	w.mu.RLock()
	nf := w.spread(srcs)
	w.mu.RUnlock()
	var of DynField
	if p := w.dyn.Swap(&nf); p != nil {
		of = *p
	}
	dirty := map[SectionPos]struct{}{}
	var zero [SectionVolume]uint8
	diff := func(p SectionPos, a, b *[SectionVolume]uint8) {
		if a == nil {
			a = &zero
		}
		if b == nil {
			b = &zero
		}
		if *a == *b {
			return
		}
		for i := range a {
			if a[i] == b[i] {
				continue
			}
			lx, ly, lz := i&15, i>>8, i>>4&15
			for dy := -1; dy <= 1; dy++ {
				if dy == -1 && ly != 0 || dy == 1 && ly != 15 {
					continue
				}
				for dz := -1; dz <= 1; dz++ {
					if dz == -1 && lz != 0 || dz == 1 && lz != 15 {
						continue
					}
					for dx := -1; dx <= 1; dx++ {
						if dx == -1 && lx != 0 || dx == 1 && lx != 15 {
							continue
						}
						dirty[SectionPos{p.X + dx, p.Y + dy, p.Z + dz}] = struct{}{}
					}
				}
			}
		}
	}
	for p, a := range of {
		diff(p, a, nf[p])
	}
	for p, b := range nf {
		if of[p] == nil {
			diff(p, nil, b)
		}
	}
	out := make([]SectionPos, 0, len(dirty))
	for p := range dirty {
		out = append(out, p)
	}
	return out
}

// DynLights returns the published dynamic light field (nil: none).
func (w *World) DynLights() DynField {
	if p := w.dyn.Load(); p != nil {
		return *p
	}
	return nil
}

// RenderLight is Light with the dynamic lights: the block light is the
// brighter of the stored light and the dynamic field, as the client world
// of the mod reads it (entity, hand and particle brightness).
func (w *World) RenderLight(x, y, z int) (sky, block uint8) {
	sky, block = w.Light(x, y, z)
	if a := w.DynLights()[SectionPos{x >> 4, y >> 4, z >> 4}]; a != nil {
		block = max(block, a[SectionIndex(x, y, z)])
	}
	return
}
