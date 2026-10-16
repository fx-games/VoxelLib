//go:build js

package gl

// WebGL2 behind the go-gl names the engine calls. GL object names are indices
// into objs; pointers are turned into typed-array views onto wasm memory, so
// nothing is copied on the way to the GPU.
//
// What WebGL2 lacks is either emulated or dropped here:
//   - CLAMP_TO_BORDER becomes CLAMP_TO_EDGE, border colours are ignored
//   - PolygonMode (wireframe), DebugMessageCallback, LINE_SMOOTH: no-ops
//   - TIME_ELAPSED queries: never started, results read back as 0
//   - GetTexImage: reads TEXTURE_2D through a framebuffer, others are left zero
//   - geometry shaders: CreateShader returns 0, callers have to skip them
//   - usamplerBuffer: rewritten to a 4096-wide usampler2D (see glhf.TextureBuffer)

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"syscall/js"
	"unsafe"
)

type DebugProc func(source, gltype, id, severity uint32, length int32, message string, userParam unsafe.Pointer)

const (
	ZERO                             = 0
	ONE                              = 1
	FALSE                            = 0
	NONE                             = 0
	NO_ERROR                         = 0
	POINTS                           = 0x0000
	LINES                            = 0x0001
	TRIANGLES                        = 0x0004
	DEPTH_BUFFER_BIT                 = 0x00000100
	COLOR_BUFFER_BIT                 = 0x00004000
	LEQUAL                           = 0x0203
	SRC_ALPHA                        = 0x0302
	ONE_MINUS_SRC_ALPHA              = 0x0303
	DST_ALPHA                        = 0x0304
	ONE_MINUS_DST_ALPHA              = 0x0305
	FRONT_AND_BACK                   = 0x0408
	BACK                             = 0x0405
	CULL_FACE                        = 0x0B44
	DEPTH_TEST                       = 0x0B71
	BLEND                            = 0x0BE2
	SCISSOR_TEST                     = 0x0C11
	LINE_SMOOTH                      = 0x0B20
	MAX_TEXTURE_SIZE                 = 0x0D33
	TEXTURE_2D                       = 0x0DE1
	TEXTURE_BORDER_COLOR             = 0x1004
	UNSIGNED_BYTE                    = 0x1401
	UNSIGNED_SHORT                   = 0x1403
	INT                              = 0x1404
	UNSIGNED_INT                     = 0x1405
	FLOAT                            = 0x1406
	HALF_FLOAT                       = 0x140B
	DEPTH_COMPONENT                  = 0x1902
	RED                              = 0x1903
	RGBA                             = 0x1908
	LINE                             = 0x1B01
	FILL                             = 0x1B02
	VERSION                          = 0x1F02
	NEAREST                          = 0x2600
	LINEAR                           = 0x2601
	LINEAR_MIPMAP_LINEAR             = 0x2703
	TEXTURE_MAG_FILTER               = 0x2800
	TEXTURE_MIN_FILTER               = 0x2801
	TEXTURE_WRAP_S                   = 0x2802
	TEXTURE_WRAP_T                   = 0x2803
	POLYGON_OFFSET_FILL              = 0x8037
	TEXTURE_BINDING_2D               = 0x8069
	TEXTURE_BINDING_3D               = 0x806A
	TEXTURE_3D                       = 0x806F
	TEXTURE_WRAP_R                   = 0x8072
	FUNC_ADD                         = 0x8006
	CLAMP_TO_BORDER                  = 0x812D
	CLAMP_TO_EDGE                    = 0x812F
	TEXTURE0                         = 0x84C0
	TEXTURE1                         = 0x84C1
	TEXTURE2                         = 0x84C2
	TEXTURE_RECTANGLE                = 0x84F5
	TEXTURE_BINDING_RECTANGLE        = 0x84F6
	DEPTH_STENCIL                    = 0x84F9
	UNSIGNED_INT_24_8                = 0x84FA
	TEXTURE_CUBE_MAP                 = 0x8513
	TEXTURE_BINDING_CUBE_MAP         = 0x8514
	TEXTURE_CUBE_MAP_POSITIVE_X      = 0x8515
	QUERY_RESULT                     = 0x8866
	QUERY_RESULT_AVAILABLE           = 0x8867
	MAX_TEXTURE_IMAGE_UNITS          = 0x8872
	ARRAY_BUFFER                     = 0x8892
	ELEMENT_ARRAY_BUFFER             = 0x8893
	ARRAY_BUFFER_BINDING             = 0x8894
	ELEMENT_ARRAY_BUFFER_BINDING     = 0x8895
	STREAM_DRAW                      = 0x88E0
	STATIC_DRAW                      = 0x88E4
	DYNAMIC_DRAW                     = 0x88E8
	TIME_ELAPSED                     = 0x88BF
	DEPTH24_STENCIL8                 = 0x88F0
	TEXTURE_COMPARE_MODE             = 0x884C
	TEXTURE_COMPARE_FUNC             = 0x884D
	COMPARE_REF_TO_TEXTURE           = 0x884E
	RGBA16F                          = 0x881A
	MAX_ARRAY_TEXTURE_LAYERS         = 0x88FF
	FRAGMENT_SHADER                  = 0x8B30
	VERTEX_SHADER                    = 0x8B31
	COMPILE_STATUS                   = 0x8B81
	LINK_STATUS                      = 0x8B82
	VALIDATE_STATUS                  = 0x8B83
	INFO_LOG_LENGTH                  = 0x8B84
	CURRENT_PROGRAM                  = 0x8B8D
	MAX_COMBINED_TEXTURE_IMAGE_UNITS = 0x8B4D
	TEXTURE_2D_ARRAY                 = 0x8C1A
	TEXTURE_BINDING_2D_ARRAY         = 0x8C1D
	TEXTURE_BUFFER                   = 0x8C2A
	TEXTURE_BINDING_BUFFER           = 0x8C2C
	INTERLEAVED_ATTRIBS              = 0x8C8C
	RASTERIZER_DISCARD               = 0x8C89
	TRANSFORM_FEEDBACK_BUFFER        = 0x8C8E
	DEPTH_COMPONENT32F               = 0x8CAC
	FRAMEBUFFER_BINDING              = 0x8CA6
	DRAW_FRAMEBUFFER_BINDING         = 0x8CA6
	READ_FRAMEBUFFER                 = 0x8CA8
	DRAW_FRAMEBUFFER                 = 0x8CA9
	READ_FRAMEBUFFER_BINDING         = 0x8CAA
	COLOR_ATTACHMENT0                = 0x8CE0
	DEPTH_ATTACHMENT                 = 0x8D00
	FRAMEBUFFER                      = 0x8D40
	R8UI                             = 0x8232
	R16UI                            = 0x8234
	RED_INTEGER                      = 0x8D94
	RGBA_INTEGER                     = 0x8D99
	DEPTH_STENCIL_ATTACHMENT         = 0x821A
	DEBUG_OUTPUT                     = 0x92E0
	GEOMETRY_SHADER                  = 0x8DD9
	TRANSFORM_FEEDBACK               = 0x8E22
	VERTEX_ARRAY_BINDING             = 0x85B5
	UNIFORM_BUFFER                   = 0x8A11
	UNIFORM_BUFFER_BINDING           = 0x8A28
	TEXTURE_CUBE_MAP_ARRAY           = 0x9009
	TEXTURE_BINDING_CUBE_MAP_ARRAY   = 0x900A
)

