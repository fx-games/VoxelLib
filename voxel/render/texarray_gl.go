//go:build !wgpu

package render

import "github.com/memmaker/voxellib/platform/gl"

// gpuTexArray is the GL_TEXTURE_2D_ARRAY and its layer map texture.
type gpuTexArray struct{ id, layerMap uint32 }

func (t *textureArray) allocGPU() {
	gl.GenTextures(1, &t.gpu.id)
	gl.BindTexture(gl.TEXTURE_2D_ARRAY, t.gpu.id)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MIN_FILTER, gl.NEAREST_MIPMAP_LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_WRAP_S, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_WRAP_T, gl.REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MAX_LEVEL, int32(t.levels))
	for l := 0; l <= t.levels; l++ {
		s := t.size >> l
		gl.TexImage3D(gl.TEXTURE_2D_ARRAY, int32(l), gl.RGBA8, int32(s), int32(s), int32(t.layers), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	}
	gl.GenTextures(1, &t.gpu.layerMap)
	gl.BindTexture(gl.TEXTURE_2D, t.gpu.layerMap)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 2)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.R16UI, int32(t.layers), 1, 0, gl.RED_INTEGER, gl.UNSIGNED_SHORT, nil)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
}

// uploadLayer writes one layer's mip chain (mips[l] is level l).
func (t *textureArray) uploadLayer(i int, mips [][]byte) {
	gl.BindTexture(gl.TEXTURE_2D_ARRAY, t.gpu.id)
	for l, px := range mips {
		s := t.size >> l
		gl.TexSubImage3D(gl.TEXTURE_2D_ARRAY, int32(l), 0, 0, int32(i), int32(s), int32(s), 1, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px))
	}
}

func (t *textureArray) uploadMap() {
	gl.BindTexture(gl.TEXTURE_2D, t.gpu.layerMap)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 2)
	gl.TexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, int32(len(t.mapPix)), 1, gl.RED_INTEGER, gl.UNSIGNED_SHORT, gl.Ptr(t.mapPix))
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 4)
}

func (t *textureArray) delete() {
	gl.DeleteTextures(1, &t.gpu.id)
	gl.DeleteTextures(1, &t.gpu.layerMap)
}
