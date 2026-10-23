// Package minimap is the reference's corner minimap: a square of the world
// around the player, coloured per block from texture averages with biome
// tints, slope and depth shading and light, turned with the view, framed,
// with entity dots, compass letters and the coordinates line, and a cave
// view below a roof. One wrapping texture holds the colours; a few chunks
// are rewritten per frame and the texture is uploaded when a colour changed.
//
// The surface view follows the reference's world-map pixels (the minimap
// draws those when the world map is present), the cave view the minimap's
// own writer. Java float32 and int truncation are kept: products that feed
// an addition are rounded explicitly so Go cannot fuse them.
package minimap

import (
	"math"

	"github.com/memmaker/voxellib/fmath"
	"github.com/memmaker/voxellib/voxel"
)

// Block tints (the reference's colour types).
const (
	TintNone    = 0
	TintGrass   = 1
	TintFoliage = 2
	TintCustom  = 3 // Block.Custom
)

// Overlay tints of a translucent block.
const (
	OverlayTintNone   = 0
	OverlayTintWater  = 1 // biome water colour
	OverlayTintCustom = 2 // Block.OverlayCustom
)

// Biome colour kinds for Source.BiomeColour.
const (
	BiomeGrass   = 0
	BiomeFoliage = 1
	BiomeWater   = 2
)

// Block is what the map reads of one block state.
type Block struct {
	Colour        uint32 // 0xRRGGBB: the top texture's sampled average, or the material colour
	Icon          int32  // top texture id: stacked translucent blocks with one icon are one layer
	Tint          uint8
	Custom        uint32
	OverlayTint   uint8
	OverlayCustom uint32
	Invisible     bool    // the surface view looks through it
	CaveSkip      bool    // the cave view looks through it
	Overlay       bool    // translucent: a layer over what lies below (liquids, ice, glass)
	Transparency  float32 // a layer's share: 0.66 liquids, 0.83 ice, 0.5 others
	Opacity       int     // light opacity: sky light a layer takes away
	Glowing       bool    // emits light: brightened and lit evenly
	Ore           bool    // never glowing in the cave view
	Roof          bool    // an opaque material: a cave roof
}

// Source is what the game supplies for the colours.
type Source struct {
	// Blocks by state id; states past the end are invisible.
	Blocks []Block
	// Biome is the biome id of a column.
	Biome func(x, z int) int
	// BiomeColour is the biome's grass, foliage or water colour (0xRRGGBB)
	// at a position.
	BiomeColour func(kind, biome, x, y, z int) uint32
	// BlockTint is a block's colour multiplier at a position (0xRRGGBB,
	// 0xFFFFFF none), as the world renderer tints it; the cave view uses it.
	BlockTint func(id voxel.StateID, x, y, z int) uint32
}

func (s *Source) block(id voxel.StateID) *Block {
	if int(id) < len(s.Blocks) {
		return &s.Blocks[id]
	}
	return &invisible
}

var invisible = Block{Invisible: true, CaveSkip: true}

// maxLayers is the reference's overlay limit per pixel.
const maxLayers = 10

// layer is one translucent run over a surface pixel.
type layer struct {
	state   voxel.StateID
	light   uint8
	opacity int16
}

// surfPixel is a world-map pixel: the first opaque block and the layers
// above it.
type surfPixel struct {
	state  voxel.StateID
	height int16
	light  uint8 // block light above the block
	biome  int16 // -1: no tinted block
	layers []layer
	// The cached slopes (MapBlock's verticalSlope, diagonalSlope and
	// !slopeUnknown): set when written, or when coloured if a neighbour
	// was missing then; cleared when a neighbour's edge changes.
	vs, ds     int8
	slopeKnown bool
}

// nibble reads a 4-bit light value of a section.
func nibble(a []byte, i int) uint8 {
	if a == nil {
		return 0
	}
	return a[i>>1] >> ((uint(i) & 1) << 2) & 15
}

func blockLight(c *voxel.Chunk, x, y, z int) uint8 {
	s := c.Section(y)
	if s == nil {
		return 0
	}
	return nibble(s.Block, voxel.SectionIndex(x, y, z))
}

func skyLight(c *voxel.Chunk, x, y, z int) uint8 {
	s := c.Section(y)
	if s == nil {
		return 15
	}
	return nibble(s.Sky, voxel.SectionIndex(x, y, z))
}

func topY(c *voxel.Chunk) int { return c.MinY() + len(c.Sections)*16 - 1 }