var (
	c      js.Value
	objs   = []js.Value{js.Null()} // name 0 is "none"
	free   []uint32
	u8     = js.Global().Get("Uint8Array")
	u16    = js.Global().Get("Uint16Array")
	u32    = js.Global().Get("Uint32Array")
	i32    = js.Global().Get("Int32Array")
	f32    = js.Global().Get("Float32Array")
	jsNull = js.Null()
)

func add(v js.Value) uint32 {
	if v.IsNull() || v.IsUndefined() {
		return 0
	}
	var id uint32
	if n := len(free); n > 0 {
		id, free = free[n-1], free[:n-1]
		objs[id] = v
	} else {
		id = uint32(len(objs))
		objs = append(objs, v)
	}
	v.Set("__id", id)
	return id
}

func obj(id uint32) js.Value {
	if id == 0 || int(id) >= len(objs) {
		return jsNull
	}
	return objs[id]
}

func del(id uint32, method string) {
	if id == 0 || int(id) >= len(objs) {
		return
	}
	c.Call(method, objs[id])
	objs[id] = jsNull
	free = append(free, id)
}

// view is a typed array over n elements of Go memory at p. The buffer is looked
// up on the JS side: marshalling the arguments can grow wasm memory, which
// detaches any ArrayBuffer fetched beforehand.
func view(ctor js.Value, p unsafe.Pointer, n int) js.Value {
	return viewOf.Invoke(ctor, uintptr(p), n)
}

var viewOf = js.Global().Get("Function").New("C", "p", "n", "return new C(globalThis.__goMem.buffer, p, n)")

func Init() error {
	canvas := js.Global().Get("document").Call("getElementById", "canvas")
	c = canvas.Call("getContext", "webgl2", map[string]any{"antialias": false, "alpha": false, "preserveDrawingBuffer": true})
	if c.IsNull() {
		return fmt.Errorf("WebGL2 is not available")
	}
	c.Call("getExtension", "EXT_color_buffer_float") // HDR / RGBA16F render targets
	c.Call("getExtension", "EXT_color_buffer_half_float")
	c.Call("getExtension", "OES_texture_float_linear")
	return nil
}

