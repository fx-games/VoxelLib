//go:build !js

// Package gl is the slice of OpenGL the engine uses. On the desktop it is go-gl;
// on js it is WebGL2 (gl_js.go).
package gl

import (
	"unsafe"

	gl "github.com/go-gl/gl/v3.3-core/gl"
)

var _ unsafe.Pointer

type DebugProc = gl.DebugProc

const (
	ARRAY_BUFFER                     = gl.ARRAY_BUFFER
	ARRAY_BUFFER_BINDING             = gl.ARRAY_BUFFER_BINDING
	BACK                             = gl.BACK
	BLEND                            = gl.BLEND
	CLAMP_TO_BORDER                  = gl.CLAMP_TO_BORDER
	CLAMP_TO_EDGE                    = gl.CLAMP_TO_EDGE
	COLOR_ATTACHMENT0                = gl.COLOR_ATTACHMENT0
	COLOR_BUFFER_BIT                 = gl.COLOR_BUFFER_BIT
	COMPARE_REF_TO_TEXTURE           = gl.COMPARE_REF_TO_TEXTURE
	COMPILE_STATUS                   = gl.COMPILE_STATUS
	CULL_FACE                        = gl.CULL_FACE
	CURRENT_PROGRAM                  = gl.CURRENT_PROGRAM
	DEBUG_OUTPUT                     = gl.DEBUG_OUTPUT
	DEPTH24_STENCIL8                 = gl.DEPTH24_STENCIL8
	DEPTH_ATTACHMENT                 = gl.DEPTH_ATTACHMENT
	DEPTH_BUFFER_BIT                 = gl.DEPTH_BUFFER_BIT
	DEPTH_COMPONENT                  = gl.DEPTH_COMPONENT
	DEPTH_COMPONENT32F               = gl.DEPTH_COMPONENT32F
	DEPTH_STENCIL                    = gl.DEPTH_STENCIL
	DEPTH_STENCIL_ATTACHMENT         = gl.DEPTH_STENCIL_ATTACHMENT
	DEPTH_TEST                       = gl.DEPTH_TEST
	DRAW_FRAMEBUFFER                 = gl.DRAW_FRAMEBUFFER
	DRAW_FRAMEBUFFER_BINDING         = gl.DRAW_FRAMEBUFFER_BINDING
	DST_ALPHA                        = gl.DST_ALPHA
	DYNAMIC_DRAW                     = gl.DYNAMIC_DRAW
	ELEMENT_ARRAY_BUFFER             = gl.ELEMENT_ARRAY_BUFFER
	ELEMENT_ARRAY_BUFFER_BINDING     = gl.ELEMENT_ARRAY_BUFFER_BINDING
	FALSE                            = gl.FALSE
	FILL                             = gl.FILL
	FLOAT                            = gl.FLOAT
	FRAGMENT_SHADER                  = gl.FRAGMENT_SHADER
	FRAMEBUFFER                      = gl.FRAMEBUFFER
	FRAMEBUFFER_BINDING              = gl.FRAMEBUFFER_BINDING
	FRONT_AND_BACK                   = gl.FRONT_AND_BACK
	FUNC_ADD                         = gl.FUNC_ADD
	GEOMETRY_SHADER                  = gl.GEOMETRY_SHADER
	INFO_LOG_LENGTH                  = gl.INFO_LOG_LENGTH
	INT                              = gl.INT
	INTERLEAVED_ATTRIBS              = gl.INTERLEAVED_ATTRIBS
	LEQUAL                           = gl.LEQUAL
	LINE                             = gl.LINE
	LINEAR                           = gl.LINEAR
	LINEAR_MIPMAP_LINEAR             = gl.LINEAR_MIPMAP_LINEAR
	LINES                            = gl.LINES
	LINE_SMOOTH                      = gl.LINE_SMOOTH
	LINK_STATUS                      = gl.LINK_STATUS
	MAX_ARRAY_TEXTURE_LAYERS         = gl.MAX_ARRAY_TEXTURE_LAYERS
	MAX_COMBINED_TEXTURE_IMAGE_UNITS = gl.MAX_COMBINED_TEXTURE_IMAGE_UNITS
	MAX_TEXTURE_IMAGE_UNITS          = gl.MAX_TEXTURE_IMAGE_UNITS
	MAX_TEXTURE_SIZE                 = gl.MAX_TEXTURE_SIZE
	NEAREST                          = gl.NEAREST
	NONE                             = gl.NONE
	NO_ERROR                         = gl.NO_ERROR
	ONE                              = gl.ONE
	ONE_MINUS_DST_ALPHA              = gl.ONE_MINUS_DST_ALPHA
	ONE_MINUS_SRC_ALPHA              = gl.ONE_MINUS_SRC_ALPHA
	POINTS                           = gl.POINTS
	POLYGON_OFFSET_FILL              = gl.POLYGON_OFFSET_FILL
	QUERY_RESULT                     = gl.QUERY_RESULT
	QUERY_RESULT_AVAILABLE           = gl.QUERY_RESULT_AVAILABLE
	R16UI                            = gl.R16UI
	R8UI                             = gl.R8UI
	RASTERIZER_DISCARD               = gl.RASTERIZER_DISCARD
	READ_FRAMEBUFFER                 = gl.READ_FRAMEBUFFER
	READ_FRAMEBUFFER_BINDING         = gl.READ_FRAMEBUFFER_BINDING
	RED_INTEGER                      = gl.RED_INTEGER
	RGBA                             = gl.RGBA
	RGBA16F                          = gl.RGBA16F
	RGBA_INTEGER                     = gl.RGBA_INTEGER
	SCISSOR_TEST                     = gl.SCISSOR_TEST
	SRC_ALPHA                        = gl.SRC_ALPHA
	STATIC_DRAW                      = gl.STATIC_DRAW
	STREAM_DRAW                      = gl.STREAM_DRAW
	TEXTURE0                         = gl.TEXTURE0
	TEXTURE1                         = gl.TEXTURE1
	TEXTURE2                         = gl.TEXTURE2
	TEXTURE_2D                       = gl.TEXTURE_2D
	TEXTURE_2D_ARRAY                 = gl.TEXTURE_2D_ARRAY
	TEXTURE_3D                       = gl.TEXTURE_3D
	TEXTURE_BINDING_2D               = gl.TEXTURE_BINDING_2D
	TEXTURE_BINDING_2D_ARRAY         = gl.TEXTURE_BINDING_2D_ARRAY
	TEXTURE_BINDING_3D               = gl.TEXTURE_BINDING_3D
	TEXTURE_BINDING_BUFFER           = gl.TEXTURE_BINDING_BUFFER
	TEXTURE_BINDING_CUBE_MAP         = gl.TEXTURE_BINDING_CUBE_MAP
	TEXTURE_BINDING_CUBE_MAP_ARRAY   = gl.TEXTURE_BINDING_CUBE_MAP_ARRAY
	TEXTURE_BINDING_RECTANGLE        = gl.TEXTURE_BINDING_RECTANGLE
	TEXTURE_BORDER_COLOR             = gl.TEXTURE_BORDER_COLOR
	TEXTURE_BUFFER                   = gl.TEXTURE_BUFFER
	TEXTURE_COMPARE_FUNC             = gl.TEXTURE_COMPARE_FUNC
	TEXTURE_COMPARE_MODE             = gl.TEXTURE_COMPARE_MODE
	TEXTURE_CUBE_MAP                 = gl.TEXTURE_CUBE_MAP
	TEXTURE_CUBE_MAP_ARRAY           = gl.TEXTURE_CUBE_MAP_ARRAY
	TEXTURE_CUBE_MAP_POSITIVE_X      = gl.TEXTURE_CUBE_MAP_POSITIVE_X
	TEXTURE_MAG_FILTER               = gl.TEXTURE_MAG_FILTER
	TEXTURE_MIN_FILTER               = gl.TEXTURE_MIN_FILTER
	TEXTURE_RECTANGLE                = gl.TEXTURE_RECTANGLE
	TEXTURE_WRAP_R                   = gl.TEXTURE_WRAP_R
	TEXTURE_WRAP_S                   = gl.TEXTURE_WRAP_S
	TEXTURE_WRAP_T                   = gl.TEXTURE_WRAP_T
	TIME_ELAPSED                     = gl.TIME_ELAPSED
	TRANSFORM_FEEDBACK               = gl.TRANSFORM_FEEDBACK
	TRANSFORM_FEEDBACK_BUFFER        = gl.TRANSFORM_FEEDBACK_BUFFER
	TRIANGLES                        = gl.TRIANGLES
	UNIFORM_BUFFER                   = gl.UNIFORM_BUFFER
	UNIFORM_BUFFER_BINDING           = gl.UNIFORM_BUFFER_BINDING
	UNSIGNED_BYTE                    = gl.UNSIGNED_BYTE
	UNSIGNED_INT                     = gl.UNSIGNED_INT
	UNSIGNED_INT_24_8                = gl.UNSIGNED_INT_24_8
	VALIDATE_STATUS                  = gl.VALIDATE_STATUS
	VERSION                          = gl.VERSION
	VERTEX_ARRAY_BINDING             = gl.VERTEX_ARRAY_BINDING
	VERTEX_SHADER                    = gl.VERTEX_SHADER
	ZERO                             = gl.ZERO
)

