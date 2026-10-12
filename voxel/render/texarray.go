package render

import (
	"image"
	"image/color"

	"github.com/go-gl/gl/v3.3-core/gl"
)

// TextureSource supplies block textures by resource name ("block/stone").
// *mcres.Pack implements it.
type TextureSource interface {
	Texture(name string) (image.Image, error)
}

// textureArray holds every block texture as one layer of a GL_TEXTURE_2D_ARRAY:
// no atlas bleeding, and greedy-merged quads can tile a texture with GL_REPEAT.
type textureArray struct {
	id       uint32
	size     int
	layers   int
	tintable []bool
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
	for i, n := range names {
		img, err := src.Texture(n)
		if err != nil {
			continue
		}
		imgs[i] = img
		if w := img.Bounds().Dx(); w > size && w <= 512 {
			size = w
		}
	}
	ta := &textureArray{size: size, layers: len(names), tintable: make([]bool, len(names))}
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
		gl.TexImage3D(gl.TEXTURE_2D_ARRAY, int32(l), gl.RGBA8, int32(s), int32(s), int32(len(names)), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	}
	for i, img := range imgs {
		if img == nil {
			img = missing(size)
		} else {
			ta.tintable[i] = isGrey(img)
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
	return ta
}

func (t *textureArray) delete() { gl.DeleteTextures(1, &t.id) }
