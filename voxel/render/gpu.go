package render

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"time"
	"unsafe"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	_ "github.com/gogpu/wgpu/hal/allbackends"
	"github.com/memmaker/voxellib/platform/glfw"
)

// Backend names the graphics backend this binary was built with.
const Backend = "wgpu"

// dev is the process-wide WebGPU state: one window, one device. Draw calls
// from every renderer record into the current render pass; uniforms and
// streamed vertices go to per-frame ring buffers written before submit.
var dev struct {
	instance *wgpu.Instance
	adapter  *wgpu.Adapter
	device   *wgpu.Device
	queue    *wgpu.Queue
	surface  *wgpu.Surface
	format   gputypes.TextureFormat
	present  gputypes.PresentMode
	bgra     bool
	w, h     int

	// mailbox (vsync off): frames render offscreen and are blitted to a
	// drawable only when acquireLoop has one ready, else dropped.
	mailbox     bool
	acquiring   bool // acquireLoop owes a drawable
	acquireNext chan struct{}
	drawable    chan *wgpu.SurfaceTexture

	depth     *wgpu.Texture
	depthView *wgpu.TextureView

	// frame state
	enc        *wgpu.CommandEncoder
	pass       *wgpu.RenderPassEncoder
	surfTex    *wgpu.SurfaceTexture
	target     *wgpu.TextureView
	clearColor gputypes.Color
	clearNext  bool // next pass clears colour (first pass of the frame)
	clearDepth bool // next pass clears depth

	multiDraw bool // FeatureMultiDrawIndirect
	debug     bool // VOXELLIB_WGPU_DEBUG: log validation errors per frame

	// Capture frames (and every mailbox frame) render offscreen so ReadFrame
	// can copy them (the swap chain image can't be read back), then blit to
	// the swap chain.
	capture, capturing bool
	capTex             texture2D
	capW, capH         int
	capBG              *wgpu.BindGroup

	uniforms, vertices, indirect stream

	uniLayout  *wgpu.BindGroupLayout // group 0: dynamic uniform block
	nearestRep *wgpu.Sampler         // block textures
	nearestClp *wgpu.Sampler         // sprites, models, HUD
	linearClp  *wgpu.Sampler         // lightmap
	pipes      pipelines
}

// uniformSlot is the bound size of a dynamic uniform block; every shader's
// uniform struct fits in it.
const uniformSlot = 8192

// WindowHints sets the GLFW hints the backend needs; call before
// glfw.CreateWindow.
func WindowHints() {
	glfw.WindowHint(glfw.ClientAPI, glfw.NoAPI)
}

