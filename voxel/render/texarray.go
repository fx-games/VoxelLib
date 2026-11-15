package render

import (
	"fmt"
	"image"
	"image/color"
	"math/bits"

	"github.com/memmaker/voxellib/voxel"
)

// TextureSource supplies block textures by resource name ("block/stone").
// *respack.Pack implements it.
type TextureSource interface {
	Texture(name string) (image.Image, error)
}

// AnimatedTextureSource is optionally implemented by a TextureSource whose
// textures may be animated (reference .png.mcmeta strips). Frame 0 is what
// Texture returns. *respack.Pack implements it.
type AnimatedTextureSource interface {
	TextureFrames(name string) (frames []image.Image, ticksPerFrame int, err error)
}

// textureArray holds the block textures as tiles of a 2D array texture,
// each at its own size, like the reference's atlas sprites: a 16 px
// texture takes a 16 px tile however large other textures (64 px entity
// sheets, hi-res packs) are. Tiles never bleed (nearest texels within a
// tile, repeat by fract: see wgslBlockTex). The texture table (registry
// TextureEntry: a texture or a region of one) maps to tiles through the
// layer map (one RGBA32Uint texel per entry: tile code, region rect), which
// also points animated textures at their current frame. Entries learned
// later are appended (see add) without re-reading the others.
type textureArray struct {
	gpu       gpuTexArray
	layerSize int // array layer side, a power of two
	levels    int // highest mip level
	entries   []voxel.TextureEntry
	texOf     map[string]int // texture name -> texs index
	texs      []texImage
	tintable  []bool // per entry
	cut       []bool // per entry: a frame has a texel the 0.1 alpha test discards
	slots     []slot
	open      map[int]openLayer // per tile size: the layer being filled
	layers    int               // array layers in use
	capLayers int               // GPU capacity (array layers)
	capMap    int               // GPU capacity (layer map width)
	gen       int               // bumped when the GPU textures are reallocated
	lastTick  int

	src     TextureSource
	missing func(int) image.Image
	isGrey  func(image.Image) bool
}

// texImage is one texture: its frames' slots (one unless animated).
type texImage struct {
	frames []int
	ticks  int
	cur    int // current frame's slot
	tint   bool
	cut    bool
}

// slot is one image placed in a tile: array layer, tile grid (the layer
// holds 2^grid x 2^grid tiles of its size) and tile index, and its mips.
type slot struct {
	layer, grid, tile int
	mips              [][]byte
}

// openLayer is the array layer tiles of one size are being added to.
type openLayer struct{ layer, next int }

// minTile is the smallest tile side: mipLevels halvings leave a texel.
const minTile = 1 << mipLevels

// maxLayerSize bounds the array layer side (tile codes hold 14 bits of
// tile index, grids up to 7).
const maxLayerSize = minTile << 7

const mipLevels = 4

func toNRGBA(img image.Image, size int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	b := img.Bounds()
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sx := b.Min.X + x*b.Dx()/size
			sy := b.Min.Y + y*b.Dy()/size
			out.Set(x, y, color.NRGBAModel.Convert(img.At(sx, sy)))
		}
	}
	return out
}

// downsample halves an image, averaging colour only over visible texels (so
// cutout edges don't darken) and boosting alpha a bit so foliage doesn't
// dissolve at distance.
func downsample(src []byte, size int) []byte {
	n := size / 2
	dst := make([]byte, n*n*4)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var r, g, b, a, cnt int
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					i := ((y*2+dy)*size + (x*2 + dx)) * 4
					pa := int(src[i+3])
					a += pa
					if pa > 0 {
						r += int(src[i])
						g += int(src[i+1])
						b += int(src[i+2])
						cnt++
					}
				}
			}
			o := (y*n + x) * 4
			if cnt > 0 {
				dst[o], dst[o+1], dst[o+2] = byte(r/cnt), byte(g/cnt), byte(b/cnt)
			}
			av := a / 4 * 5 / 4
			if av > 255 {
				av = 255
			}
			dst[o+3] = byte(av)
		}
	}
	return dst
}

func newTextureArray(entries []voxel.TextureEntry, src TextureSource, missing func(int) image.Image, isGrey func(image.Image) bool) *textureArray {
	t := &textureArray{lastTick: -1, texOf: map[string]int{}, open: map[int]openLayer{}, levels: mipLevels,
		src: src, missing: missing, isGrey: isGrey}
	frames := t.load(entries)
	largest, area := minTile, 0
	for _, f := range frames {
		s := tileSize(f)
		largest, area = max(largest, s), area+s*s
	}
	maxLayers, maxDim := gpuArrayLimits()
	// a layer at least the largest tile, larger while the tiles would fill
	// more than half the GPU's array layers
	t.layerSize = max(largest, 128)
	for area > t.layerSize*t.layerSize*maxLayers/2 && t.layerSize*2 <= min(maxDim, maxLayerSize) {
		t.layerSize *= 2
	}
	t.place(entries, frames)
	return t
}

// add appends the entries after the known ones; false when a new texture
// is larger than the layers (rebuild with newTextureArray).
func (t *textureArray) add(entries []voxel.TextureEntry) bool {
	if len(entries) <= len(t.entries) {
		return true
	}
	frames := t.load(entries)
	for _, f := range frames {
		if tileSize(f) > t.layerSize {
			return false
		}
	}
	t.place(entries, frames)
	return true
}

// tileSize is the tile an image takes: its larger side as a power of two,
// at least minTile.
func tileSize(img image.Image) int {
	b := img.Bounds()
	return max(minTile, 1<<bits.Len(uint(max(b.Dx(), b.Dy())-1)))
}