func ActiveTexture(texture uint32)                { gl.ActiveTexture(texture) }
func AttachShader(program uint32, shader uint32)  { gl.AttachShader(program, shader) }
func BeginQuery(target uint32, id uint32)         { gl.BeginQuery(target, id) }
func BeginTransformFeedback(primitiveMode uint32) { gl.BeginTransformFeedback(primitiveMode) }
func BindBuffer(target uint32, buffer uint32)     { gl.BindBuffer(target, buffer) }
func BindBufferBase(target uint32, index uint32, buffer uint32) {
	gl.BindBufferBase(target, index, buffer)
}
func BindFramebuffer(target uint32, framebuffer uint32) { gl.BindFramebuffer(target, framebuffer) }
func BindTexture(target uint32, texture uint32)         { gl.BindTexture(target, texture) }
func BindTransformFeedback(target uint32, id uint32)    { gl.BindTransformFeedback(target, id) }
func BindVertexArray(array uint32)                      { gl.BindVertexArray(array) }
func BlendEquation(mode uint32)                         { gl.BlendEquation(mode) }
func BlendFunc(sfactor uint32, dfactor uint32)          { gl.BlendFunc(sfactor, dfactor) }
func BlitFramebuffer(srcX0 int32, srcY0 int32, srcX1 int32, srcY1 int32, dstX0 int32, dstY0 int32, dstX1 int32, dstY1 int32, mask uint32, filter uint32) {
	gl.BlitFramebuffer(srcX0, srcY0, srcX1, srcY1, dstX0, dstY0, dstX1, dstY1, mask, filter)
}
func BufferData(target uint32, size int, data unsafe.Pointer, usage uint32) {
	gl.BufferData(target, size, data, usage)
}
func BufferSubData(target uint32, offset int, size int, data unsafe.Pointer) {
	gl.BufferSubData(target, offset, size, data)
}
func Clear(mask uint32) { gl.Clear(mask) }
func ClearColor(red float32, green float32, blue float32, alpha float32) {
	gl.ClearColor(red, green, blue, alpha)
}
func ClearDepth(depth float64)         { gl.ClearDepth(depth) }
func CompileShader(shader uint32)      { gl.CompileShader(shader) }
func CreateProgram() uint32            { return gl.CreateProgram() }
func CreateShader(xtype uint32) uint32 { return gl.CreateShader(xtype) }
func CullFace(mode uint32)             { gl.CullFace(mode) }
func DebugMessageCallback(callback DebugProc, userParam unsafe.Pointer) {
	gl.DebugMessageCallback(callback, userParam)
}
func DeleteBuffers(n int32, buffers *uint32)           { gl.DeleteBuffers(n, buffers) }
func DeleteFramebuffers(n int32, framebuffers *uint32) { gl.DeleteFramebuffers(n, framebuffers) }
func DeleteProgram(program uint32)                     { gl.DeleteProgram(program) }
func DeleteShader(shader uint32)                       { gl.DeleteShader(shader) }
func DeleteTextures(n int32, textures *uint32)         { gl.DeleteTextures(n, textures) }
func DeleteVertexArrays(n int32, arrays *uint32)       { gl.DeleteVertexArrays(n, arrays) }
func DepthFunc(xfunc uint32)                           { gl.DepthFunc(xfunc) }
func DepthMask(flag bool)                              { gl.DepthMask(flag) }
func DepthRangef(n float32, f float32)                 { gl.DepthRangef(n, f) }
func Disable(cap uint32)                               { gl.Disable(cap) }
func DrawArrays(mode uint32, first int32, count int32) { gl.DrawArrays(mode, first, count) }
func DrawArraysInstanced(mode uint32, first int32, count int32, instancecount int32) {
	gl.DrawArraysInstanced(mode, first, count, instancecount)
}
func DrawBuffer(buf uint32)             { gl.DrawBuffer(buf) }
func DrawBuffers(n int32, bufs *uint32) { gl.DrawBuffers(n, bufs) }
func DrawElementsInstanced(mode uint32, count int32, xtype uint32, indices unsafe.Pointer, instancecount int32) {
	gl.DrawElementsInstanced(mode, count, xtype, indices, instancecount)
}
func DrawElementsWithOffset(mode uint32, count int32, xtype uint32, indices uintptr) {
	gl.DrawElementsWithOffset(mode, count, xtype, indices)
}
func DrawTransformFeedback(mode uint32, id uint32) { gl.DrawTransformFeedback(mode, id) }
func Enable(cap uint32)                            { gl.Enable(cap) }
func EnableVertexAttribArray(index uint32)         { gl.EnableVertexAttribArray(index) }
func EndQuery(target uint32)                       { gl.EndQuery(target) }
func EndTransformFeedback()                        { gl.EndTransformFeedback() }
func Flush()                                       { gl.Flush() }
func FramebufferTexture2D(target uint32, attachment uint32, textarget uint32, texture uint32, level int32) {
	gl.FramebufferTexture2D(target, attachment, textarget, texture, level)
}
func FramebufferTextureLayer(target uint32, attachment uint32, texture uint32, level int32, layer int32) {
	gl.FramebufferTextureLayer(target, attachment, texture, level, layer)
}
func GenBuffers(n int32, buffers *uint32)                 { gl.GenBuffers(n, buffers) }
func GenFramebuffers(n int32, framebuffers *uint32)       { gl.GenFramebuffers(n, framebuffers) }
func GenQueries(n int32, ids *uint32)                     { gl.GenQueries(n, ids) }
func GenTextures(n int32, textures *uint32)               { gl.GenTextures(n, textures) }
func GenTransformFeedbacks(n int32, ids *uint32)          { gl.GenTransformFeedbacks(n, ids) }
func GenVertexArrays(n int32, arrays *uint32)             { gl.GenVertexArrays(n, arrays) }
func GenerateMipmap(target uint32)                        { gl.GenerateMipmap(target) }
func GetAttribLocation(program uint32, name *uint8) int32 { return gl.GetAttribLocation(program, name) }
func GetBufferSubData(target uint32, offset int, size int, data unsafe.Pointer) {
	gl.GetBufferSubData(target, offset, size, data)
}
func GetError() uint32                      { return gl.GetError() }
func GetIntegerv(pname uint32, data *int32) { gl.GetIntegerv(pname, data) }
func GetProgramInfoLog(program uint32, bufSize int32, length *int32, infoLog *uint8) {
	gl.GetProgramInfoLog(program, bufSize, length, infoLog)
}
func GetProgramiv(program uint32, pname uint32, params *int32) {
	gl.GetProgramiv(program, pname, params)
}
func GetQueryObjectui64v(id uint32, pname uint32, params *uint64) {
	gl.GetQueryObjectui64v(id, pname, params)
}
func GetQueryObjectuiv(id uint32, pname uint32, params *uint32) {
	gl.GetQueryObjectuiv(id, pname, params)
}
func GetShaderInfoLog(shader uint32, bufSize int32, length *int32, infoLog *uint8) {
	gl.GetShaderInfoLog(shader, bufSize, length, infoLog)
}
func GetShaderiv(shader uint32, pname uint32, params *int32) { gl.GetShaderiv(shader, pname, params) }
func GetString(name uint32) *uint8                           { return gl.GetString(name) }
func GetTexImage(target uint32, level int32, format uint32, xtype uint32, pixels unsafe.Pointer) {
	gl.GetTexImage(target, level, format, xtype, pixels)
}
func GetUniformBlockIndex(program uint32, uniformBlockName *uint8) uint32 {
	return gl.GetUniformBlockIndex(program, uniformBlockName)
}
func GetUniformLocation(program uint32, name *uint8) int32 {
	return gl.GetUniformLocation(program, name)
}
func GoStr(cstr *uint8) string                    { return gl.GoStr(cstr) }
func Init() error                                 { return gl.Init() }
func IsShader(shader uint32) bool                 { return gl.IsShader(shader) }
func LineWidth(width float32)                     { gl.LineWidth(width) }
func LinkProgram(program uint32)                  { gl.LinkProgram(program) }
func PolygonMode(face uint32, mode uint32)        { gl.PolygonMode(face, mode) }
func PolygonOffset(factor float32, units float32) { gl.PolygonOffset(factor, units) }
func Ptr(data interface{}) unsafe.Pointer         { return gl.Ptr(data) }
func PtrOffset(offset int) unsafe.Pointer         { return gl.PtrOffset(offset) }
func ReadBuffer(src uint32)                       { gl.ReadBuffer(src) }
func ReadPixels(x int32, y int32, width int32, height int32, format uint32, xtype uint32, pixels unsafe.Pointer) {
	gl.ReadPixels(x, y, width, height, format, xtype, pixels)
}
func Scissor(x int32, y int32, width int32, height int32) { gl.Scissor(x, y, width, height) }
func ShaderSource(shader uint32, count int32, xstring **uint8, length *int32) {
	gl.ShaderSource(shader, count, xstring, length)
}
func Str(str string) *uint8                            { return gl.Str(str) }
func Strs(strs ...string) (cstrs **uint8, free func()) { return gl.Strs(strs...) }
func TexBuffer(target uint32, internalformat uint32, buffer uint32) {
	gl.TexBuffer(target, internalformat, buffer)
}
func TexImage2D(target uint32, level int32, internalformat int32, width int32, height int32, border int32, format uint32, xtype uint32, pixels unsafe.Pointer) {
	gl.TexImage2D(target, level, internalformat, width, height, border, format, xtype, pixels)
}
func TexImage3D(target uint32, level int32, internalformat int32, width int32, height int32, depth int32, border int32, format uint32, xtype uint32, pixels unsafe.Pointer) {
	gl.TexImage3D(target, level, internalformat, width, height, depth, border, format, xtype, pixels)
}
func TexParameterfv(target uint32, pname uint32, params *float32) {
	gl.TexParameterfv(target, pname, params)
}
func TexParameteri(target uint32, pname uint32, param int32) { gl.TexParameteri(target, pname, param) }
func TexSubImage2D(target uint32, level int32, xoffset int32, yoffset int32, width int32, height int32, format uint32, xtype uint32, pixels unsafe.Pointer) {
	gl.TexSubImage2D(target, level, xoffset, yoffset, width, height, format, xtype, pixels)
}
func TexSubImage3D(target uint32, level int32, xoffset int32, yoffset int32, zoffset int32, width int32, height int32, depth int32, format uint32, xtype uint32, pixels unsafe.Pointer) {
	gl.TexSubImage3D(target, level, xoffset, yoffset, zoffset, width, height, depth, format, xtype, pixels)
}
func TransformFeedbackVaryings(program uint32, count int32, varyings **uint8, bufferMode uint32) {
	gl.TransformFeedbackVaryings(program, count, varyings, bufferMode)
}
func Uniform1fv(location int32, count int32, value *float32) { gl.Uniform1fv(location, count, value) }
func Uniform1i(location int32, v0 int32)                     { gl.Uniform1i(location, v0) }
func Uniform1iv(location int32, count int32, value *int32)   { gl.Uniform1iv(location, count, value) }
func Uniform1uiv(location int32, count int32, value *uint32) { gl.Uniform1uiv(location, count, value) }
func Uniform2fv(location int32, count int32, value *float32) { gl.Uniform2fv(location, count, value) }
func Uniform2iv(location int32, count int32, value *int32)   { gl.Uniform2iv(location, count, value) }
func Uniform3fv(location int32, count int32, value *float32) { gl.Uniform3fv(location, count, value) }
func Uniform4fv(location int32, count int32, value *float32) { gl.Uniform4fv(location, count, value) }
func Uniform4iv(location int32, count int32, value *int32)   { gl.Uniform4iv(location, count, value) }
func UniformBlockBinding(program uint32, uniformBlockIndex uint32, uniformBlockBinding uint32) {
	gl.UniformBlockBinding(program, uniformBlockIndex, uniformBlockBinding)
}
func UniformMatrix2fv(location int32, count int32, transpose bool, value *float32) {
	gl.UniformMatrix2fv(location, count, transpose, value)
}
func UniformMatrix2x3fv(location int32, count int32, transpose bool, value *float32) {
	gl.UniformMatrix2x3fv(location, count, transpose, value)
}
func UniformMatrix2x4fv(location int32, count int32, transpose bool, value *float32) {
	gl.UniformMatrix2x4fv(location, count, transpose, value)
}
func UniformMatrix3fv(location int32, count int32, transpose bool, value *float32) {
	gl.UniformMatrix3fv(location, count, transpose, value)
}
func UniformMatrix3x2fv(location int32, count int32, transpose bool, value *float32) {
	gl.UniformMatrix3x2fv(location, count, transpose, value)
}
func UniformMatrix3x4fv(location int32, count int32, transpose bool, value *float32) {
	gl.UniformMatrix3x4fv(location, count, transpose, value)
}
func UniformMatrix4fv(location int32, count int32, transpose bool, value *float32) {
	gl.UniformMatrix4fv(location, count, transpose, value)
}
func UniformMatrix4x2fv(location int32, count int32, transpose bool, value *float32) {
	gl.UniformMatrix4x2fv(location, count, transpose, value)
}
func UniformMatrix4x3fv(location int32, count int32, transpose bool, value *float32) {
	gl.UniformMatrix4x3fv(location, count, transpose, value)
}
func UseProgram(program uint32)                        { gl.UseProgram(program) }
func ValidateProgram(program uint32)                   { gl.ValidateProgram(program) }
func VertexAttribDivisor(index uint32, divisor uint32) { gl.VertexAttribDivisor(index, divisor) }
func VertexAttribIPointerWithOffset(index uint32, size int32, xtype uint32, stride int32, offset uintptr) {
	gl.VertexAttribIPointerWithOffset(index, size, xtype, stride, offset)
}
func VertexAttribPointerWithOffset(index uint32, size int32, xtype uint32, normalized bool, stride int32, offset uintptr) {
	gl.VertexAttribPointerWithOffset(index, size, xtype, normalized, stride, offset)
}
func Viewport(x int32, y int32, width int32, height int32) { gl.Viewport(x, y, width, height) }