// Init creates the device and a surface for win; call before New.
func Init(win *glfw.Window, vsync bool) error {
	inst, err := wgpu.CreateInstance(nil)
	if err != nil {
		return fmt.Errorf("wgpu: instance: %w", err)
	}
	dev.instance = inst
	if dev.surface, err = inst.CreateSurfaceUnsafe(surfaceTarget(win)); err != nil {
		return fmt.Errorf("wgpu: surface: %w", err)
	}
	if dev.adapter, err = inst.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference:   gputypes.PowerPreferenceHighPerformance,
		CompatibleSurface: dev.surface,
	}); err != nil {
		return fmt.Errorf("wgpu: adapter: %w", err)
	}
	var feats gputypes.Features
	if dev.adapter.Features().Contains(gputypes.FeatureMultiDrawIndirect) {
		feats |= gputypes.Features(gputypes.FeatureMultiDrawIndirect)
		dev.multiDraw = true
	}
	if dev.device, err = dev.adapter.RequestDevice(&wgpu.DeviceDescriptor{
		Label:            "voxellib",
		RequiredFeatures: feats,
		RequiredLimits:   dev.adapter.Limits(),
	}); err != nil {
		return fmt.Errorf("wgpu: device: %w", err)
	}
	dev.queue = dev.device.Queue()
	if os.Getenv("VOXELLIB_WGPU_DEBUG") != "" {
		info := dev.adapter.Info()
		log.Printf("wgpu: %s (%v), multidraw %v ", info.Name, info.Backend, dev.multiDraw)
		dev.debug = true
	}

	dev.format = gputypes.TextureFormatBGRA8Unorm
	if caps := dev.adapter.GetSurfaceCapabilities(dev.surface); caps != nil && len(caps.Formats) > 0 {
		dev.format = caps.Formats[0]
		for _, f := range caps.Formats {
			if f == gputypes.TextureFormatBGRA8Unorm || f == gputypes.TextureFormatRGBA8Unorm {
				dev.format = f // linear like the GL default framebuffer
				break
			}
		}
	}
	dev.bgra = dev.format == gputypes.TextureFormatBGRA8Unorm || dev.format == gputypes.TextureFormatBGRA8UnormSrgb
	dev.present = gputypes.PresentModeImmediate
	if vsync {
		dev.present = gputypes.PresentModeFifo
	}
	if dev.mailbox = !vsync; dev.mailbox {
		dev.drawable, dev.acquireNext = make(chan *wgpu.SurfaceTexture, 1), make(chan struct{}, 1)
		go acquireLoop()
	}

	dev.uniforms = stream{usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst, chunk: 4 << 20, align: 256, label: "uniforms"}
	dev.vertices = stream{usage: wgpu.BufferUsageVertex | wgpu.BufferUsageCopyDst, chunk: 1 << 20, align: 16, label: "stream vertices"}
	dev.indirect = stream{usage: wgpu.BufferUsageIndirect | wgpu.BufferUsageCopyDst, chunk: 1 << 20, align: 4, label: "indirect"}
	if dev.uniLayout, err = dev.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "uniform block",
		Entries: []gputypes.BindGroupLayoutEntry{{
			Binding:    0,
			Visibility: gputypes.ShaderStageVertex | gputypes.ShaderStageFragment,
			Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, HasDynamicOffset: true, MinBindingSize: uniformSlot},
		}},
	}); err != nil {
		return err
	}
	if dev.nearestRep, err = dev.device.CreateSampler(&wgpu.SamplerDescriptor{
		Label:        "blocks",
		AddressModeU: gputypes.AddressModeRepeat, AddressModeV: gputypes.AddressModeRepeat, AddressModeW: gputypes.AddressModeRepeat,
		MagFilter: gputypes.FilterModeNearest, MinFilter: gputypes.FilterModeNearest, MipmapFilter: gputypes.FilterModeLinear,
		LodMaxClamp: 32,
	}); err != nil {
		return err
	}
	if dev.nearestClp, err = dev.device.CreateSampler(&wgpu.SamplerDescriptor{
		Label:        "sprites",
		AddressModeU: gputypes.AddressModeClampToEdge, AddressModeV: gputypes.AddressModeClampToEdge, AddressModeW: gputypes.AddressModeClampToEdge,
		MagFilter: gputypes.FilterModeNearest, MinFilter: gputypes.FilterModeNearest, MipmapFilter: gputypes.FilterModeNearest,
		LodMaxClamp: 32,
	}); err != nil {
		return err
	}
	if dev.linearClp, err = dev.device.CreateSampler(&wgpu.SamplerDescriptor{
		Label:        "lightmap",
		AddressModeU: gputypes.AddressModeClampToEdge, AddressModeV: gputypes.AddressModeClampToEdge, AddressModeW: gputypes.AddressModeClampToEdge,
		MagFilter: gputypes.FilterModeLinear, MinFilter: gputypes.FilterModeLinear, MipmapFilter: gputypes.FilterModeNearest,
		LodMaxClamp: 32,
	}); err != nil {
		return err
	}
	return dev.pipes.init()
}