func Ptr(data any) unsafe.Pointer {
	if data == nil {
		return nil
	}
	v := reflect.ValueOf(data)
	switch v.Kind() {
	case reflect.Ptr, reflect.UnsafePointer:
		return v.UnsafePointer()
	case reflect.Slice:
		if v.Len() == 0 {
			return nil
		}
		return v.Index(0).Addr().UnsafePointer()
	case reflect.Uintptr:
		return unsafe.Pointer(uintptr(v.Uint()))
	}
	panic(fmt.Errorf("gl.Ptr: unsupported type %T", data))
}

func PtrOffset(offset int) unsafe.Pointer { return unsafe.Pointer(uintptr(offset)) }

func Str(s string) *uint8 {
	if !strings.HasSuffix(s, "\x00") {
		s += "\x00"
	}
	b := []byte(s)
	return &b[0]
}

func GoStr(p *uint8) string {
	if p == nil {
		return ""
	}
	var b []byte
	for q := unsafe.Pointer(p); *(*byte)(q) != 0; q = unsafe.Add(q, 1) {
		b = append(b, *(*byte)(q))
	}
	return string(b)
}

func Strs(strs ...string) (**uint8, func()) {
	ps := make([]*uint8, len(strs))
	for i, s := range strs {
		ps[i] = Str(s)
	}
	return &ps[0], func() {}
}

func ActiveTexture(t uint32)   { c.Call("activeTexture", t) }
func AttachShader(p, s uint32) { c.Call("attachShader", obj(p), obj(s)) }
func BindBuffer(t, b uint32) {
	if t == ARRAY_BUFFER {
		curVBO = b
	}
	c.Call("bindBuffer", t, obj(b))
}
func BindBufferBase(t, i, b uint32)      { c.Call("bindBufferBase", t, i, obj(b)) }
func BindFramebuffer(t, f uint32)        { c.Call("bindFramebuffer", t, obj(f)) }
func BindTexture(t, tex uint32)          { c.Call("bindTexture", t, obj(tex)) }
func BindTransformFeedback(t, id uint32) { c.Call("bindTransformFeedback", t, obj(id)) }
func BindVertexArray(a uint32) {
	curVAO = a
	c.Call("bindVertexArray", obj(a))
}
func BlendEquation(m uint32)              { c.Call("blendEquation", m) }
func BlendFunc(s, d uint32)               { c.Call("blendFunc", s, d) }
func BeginTransformFeedback(m uint32)     { c.Call("beginTransformFeedback", m) }
func EndTransformFeedback()               { c.Call("endTransformFeedback") }
func Clear(m uint32)                      { c.Call("clear", m) }
func ClearColor(r, g, b, a float32)       { c.Call("clearColor", r, g, b, a) }
func ClearDepth(d float64)                { c.Call("clearDepth", d) }
func CompileShader(s uint32)              { c.Call("compileShader", obj(s)) }
func CreateProgram() uint32               { return add(c.Call("createProgram")) }
func CullFace(m uint32)                   { c.Call("cullFace", m) }
func DeleteProgram(p uint32)              { del(p, "deleteProgram") }
func DeleteShader(s uint32)               { del(s, "deleteShader") }
func DepthFunc(f uint32)                  { c.Call("depthFunc", f) }
func DepthMask(f bool)                    { c.Call("depthMask", f) }
func DepthRangef(n, f float32)            { c.Call("depthRange", n, f) }
func DrawArrays(m uint32, first, n int32) { c.Call("drawArrays", m, first, n) }
func DrawArraysInstanced(m uint32, first, n, inst int32) {
	c.Call("drawArraysInstanced", m, first, n, inst)
}
func DrawElementsInstanced(m uint32, n int32, t uint32, indices unsafe.Pointer, inst int32) {
	c.Call("drawElementsInstanced", m, n, t, uintptr(indices), inst)
}
func DrawElementsWithOffset(m uint32, n int32, t uint32, off uintptr) {
	c.Call("drawElements", m, n, t, off)
}
func DrawTransformFeedback(m, id uint32) {} // not in WebGL2; particles are off on js
func EnableVertexAttribArray(i uint32) {
	if i != noAttrib {
		c.Call("enableVertexAttribArray", i)
	}
}

// noAttrib is GetAttribLocation's -1 for an attribute the compiler dropped. GL
// shrugs at it with an error nobody reads; WebGL warns on the console each time.
const noAttrib = 0xFFFFFFFF

