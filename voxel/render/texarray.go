package render

import (
	"image"
	"image/color"
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

// anim is one animated texture: name base's frames 1.. live in physical
// layers extra.. .
type anim struct{ base, extra, count, ticks int }

// textureArray holds every block texture as one layer of a 2D array texture:
// no atlas bleeding, and greedy-merged quads can tile a texture with repeat
// wrapping. Texture ids (registry names) map to physical layers through a
// layer map (a 1-row R16UI texture the shader reads), which also points
// animated textures at their current frame. Names learned later are
// appended into spare layers (see add) without re-reading the others.
type textureArray struct {
	gpu               gpuTexArray
	size              int
	levels            int // highest mip level
	tintable          []bool
	anims             []anim
	mapPix            []uint16   // per name: current physical layer
	base              []uint16   // per name: frame 0's physical layer
	mips              [][][]byte // per physical layer: cached mip chain
	capLayers, capMap int        // GPU capacity (physical layers, layer map width)
	gen               int        // bumped when the GPU textures are reallocated
	lastTick          int

	src     TextureSource
	missing func(int) image.Image
	isGrey  func(image.Image) bool
}

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

func newTextureArray(names []string, src TextureSource, missing func(int) image.Image, isGrey func(image.Image) bool) *textureArray {
	t := &textureArray{size: 16, lastTick: -1, src: src, missing: missing, isGrey: isGrey}
	imgs := t.load(names, 0)
	for _, img := range imgs {
		if img != nil {
			if w := img.Bounds().Dx(); w > t.size && w <= 512 {
				t.size = w
			}
		}
	}
	t.levels = mipLevels
	for s := t.size; s>>t.levels == 0; t.levels-- {
	}
	t.place(names, imgs)
	return t
}

// add appends textures for names[len(t.base):]; false if one is larger
// than the array's layer size (rebuild with newTextureArray).
func (t *textureArray) add(names []string) bool {
	if len(names) <= len(t.base) {
		return true
	}
	imgs := t.load(names, len(t.base))
	for _, img := range imgs {
		if img != nil {
			if w := img.Bounds().Dx(); w > t.size && w <= 512 {
				return false
			}
		}
	}
	t.place(names, imgs)
	return true
}

// load reads names[from:]: frame 0 per name then the extra animation
// frames; anims get name indices and extra offsets relative to the result.
func (t *textureArray) load(names []string, from int) []image.Image {
	imgs := make([]image.Image, len(names)-from)
	var extra []image.Image
	asrc, _ := t.src.(AnimatedTextureSource)
	for i, n := range names[from:] {
		if asrc != nil {
			frames, ticks, err := asrc.TextureFrames(n)
			if err != nil || len(frames) == 0 {
				continue
			}
			imgs[i] = frames[0]
			if len(frames) > 1 {
				t.anims = append(t.anims, anim{base: from + i, extra: -1 - len(extra), count: len(frames), ticks: max(ticks, 1)})
				extra = append(extra, frames[1:]...)
			}
		} else if img, err := t.src.Texture(n); err == nil {
			imgs[i] = img
		}
	}
	return append(imgs, extra...)
}

// place converts and uploads imgs (from load) as the layers of the names
// after len(t.base), growing the GPU textures if needed.
func (t *textureArray) place(names []string, imgs []image.Image) {
	first := len(t.mips)
	nNew := len(names) - len(t.base)
	for i, img := range imgs {
		if i < nNew {
			t.tintable = append(t.tintable, img != nil && t.isGrey(img))
		}
		if img == nil {
			img = t.missing(t.size)
		}
		px := toNRGBA(img, t.size).Pix
		chain := make([][]byte, t.levels+1)
		for l := range chain {
			chain[l] = px
			if l < t.levels {
				px = downsample(px, t.size>>l)
			}
		}
		t.mips = append(t.mips, chain)
	}
	for i := 0; i < nNew; i++ {
		t.base = append(t.base, uint16(first+i))
	}
	t.mapPix = append(t.mapPix, t.base[len(t.mapPix):]...)
	for k := range t.anims {
		if a := &t.anims[k]; a.extra < 0 {
			a.extra = first + nNew - 1 - a.extra
		}
	}
	if len(t.mips) > t.capLayers || len(t.mapPix) > t.capMap {
		if t.capLayers > 0 {
			t.delete()
		}
		t.capLayers = max(len(t.mips)+len(t.mips)/4+16, 64)
		t.capMap = max(len(t.mapPix)+len(t.mapPix)/4+16, 64)
		t.allocGPU()
		first = 0
	}
	for i := first; i < len(t.mips); i++ {
		t.uploadLayer(i, t.mips[i])
	}
	t.uploadMap()
}

// animate points animated layers at their frame for game tick (20 Hz).
func (t *textureArray) animate(tick int) {
	if len(t.anims) == 0 || tick == t.lastTick {
		return
	}
	t.lastTick = tick
	changed := false
	for _, a := range t.anims {
		f := (tick / a.ticks) % a.count
		l := t.base[a.base]
		if f > 0 {
			l = uint16(a.extra + f - 1)
		}
		if t.mapPix[a.base] != l {
			t.mapPix[a.base] = l
			changed = true
		}
	}
	if changed {
		t.uploadMap()
	}
}