// Added for voxellib's render package.
const (
	CCW                      = gl.CCW
	MAP_INVALIDATE_RANGE_BIT = gl.MAP_INVALIDATE_RANGE_BIT
	MAP_UNSYNCHRONIZED_BIT   = gl.MAP_UNSYNCHRONIZED_BIT
	MAP_WRITE_BIT            = gl.MAP_WRITE_BIT
	NEAREST_MIPMAP_LINEAR    = gl.NEAREST_MIPMAP_LINEAR
	ONE_MINUS_DST_COLOR      = gl.ONE_MINUS_DST_COLOR
	REPEAT                   = gl.REPEAT
	RGBA8                    = gl.RGBA8
	TEXTURE_MAX_LEVEL        = gl.TEXTURE_MAX_LEVEL
	TRIANGLE_STRIP           = gl.TRIANGLE_STRIP
	UNPACK_ALIGNMENT         = gl.UNPACK_ALIGNMENT
	UNSIGNED_SHORT           = gl.UNSIGNED_SHORT
)

func FrontFace(mode uint32)                     { gl.FrontFace(mode) }
func PixelStorei(pname uint32, param int32)     { gl.PixelStorei(pname, param) }
func Uniform1f(l int32, v0 float32)             { gl.Uniform1f(l, v0) }
func Uniform2f(l int32, v0, v1 float32)         { gl.Uniform2f(l, v0, v1) }
func Uniform3f(l int32, v0, v1, v2 float32)     { gl.Uniform3f(l, v0, v1, v2) }
func Uniform4f(l int32, v0, v1, v2, v3 float32) { gl.Uniform4f(l, v0, v1, v2, v3) }
func MapBufferRange(target uint32, offset int, length int, access uint32) unsafe.Pointer {
	return gl.MapBufferRange(target, offset, length, access)
}
func UnmapBuffer(target uint32) bool { return gl.UnmapBuffer(target) }
func MultiDrawElementsBaseVertex(mode uint32, count *int32, xtype uint32, indices *unsafe.Pointer, drawcount int32, basevertex *int32) {
	gl.MultiDrawElementsBaseVertex(mode, count, xtype, indices, drawcount, basevertex)
}