func Flush() { c.Call("flush") }
func FramebufferTexture2D(t, a, tt, tex uint32, level int32) {
	c.Call("framebufferTexture2D", t, a, tt, obj(tex), level)
}
func FramebufferTextureLayer(t, a, tex uint32, level, layer int32) {
	c.Call("framebufferTextureLayer", t, a, obj(tex), level, layer)
}
func GenerateMipmap(t uint32)                        { c.Call("generateMipmap", t) }
func GetError() uint32                               { return uint32(c.Call("getError").Int()) }
func IsShader(s uint32) bool                         { return s != 0 && c.Call("isShader", obj(s)).Bool() }
func LineWidth(w float32)                            { c.Call("lineWidth", w) }
func LinkProgram(p uint32)                           { c.Call("linkProgram", obj(p)) }
func PolygonMode(face, mode uint32)                  {}
func PolygonOffset(f, u float32)                     { c.Call("polygonOffset", f, u) }
func ReadBuffer(src uint32)                          { c.Call("readBuffer", src) }
func Scissor(x, y, w, h int32)                       { c.Call("scissor", x, y, w, h) }
func UseProgram(p uint32)                            { c.Call("useProgram", obj(p)) }
func ValidateProgram(p uint32)                       { c.Call("validateProgram", obj(p)) }
func Viewport(x, y, w, h int32)                      { c.Call("viewport", x, y, w, h) }
func VertexAttribDivisor(i, d uint32)                { c.Call("vertexAttribDivisor", i, d) }
func DebugMessageCallback(DebugProc, unsafe.Pointer) {}
func BlitFramebuffer(x0, y0, x1, y1, dx0, dy0, dx1, dy1 int32, mask, filter uint32) {
	c.Call("blitFramebuffer", x0, y0, x1, y1, dx0, dy0, dx1, dy1, mask, filter)
}
func VertexAttribPointerWithOffset(i uint32, size int32, t uint32, norm bool, stride int32, off uintptr) {
	if i == noAttrib {
		return
	}
	c.Call("vertexAttribPointer", i, size, t, norm, stride, off)
	recordAttrib(i, attrib{false, size, t, norm, stride, off, curVBO})
}
func VertexAttribIPointerWithOffset(i uint32, size int32, t uint32, stride int32, off uintptr) {
	if i == noAttrib {
		return
	}
	c.Call("vertexAttribIPointer", i, size, t, stride, off)
	recordAttrib(i, attrib{true, size, t, false, stride, off, curVBO})
}

func Enable(cap uint32) {
	if cap != LINE_SMOOTH && cap != DEBUG_OUTPUT {
		c.Call("enable", cap)
	}
}
func Disable(cap uint32) {
	if cap != LINE_SMOOTH && cap != DEBUG_OUTPUT {
		c.Call("disable", cap)
	}
}

func gen(n int32, out *uint32, method string) {
	ids := unsafe.Slice(out, n)
	for i := range ids {
		ids[i] = add(c.Call(method))
	}
}
func dels(n int32, in *uint32, method string) {
	for _, id := range unsafe.Slice(in, n) {
		del(id, method)
	}
}

func GenBuffers(n int32, b *uint32)            { gen(n, b, "createBuffer") }
func GenFramebuffers(n int32, b *uint32)       { gen(n, b, "createFramebuffer") }
func GenTextures(n int32, b *uint32)           { gen(n, b, "createTexture") }
func GenVertexArrays(n int32, b *uint32)       { gen(n, b, "createVertexArray") }
func GenTransformFeedbacks(n int32, b *uint32) { gen(n, b, "createTransformFeedback") }
func GenQueries(n int32, b *uint32)            { gen(n, b, "createQuery") }
func DeleteBuffers(n int32, b *uint32)         { dels(n, b, "deleteBuffer") }
func DeleteFramebuffers(n int32, b *uint32)    { dels(n, b, "deleteFramebuffer") }
func DeleteTextures(n int32, b *uint32)        { dels(n, b, "deleteTexture") }
func DeleteVertexArrays(n int32, b *uint32)    { dels(n, b, "deleteVertexArray") }

// No timer queries in plain WebGL2: the frame profiler sees zeros.
func BeginQuery(target, id uint32) {}
func EndQuery(target uint32)       {}
func GetQueryObjectuiv(id, pname uint32, out *uint32) {
	if pname == QUERY_RESULT_AVAILABLE {
		*out = 1
	} else {
		*out = 0
	}
}
func GetQueryObjectui64v(id, pname uint32, out *uint64) { *out = 0 }

func DrawBuffer(buf uint32) { c.Call("drawBuffers", []any{buf}) }
func DrawBuffers(n int32, bufs *uint32) {
	l := make([]any, n)
	for i, b := range unsafe.Slice(bufs, n) {
		l[i] = b
	}
	c.Call("drawBuffers", l)
}

func BufferData(t uint32, size int, data unsafe.Pointer, usage uint32) {
	if t == TEXTURE_BUFFER {
		t = ARRAY_BUFFER
	}
	if t == UNIFORM_BUFFER {
		// WebGL refuses to draw when a bound UBO is smaller than the block's
		// std140 size. Desktop drivers let that slide - a block a shader declares
		// but never binds reads binding 0, the smaller Main block - so every UBO
		// gets room for the largest block (Lights, 4 KB) and some.
		c.Call("bufferData", t, max(size+256, 16384), usage)
		if data != nil {
			c.Call("bufferSubData", t, 0, view(u8, data, size))
		}
		return
	}
	if data == nil {
		c.Call("bufferData", t, size, usage)
		return
	}
	c.Call("bufferData", t, view(u8, data, size), usage)
}
func BufferSubData(t uint32, off, size int, data unsafe.Pointer) {
	c.Call("bufferSubData", t, off, view(u8, data, size))
}
func GetBufferSubData(t uint32, off, size int, data unsafe.Pointer) {
	c.Call("getBufferSubData", t, off, view(u8, data, size))
}

