package render

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

type gpuOverlay struct {
	tex texture2D
	bg  *wgpu.BindGroup
	u   ubuf
}

func (o *Overlay) initGPU() error { return nil }

func (o *Overlay) hasImage() bool { return o.gpu.tex.tex != nil }

// setPixels uploads tightly packed RGBA; same means the size is unchanged.
func (o *Overlay) setPixels(px []byte, w, h int, same bool) {
	if o.gpu.tex.tex == nil || !same {
		if o.gpu.bg != nil {
			o.gpu.bg.Release()
			o.gpu.tex.release()
		}
		o.gpu.tex = newTexture("overlay", w, h, 1, 1, gputypes.TextureFormatRGBA8Unorm, gputypes.TextureViewDimension2D)
		o.gpu.bg = textureGroup(o.gpu.tex)
	}
	o.gpu.tex.write(0, 0, w, h, 4, px)
}

func (o *Overlay) drawRect(rect [4]float32) {
	p := curPass()
	if p == nil {
		return
	}
	p.SetPipeline(dev.pipes.overlay)
	bindUniforms(p, o.gpu.u.reset().f(rect[:]...))
	p.SetBindGroup(1, o.gpu.bg, nil)
	dev.draws++

	p.Draw(gputypes.DrawArgs{VertexCount: 4, InstanceCount: 1})
}
