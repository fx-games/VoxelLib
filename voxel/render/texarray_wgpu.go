//go:build wgpu

package render

import "github.com/gogpu/gputypes"

// gpuTexArray is the block texture array and its layer map texture.
type gpuTexArray struct{ array, layerMap texture2D }

func (t *textureArray) allocGPU() {
	t.gpu.array = newTexture("blocks", t.size, t.size, t.layers, t.levels+1, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureViewDimension2DArray)
	t.gpu.layerMap = newTexture("layer map", t.layers, 1, 1, 1, gputypes.TextureFormatR16Uint, gputypes.TextureViewDimension2D)
}

// uploadLayer writes one layer's mip chain (mips[l] is level l).
func (t *textureArray) uploadLayer(i int, mips [][]byte) {
	for l, px := range mips {
		s := t.size >> l
		t.gpu.array.write(l, i, s, s, 4, px)
	}
}

func (t *textureArray) uploadMap() {
	t.gpu.layerMap.write(0, 0, len(t.mapPix), 1, 2, bytesOf(t.mapPix))
}

func (t *textureArray) delete() {
	t.gpu.array.release()
	t.gpu.layerMap.release()
}