// configure (re)creates the swap chain and depth buffer for w x h.
func configure(w, h int) error {
	if w == dev.w && h == dev.h {
		return nil
	}
	if dev.acquiring { // ponytail: presents one stale frame on resize
		dev.acquiring = false
		if setSurface(<-dev.drawable) {
			present()
		}
	}
	if err := dev.surface.Configure(dev.device, &wgpu.SurfaceConfiguration{
		Width: uint32(w), Height: uint32(h), Format: dev.format,
		Usage:       gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
		PresentMode: dev.present, AlphaMode: gputypes.CompositeAlphaModeOpaque,
	}); err != nil {
		return err
	}
	if dev.depth != nil {
		dev.depthView.Release()
		dev.depth.Release()
	}
	var err error
	if dev.depth, err = dev.device.CreateTexture(&wgpu.TextureDescriptor{
		Label: "depth", Size: wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D,
		Format: gputypes.TextureFormatDepth32Float, Usage: gputypes.TextureUsageRenderAttachment,
	}); err != nil {
		return err
	}
	if dev.depthView, err = dev.device.CreateTextureView(dev.depth, nil); err != nil {
		return err
	}
	dev.w, dev.h = w, h
	return nil
}

// BeginFrame acquires the next swap chain image; the first pass clears it
// to the given colour.
func BeginFrame(w, h int, r, g, b float32) {
	if w == 0 || h == 0 {
		return
	}
	if err := configure(w, h); err != nil {
		panic(err)
	}
	// Frames always render offscreen; the drawable is acquired in EndFrame
	// (vsync) or by acquireLoop (mailbox), so waiting for it never delays
	// the next frame's input and update.
	if dev.capturing, dev.capture = true, false; dev.capturing {
		if dev.capW != w || dev.capH != h {
			if dev.capBG != nil {
				dev.capBG.Release()
				dev.capTex.release()
			}
			dev.capTex = newTargetTexture(w, h)
			dev.capBG = textureGroup(dev.capTex)
			dev.capW, dev.capH = w, h
		}
	}
	var err error
	if dev.enc, err = dev.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "frame"}); err != nil {
		panic(err)
	}
	dev.clearColor = gputypes.Color{R: float64(r), G: float64(g), B: float64(b), A: 1}
	dev.clearNext, dev.clearDepth = true, true
	if dev.debug {
		dev.device.PushErrorScope(wgpu.ErrorFilterValidation)
	}
}

// curPass returns the render pass to record into, beginning one if needed;
// nil outside a frame.
func curPass() *wgpu.RenderPassEncoder {
	if dev.pass != nil || dev.enc == nil {
		return dev.pass
	}
	target := dev.target
	if dev.capturing {
		target = dev.capTex.view
	}
	color := wgpu.RenderPassColorAttachment{View: target, LoadOp: gputypes.LoadOpLoad, StoreOp: gputypes.StoreOpStore}
	if dev.clearNext {
		color.LoadOp, color.ClearValue = gputypes.LoadOpClear, dev.clearColor
	}
	depth := &wgpu.RenderPassDepthStencilAttachment{View: dev.depthView, DepthLoadOp: gputypes.LoadOpLoad, DepthStoreOp: gputypes.StoreOpStore}
	if dev.clearDepth {
		depth.DepthLoadOp, depth.DepthClearValue = gputypes.LoadOpClear, 1
	}
	p, err := dev.enc.BeginRenderPass(&wgpu.RenderPassDescriptor{Label: "main",
		ColorAttachments: []wgpu.RenderPassColorAttachment{color}, DepthStencilAttachment: depth})
	if err != nil {
		panic(err)
	}
	dev.pass, dev.clearNext, dev.clearDepth = p, false, false
	return p
}

func endPass() {
	if dev.pass != nil {
		if err := dev.pass.End(); err != nil {
			panic(err)
		}
		dev.pass = nil
	}
}

// clearDepth makes later draws start from a cleared depth buffer (first-
// person hand over the world).
func clearDepth() {
	endPass()
	dev.clearDepth = true
}

// submit ends the pass, writes the streamed data and submits the encoder.
func submit(final bool) {
	if dev.enc == nil {
		return
	}
	if dev.clearNext || dev.clearDepth {
		curPass() // nothing drawn since a clear was requested: still clear
	}
	if dev.capturing && final && dev.target != nil {
		endPass()
		p, err := dev.enc.BeginRenderPass(&wgpu.RenderPassDescriptor{Label: "capture blit",
			ColorAttachments: []wgpu.RenderPassColorAttachment{{View: dev.target, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore}}})
		if err != nil {
			panic(err)
		}
		p.SetPipeline(dev.pipes.blit)
		p.SetBindGroup(0, dev.capBG, nil)
		p.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
		dev.pass = p
	}
	endPass()
	dev.uniforms.flush()
	dev.vertices.flush()
	dev.indirect.flush()
	cmd, err := dev.enc.Finish()
	if err != nil {
		panic(err)
	}
	dev.enc = nil
	if _, err := dev.queue.Submit(cmd); err != nil {
		panic(err)
	}

}

