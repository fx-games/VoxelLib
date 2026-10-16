package render

import (
	"image"
	"image/color"

	"github.com/memmaker/voxellib/platform/gl"
)

// TextureSource supplies block textures by resource name ("block/stone").
// *mcres.Pack implements it.
type TextureSource interface {
	Texture(name string) (image.Image, error)
}

// AnimatedTextureSource is optionally implemented by a TextureSource whose
// textures may be animated (Minecraft .png.mcmeta strips). Frame 0 is what
// Texture returns. *mcres.Pack implements it.
type AnimatedTextureSource interface {
	TextureFrames(name string) (frames []image.Image, ticksPerFrame int, err error)
}

// anim is one animated texture: frames 1.. live in layers extra.. .
type anim struct{ base, extra, count, ticks int }

// textureArray holds every block texture as one layer of a GL_TEXTURE_2D_ARRAY:
// no atlas bleeding, and greedy-merged quads can tile a texture with GL_REPEAT.
type textureArray struct {
	id       uint32
	size     int
	layers   int
	tintable []bool
	// Animation: the shader looks every layer up in layerMap (a 1-row R16UI
	// texture) so animated layers can point at their current frame.
	anims    []anim
	layerMap uint32
	mapPix   []uint16
	lastTick int
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
	imgs := make([]image.Image, len(names))
	size := 16
	var anims []anim
	var extra []image.Image
	asrc, _ := src.(AnimatedTextureSource)
	for i, n := range names {
		var img image.Image
		if asrc != nil {
			frames, ticks, err := asrc.TextureFrames(n)
			if err != nil || len(frames) == 0 {
				continue
			}
			img = frames[0]
			if len(frames) > 1 {
				anims = append(anims, anim{base: i, extra: len(names) + len(extra), count: len(frames), ticks: max(ticks, 1)})
				extra = append(extra, frames[1:]...)
			}
		} else {
			var err error
			if img, err = src.Texture(n); err != nil {
				continue
			}
		}
		imgs[i] = img
		if w := img.Bounds().Dx(); w > size && w <= 512 {
			size = w
		}
	}
	tint := make([]bool, len(names))
	imgs = append(imgs, extra...)
	ta := &textureArray{size: size, layers: len(imgs), anims: anims, lastTick: -1}
	gl.GenTextures(1, &ta.id)
	gl.BindTexture(gl.TEXTURE_2D_ARRAY, ta.id)
	levels := mipLevels
	for s := size; s>>levels == 0; levels-- {
	}
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MIN_FILTER, gl.NEAREST_MIPMAP_LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_WRAP_S, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_WRAP_T, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MAX_LEVEL, int32(levels))
	for l := 0; l <= levels; l++ {
		s := size >> l
		gl.TexImage3D(gl.TEXTURE_2D_ARRAY, int32(l), gl.RGBA8, int32(s), int32(s), int32(len(imgs)), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	}
	for i, img := range imgs {
		if img == nil {
			img = missing(size)
		} else if i < len(tint) {
			tint[i] = isGrey(img)
		}
		px := toNRGBA(img, size).Pix
		for l := 0; l <= levels; l++ {
			s := size >> l
			gl.TexSubImage3D(gl.TEXTURE_2D_ARRAY, int32(l), 0, 0, int32(i), int32(s), int32(s), 1, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px))
			if l < levels {
				px = downsample(px, s)
			}
		}
	}
	ta.tintable = tint

	ta.mapPix = make([]uint16, len(imgs))
	for i := range ta.mapPix {
		ta.mapPix[i] = uint16(i)
	}
	gl.GenTextures(1, &ta.layerMap)
	gl.BindTexture(gl.TEXTURE_2D, ta.layerMap)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 2)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.R16UI, int32(len(imgs)), 1, 0, gl.RED_INTEGER, gl.UNSIGNED_SHORT, gl.Ptr(ta.mapPix))
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
	return ta
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
		l := uint16(a.base)
		if f > 0 {
			l = uint16(a.extra + f - 1)
		}
		if t.mapPix[a.base] != l {
			t.mapPix[a.base] = l
			changed = true
		}
	}
	if changed {
		gl.BindTexture(gl.TEXTURE_2D, t.layerMap)
		gl.PixelStorei(gl.UNPACK_ALIGNMENT, 2)
		gl.TexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, int32(len(t.mapPix)), 1, gl.RED_INTEGER, gl.UNSIGNED_SHORT, gl.Ptr(t.mapPix))
		gl.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
	}
}

func (t *textureArray) delete() {
	gl.DeleteTextures(1, &t.id)
	gl.DeleteTextures(1, &t.layerMap)
}
