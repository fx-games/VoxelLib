package render

import (
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"math"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
	"github.com/memmaker/voxellib/platform/glfw"
)

func init() { runtime.LockOSThread() } // glfw wants the main thread

// gpuOK reports whether TestMain got a device (a hidden window).
var gpuOK bool

func TestMain(m *testing.M) {
	if glfw.Init() == nil {
		WindowHints()
		glfw.WindowHint(glfw.Visible, glfw.False)
		glfw.WindowHint(glfw.Focused, glfw.False)
		if win, err := glfw.CreateWindow(64, 64, "test", nil, nil); err == nil {
			gpuOK = Init(win, false) == nil
		}
	}
	os.Exit(m.Run())
}

// TestPackCasterShadow draws a flat item, half of its texels cut by the
// alpha test, as a shadow caster at noon: its top face's depth must be in
// both shadow maps where ShadowProjection, ShadowModelView and
// ShadowDistort put it, and the cut half must cast nothing.
func TestPackCasterShadow(t *testing.T) {
	if !gpuOK {
		t.Skip("no GPU device")
	}
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 8 { // left half opaque
			img.Set(x, y, color.NRGBA{200, 100, 50, 255})
		}
	}
	ir, err := NewItemRenderer(nil, []string{"test"}, []image.Image{img})
	if err != nil {
		t.Fatal(err)
	}
	defer ir.Delete()

	pk := newPackGPU(PackQuality{}.orDefault())
	pk.resize(64, 64)
	mv, _ := ShadowModelView(0, 1, 65, 1) // noon, no snap offset
	pk.u.ShadowMV, pk.u.ShadowProj = mv, ShadowProjection()
	writeBuffer(pk.ubuf, 0, unsafe.Slice((*byte)(unsafe.Pointer(&pk.u)), unsafe.Sizeof(pk.u)))

	if dev.enc, err = dev.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "test"}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []*wgpu.TextureView{pk.shadow.view, pk.shadow1.view} {
		endPassOf(beginPass("clear", []wgpu.RenderPassColorAttachment{{View: pk.dummyShadow.view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpDiscard}},
			&wgpu.RenderPassDepthStencilAttachment{View: d, DepthLoadOp: gputypes.LoadOpClear, DepthClearValue: 1, DepthStoreOp: gputypes.StoreOpStore}))
	}
	// A half-block slab lying flat (local XY → world XZ), camera-relative:
	// small, as depth is interpolated linearly across the distorted
	// triangles (by 1e-4 over a 4-block quad here).
	at := mgl32.Vec3{10, -5, 6}
	model := mgl32.Translate3D(at[0], at[1], at[2]).Mul4(mgl32.HomogRotate3DX(-math.Pi / 2)).Mul4(mgl32.Scale3D(0.5, 0.5, 0.5))
	u := (&ubuf{}).mat(mgl32.Ident4()).mat(model).f(1, 1, 1, 255).f(0, 0, 0, 0).f(0, 0, 0, 0)
	c, off := dev.uniforms.alloc(u.b, uniformSlot)
	lastUniforms, dynOffset[0] = c.bg, off
	dev.pk, pk.lastPl = pk, dev.pipes.item
	ir.gpu.fog = Fog{}
	packCast(ir.gpu.bg, ir.gpu.vbo, nil, uint32(ir.ranges[0][0]), uint32(ir.ranges[0][1]), model, 1)
	if len(pk.casters) != 1 {
		t.Fatalf("recorded %d casters", len(pk.casters))
	}
	pk.castShadows()
	dev.pk = nil

	read := func(src *wgpu.TextureView) []float32 { return readDepth(t, pk, src) }
	maps := map[string][]float32{"shadowtex0": read(pk.shadow.view), "shadowtex1": read(pk.shadow1.view)}

	top := float32(0.5 / 32) // half the slab's thickness (one texel of 16, ×0.5)
	clipOf := func(p mgl32.Vec3) mgl32.Vec4 { return ShadowDistort(pk.u.ShadowProj.Mul4(mv).Mul4x1(p.Vec4(1))) }
	pixel := func(q mgl32.Vec4) (float32, float32) {
		return (q[0]*0.5 + 0.5) * packShadowRes, (0.5 - q[1]*0.5) * packShadowRes
	}
	project := func(p mgl32.Vec3) (int, int) { x, y := pixel(clipOf(p)); return int(x), int(y) }
	// onTop is the point of the slab's top face seen through texel (x, y)'s
	// centre (Newton on the face's x and z).
	onTop := func(x, y int, p mgl32.Vec3) mgl32.Vec3 {
		for range 8 {
			px, py := pixel(clipOf(p))
			ex, ey := float32(x)+0.5-px, float32(y)+0.5-py
			const h = 1e-3
			ax, ay := pixel(clipOf(p.Add(mgl32.Vec3{h, 0, 0})))
			bx, by := pixel(clipOf(p.Add(mgl32.Vec3{0, 0, h})))
			j := mgl32.Mat2{(ax - px) / h, (ay - py) / h, (bx - px) / h, (by - py) / h}
			d := j.Inv().Mul2x1(mgl32.Vec2{ex, ey})
			p = p.Add(mgl32.Vec3{d[0], 0, d[1]})
		}
		return p
	}
	for name, d := range maps {
		p := at.Add(mgl32.Vec3{-0.125, top, 0.1})
		x, y := project(p)
		q := clipOf(onTop(x, y, p))
		if got, want := d[y*packShadowRes+x], 0.5*(q[2]+q[3]); math.Abs(float64(got-want)) > 2e-6 {
			t.Errorf("%s: opaque half at (%d,%d) depth %v, want %v", name, x, y, got, want)
		}
		x, y = project(at.Add(mgl32.Vec3{0.125, top, 0.1}))
		if got := d[y*packShadowRes+x]; got != 1 {
			t.Errorf("%s: alpha-cut half at (%d,%d) depth %v, want 1", name, x, y, got)
		}
		x, y = project(at.Add(mgl32.Vec3{-0.75, top, 0}))
		if got := d[y*packShadowRes+x]; got != 1 {
			t.Errorf("%s: beside the slab at (%d,%d) depth %v, want 1", name, x, y, got)
		}
	}
}