// EndFrame submits and presents the frame.
func EndFrame(*glfw.Window) {
	if dev.enc == nil {
		return
	}
	if dev.mailbox && dev.acquiring {
		select {
		case st := <-dev.drawable:
			dev.acquiring = false
			setSurface(st)
		default: // no drawable free: drop this frame, like GL without vsync
		}
	} else if !dev.mailbox {
		setSurface(nextDrawable())
	}
	submit(true)
	if dev.debug {
		if e := dev.device.PopErrorScope(); e != nil {
			log.Printf("wgpu: frame error: %v", e)
		}
	}
	present()
	dev.capturing = false
	if dev.mailbox && !dev.acquiring {
		dev.acquiring = true
		dev.acquireNext <- struct{}{}
	}
	dev.uniforms.reset()
	dev.vertices.reset()
	dev.indirect.reset()
}

// CaptureNextFrame makes the next frame readable by ReadFrame. The wgpu
// backend renders that frame offscreen and copies it to the window.
func CaptureNextFrame() { dev.capture = true }

func newTargetTexture(w, h int) texture2D {
	t, err := dev.device.CreateTexture(&wgpu.TextureDescriptor{
		Label: "capture", Size: wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D, Format: dev.format,
		Usage: gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		panic(err)
	}
	v, err := dev.device.CreateTextureView(t, nil)
	if err != nil {
		panic(err)
	}
	return texture2D{t, v}
}

// ReadFrame reads the frame drawn so far into an opaque, top-down image.
// It waits for the GPU, so use it for screenshots only, in a frame
// announced with CaptureNextFrame.
func ReadFrame(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if !dev.capturing || dev.enc == nil {
		return img
	}
	if dev.clearNext {
		curPass()
	}
	endPass()
	row := (uint32(w)*4 + 255) &^ 255
	size := uint64(row) * uint64(h)
	buf, err := dev.device.CreateBuffer(&wgpu.BufferDescriptor{Label: "readback", Size: size, Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead})
	if err != nil {
		panic(err)
	}
	defer buf.Release()
	// The copy goes in its own submission: gogpu/wgpu's usage tracker
	// rejects a copy from the attachment in the encoder that rendered it.
	submit(false)
	if dev.enc, err = dev.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "readback"}); err != nil {
		panic(err)
	}
	src := dev.surfTex.AsTexture() // reads back black on Metal
	if dev.capturing {
		src = dev.capTex.tex
	}
	dev.enc.CopyTextureToBuffer(src, buf, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: row, RowsPerImage: uint32(h)},
		TextureBase:  wgpu.ImageCopyTexture{Texture: src},
		Size:         wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1},
	}})
	submit(false)
	if dev.enc, err = dev.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "frame (after readback)"}); err != nil {
		panic(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := buf.Map(ctx, wgpu.MapModeRead, 0, size); err != nil {
		panic(err)
	}
	rng, err := buf.MappedRange(0, size)
	if err != nil {
		panic(err)
	}
	px := rng.Bytes()
	for y := 0; y < h; y++ {
		src := px[uint32(y)*row : uint32(y)*row+uint32(w)*4]
		dst := img.Pix[y*w*4 : (y+1)*w*4]
		copy(dst, src)
		for i := 0; i < len(dst); i += 4 {
			if dev.bgra {
				dst[i], dst[i+2] = dst[i+2], dst[i]
			}
			dst[i+3] = 255
		}
	}
	buf.Unmap()
	return img
}

// GPUTimer would measure GPU time; the wgpu backend does not support it yet
// and always reports 0.
type GPUTimer struct{}

// Measure runs f and returns 0.
func (t *GPUTimer) Measure(f func()) time.Duration { f(); return 0 }

// stream is a set of per-frame buffer chunks for data written each frame:
// allocations are copied to a CPU shadow and written with one
// Queue.WriteBuffer per chunk before submit. Chunks are reused every frame;
// WebGPU orders the writes after the previous frame's GPU work.
type stream struct {
	usage  gputypes.BufferUsage
	chunk  int
	align  int
	label  string
	chunks []*streamChunk
	cur    int
}

type streamChunk struct {
	buf     *wgpu.Buffer
	shadow  []byte
	used    int
	flushed int
	bg      *wgpu.BindGroup // uniform chunks: group 0 bound at dynamic offsets
}

// alloc copies data into the stream; it returns the chunk and the offset.
func (s *stream) alloc(data []byte, reserve int) (*streamChunk, uint32) {
	need := max(len(data), reserve)
	for {
		if s.cur >= len(s.chunks) {
			size := max(s.chunk, need)
			b, err := dev.device.CreateBuffer(&wgpu.BufferDescriptor{Label: s.label, Size: uint64(size), Usage: s.usage})
			if err != nil {
				panic(err)
			}
			c := &streamChunk{buf: b, shadow: make([]byte, size)}
			if s.usage&wgpu.BufferUsageUniform != 0 {
				if c.bg, err = dev.device.CreateBindGroup(&wgpu.BindGroupDescriptor{Label: s.label, Layout: dev.uniLayout,
					Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: b, Size: uniformSlot}}}); err != nil {
					panic(err)
				}
			}
			s.chunks = append(s.chunks, c)
		}
		c := s.chunks[s.cur]
		off := (c.used + s.align - 1) &^ (s.align - 1)
		if off+need <= len(c.shadow) {
			copy(c.shadow[off:], data)
			c.used = off + len(data)
			if reserve > len(data) {
				// Zero the tail like GL's unset uniforms: a model reads
				// bones its skin did not set.
				clear(c.shadow[off+len(data) : off+reserve])
				c.used = off + reserve
			}
			return c, uint32(off)
		}
		s.cur++
	}
}