// loadSurface is MapWriter.loadPixel (not caving): from three above the
// height map down, through invisible blocks, collecting translucent layers
// until an opaque block.
func (s *Source) loadSurface(c *voxel.Chunk, x, z int, px *surfPixel) {
	top := topY(c)
	high := min(int(c.Height[z<<4|x])+3, top+1)
	low := max(c.MinY(), 0)
	px.layers = px.layers[:0]
	var state voxel.StateID
	var light uint8
	h := high
	for ; h >= low; h-- {
		id := c.Get(x, h, z)
		if id == voxel.Air {
			continue
		}
		b := s.block(id)
		if b.Invisible {
			continue
		}
		light = blockLight(c, x, min(h+1, top), z)
		if !b.Overlay {
			state = id
			break
		}
		// OverlayBuilder.build: a new layer when the icon changes.
		n := len(px.layers)
		if n == 0 || px.layers[n-1].state != id && s.block(px.layers[n-1].state).Icon != b.Icon && n < maxLayers {
			px.layers = append(px.layers, layer{state: id, light: light})
			n++
		}
		px.layers[n-1].opacity += int16(b.Opacity)
	}
	if h < low {
		h = low
		light = 0
	}
	px.state, px.height, px.light = state, int16(h), light
	px.biome = -1
	wx, wz := c.Pos.X<<4|x, c.Pos.Z<<4|z
	tinted := state != voxel.Air && (s.block(state).Tint == TintGrass || s.block(state).Tint == TintFoliage)
	for _, l := range px.layers {
		tinted = tinted || s.block(l.state).OverlayTint == OverlayTintWater
	}
	if tinted {
		px.biome = int16(s.Biome(wx, wz))
	}
}

// brightness is MapPixel.getBlockBrightness.
func brightness(min float32, l, sun int) float32 {
	return (min + float32(max(sun, l))) / (15 + min)
}

// tint is the colour-multiplier step: rgb scaled by c's channels.
func tint(r, g, b int, c uint32) (int, int, int) {
	rm, gm, bm := float32(r)/255, float32(g)/255, float32(b)/255
	return int(float32(c>>16&255) * rm), int(float32(c>>8&255) * gm), int(float32(c&255) * bm)
}

// glow is MinimapWriterHelper.getGlowingColour / MapPixel's brightener.
func glow(r, g, b int) (int, int, int) {
	total := r + g + b
	if total == 0 {
		return 0, 0, 0 // Java: 0 * Infinity = NaN, (int)NaN = 0
	}
	k := max(1, float32(407)/float32(total))
	return int(float32(r) * k), int(float32(g) * k), int(float32(b) * k)
}

// slopeLight is the terrainSlopes 2 light: ambient plus the direct light
// the slope facing north-west catches, per colour channel with the
// dimension's shadow colour.
func slopeLight(vertical, diagonal int, glowing bool, shadow [3]float32) [3]float32 {
	ambientColoured, ambientWhite, maxDirect := float32(0.2), float32(0.5), float32(0.6666667)
	if glowing {
		ambientColoured, ambientWhite, maxDirect = 0, 1, 0.22222224
	}
	var cos float32
	crossZ := float32(-vertical)
	if crossZ < 1 {
		if vertical == 1 && diagonal == 1 {
			cos = 1
		} else {
			crossX := float32(vertical - diagonal)
			cast := 1 - crossZ
			mag := float32(math.Sqrt(float64(float32(crossX*crossX) + 1 + float32(crossZ*crossZ))))
			cos = float32(float64(cast/mag) / math.Sqrt(2))
		}
	}
	var direct float32
	if cos == 1 {
		direct = maxDirect
	} else if cos > 0 {
		direct = float32(math.Ceil(float64(cos*10))) / 10 * maxDirect * 0.88388
	}
	white := ambientWhite + direct
	var out [3]float32
	for i := range out {
		out[i] = float32(shadow[i]*ambientColoured) + white
	}
	return out
}

// depth is the terrainDepth factor with slopes on: height/63 in 0.9..1.
func depth(height int) float32 {
	return min(max(float32(height)/63, 0.9), 1)
}

// surface is the store of written surface pixels the colours read their
// neighbours from: the height (-1 unknown) and the biome (-1 none or
// unknown) of a column.
type surface interface {
	pixel(wx, wz int) *surfPixel
}

func heightAt(s surface, wx, wz int) int {
	if p := s.pixel(wx, wz); p != nil {
		return int(p.height)
	}
	return -1
}

