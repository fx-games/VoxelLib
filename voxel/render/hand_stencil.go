package render

import (
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// handStencilFormat is the depth buffer of DrawHandInstance calls that use
// a stencil: their own buffer, so the world, GUI and shader-pack passes
// keep Depth32Float. WebGPU guarantees Depth24PlusStencil8 (Metal maps it
// to Depth32FloatStencil8).
const handStencilFormat = gputypes.TextureFormatDepth24PlusStencil8

// stencilMask is the stencil read and write mask.
const stencilMask = 0xFF

// StencilCompare is a stencil test: Ref <op> stored value, as GL and
// WebGPU (StencilGreater passes where Ref > stored).
type StencilCompare uint8

const (
	StencilAlways StencilCompare = iota
	StencilNever
	StencilEqual
	StencilNotEqual
	StencilLess
	StencilLessEqual
	StencilGreater
	StencilGreaterEqual
)

var stencilCompares = [...]gputypes.CompareFunction{
	StencilAlways: gputypes.CompareFunctionAlways, StencilNever: gputypes.CompareFunctionNever,
	StencilEqual: gputypes.CompareFunctionEqual, StencilNotEqual: gputypes.CompareFunctionNotEqual,
	StencilLess: gputypes.CompareFunctionLess, StencilLessEqual: gputypes.CompareFunctionLessEqual,
	StencilGreater: gputypes.CompareFunctionGreater, StencilGreaterEqual: gputypes.CompareFunctionGreaterEqual,
}

// StencilOp is what a draw does to the stored stencil value.
type StencilOp uint8

const (
	StencilKeep StencilOp = iota
	StencilReplace
	StencilInvert
	StencilZero
)

var stencilOps = [...]gputypes.StencilOperation{
	StencilKeep: gputypes.StencilOperationKeep, StencilReplace: gputypes.StencilOperationReplace,
	StencilInvert: gputypes.StencilOperationInvert, StencilZero: gputypes.StencilOperationZero,
}

// HandStencil is one DrawHandInstance draw's stencil and write state; the
// zero value is a plain draw (no stencil test, colour and depth written,
// depth tested). Where the stencil or depth test fails, the stored value
// is kept.
type HandStencil struct {
	Test         bool           // stencil test on
	Compare      StencilCompare // Ref <op> stored value
	Ref          uint8
	Pass         StencilOp // applied where the stencil and depth tests pass
	NoColor      bool      // write no colour
	NoDepthWrite bool
	DepthAlways  bool // the depth test always passes
	// ScopeView draws the scope view (BeginScopeView) over the screen
	// instead of the instance's model; nothing without one this frame.
	ScopeView bool
}

// handVariant names an engine pipeline made for the stencil hand pass
// (Ref is set per draw, so it is zero here).
type handVariant struct {
	pl *wgpu.RenderPipeline
	st HandStencil
}

// handStencil is the current draw's state while a stencil hand pass is
// open (setPipe picks the variant), nil otherwise.
var handStencil *HandStencil

// usesStencil reports whether any instance has a stencil state.
func usesStencil(ins []ModelInstance) bool {
	for i := range ins {
		if ins[i].Stencil != (HandStencil{}) {
			return true
		}
	}
	return false
}

// beginHandStencil starts a pass on the frame's colour target with the
// hand depth-stencil buffer, depth cleared to 1 and stencil to 0. A
// shader-pack frame's world ends first (packResolve): the pack's
// gbuffers have no stencil, so this hand draws with the engine's
// programs over the composited frame.
func beginHandStencil() {
	packResolve()
	endPass()
	if dev.enc == nil {
		return
	}
	target, _ := frameTargets()
	w, h := dev.w, dev.h
	if dev.inWorld {
		w, h = dev.ww, dev.wh
	}
	if dev.handDepthW != w || dev.handDepthH != h {
		dev.handDepth.release()
		dev.handDepth = attachment("hand depth stencil", w, h, handStencilFormat, 0)
		dev.handDepthW, dev.handDepthH = w, h
	}
	color := wgpu.RenderPassColorAttachment{View: target, LoadOp: gputypes.LoadOpLoad, StoreOp: gputypes.StoreOpStore}
	if dev.clearNext {
		color.LoadOp, color.ClearValue = gputypes.LoadOpClear, dev.clearColor
	}
	dev.pass = beginPass("hand stencil", []wgpu.RenderPassColorAttachment{color}, &wgpu.RenderPassDepthStencilAttachment{
		View: dev.handDepth.view, DepthLoadOp: gputypes.LoadOpClear, DepthClearValue: 1, DepthStoreOp: gputypes.StoreOpDiscard,
		StencilLoadOp: gputypes.LoadOpClear, StencilClearValue: 0, StencilStoreOp: gputypes.StoreOpDiscard})
	dev.clearNext = false
}

// endHandStencil ends the stencil hand pass; later draws start on a
// cleared main depth buffer (the hand's depth is not in it).
func endHandStencil() {
	handStencil = nil
	endPass()
	dev.clearDepth = true
}

// setHandPipe sets pl's variant for the state st and its reference.
func setHandPipe(p *wgpu.RenderPassEncoder, pl *wgpu.RenderPipeline, st HandStencil) {
	ref := st.Ref
	st.Ref = 0
	ps := &dev.pipes
	key := handVariant{pl, st}
	v := ps.handVariants[key]
	if v == nil {
		sp := ps.specs[pl]
		depth := &wgpu.DepthStencilState{Format: handStencilFormat, DepthWriteEnabled: sp.depthWrite && !st.NoDepthWrite,
			DepthCompare: gputypes.CompareFunctionLessEqual, StencilReadMask: stencilMask, StencilWriteMask: stencilMask}
		if !sp.depthTest || st.DepthAlways {
			depth.DepthCompare = gputypes.CompareFunctionAlways
		}
		face := wgpu.StencilFaceState{Compare: gputypes.CompareFunctionAlways, FailOp: gputypes.StencilOperationKeep,
			DepthFailOp: gputypes.StencilOperationKeep, PassOp: stencilOps[st.Pass]}
		if st.Test {
			face.Compare = stencilCompares[st.Compare]
		}
		depth.StencilFront, depth.StencilBack = face, face
		mask := gputypes.ColorWriteMask(gputypes.ColorWriteMaskAll)
		if st.NoColor {
			mask = gputypes.ColorWriteMaskNone
		}
		var err error
		if v, err = ps.build(sp, pl == ps.itemGlint || pl == ps.modelGlint, depth, mask); err != nil {
			panic(err)
		}
		ps.handVariants[key] = v
	}
	p.SetPipeline(v)
	p.SetStencilReference(uint32(ref))
}