// readDepth copies a shadow-sized depth target through an R32Float
// target (pack targets have no CopySrc) and reads it back.
func readDepth(t *testing.T, pk *packGPU, src *wgpu.TextureView) []float32 {
	t.Helper()
	const n = packShadowRes
	tex, err := dev.device.CreateTexture(&wgpu.TextureDescriptor{Label: "depth readback", Size: wgpu.Extent3D{Width: n, Height: n, DepthOrArrayLayers: 1},
		MipLevelCount: 1, SampleCount: 1, Dimension: gputypes.TextureDimension2D, Format: gputypes.TextureFormatR32Float,
		Usage: gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc})
	if err != nil {
		t.Fatal(err)
	}
	defer tex.Release()
	view, err := dev.device.CreateTextureView(tex, &wgpu.TextureViewDescriptor{Format: gputypes.TextureFormatR32Float,
		Dimension: gputypes.TextureViewDimension2D, Aspect: gputypes.TextureAspectAll, MipLevelCount: 1, ArrayLayerCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	pl := pk.pipeline("depth readback", `
@group(0) @binding(0) var src: texture_depth_2d;
@vertex
fn vs(@builtin(vertex_index) id: u32) -> @builtin(position) vec4f {
	let p = vec2f(f32((id << 1u) & 2u), f32(id & 2u));
	return vec4f(p * 2.0 - 1.0, 0.0, 1.0);
}
@fragment
fn fs(@builtin(position) p: vec4f) -> @location(0) vec4f { return vec4f(textureLoad(src, vec2i(p.xy), 0), 0.0, 0.0, 1.0); }
`, []*wgpu.BindGroupLayout{pk.copyLayout}, nil, nil,
		[]gputypes.ColorTargetState{{Format: gputypes.TextureFormatR32Float, WriteMask: gputypes.ColorWriteMaskAll}}, nil)
	if dev.enc == nil {
		if dev.enc, err = dev.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "test"}); err != nil {
			t.Fatal(err)
		}
	}
	p := beginPass("depth readback", []wgpu.RenderPassColorAttachment{{View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore}}, nil)
	p.SetPipeline(pl)
	bg := bindGroup("depth readback", pk.copyLayout, wgpu.BindGroupEntry{Binding: 0, TextureView: src})
	defer bg.Release()
	p.SetBindGroup(0, bg, nil)
	p.Draw(gputypes.DrawArgs{VertexCount: 3, InstanceCount: 1})
	endPassOf(p)
	submit(false)

	buf, err := dev.device.CreateBuffer(&wgpu.BufferDescriptor{Label: "readback", Size: n * n * 4, Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead})
	if err != nil {
		t.Fatal(err)
	}
	defer buf.Release()
	if dev.enc, err = dev.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "readback"}); err != nil {
		t.Fatal(err)
	}
	dev.enc.CopyTextureToBuffer(tex, buf, []wgpu.BufferTextureCopy{{
		BufferLayout: wgpu.ImageDataLayout{BytesPerRow: n * 4, RowsPerImage: n},
		TextureBase:  wgpu.ImageCopyTexture{Texture: tex},
		Size:         wgpu.Extent3D{Width: n, Height: n, DepthOrArrayLayers: 1},
	}})
	submit(false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := buf.Map(ctx, wgpu.MapModeRead, 0, n*n*4); err != nil {
		t.Fatal(err)
	}
	rng, err := buf.MappedRange(0, n*n*4)
	if err != nil {
		t.Fatal(err)
	}
	b := rng.Bytes()
	out := make([]float32, n*n)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	buf.Unmap()
	return out
}
