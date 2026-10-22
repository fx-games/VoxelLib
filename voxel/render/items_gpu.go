package render

import (
	"github.com/go-gl/mathgl/mgl32"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

type gpuItemRenderer struct {
	vbo *wgpu.Buffer
	tex texture2D
	bg  *wgpu.BindGroup
	fog Fog
	u   ubuf
}

func (ir *ItemRenderer) initGPU(verts []float32, size int, layers [][]byte) error {
	g := &ir.gpu
	g.vbo = newBuffer("item vertices", wgpu.BufferUsageVertex, bytesOf(verts), 0)
	g.tex = newTexture("items", size, size, max(len(layers), 1), 1, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureViewDimension2DArray)
	for i, px := range layers {
		g.tex.write(0, i, size, size, 4, px)
	}
	g.bg = bindGroup("items", dev.pipes.texArray,
		wgpu.BindGroupEntry{Binding: 0, TextureView: g.tex.view},
		wgpu.BindGroupEntry{Binding: 1, Sampler: dev.nearestClp})
	return nil
}

func (ir *ItemRenderer) begin(fog Fog) { ir.gpu.fog = fog }

func (ir *ItemRenderer) drawRange(i int, mvp, model mgl32.Mat4, sky, block uint8) {
	p := curPass()
	if p == nil {
		return
	}
	g := &ir.gpu
	f := g.fog
	p.SetPipeline(dev.pipes.item)
	bindUniforms(p, g.u.reset().mat(clip(mvp)).mat(model).f(float32(sky)/15, float32(block)/15, ir.r.Opt.Daylight, ir.r.Opt.BlockBrightness).
		f(f.Color[0], f.Color[1], f.Color[2], float32(i)).f(f.Start, f.End, 0, 0))
	p.SetBindGroup(1, g.bg, nil)
	p.SetVertexBuffer(0, g.vbo, 0)
	p.Draw(gputypes.DrawArgs{VertexCount: uint32(ir.ranges[i][1]), InstanceCount: 1, FirstVertex: uint32(ir.ranges[i][0])})
}

func (ir *ItemRenderer) end() {}

// Delete frees GPU resources.
func (ir *ItemRenderer) Delete() {
	ir.gpu.bg.Release()
	ir.gpu.tex.release()
	ir.gpu.vbo.Release()
}