func (s *stream) flush() {
	for _, c := range s.chunks[:min(s.cur+1, len(s.chunks))] {
		if c.used > c.flushed {
			lo := c.flushed &^ 3
			hi := (c.used + 3) &^ 3
			writeBuffer(c.buf, lo, c.shadow[lo:hi])
			c.flushed = c.used
		}
	}
}

func (s *stream) reset() {
	for _, c := range s.chunks {
		c.used, c.flushed = 0, 0
	}
	s.cur = 0
}

// bindUniforms writes a uniform block and binds it as group 0.
func bindUniforms(p *wgpu.RenderPassEncoder, u *ubuf) {
	c, off := dev.uniforms.alloc(u.b, uniformSlot)
	p.SetBindGroup(0, c.bg, []uint32{off})
}

// ubuf packs a uniform block with WGSL layout (only mat4 and vec4 members,
// so no padding rules apply beyond 16-byte vec4s).
type ubuf struct{ b []byte }

func (u *ubuf) reset() *ubuf { u.b = u.b[:0]; return u }

func (u *ubuf) f(vs ...float32) *ubuf {
	for _, v := range vs {
		u.b = binary.LittleEndian.AppendUint32(u.b, math.Float32bits(v))
	}
	return u
}

func (u *ubuf) mat(m mgl32.Mat4) *ubuf { return u.f(m[:]...) }

// clip maps a GL clip-space matrix (z in -w..w) to WebGPU's (z in 0..w).
func clip(m mgl32.Mat4) mgl32.Mat4 {
	return mgl32.Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0.5, 0, 0, 0, 0.5, 1}.Mul4(m)
}

func bytesOf[T any](s []T) []byte {
	if len(s) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&s[0])), len(s)*int(unsafe.Sizeof(s[0])))
}

// newBuffer creates a buffer holding data.
func newBuffer(label string, usage gputypes.BufferUsage, data []byte, size int) *wgpu.Buffer {
	size = max(size, len(data), 4)
	b, err := dev.device.CreateBuffer(&wgpu.BufferDescriptor{Label: label, Size: uint64((size + 3) &^ 3), Usage: usage | wgpu.BufferUsageCopyDst})
	if err != nil {
		panic(err)
	}
	if len(data) > 0 {
		writeBuffer(b, 0, data)
	}
	return b
}

