package render

import (
	"image"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

type gpuParticleRenderer struct {
	quad  *wgpu.Buffer
	u     ubuf
	atlas *skyTexture // nil: skyWhite
}

func (pr *ParticleRenderer) setAtlas(img image.Image) {
	if a := pr.gpu.atlas; a != nil {
		a.bg.Release()
		a.tex.release()
	}
	pr.gpu.atlas = uploadSkyTexture(img)
}

func (pr *ParticleRenderer) initGPU() error {
	pr.gpu.quad = newBuffer("particle quad", wgpu.BufferUsageVertex, bytesOf(particleCorners), 0)
	return nil
}

func (pr *ParticleRenderer) begin(*particleFrame) {}

func (pr *ParticleRenderer) drawPass(pf *particleFrame, additive bool, buf []float32) {
	p := curPass()
	if p == nil {
		return
	}
	pl, add := dev.pipes.particleAlpha, float32(0)
	if additive {
		pl, add = dev.pipes.particleAdd, 1
	}
	p.SetPipeline(pl)
	t := pr.gpu.atlas
	if t == nil {
		t = skyWhiteTexture()
	}
	p.SetBindGroup(1, t.bg, nil)
	f := pf.fog
	bindUniforms(p, pr.gpu.u.reset().mat(clip(pf.vp)).f(pf.right[0], pf.right[1], pf.right[2], 0).f(pf.up[0], pf.up[1], pf.up[2], 0).
		f(pr.r.Opt.Daylight, pr.r.Opt.BlockBrightness, add, 0).f(f.Color[0], f.Color[1], f.Color[2], 0).f(f.Start, f.End, 0, 0))
	c, off := dev.vertices.alloc(bytesOf(buf), 0)
	p.SetVertexBuffer(0, pr.gpu.quad, 0)
	p.SetVertexBuffer(1, c.buf, uint64(off))
	p.Draw(gputypes.DrawArgs{VertexCount: 6, InstanceCount: uint32(len(buf) / particleStride)})
}

func (pr *ParticleRenderer) end() {}
