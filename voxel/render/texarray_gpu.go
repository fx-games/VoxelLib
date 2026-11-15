package render

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// gpuTexArray is the block texture array and its layer map texture.
type gpuTexArray struct{ array, layerMap texture2D }

// wgslBlockTex samples the block texture array by a layer map code
// (slot.code: layer, tile, grid): the tile at fract(uv) (kept below 1: of
// a tiny negative u fract rounds to 1, the next tile) with the unwrapped
// uv's gradients, so repeat wrapping and texels match a whole texture and
// the mip level has no seams at tile wraps. It samples the shader's tex and
// samp: naga's WGSL lowering drops the array index of textureSampleGrad on
// a texture parameter. regionUV maps a face's uv (0..1 over its region)
// into the region rect of its layer map texel lm (regionUnit steps).
const wgslBlockTex = `
fn blockTex(uv: vec2f, code: u32) -> vec4f {
	let g = (code >> 25u) & 7u;
	let i = (code >> 11u) & 16383u;
	let m = (1u << g) - 1u;
	let n = f32(1u << g);
	let p = (min(fract(uv), vec2f(1.0 - 1.0 / 4096.0)) + vec2f(f32(i & m), f32(i >> g))) / n;
	return textureSampleGrad(tex, samp, p, code & 2047u, dpdx(uv) / n, dpdy(uv) / n);
}
fn regionUV(uv: vec2f, lm: vec4<u32>) -> vec2f {
	let lo = vec2f(f32(lm.y & 65535u), f32(lm.y >> 16u)) / 32768.0;
	let hi = vec2f(f32(lm.z & 65535u), f32(lm.z >> 16u)) / 32768.0;
	if (all(lo == vec2f(0.0)) && all(hi == vec2f(1.0))) { return uv; }
	return mix(lo, hi, uv);
}
`

// gpuArrayLimits: the device's array layers and 2D texture size limits.
func gpuArrayLimits() (layers, dim int) {
	l := dev.device.Limits()
	return int(l.MaxTextureArrayLayers), int(l.MaxTextureDimension2D)
}

func (t *textureArray) allocGPU() {
	t.gen++
	t.gpu.array = newTexture("blocks", t.layerSize, t.layerSize, t.capLayers, t.levels+1, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureViewDimension2DArray)
	t.gpu.layerMap = newTexture("layer map", t.capMap, 1, 1, 1, gputypes.TextureFormatRGBA32Uint, gputypes.TextureViewDimension2D)
}

// uploadSlot writes a slot's mip chain into its tile.
func (t *textureArray) uploadSlot(sl slot) {
	mustRenderThread("texture array write")
	x, y := sl.tileXY()
	for l, px := range sl.mips {
		s := t.layerSize >> sl.grid >> l
		if err := dev.queue.WriteTexture(&wgpu.ImageCopyTexture{Texture: t.gpu.array.tex, MipLevel: uint32(l),
			Origin: wgpu.Origin3D{X: uint32(x * s), Y: uint32(y * s), Z: uint32(sl.layer)}},
			px, &wgpu.ImageDataLayout{BytesPerRow: uint32(s * 4), RowsPerImage: uint32(s)},
			&wgpu.Extent3D{Width: uint32(s), Height: uint32(s), DepthOrArrayLayers: 1}); err != nil {
			panic(err)
		}
	}
}

func (t *textureArray) uploadMap() {
	px := make([]uint32, 0, 4*len(t.entries))
	for i := range t.entries {
		e := t.mapEntry(i)
		px = append(px, e[:]...)
	}
	t.gpu.layerMap.write(0, 0, len(t.entries), 1, 16, bytesOf(px))
}

func (t *textureArray) delete() {
	t.gpu.array.release()
	t.gpu.layerMap.release()
}
