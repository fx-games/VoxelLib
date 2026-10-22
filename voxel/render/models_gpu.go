package render

import (
	"image"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

type gpuModelRenderer struct {
	u     ubuf
	light modelLight
}

type gpuModel struct {
	vbo, ebo *wgpu.Buffer
	parts    []gpuPart
	textures []texture2D
	groups   []*wgpu.BindGroup
}

func (mr *ModelRenderer) initGPU() error { return nil }

func (mr *ModelRenderer) upload(vb []float32, ib []uint32, texs []image.Image) *gpuModel {
	g := &gpuModel{
		vbo: newBuffer("model vertices", wgpu.BufferUsageVertex, bytesOf(vb), 0),
		ebo: newBuffer("model indices", wgpu.BufferUsageIndex, bytesOf(ib), 0),
	}
	for _, img := range texs {
		t := uploadTexture2D(img)
		g.textures = append(g.textures, t)
		g.groups = append(g.groups, textureGroup(t))
	}
	return g
}

func uploadTexture2D(img image.Image) texture2D {
	px := toNRGBARect(img)
	w, h := px.Rect.Dx(), px.Rect.Dy()
	t := newTexture("model texture", w, h, 1, 1, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureViewDimension2D)
	t.write(0, 0, w, h, 4, px.Pix)
	return t
}

func textureGroup(t texture2D) *wgpu.BindGroup {
	return bindGroup("texture", dev.pipes.tex2D,
		wgpu.BindGroupEntry{Binding: 0, TextureView: t.view},
		wgpu.BindGroupEntry{Binding: 1, Sampler: dev.nearestClp})
}

func (g *gpuModel) delete() {
	g.vbo.Release()
	g.ebo.Release()
	for i, t := range g.textures {
		g.groups[i].Release()
		t.release()
	}
}

func (mr *ModelRenderer) begin(l modelLight) {
	l.vp = clip(l.vp)
	mr.gpu.light = l
	if p := curPass(); p != nil {
		p.SetPipeline(dev.pipes.model)
	}
}

func (mr *ModelRenderer) end() {}

func (mr *ModelRenderer) drawModel(g *gpuModel, model mgl32.Mat4, skin []mgl32.Mat4, sky, block uint8, fade float32) {
	p := curPass()
	if p == nil {
		return
	}
	l := &mr.gpu.light
	u := mr.gpu.u.reset().mat(l.vp).mat(model).f(float32(sky)/15, float32(block)/15, l.daylight, l.block).
		f(l.fog.Color[0], l.fog.Color[1], l.fog.Color[2], fade).f(l.fog.Start, l.fog.End, 0, 0)
	for _, b := range bones(skin) {
		u.mat(b)
	}
	p.SetPipeline(dev.pipes.model) // clearDepth between begin and draw starts a new pass
	bindUniforms(p, u)
	p.SetVertexBuffer(0, g.vbo, 0)
	p.SetIndexBuffer(g.ebo, gputypes.IndexFormatUint32, 0)
	for _, pt := range g.parts {
		t := 0
		if pt.tex >= 0 && pt.tex < len(g.textures) {
			t = pt.tex
		}
		p.SetBindGroup(1, g.groups[t], nil)
		p.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: uint32(pt.count), InstanceCount: 1, FirstIndex: uint32(pt.first)})
	}
}
