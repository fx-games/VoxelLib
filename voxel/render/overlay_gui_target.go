package render

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// NewGUITarget makes a w x h offscreen picture GUIBatch.Target draws into
// (the reference's frame buffer objects); draw it like any GUITexture.
func NewGUITarget(w, h int) *GUITexture {
	t := &GUITexture{w: w, h: h}
	t.tex = attachment("gui target", w, h, dev.format, gputypes.TextureUsageTextureBinding)
	t.depth = attachment("gui target depth", w, h, gputypes.TextureFormatDepth32Float, 0)
	t.bg = bindGroup("gui target", dev.pipes.tex2D,
		wgpu.BindGroupEntry{Binding: 0, TextureView: t.tex.view},
		wgpu.BindGroupEntry{Binding: 1, Sampler: dev.nearestRep})
	return t
}

func attachment(label string, w, h int, format gputypes.TextureFormat, usage gputypes.TextureUsage) texture2D {
	t, err := dev.device.CreateTexture(&wgpu.TextureDescriptor{
		Label: label, Size: wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D, Format: format,
		Usage: gputypes.TextureUsageRenderAttachment | usage,
	})
	if err != nil {
		panic(err)
	}
	v, err := dev.device.CreateTextureView(t, &wgpu.TextureViewDescriptor{Label: label, Format: format,
		Dimension: gputypes.TextureViewDimension2D, Aspect: gputypes.TextureAspectAll, MipLevelCount: 1, ArrayLayerCount: 1})
	if err != nil {
		panic(err)
	}
	return texture2D{t, v}
}

// beginPass starts a pass into the target, clearing it when s asks.
func (t *GUITexture) beginPass(s guiSeg) *wgpu.RenderPassEncoder {
	color := wgpu.RenderPassColorAttachment{View: t.tex.view, LoadOp: gputypes.LoadOpLoad, StoreOp: gputypes.StoreOpStore}
	if s.clear {
		c := s.clearC
		color.LoadOp = gputypes.LoadOpClear
		color.ClearValue = gputypes.Color{R: float64(c>>16&255) / 255, G: float64(c>>8&255) / 255, B: float64(c&255) / 255, A: float64(c>>24) / 255}
	}
	return beginPass("gui target", []wgpu.RenderPassColorAttachment{color}, &wgpu.RenderPassDepthStencilAttachment{
		View: t.depth.view, DepthLoadOp: gputypes.LoadOpClear, DepthClearValue: 1, DepthStoreOp: gputypes.StoreOpDiscard})
}

// bindGroup is the texture with the nearest (repeating) or linear
// (repeating) sampler.
func (t *GUITexture) bindGroup(linear bool) *wgpu.BindGroup {
	if !linear {
		return t.bg
	}
	if t.bgLin == nil {
		if guiState.linear == nil {
			s, err := dev.device.CreateSampler(&wgpu.SamplerDescriptor{
				Label:        "gui linear",
				AddressModeU: gputypes.AddressModeRepeat, AddressModeV: gputypes.AddressModeRepeat, AddressModeW: gputypes.AddressModeRepeat,
				MagFilter: gputypes.FilterModeLinear, MinFilter: gputypes.FilterModeLinear, MipmapFilter: gputypes.FilterModeNearest,
				LodMaxClamp: 32,
			})
			if err != nil {
				panic(err)
			}
			guiState.linear = s
		}
		t.bgLin = bindGroup("gui texture linear", dev.pipes.tex2D,
			wgpu.BindGroupEntry{Binding: 0, TextureView: t.tex.view},
			wgpu.BindGroupEntry{Binding: 1, Sampler: guiState.linear})
	}
	return t.bgLin
}