// load reads the textures of entries[len(t.entries):] not read before,
// appending them to t.texs; it returns their frames in that order (slots
// are appended in the same order by place).
func (t *textureArray) load(entries []voxel.TextureEntry) []image.Image {
	var out []image.Image
	asrc, _ := t.src.(AnimatedTextureSource)
	for _, e := range entries[len(t.entries):] {
		if _, ok := t.texOf[e.Name]; ok {
			continue
		}
		var frames []image.Image
		ticks := 1
		if asrc != nil {
			if fs, tk, err := asrc.TextureFrames(e.Name); err == nil && len(fs) > 0 {
				frames, ticks = fs, max(tk, 1)
			}
		} else if img, err := t.src.Texture(e.Name); err == nil {
			frames = []image.Image{img}
		}
		ti := texImage{ticks: ticks}
		if len(frames) == 0 {
			frames = []image.Image{t.missing(minTile)}
		} else {
			ti.tint = t.isGrey(frames[0])
		}
		for range frames {
			ti.frames = append(ti.frames, len(t.slots)+len(out)+len(ti.frames))
		}
		ti.cur = ti.frames[0]
		t.texOf[e.Name] = len(t.texs)
		t.texs = append(t.texs, ti)
		out = append(out, frames...)
	}
	return out
}

// place tiles frames (from load) and records entries, growing the GPU
// textures if needed, then uploads what is new and the layer map.
func (t *textureArray) place(entries []voxel.TextureEntry, frames []image.Image) {
	first := len(t.slots)
	for _, img := range frames {
		s := tileSize(img)
		px := toNRGBA(img, s).Pix
		chain := make([][]byte, t.levels+1)
		for l := range chain {
			chain[l] = px
			if l < t.levels {
				px = downsample(px, s>>l)
			}
		}
		t.slots = append(t.slots, t.tile(s, chain))
	}
	for k := range t.texs {
		ti := &t.texs[k]
		if ti.frames[len(ti.frames)-1] >= first {
			for _, f := range ti.frames {
				ti.cut = ti.cut || alphaCut(t.slots[f].mips[0])
			}
		}
	}
	for _, e := range entries[len(t.entries):] {
		ti := t.texs[t.texOf[e.Name]]
		t.tintable = append(t.tintable, ti.tint)
		t.cut = append(t.cut, ti.cut)
	}
	t.entries = append(t.entries, entries[len(t.entries):]...)
	if t.layers > t.capLayers || len(t.entries) > t.capMap {
		if t.capLayers > 0 {
			t.delete()
		}
		maxLayers, _ := gpuArrayLimits()
		if t.layers > maxLayers {
			panic(fmt.Sprintf("render: block textures need %d array layers of %dpx, the GPU has %d", t.layers, t.layerSize, maxLayers))
		}
		t.capLayers = min(max(t.layers+t.layers/4+4, 8), maxLayers)
		t.capMap = max(len(t.entries)+len(t.entries)/4+16, 64)
		t.allocGPU()
		first = 0
	}
	for i := first; i < len(t.slots); i++ {
		t.uploadSlot(t.slots[i])
	}
	t.uploadMap()
}

// tile places an image of tile side s in the next free tile of its size.
func (t *textureArray) tile(s int, mips [][]byte) slot {
	grid := bits.Len(uint(t.layerSize/s)) - 1
	o, ok := t.open[s]
	if !ok || o.next == 1<<(2*grid) {
		o = openLayer{layer: t.layers}
		t.layers++
	}
	sl := slot{layer: o.layer, grid: grid, tile: o.next, mips: mips}
	o.next++
	t.open[s] = o
	return sl
}

// alphaCut: a texel of the RGBA image has alpha <= 0.1 (the 8-bit 25 or
// less), which the pack's glAlphaFunc(GL_GREATER, 0.1) discards. The mips
// average with alpha scaled up, so they hold no such texel when level 0
// has none.
func alphaCut(px []byte) bool {
	for i := 3; i < len(px); i += 4 {
		if px[i] <= 25 {
			return true
		}
	}
	return false
}


// code is what the layer map stores for slot s: array layer:11, tile:14,
// grid:3 (wgslBlockTex decodes it).
func (s slot) code() uint32 { return uint32(s.layer) | uint32(s.tile)<<11 | uint32(s.grid)<<25 }

// tileXY is the tile's column and row in its layer.
func (s slot) tileXY() (x, y int) { return s.tile & (1<<s.grid - 1), s.tile >> s.grid }

// regionUnit is the layer map's rect step: 1/32768 of a texture (exact for
// quarter texels of textures up to 8192 px).
const regionUnit = 32768

// mapEntry is entry i's layer map texel: tile code and region rect.
func (t *textureArray) mapEntry(i int) [4]uint32 {
	e := t.entries[i]
	q := func(f float32) uint32 { return uint32(f*regionUnit + 0.5) }
	r := e.Region
	return [4]uint32{t.slots[t.texs[t.texOf[e.Name]].cur].code(), q(r[0]) | q(r[1])<<16, q(r[2]) | q(r[3])<<16, 0}
}

// animate points animated textures at their frame for game tick (20 Hz).
func (t *textureArray) animate(tick int) bool {
	if tick == t.lastTick {
		return false
	}
	t.lastTick = tick
	changed := false
	for k := range t.texs {
		ti := &t.texs[k]
		if len(ti.frames) < 2 {
			continue
		}
		if f := ti.frames[(tick/ti.ticks)%len(ti.frames)]; f != ti.cur {
			ti.cur, changed = f, true
		}
	}
	if changed {
		t.uploadMap()
	}
	return changed
}