// biomeColour is MapWriter.getBiomeColor: the 3x3 average of the stored
// biomes' colours (-1 when none is known, which tints nothing).
func (src *Source) biomeColour(s surface, kind, wx, y, wz int) uint32 {
	var r, g, b, n int
	for o := -1; o <= 1; o++ {
		for p := -1; p <= 1; p++ {
			px := s.pixel(wx+o, wz+p)
			if px == nil || px.biome < 0 {
				continue
			}
			c := src.BiomeColour(kind, int(px.biome), wx+o, y, wz+p)
			r += int(c >> 16 & 255)
			g += int(c >> 8 & 255)
			b += int(c & 255)
			n++
		}
	}
	if n == 0 {
		return 0xFFFFFFFF
	}
	return uint32(r/n&255)<<16 | uint32(g/n&255)<<8 | uint32(b/n&255)
}

// fixSlopes is MapBlock.fixHeightType: px's height steps to the stored
// pixels north and north-west. When one is unknown, writing leaves the
// slopes unknown; otherwise those of the next pixel south-east in the chunk
// are taken (none at the chunk's corner).
func fixSlopes(s surface, wx, wz, height int, px *surfPixel, writing bool) {
	ph, pd := heightAt(s, wx, wz-1), heightAt(s, wx-1, wz-1)
	if ph == -1 || pd == -1 {
		if writing {
			return
		}
		rx, rz := wx, wz
		if wx&15 < 15 {
			rx++
		}
		if wz&15 < 15 {
			rz++
		}
		if rx == wx && rz == wz {
			px.vs, px.ds, px.slopeKnown = 0, 0, true
			return
		}
		fixSlopes(s, rx, rz, heightAt(s, rx, rz), px, false)
		return
	}
	clamp := func(v int) int8 { return int8(max(-128, min(127, v))) }
	px.vs, px.ds, px.slopeKnown = clamp(height-ph), clamp(height-pd), true
}

// surfaceColour is MapPixel.getPixelColours for a written pixel: its
// colour (0xRRGGBB) and light alpha (0..255; the map shows
// rgb * max(daylight, alpha)).
func (src *Source) surfaceColour(s surface, wx, wz int, px *surfPixel, shadow [3]float32) (rgb uint32, alpha uint8) {
	b := src.block(px.state)
	var r, g, bl int
	if px.state != voxel.Air {
		r, g, bl = int(b.Colour>>16&255), int(b.Colour>>8&255), int(b.Colour&255)
	}
	height := int(px.height)
	if px.state != voxel.Air && b.Tint != TintNone {
		c := b.Custom
		switch b.Tint {
		case TintGrass:
			c = src.biomeColour(s, BiomeGrass, wx, height, wz)
		case TintFoliage:
			c = src.biomeColour(s, BiomeFoliage, wx, height, wz)
		}
		r, g, bl = tint(r, g, bl, c)
	}
	glowing := px.state != voxel.Air && b.Glowing
	topLight := int(px.light)
	if glowing {
		r, g, bl = glow(r, g, bl)
		topLight = 15
	}
	bright := [3]float32{1, 1, 1}
	var or, og, ob int
	mult := float32(1)
	if len(px.layers) > 0 {
		sun := 15
		for i, l := range px.layers {
			lb := src.block(l.state)
			cr, cg, cb := int(lb.Colour>>16&255), int(lb.Colour>>8&255), int(lb.Colour&255)
			switch lb.OverlayTint {
			case OverlayTintWater:
				cr, cg, cb = tint(cr, cg, cb, src.biomeColour(s, BiomeWater, wx, -1, wz))
			case OverlayTintCustom:
				cr, cg, cb = tint(cr, cg, cb, lb.OverlayCustom)
			}
			if lb.Glowing {
				cr, cg, cb = glow(cr, cg, cb)
			}
			if i == 0 {
				topLight = int(l.light)
			}
			k := brightness(9, int(l.light), sun) * lb.Transparency * mult
			or = int(float32(or) + float32(float32(cr)*k))
			og = int(float32(og) + float32(float32(cg)*k))
			ob = int(float32(ob) + float32(float32(cb)*k))
			sun = max(sun-int(l.opacity), 0)
			mult *= 1 - lb.Transparency
		}
		if !glowing {
			v := brightness(9, int(px.light), sun)
			bright = [3]float32{v, v, v}
		}
	}
	d := float32(1)
	if !glowing && height != -1 {
		d = depth(height)
	}
	if !px.slopeKnown {
		if px.state != voxel.Air {
			fixSlopes(s, wx, wz, height, px, false)
		} else {
			px.vs, px.ds, px.slopeKnown = 0, 0, true
		}
	}
	sl := slopeLight(int(px.vs), int(px.ds), glowing, shadow)
	for i := range bright {
		bright[i] = bright[i] * sl[i] * d
	}
	out := func(c int, k float32, o int) uint32 {
		return uint32(min(int(float32(float32(float32(c)*k)*mult)+float32(o)), 255))
	}
	rgb = out(r, bright[0], or)<<16 | out(g, bright[1], og)<<8 | out(bl, bright[2], ob)
	if topLight != 0 {
		alpha = uint8(int(brightness(9, topLight, 0) * 255))
	}
	return rgb, alpha
}