// bindings lists the getParameter names that answer with a GL object.
var bindings = map[uint32]bool{
	ARRAY_BUFFER_BINDING: true, ELEMENT_ARRAY_BUFFER_BINDING: true, CURRENT_PROGRAM: true,
	FRAMEBUFFER_BINDING: true, READ_FRAMEBUFFER_BINDING: true, VERTEX_ARRAY_BINDING: true,
	TEXTURE_BINDING_2D: true, TEXTURE_BINDING_3D: true, TEXTURE_BINDING_2D_ARRAY: true,
	TEXTURE_BINDING_CUBE_MAP: true, UNIFORM_BUFFER_BINDING: true,
}

func GetIntegerv(pname uint32, out *int32) {
	switch pname {
	case TEXTURE_BINDING_BUFFER, TEXTURE_BINDING_CUBE_MAP_ARRAY, TEXTURE_BINDING_RECTANGLE:
		*out = int32(texBufferBinding)
		return
	}
	v := c.Call("getParameter", pname)
	switch {
	case v.IsNull() || v.IsUndefined():
		*out = 0
	case bindings[pname]:
		*out = int32(v.Get("__id").Int())
	case v.Type() == js.TypeBoolean:
		if v.Bool() {
			*out = 1
		} else {
			*out = 0
		}
	default:
		*out = int32(v.Int())
	}
}

var texBufferBinding uint32

func GetString(name uint32) *uint8 { return Str(c.Call("getParameter", name).String()) }

func CreateShader(t uint32) uint32 {
	if t == GEOMETRY_SHADER {
		return 0
	}
	return add(c.Call("createShader", t))
}

func ShaderSource(s uint32, count int32, src **uint8, length *int32) {
	var b strings.Builder
	ps := unsafe.Slice(src, count)
	for i, p := range ps {
		if length != nil {
			b.Write(unsafe.Slice(p, unsafe.Slice(length, count)[i]))
		} else {
			b.WriteString(GoStr(p))
		}
	}
	c.Call("shaderSource", obj(s), ToES(strings.TrimRight(b.String(), "\x00")))
}

var (
	versionRE   = regexp.MustCompile(`#version\s+\d+(\s+core)?`)
	texBufferRE = regexp.MustCompile(`uniform\s+usamplerBuffer\s+(\w+)\s*;`)
	noPerspRE   = regexp.MustCompile(`\bnoperspective\s+`)
)

const esHeader = `#version 300 es
precision highp float;
precision highp int;
precision highp sampler2D;
precision highp sampler3D;
precision highp sampler2DArray;
precision highp sampler2DShadow;
precision highp sampler2DArrayShadow;
precision highp samplerCube;
precision highp usampler2D;
precision highp usampler3D;
precision highp isampler2D;
`

// ToES rewrites a GLSL 330/400 core shader to GLSL ES 3.00.
func ToES(src string) string {
	src = versionRE.ReplaceAllString(src, "")
	src = noPerspRE.ReplaceAllString(src, "")
	for _, m := range texBufferRE.FindAllStringSubmatch(src, -1) {
		name := m[1]
		src = strings.ReplaceAll(src, "texelFetch("+name+",", name+"Fetch(")
		src = strings.Replace(src, m[0], "uniform highp usampler2D "+name+";\n"+
			"uvec4 "+name+"Fetch(int i) { return texelFetch("+name+", ivec2(i & 4095, i >> 12), 0); }\n", 1)
	}
	return esHeader + src
}

func GetShaderiv(s, pname uint32, out *int32) {
	switch pname {
	case COMPILE_STATUS:
		*out = b2i(c.Call("getShaderParameter", obj(s), pname).Bool())
		if *out == 0 {
			logShaderErrors(c.Call("getShaderInfoLog", obj(s)).String(), c.Call("getShaderSource", obj(s)).String())
		}
	case INFO_LOG_LENGTH:
		*out = int32(len(c.Call("getShaderInfoLog", obj(s)).String()) + 1)
	default:
		*out = int32(c.Call("getShaderParameter", obj(s), pname).Int())
	}
}

func GetProgramiv(p, pname uint32, out *int32) {
	switch pname {
	case LINK_STATUS, VALIDATE_STATUS:
		*out = b2i(c.Call("getProgramParameter", obj(p), pname).Bool())
	case INFO_LOG_LENGTH:
		l := c.Call("getProgramInfoLog", obj(p)).String()
		*out = 0
		if l != "" {
			*out = int32(len(l) + 1)
		}
	default:
		*out = int32(c.Call("getProgramParameter", obj(p), pname).Int())
	}
}