func writeBuffer(b *wgpu.Buffer, off int, data []byte) {
	if len(data)&3 != 0 {
		data = append(data[:len(data):len(data)], make([]byte, 4-len(data)&3)...)
	}
	// Pieces that fit wgpu's 256 KiB staging chunks: larger writes would
	// get a fresh one-off staging buffer each.
	for len(data) > 0 {
		n := min(len(data), stagingPiece)
		if err := dev.queue.WriteBuffer(b, uint64(off), data[:n]); err != nil {
			panic(err)
		}
		off, data = off+n, data[n:]
	}
}

const stagingPiece = 128 << 10

// texture2D is a sampled texture with its view.
type texture2D struct {
	tex  *wgpu.Texture
	view *wgpu.TextureView
}

func newTexture(label string, w, h, layers, mips int, format gputypes.TextureFormat, dim gputypes.TextureViewDimension) texture2D {
	t, err := dev.device.CreateTexture(&wgpu.TextureDescriptor{
		Label: label, Size: wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: uint32(layers)},
		MipLevelCount: uint32(mips), SampleCount: 1, Dimension: gputypes.TextureDimension2D, Format: format,
		Usage: gputypes.TextureUsageTextureBinding | gputypes.TextureUsageCopyDst,
	})
	if err != nil {
		panic(err)
	}
	v, err := dev.device.CreateTextureView(t, &wgpu.TextureViewDescriptor{Label: label, Format: format, Dimension: dim,
		Aspect: gputypes.TextureAspectAll, MipLevelCount: uint32(mips), ArrayLayerCount: uint32(layers)})
	if err != nil {
		panic(err)
	}
	return texture2D{t, v}
}

// write uploads one layer of one mip level; bpp is bytes per texel.
func (t texture2D) write(level, layer, w, h, bpp int, px []byte) {
	if err := dev.queue.WriteTexture(&wgpu.ImageCopyTexture{Texture: t.tex, MipLevel: uint32(level), Origin: wgpu.Origin3D{Z: uint32(layer)}},
		px, &wgpu.ImageDataLayout{BytesPerRow: uint32(w * bpp), RowsPerImage: uint32(h)},
		&wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1}); err != nil {
		panic(err)
	}
}

func (t texture2D) release() {
	if t.tex != nil {
		t.view.Release()
		t.tex.Release()
	}
}

func bindGroup(label string, layout *wgpu.BindGroupLayout, entries ...wgpu.BindGroupEntry) *wgpu.BindGroup {
	bg, err := dev.device.CreateBindGroup(&wgpu.BindGroupDescriptor{Label: label, Layout: layout, Entries: entries})
	if err != nil {
		panic(err)
	}
	return bg
}

// nextDrawable blocks for the next swap chain image; nil on failure.
func nextDrawable() *wgpu.SurfaceTexture {
	st, _, err := dev.surface.GetCurrentTexture()
	if err != nil {
		return nil
	}
	return st
}

func setSurface(st *wgpu.SurfaceTexture) bool {
	if st == nil {
		dev.w = 0 // reconfigure next frame
		return false
	}
	dev.surfTex = st
	var err error
	if dev.target, err = st.CreateView(nil); err != nil {
		panic(err)
	}
	return true
}

func present() {
	if dev.surfTex == nil {
		return
	}
	if err := dev.surface.Present(dev.surfTex); err != nil {
		dev.w = 0
	}
	dev.target.Release()
	dev.surfTex, dev.target = nil, nil
}

// acquireLoop waits for drawables off the render thread (mailbox mode):
// nextDrawable blocks until the compositor frees one, often for a whole
// refresh, while frames keep rendering offscreen. One acquire is in flight
// at a time, so the surface is never used from two goroutines at once.
func acquireLoop() {
	for range dev.acquireNext {
		dev.drawable <- nextDrawable()
	}
}

// FinishGPU blocks until the GPU has finished all submitted work. For
// benchmarks only (it serialises CPU and GPU).
func FinishGPU() { dev.device.WaitIdle() }