// daylight is the minimap brightness of world-map pixels:
// 0.375 + 0.625 * the clamped sun brightness factor.
func daylight(sun float32) float32 {
	return 0.375 + 0.625*min(max(sun, 0), 1)
}

// shade is the texture combiner of the world-map minimap: rgb times
// max(daylight, alpha).
func shade(rgb uint32, alpha uint8, day float32) uint32 {
	k := max(day, float32(alpha)/255)
	ch := func(c uint32) uint32 { return uint32(math.Round(float64(float32(c&255) * k))) }
	return ch(rgb>>16)<<16 | ch(rgb>>8)<<8 | ch(rgb)
}

// SunBrightness is World.getSunBrightnessFactor: celestial angle a
// (0..1), rain and thunder strengths.
func SunBrightness(a, rain, thunder float32) float32 {
	f := 1 - (float32(fmath.Cos(float32(a*float32(math.Pi))*2)*2) + 0.5)
	f = min(max(f, 0), 1)
	f = 1 - f
	f = float32(float64(f) * (1 - float64(rain*5)/16))
	f = float32(float64(f) * (1 - float64(thunder*5)/16))
	return f
}

// Levels is the number of daylight levels the cave view keeps per pixel
// with lighting on (one without).
const Levels = 5

// cavePixel is a cave-view pixel: the colour per daylight level and the
// height of the block shown.
type cavePixel struct {
	rgb    [Levels]uint32
	height int16
}

// caveBrightness is MinimapWriter.getBlockBrightness with levels levels.
func caveBrightness(levels int, min float32, sun, level, blockLight int) float32 {
	if levels == 1 {
		return (min + float32(sun)) / (15 + min)
	}
	k := float32(1)
	if level != -1 && level != 0 {
		k = (float32(levels) - 1 - float32(level)) / (float32(levels) - 1)
	}
	return (min + max(float32(k*float32(sun)), float32(blockLight))) / (15 + min)
}

// caveHeights is what loadCave reads of a neighbour column: when its chunk
// is stored (stored), the height there and whether it was written for this
// cave level (same); else the world's height map there (-1 not loaded).
type caveHeights func(wx, wz int) (h int, stored, same bool)

