package render

import (
	"encoding/binary"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/memmaker/voxellib/voxel"
)

// gpuRenderer is the WebGPU state of a Renderer.
type gpuRenderer struct {
	ebo, lineVBO *wgpu.Buffer
	texBG        *wgpu.BindGroup // terrain textures, rebuilt with the array
	bgFor        *textureArray
	u            ubuf
	ind          []byte
}

func (r *Renderer) initGPU() error {
	g := &r.gpu
	g.ebo = newBuffer("quad indices", wgpu.BufferUsageIndex, bytesOf(quadIndices()), 0)
	g.lineVBO = newBuffer("lines", wgpu.BufferUsageVertex, bytesOf(lineVertices()), 0)
	return nil
}

// drawTerrain draws the sky and r.batches.
func (r *Renderer) drawTerrain(vp mgl32.Mat4) {
	p := curPass()
	if p == nil {
		return
	}
	g := &r.gpu
	fog := r.CurrentFog()
	skyCol, fogCol := r.skyColors()
	p.SetPipeline(dev.pipes.sky)
	bindUniforms(p, g.u.reset().mat(vp.Inv()).f(fogCol[0], fogCol[1], fogCol[2], 0).f(skyCol[0], skyCol[1], skyCol[2], 0).
		f(r.Opt.SunDir[0], r.Opt.SunDir[1], r.Opt.SunDir[2], 0))
	p.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})

	if len(r.batches) == 0 {
		return
	}
	if g.bgFor != r.tex {
		if g.texBG != nil {
			g.texBG.Release()
		}
		g.texBG = bindGroup("terrain textures", dev.pipes.terrainTex,
			wgpu.BindGroupEntry{Binding: 0, TextureView: r.tex.gpu.array.view},
			wgpu.BindGroupEntry{Binding: 1, Sampler: dev.nearestRep},
			wgpu.BindGroupEntry{Binding: 2, TextureView: r.tex.gpu.layerMap.view})
		g.bgFor = r.tex
	}
	cvp := clip(vp)
	layer := voxel.LayerCount
	for _, b := range r.batches {
		if b.layer != layer {
			layer = b.layer
			if layer == voxel.LayerTranslucent {
				p.SetPipeline(dev.pipes.terrainTrans)
			} else if layer == voxel.LayerSolid || layer == voxel.LayerCutout {
				p.SetPipeline(dev.pipes.terrainOpaque)
			}
			p.SetBindGroup(1, g.texBG, nil)
			p.SetIndexBuffer(g.ebo, gputypes.IndexFormatUint32, 0)
		}
		cut := float32(0)
		switch layer {
		case voxel.LayerCutout:
			cut = 0.5
		case voxel.LayerTranslucent:
			cut = 0.01
		}
		o := r.regionOrigin(b.reg)
		bindUniforms(p, g.u.reset().mat(cvp).f(o[0], o[1], o[2], 0).f(fogCol[0], fogCol[1], fogCol[2], 0).
			f(r.Opt.Daylight, r.Opt.BlockBrightness, cut, 0).f(fog.Start, fog.End, 0, 0))
		p.SetVertexBuffer(0, b.reg.gpu.vbo, 0)
		r.multiDraw(p, b)
	}
}

// multiDraw issues a batch as one indirect multidraw when the device
// supports it, else as one DrawIndexed per range.
func (r *Renderer) multiDraw(p *wgpu.RenderPassEncoder, b drawBatch) {
	counts, bases := r.counts[b.first:b.first+b.n], r.bases[b.first:b.first+b.n]
	if !dev.multiDraw {
		for i, c := range counts {
			p.DrawIndexed(gputypes.DrawIndexedArgs{IndexCount: uint32(c), InstanceCount: 1, BaseVertex: bases[i]})
		}
		return
	}
	g := &r.gpu
	g.ind = g.ind[:0]
	for i, c := range counts {
		g.ind = binary.LittleEndian.AppendUint32(g.ind, uint32(c))
		g.ind = binary.LittleEndian.AppendUint32(g.ind, 1)
		g.ind = binary.LittleEndian.AppendUint32(g.ind, 0)
		g.ind = binary.LittleEndian.AppendUint32(g.ind, uint32(bases[i]))
		g.ind = binary.LittleEndian.AppendUint32(g.ind, 0)
	}
	c, off := dev.indirect.alloc(g.ind, 0)
	p.MultiDrawIndexedIndirect(c.buf, uint64(off), uint32(len(counts)))
}

// DrawBlockOutline draws a wireframe box around a block (Minecraft's
// selection outline).
func (r *Renderer) DrawBlockOutline(cam *Camera, x, y, z int) {
	r.drawLines(dev.pipes.outline, 0, 24, clip(outlineMVP(cam, x, y, z)), mgl32.Vec4{0, 0, 0, 0.6})
}

// DrawCrosshair draws a screen-centre cross; aspect = width/height.
func (r *Renderer) DrawCrosshair(aspect float32) {
	r.drawLines(dev.pipes.crosshair, 24, 4, crosshairMVP(aspect), mgl32.Vec4{1, 1, 1, 1})
}

func (r *Renderer) drawLines(pl *wgpu.RenderPipeline, first, n uint32, mvp mgl32.Mat4, col mgl32.Vec4) {
	p := curPass()
	if p == nil {
		return
	}
	p.SetPipeline(pl)
	bindUniforms(p, r.gpu.u.reset().mat(mvp).f(col[:]...))
	p.SetVertexBuffer(0, r.gpu.lineVBO, 0)
	p.Draw(gputypes.DrawArgs{VertexCount: n, InstanceCount: 1, FirstVertex: first})
}

func (r *Renderer) destroyGPU() {
	r.gpu.ebo.Release()
	r.gpu.lineVBO.Release()
}
