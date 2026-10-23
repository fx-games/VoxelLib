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
	radius   float32 // largest bind-pose extent from the origin
	bones    int     // skinning matrices the vertices may use
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

type gpuSkin struct {
	tex   texture2D
	group *wgpu.BindGroup
}

func (mr *ModelRenderer) uploadSkin(img image.Image) *gpuSkin {
	t := uploadTexture2D(img)
	return &gpuSkin{t, textureGroup(t)}
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

func (mr *ModelRenderer) drawModel(g *gpuModel, model mgl32.Mat4, skin []mgl32.Mat4, d drawParams) {
	p := curPass()
	if p == nil {
		return
	}
	l := &mr.gpu.light
	color := d.color
	if color == ([4]float32{}) {
		color = [4]float32{1, 1, 1, 1}
	}
	cutoff := d.cutoff
	if cutoff == 0 {
		cutoff = 0.5
	}
	emissive := float32(0)
	if d.emissive {
		emissive = 1
	}
	lc := mr.r.LightColor(d.sky, d.block) // look agent: lightmap colour (modelWGSL u.light.rgb)
	u := mr.gpu.u.reset().mat(l.vp).mat(model).f(lc[0], lc[1], lc[2], 0).
		f(l.fog.Color[0], l.fog.Color[1], l.fog.Color[2], d.fade).f(l.fog.Start, l.fog.End, cutoff, emissive).
		f(color[:]...).f(d.overlay[:]...)
	for _, b := range bones(skin, g.bones) {
		u.mat(b)
	}
	// clearDepth between begin and draw starts a new pass, so set the
	// pipeline per draw.
	switch d.blend {
	case BlendAlpha:
		p.SetPipeline(dev.pipes.modelAlpha)
	case BlendAdditive:
		p.SetPipeline(dev.pipes.modelAdd)
	default:
		p.SetPipeline(dev.pipes.model)
	}
	bindUniforms(p, u)
	p.SetVertexBuffer(0, g.vbo, 0)
	p.SetIndexBuffer(g.ebo, gputypes.IndexFormatUint32, 0)
	for _, pt := range g.parts {
		t := 0
		if pt.tex >= 0 && pt.tex < len(g.textures) {
			t = pt.tex
		}
		group := g.groups[t]
		if d.texture > 0 && d.texture <= len(mr.skins) {
			group = mr.skins[d.texture-1].group
		}
		p.SetBindGroup(1, group, nil)
		dev.draws++

		p.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: uint32(pt.count), InstanceCount: 1, FirstIndex: uint32(pt.first)})
	}
}
