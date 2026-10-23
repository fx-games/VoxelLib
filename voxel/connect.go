package voxel

import "sync/atomic"

// Connect gives every face of a block the texture its eight neighbours in
// the face's own plane pick (connected textures, as LOTR's
// LOTRConnectedTextures or OptiFine's CTM). The mesher builds the mask
// (see ConnectOffsets) and draws Icons[Mask[mask]].
type Connect struct {
	// Tag: a neighbour carrying it connects.
	Tag string
	// Plane, when 0..2, is an axis no connection crosses: a neighbour
	// offset along it never connects (a thin gate's edge faces). -1: none.
	Plane int8
	// Below, when set, makes a neighbour one block lower connect unless it
	// carries Below (LOTR's open gates standing on the ground).
	Below string
	// Icons are texture names; Mask[m] indexes them for neighbour mask m.
	Icons []string
	Mask  [256]uint8
}

// ConnectInfo is a state's baked Connect. Icons are interned on first use,
// so a block with many icons adds texture layers only for the ones drawn.
type ConnectInfo struct {
	Connect
	tag, below uint16
	ids        []atomic.Uint32 // texture id + 1, 0: not interned yet
	reg        *Registry
}

// ConnectOffsets are, per face, the world offsets of the eight in-plane
// neighbours for mask bits 0..7: top-left, top, top-right, left, right,
// bottom-left, bottom, bottom-right, where left/right run along the
// texture's u and top/bottom along its v as Minecraft 1.7's RenderBlocks
// draws the face (LOTRConnectedTextures.getConnectedIconBlock's side
// mapping: down/up (x, z), north (-x, -y), south (x, -y), west (z, -y),
// east (-z, -y)).
var ConnectOffsets = func() (o [FaceCount][8][3]int) {
	for f := Face(0); f < FaceCount; f++ {
		k := 0
		for y := -1; y <= 1; y++ {
			for x := -1; x <= 1; x++ {
				if x == 0 && y == 0 {
					continue
				}
				var d [3]int
				switch f {
				case FaceDown, FaceUp:
					d = [3]int{x, 0, y}
				case FaceNorth:
					d = [3]int{-x, -y, 0}
				case FaceSouth:
					d = [3]int{x, -y, 0}
				case FaceWest:
					d = [3]int{0, -y, x}
				case FaceEast:
					d = [3]int{0, -y, -x}
				}
				o[f][k] = d
				k++
			}
		}
	}
	return
}()

// Joins reports whether neighbour o at offset d connects.
func (c *ConnectInfo) Joins(d [3]int, o *StateInfo) bool {
	if c.Plane >= 0 && d[c.Plane] != 0 {
		return false
	}
	if c.Below != "" && d[1] == -1 && !o.hasTag(c.below) {
		return true
	}
	return o.hasTag(c.tag)
}

// Tex is the texture of face f with the neighbours nb (by offset).
func (c *ConnectInfo) Tex(f Face, nb Nb) uint16 {
	var m uint8
	for k, d := range ConnectOffsets[f] {
		if c.Joins(d, nb(d)) {
			m |= 1 << k
		}
	}
	return c.Icon(m)
}

// Icon is the texture for neighbour mask m.
func (c *ConnectInfo) Icon(m uint8) uint16 {
	i := c.Mask[m]
	if int(i) >= len(c.Icons) {
		return 0
	}
	if id := c.ids[i].Load(); id != 0 {
		return uint16(id - 1)
	}
	id := c.reg.Texture(c.Icons[i])
	c.ids[i].Store(uint32(id) + 1)
	return id
}

// bakeConnect interns c's tags; caller holds mu.
func (r *Registry) bakeConnect(c *Connect) *ConnectInfo {
	ci := &ConnectInfo{Connect: *c, tag: r.tagID(c.Tag), reg: r, ids: make([]atomic.Uint32, len(c.Icons))}
	if c.Below != "" {
		ci.below = r.tagID(c.Below)
	}
	return ci
}