func writeLog(s string, bufSize int32, length *int32, out *uint8) {
	if bufSize <= 0 || out == nil {
		return
	}
	dst := unsafe.Slice(out, bufSize)
	n := copy(dst[:bufSize-1], s)
	dst[n] = 0
	if length != nil {
		*length = int32(n)
	}
}
func GetShaderInfoLog(s uint32, bufSize int32, length *int32, out *uint8) {
	writeLog(c.Call("getShaderInfoLog", obj(s)).String(), bufSize, length, out)
}
func GetProgramInfoLog(p uint32, bufSize int32, length *int32, out *uint8) {
	writeLog(c.Call("getProgramInfoLog", obj(p)).String(), bufSize, length, out)
}

func b2i(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

func GetAttribLocation(p uint32, name *uint8) int32 {
	return int32(c.Call("getAttribLocation", obj(p), GoStr(name)).Int())
}
func GetUniformBlockIndex(p uint32, name *uint8) uint32 {
	return uint32(c.Call("getUniformBlockIndex", obj(p), GoStr(name)).Int())
}
func UniformBlockBinding(p, idx, binding uint32) {
	c.Call("uniformBlockBinding", obj(p), idx, binding)
}
func TransformFeedbackVaryings(p uint32, count int32, varyings **uint8, mode uint32) {
	l := make([]any, count)
	for i, v := range unsafe.Slice(varyings, count) {
		l[i] = GoStr(v)
	}
	c.Call("transformFeedbackVaryings", obj(p), l, mode)
}

// Uniform locations live in objs too, so -1 can mean "not there" as it does in GL.
func GetUniformLocation(p uint32, name *uint8) int32 {
	l := c.Call("getUniformLocation", obj(p), GoStr(name))
	if l.IsNull() {
		return -1
	}
	return int32(add(l))
}

func loc(l int32) js.Value {
	if l < 0 {
		return jsNull
	}
	return obj(uint32(l))
}

func Uniform1i(l, v int32) { c.Call("uniform1i", loc(l), v) }
func Uniform1fv(l, n int32, v *float32) {
	c.Call("uniform1fv", loc(l), view(f32, unsafe.Pointer(v), int(n)))
}
func Uniform2fv(l, n int32, v *float32) {
	c.Call("uniform2fv", loc(l), view(f32, unsafe.Pointer(v), int(2*n)))
}
func Uniform3fv(l, n int32, v *float32) {
	c.Call("uniform3fv", loc(l), view(f32, unsafe.Pointer(v), int(3*n)))
}
func Uniform4fv(l, n int32, v *float32) {
	c.Call("uniform4fv", loc(l), view(f32, unsafe.Pointer(v), int(4*n)))
}
func Uniform1iv(l, n int32, v *int32) {
	c.Call("uniform1iv", loc(l), view(i32, unsafe.Pointer(v), int(n)))
}
func Uniform2iv(l, n int32, v *int32) {
	c.Call("uniform2iv", loc(l), view(i32, unsafe.Pointer(v), int(2*n)))
}
func Uniform4iv(l, n int32, v *int32) {
	c.Call("uniform4iv", loc(l), view(i32, unsafe.Pointer(v), int(4*n)))
}
func Uniform1uiv(l, n int32, v *uint32) {
	c.Call("uniform1uiv", loc(l), view(u32, unsafe.Pointer(v), int(n)))
}
func mat(fn string, size int, l, n int32, t bool, v *float32) {
	c.Call(fn, loc(l), t, view(f32, unsafe.Pointer(v), size*int(n)))
}
func UniformMatrix2fv(l, n int32, t bool, v *float32)   { mat("uniformMatrix2fv", 4, l, n, t, v) }
func UniformMatrix3fv(l, n int32, t bool, v *float32)   { mat("uniformMatrix3fv", 9, l, n, t, v) }
func UniformMatrix4fv(l, n int32, t bool, v *float32)   { mat("uniformMatrix4fv", 16, l, n, t, v) }
func UniformMatrix2x3fv(l, n int32, t bool, v *float32) { mat("uniformMatrix2x3fv", 6, l, n, t, v) }
func UniformMatrix2x4fv(l, n int32, t bool, v *float32) { mat("uniformMatrix2x4fv", 8, l, n, t, v) }
func UniformMatrix3x2fv(l, n int32, t bool, v *float32) { mat("uniformMatrix3x2fv", 6, l, n, t, v) }
func UniformMatrix3x4fv(l, n int32, t bool, v *float32) { mat("uniformMatrix3x4fv", 12, l, n, t, v) }
func UniformMatrix4x2fv(l, n int32, t bool, v *float32) { mat("uniformMatrix4x2fv", 8, l, n, t, v) }
func UniformMatrix4x3fv(l, n int32, t bool, v *float32) { mat("uniformMatrix4x3fv", 12, l, n, t, v) }

func TexParameteri(t, pname uint32, v int32) {
	if v == CLAMP_TO_BORDER {
		v = CLAMP_TO_EDGE
	}
	if t == TEXTURE_BUFFER {
		return
	}
	c.Call("texParameteri", t, pname, v)
}
func TexParameterfv(t, pname uint32, v *float32) {} // only ever the border colour

func TexBuffer(t, format, buf uint32) {
	panic("gl.TexBuffer: not in WebGL2, use glhf.TextureBuffer's js build")
}

// pixels is a view of what a TexImage call reads: w*h*d texels of format/type.
func pixels(format, xtype uint32, w, h, d int32, p unsafe.Pointer) js.Value {
	if p == nil {
		return jsNull
	}
	comps := 4
	switch format {
	case RED, RED_INTEGER, DEPTH_COMPONENT:
		comps = 1
	case DEPTH_STENCIL:
		comps = 1
	}
	n := comps * int(w) * int(h) * int(d)
	switch xtype {
	case FLOAT:
		return view(f32, p, n)
	case UNSIGNED_SHORT, HALF_FLOAT:
		return view(u16, p, n)
	case UNSIGNED_INT, UNSIGNED_INT_24_8:
		return view(u32, p, n)
	}
	return view(u8, p, n)
}

// sized picks the sized internal format WebGL2 insists on where GL took an unsized one.
func sized(internal int32, format, xtype uint32) int32 {
	switch {
	case internal == DEPTH_COMPONENT && xtype == FLOAT:
		return DEPTH_COMPONENT32F
	case internal == DEPTH_COMPONENT:
		return 0x81A6 // DEPTH_COMPONENT24
	case internal == DEPTH_STENCIL:
		return DEPTH24_STENCIL8
	case internal == RGBA && xtype == FLOAT:
		return 0x8814 // RGBA32F
	case internal == RGBA:
		return 0x8058 // RGBA8
	}
	return internal
}

func TexImage2D(t uint32, level, internal, w, h, border int32, format, xtype uint32, p unsafe.Pointer) {
	if t == TEXTURE_2D && level == 0 {
		texSize[uint32(c.Call("getParameter", TEXTURE_BINDING_2D).Get("__id").Int())] = [2]int32{w, h}
	}
	c.Call("texImage2D", t, level, sized(internal, format, xtype), w, h, 0, format, xtype, pixels(format, xtype, w, h, 1, p))
}
func TexImage3D(t uint32, level, internal, w, h, d, border int32, format, xtype uint32, p unsafe.Pointer) {
	c.Call("texImage3D", t, level, sized(internal, format, xtype), w, h, d, 0, format, xtype, pixels(format, xtype, w, h, d, p))
}
func TexSubImage2D(t uint32, level, x, y, w, h int32, format, xtype uint32, p unsafe.Pointer) {
	c.Call("texSubImage2D", t, level, x, y, w, h, format, xtype, pixels(format, xtype, w, h, 1, p))
}
func TexSubImage3D(t uint32, level, x, y, z, w, h, d int32, format, xtype uint32, p unsafe.Pointer) {
	c.Call("texSubImage3D", t, level, x, y, z, w, h, d, format, xtype, pixels(format, xtype, w, h, d, p))
}

func ReadPixels(x, y, w, h int32, format, xtype uint32, p unsafe.Pointer) {
	c.Call("readPixels", x, y, w, h, format, xtype, pixels(format, xtype, w, h, 1, p))
}

// GetTexImage reads level 0 of the bound TEXTURE_2D through a scratch
// framebuffer (WebGL2 has no glGetTexImage); any other target is left zero.
func GetTexImage(t uint32, level int32, format, xtype uint32, p unsafe.Pointer) {
	if t != TEXTURE_2D || level != 0 {
		return
	}
	tex := c.Call("getParameter", TEXTURE_BINDING_2D)
	size := texSize[uint32(tex.Get("__id").Int())]
	prev := c.Call("getParameter", READ_FRAMEBUFFER_BINDING)
	fb := c.Call("createFramebuffer")
	c.Call("bindFramebuffer", READ_FRAMEBUFFER, fb)
	c.Call("framebufferTexture2D", READ_FRAMEBUFFER, COLOR_ATTACHMENT0, TEXTURE_2D, tex, 0)
	c.Call("readPixels", 0, 0, size[0], size[1], format, xtype, pixels(format, xtype, size[0], size[1], 1, p))
	c.Call("bindFramebuffer", READ_FRAMEBUFFER, prev)
	c.Call("deleteFramebuffer", fb)
}

// texSize is what GetTexImage needs and WebGL cannot be asked: each 2D texture's size.
var texSize = map[uint32][2]int32{}

var errLineRE = regexp.MustCompile(`ERROR: \d+:(\d+):`)

// logShaderErrors prints each compile error with the line of rewritten source it names.
func logShaderErrors(log, src string) {
	lines := strings.Split(src, "\n")
	var b strings.Builder
	for _, l := range strings.Split(log, "\n") {
		b.WriteString(l + "\n")
		if m := errLineRE.FindStringSubmatch(l); m != nil {
			var n int
			fmt.Sscan(m[1], &n)
			if n > 0 && n <= len(lines) {
				b.WriteString("    > " + strings.TrimSpace(lines[n-1]) + "\n")
			}
		}
	}
	println(b.String())
}

// Added for voxellib's render package.
const (
	CCW                      = 0x0901
	MAP_WRITE_BIT            = 0x0002
	MAP_INVALIDATE_RANGE_BIT = 0x0004
	MAP_UNSYNCHRONIZED_BIT   = 0x0020
	NEAREST_MIPMAP_LINEAR    = 0x2702
	ONE_MINUS_DST_COLOR      = 0x0307
	REPEAT                   = 0x2901
	RGBA8                    = 0x8058
	TEXTURE_MAX_LEVEL        = 0x813D
	TRIANGLE_STRIP           = 0x0005
	UNPACK_ALIGNMENT         = 0x0CF5
)

func FrontFace(mode uint32)                     { c.Call("frontFace", mode) }
func PixelStorei(pname uint32, param int32)     { c.Call("pixelStorei", pname, param) }
func Uniform1f(l int32, v0 float32)             { c.Call("uniform1f", loc(l), v0) }
func Uniform2f(l int32, v0, v1 float32)         { c.Call("uniform2f", loc(l), v0, v1) }
func Uniform3f(l int32, v0, v1, v2 float32)     { c.Call("uniform3f", loc(l), v0, v1, v2) }
func Uniform4f(l int32, v0, v1, v2, v3 float32) { c.Call("uniform4f", loc(l), v0, v1, v2, v3) }

// WebGL has no buffer mapping; callers fall back to BufferSubData on nil.
func MapBufferRange(target uint32, offset, length int, access uint32) unsafe.Pointer { return nil }
func UnmapBuffer(target uint32) bool                                                 { return true }

// attrib is a vertex attribute pointer as last set on a VAO, kept so the
// multi-draw fallback can re-point it at a base vertex.
type attrib struct {
	integer bool
	size    int32
	xtype   uint32
	norm    bool
	stride  int32
	off     uintptr
	vbo     uint32
}

var (
	curVAO, curVBO uint32
	vaoAttribs     = map[uint32]map[uint32]attrib{}
	baseVertexExt  js.Value
	baseVertexInit bool
)

func recordAttrib(i uint32, a attrib) {
	m := vaoAttribs[curVAO]
	if m == nil {
		m = map[uint32]attrib{}
		vaoAttribs[curVAO] = m
	}
	m[i] = a
}

// MultiDrawElementsBaseVertex has no WebGL2 equivalent. With the base-vertex
// extension it is a loop of single draws; without it the attribute pointers
// of the bound VAO are re-pointed at each base vertex before a plain
// drawElements. Shaders that read gl_VertexID only see the low two bits
// unchanged in that case, which is all the quad corner lookup needs since
// every base is a multiple of four.
func MultiDrawElementsBaseVertex(mode uint32, count *int32, xtype uint32, indices *unsafe.Pointer, drawcount int32, basevertex *int32) {
	if !baseVertexInit {
		baseVertexInit = true
		baseVertexExt = c.Call("getExtension", "WEBGL_draw_instanced_base_vertex_base_instance")
	}
	counts := unsafe.Slice(count, drawcount)
	offs := unsafe.Slice(indices, drawcount)
	bases := unsafe.Slice(basevertex, drawcount)
	if baseVertexExt.Truthy() {
		for i := range counts {
			baseVertexExt.Call("drawElementsInstancedBaseVertexBaseInstanceWEBGL", mode, counts[i], xtype, uintptr(offs[i]), 1, bases[i], 0)
		}
		return
	}
	attribs := vaoAttribs[curVAO]
	for i := range counts {
		for idx, a := range attribs {
			c.Call("bindBuffer", ARRAY_BUFFER, obj(a.vbo))
			off := a.off + uintptr(bases[i])*uintptr(a.stride)
			if a.integer {
				c.Call("vertexAttribIPointer", idx, a.size, a.xtype, a.stride, off)
			} else {
				c.Call("vertexAttribPointer", idx, a.size, a.xtype, a.norm, a.stride, off)
			}
		}
		c.Call("drawElements", mode, counts[i], xtype, uintptr(offs[i]))
	}
	for idx, a := range attribs {
		c.Call("bindBuffer", ARRAY_BUFFER, obj(a.vbo))
		if a.integer {
			c.Call("vertexAttribIPointer", idx, a.size, a.xtype, a.stride, a.off)
		} else {
			c.Call("vertexAttribPointer", idx, a.size, a.xtype, a.norm, a.stride, a.off)
		}
	}
	c.Call("bindBuffer", ARRAY_BUFFER, obj(curVBO))
}
