package render

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// gpuTexArray is the block texture array and its layer map texture.
type gpuTexArray struct{ array, layerMap texture2D }

// wgslBlockTex samples the block texture array by a layer map code
// (layerCode). Grid 0 is a plain layer, sampled as before. Otherwise it
// samples the tile at fract(uv) (kept below 1: of a tiny negative u fract
// rounds to 1, the next tile) with the unwrapped uv's gradients, so repeat
// wrapping and texels match a whole layer and the mip level has no seams
// at tile wraps. The grid is the same for every fragment, so the branch is
// dynamically uniform. It samples the shader's tex and samp: naga's WGSL
// lowering drops the array index of textureSampleGrad on a texture
// parameter.
const wgslBlockTex = `
fn blockTex(uv: vec2f, code: u32) -> vec4f {
	let g = code >> 14u;
	if (g == 0u) { return textureSample(tex, samp, uv, code); }
	let i = code & 16383u;
	let m = (1u << g) - 1u;
	let n = f32(1u << g);
	let p = (min(fract(uv), vec2f(1.0 - 1.0 / 4096.0)) + vec2f(f32(i & m), f32((i >> g) & m))) / n;
	return textureSampleGrad(tex, samp, p, i >> (2u * g), dpdx(uv) / n, dpdy(uv) / n);
}
`

// gpuArrayLimits: the device's array layers and 2D texture size limits.
func gpuArrayLimits() (layers, dim int) {
	l := dev.device.Limits()
	return int(l.MaxTextureArrayLayers), int(l.MaxTextureDimension2D)
}

func (t *textureArray) allocGPU() {
	t.gen++
	t.gpu.array = newTexture("blocks", t.size<<t.grid, t.size<<t.grid, t.arrayLayers(), t.levels+1, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureViewDimension2DArray)
	t.gpu.layerMap = newTexture("layer map", t.capMap, 1, 1, 1, gputypes.TextureFormatR16Uint, gputypes.TextureViewDimension2D)
}

// uploadLayer writes physical layer i's mip chain (mips[l] is level l)
// into its tile.
func (t *textureArray) uploadLayer(i int, mips [][]byte) {
	layer, x, y := tileOf(i, t.grid)
	for l, px := range mips {
		s := t.size >> l
		if err := dev.queue.WriteTexture(&wgpu.ImageCopyTexture{Texture: t.gpu.array.tex, MipLevel: uint32(l),
			Origin: wgpu.Origin3D{X: uint32(x * s), Y: uint32(y * s), Z: uint32(layer)}},
			px, &wgpu.ImageDataLayout{BytesPerRow: uint32(s * 4), RowsPerImage: uint32(s)},
			&wgpu.Extent3D{Width: uint32(s), Height: uint32(s), DepthOrArrayLayers: 1}); err != nil {
			panic(err)
		}
	}
}

func (t *textureArray) uploadMap() {
	codes := make([]uint16, len(t.mapPix))
	for i, l := range t.mapPix {
		codes[i] = layerCode(int(l), t.grid)
	}
	t.gpu.layerMap.write(0, 0, len(codes), 1, 2, bytesOf(codes))
}

func (t *textureArray) delete() {
	t.gpu.array.release()
	t.gpu.layerMap.release()
}