// loadCave is MinimapWriter.loadBlockColor below a cave roof at caving:
// down to depth below the player through air first, translucent layers,
// then the first block, coloured per daylight level.
func (src *Source) loadCave(c *voxel.Chunk, x, z, caving, playerY, depthBlocks, levels, slopeMode int, shadow [3]float32, heights caveHeights, px *cavePixel) (success bool) {
	top := topY(c)
	low := max(playerY-depthBlocks, 0, c.MinY())
	wx, wz := c.Pos.X<<4|x, c.Pos.Z<<4|z
	var states []voxel.StateID
	var lights, sizes []int
	underair := false
	prevTransparent := voxel.StateID(0xFFFF)
	found := false
	glowing := false
	blockY, firstY := 0, 0
	sun := 15
	for i := min(caving, top); i >= low; i-- {
		id := c.Get(x, i, z)
		if id == voxel.Air {
			underair = true
			continue
		}
		if !underair {
			continue
		}
		b := src.block(id)
		if b.CaveSkip {
			continue
		}
		blockY = i
		ly := min(top, i+1)
		if len(states) == 0 {
			firstY = i
			if levels != 1 {
				sun = int(skyLight(c, x, ly, z))
			}
		}
		bl := 0
		if levels != 1 {
			bl = int(blockLight(c, x, ly, z))
		}
		if id != prevTransparent && !b.Overlay {
			lights = append(lights, bl)
			states = append(states, id)
			glowing = b.Glowing
			found = true
			break
		}
		if len(states) < 5 && id != prevTransparent {
			states = append(states, id)
			sizes = append(sizes, 1)
			lights = append(lights, bl)
			prevTransparent = id
		} else {
			sizes[len(sizes)-1]++
		}
	}

	// The heights north and north-west: stored ones, else the world's
	// height map under no layers, else flat; the tile is written again
	// (success false) until both are stored for this cave level.
	success = true
	prev := func(qx, qz int) int {
		h, stored, same := heights(qx, qz)
		if stored {
			success = success && same
			return h
		}
		success = false
		if len(sizes) == 0 && h != -1 {
			return h
		}
		return blockY
	}
	ph, pd := prev(wx, wz-1), prev(wx-1, wz-1)
	vs, ds := 0, 0
	if slopeMode > 0 {
		if ph != -1 {
			vs = max(-128, min(127, blockY-ph))
		}
		if pd != -1 {
			ds = max(-128, min(127, blockY-pd))
		}
	}

	var under [Levels][3]int
	mult := float32(1)
	firstSun := sun
	transparent := len(sizes) > 0
	if transparent && firstSun != 15 {
		sun = 15
	}
	// calculateBlockColors
	y := firstY
	colourAt := func(id voxel.StateID, y int) (int, int, int) {
		b := src.block(id)
		r, g, bl := int(b.Colour>>16&255), int(b.Colour>>8&255), int(b.Colour&255)
		if t := src.BlockTint(id, wx, y, wz); t&0xFFFFFF != 0xFFFFFF {
			r, g, bl = tint(r, g, bl, t)
		}
		return r, g, bl
	}
	for i, size := range sizes {
		b := src.block(states[i])
		r, g, bl := colourAt(states[i], y)
		if b.Glowing {
			r, g, bl = glow(r, g, bl)
		}
		k := float32(mult*b.Transparency) * caveBrightness(levels, 9, sun, 0, lights[i])
		for l := 0; l < levels; l++ {
			under[l][0] = int(float32(under[l][0]) + float32(float32(r)*k))
			under[l][1] = int(float32(under[l][1]) + float32(float32(g)*k))
			under[l][2] = int(float32(under[l][2]) + float32(float32(bl)*k))
		}
		mult *= 1 - b.Transparency
		sun = max(sun-b.Opacity*size, 0)
		y -= size
	}
	var cr, cg, cb int
	if len(states) > 0 {
		cr, cg, cb = colourAt(states[len(states)-1], y)
	}
	glowing = found && !src.block(states[len(states)-1]).Ore && glowing

	var bright, post [Levels]float32
	if !glowing || transparent {
		bl := 0
		if len(lights) > 0 {
			bl = lights[len(lights)-1]
		}
		for l := 0; l < levels; l++ {
			post[l] = 1
			if !glowing {
				if transparent {
					bright[l] = caveBrightness(levels, 9, sun, 0, bl)
				} else {
					bright[l] = caveBrightness(levels, 9, sun, l, bl)
				}
			}
			if transparent {
				post[l] = caveBrightness(levels, 9, firstSun, l, lights[0])
			}
		}
	}
	if glowing {
		cr, cg, cb = glow(cr, cg, cb)
	}
	sec := [3]float64{1, 1, 1}
	if slopeMode == 2 {
		sl := slopeLight(vs, ds, glowing, shadow)
		for i := range sec {
			sec[i] *= float64(sl[i])
		}
	} else if slopeMode == 1 && !glowing {
		if vs > 0 {
			sec = [3]float64{1.15, 1.15, 1.15}
		} else if vs < 0 {
			sec = [3]float64{0.85, 0.85, 0.85}
		}
	}
	for l := 0; l < levels; l++ {
		b := bright[l]
		if glowing {
			b = 1
			if !transparent {
				post[l] = 1
			}
		}
		ch := func(c int, s float64, u int) uint32 {
			return uint32(min(int((float64(float32(c)*b)*s*float64(mult)+float64(u))*float64(post[l])), 255))
		}
		px.rgb[l] = ch(cr, sec[0], under[l][0])<<16 | ch(cg, sec[1], under[l][1])<<8 | ch(cb, sec[2], under[l][2])
	}
	px.height = int16(blockY)
	return success
}

// Caving is MinimapWriter.getCaving: the cave roof level above a player at
// x, y, z, or -1 under the open sky. roof is caveMaps-1 (the roof's radius).
func (src *Source) Caving(w *voxel.World, x, y, z, roofRadius int) int {
	y = max(y+1, 0)
	if y > 255 {
		return -1
	}
	w.RLock()
	defer w.RUnlock()
	c := w.ChunkUnlocked(voxel.ChunkPos{X: x >> 4, Z: z >> 4})
	if c == nil {
		return -1
	}
	if skyLight(c, x&15, y, z&15) >= 15 {
		return -1
	}
	top := min(int(c.Height[(z&15)<<4|x&15]), 255)
	if top < 0 {
		return -1
	}
next:
	for i := y; i <= top; i++ {
		for o := x - roofRadius; o <= x+roofRadius; o++ {
			for p := z - roofRadius; p <= z+roofRadius; p++ {
				if !src.block(w.BlockUnlocked(o, i, p)).Roof {
					continue next
				}
			}
		}
		return min(i, y+3)
	}
	return -1
}
