package render

import (
	"image"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// The scope view is a second rendering of the world (a magnified camera)
// into its own target, between BeginScopeView and EndScopeView; a
// DrawHandInstance draw with HandStencil.ScopeView shows it over the
// screen where its stencil test passes (a scope's lens).
var scope struct {
	color, depth texture2D
	bg           *wgpu.BindGroup
	w, h         int
	clip         gputypes.ScissorRect // the pixels BeginScopeView draws
	active       bool                 // between BeginScopeView and EndScopeView
	ready        bool                 // drawn this frame
	// the frame's pass state, restored by EndScopeView
	clearColor            gputypes.Color
	clearNext, clearDepth bool
}

// BeginScopeView redirects the world draws (Renderer.RenderView,
// ModelRenderer.Draw, ...) to the scope view, cleared, at the world's
// resolution, rasterising only inside clip (world pixels, e.g. a lens's
// ModelRenderer.HandBounds: the view shows only there), and reports
// whether it did: not outside a frame or for a clip off the screen.
//
// In a shader-pack frame it first ends the pack's world (packResolve):
// the scope view, and the stencil hand pass that shows it, draw with the
// engine's own programs over the composited frame. The pack's programs
// can't draw a second camera: its gbuffers, composites and per-frame
// uniform buffer exist once, for the main view.
func BeginScopeView(clip image.Rectangle) bool {
	clip = clip.Intersect(image.Rect(0, 0, dev.ww, dev.wh))
	if dev.enc == nil || clip.Empty() {
		return false
	}
	packResolve()
	scope.clip = gputypes.ScissorRect{X: uint32(clip.Min.X), Y: uint32(clip.Min.Y), Width: uint32(clip.Dx()), Height: uint32(clip.Dy())}
	endPass()
	w, h := dev.ww, dev.wh
	if scope.w != w || scope.h != h {
		if scope.bg != nil {
			scope.bg.Release()
			scope.color.release()
			scope.depth.release()
		}
		scope.color = attachment("scope view", w, h, dev.format, gputypes.TextureUsageTextureBinding)
		scope.depth = attachment("scope view depth", w, h, gputypes.TextureFormatDepth32Float, 0)
		scope.bg = bindGroup("scope view", dev.pipes.tex2D,
			wgpu.BindGroupEntry{Binding: 0, TextureView: scope.color.view},
			wgpu.BindGroupEntry{Binding: 1, Sampler: dev.linearClp})
		scope.w, scope.h = w, h
	}
	scope.clearColor, scope.clearNext, scope.clearDepth = dev.clearColor, dev.clearNext, dev.clearDepth
	dev.clearNext, dev.clearDepth = true, true
	scope.active = true
	return true
}

// EndScopeView ends the scope view; the frame's draws continue where
// they were.
func EndScopeView() {
	if !scope.active {
		return
	}
	if dev.clearNext || dev.clearDepth {
		curPass() // nothing drawn: still clear
	}
	endPass()
	scope.active, scope.ready = false, true
	dev.clearColor, dev.clearNext, dev.clearDepth = scope.clearColor, scope.clearNext, scope.clearDepth
}

// drawScopeView draws the scope view over the screen (screen-space
// texture coordinates) under the hand pass stencil st.
func drawScopeView(st HandStencil) {
	p := curPass()
	if p == nil || !scope.ready {
		return
	}
	setHandPipe(p, dev.pipes.blit, st)
	p.SetBindGroup(0, scope.bg, nil)
	dev.draws++
	p.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
}
