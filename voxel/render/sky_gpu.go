package render

import (
	"image"
	"image/color"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

type skyTexture struct {
	tex texture2D
	bg  *wgpu.BindGroup
}

var skyWhite *skyTexture // untextured sky draws

func uploadSkyTexture(img image.Image) *skyTexture {
	t := uploadTexture2D(img)
	return &skyTexture{t, bindGroup("sky texture", dev.pipes.tex2D,
		wgpu.BindGroupEntry{Binding: 0, TextureView: t.view},
		wgpu.BindGroupEntry{Binding: 1, Sampler: dev.nearestRep})}
}

// skyWhiteTexture is a 1x1 white texture, for untextured draws.
func skyWhiteTexture() *skyTexture {
	if skyWhite == nil {
		img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
		img.Set(0, 0, color.White)
		skyWhite = uploadSkyTexture(img)
	}
	return skyWhite
}

// drawSky draws sky draws with the camera's rotation.
func (r *Renderer) drawSky(cam *Camera, draws []SkyDraw) {
	p := curPass()
	if p == nil {
		return
	}
	for i := range draws {
		d := &draws[i]
		t := skyWhiteTexture()
		if d.Texture != "" {
			if t = r.skyTexture(d.Texture); t == nil {
				continue
			}
		}
		r.skyVerts = d.triangles(r.skyVerts[:0])
		if len(r.skyVerts) == 0 {
			continue
		}
		c := *cam
		if d.Far > 0 {
			c.Far = float64(d.Far)
		}
		m := d.Transform
		if m == (mgl32.Mat4{}) {
			m = mgl32.Ident4()
		}
		var fc mgl32.Vec3
		var fog [4]float32
		fog[3] = d.AlphaTest
		if d.Fog != nil {
			fc = d.Fog.Color
			fog[0], fog[1], fog[2] = d.Fog.Start, d.Fog.End, 1
		}
		if d.Additive {
			p.SetPipeline(dev.pipes.skyAdd)
		} else {
			p.SetPipeline(dev.pipes.skyAlpha)
		}
		bindUniforms(p, r.gpu.u.reset().mat(clip(c.ViewProj().Mul4(m))).mat(m).f(fc[0], fc[1], fc[2], 0).f(fog[:]...))
		p.SetBindGroup(1, t.bg, nil)
		vc, off := dev.vertices.alloc(bytesOf(r.skyVerts), 0)
		p.SetVertexBuffer(0, vc.buf, uint64(off))
		p.Draw(gputypes.DrawArgs{VertexCount: uint32(len(r.skyVerts)), InstanceCount: 1})
	}
}
