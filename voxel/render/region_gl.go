//go:build !wgpu

package render

import (
	"unsafe"

	"github.com/memmaker/voxellib/platform/gl"
)

// gpuRegion is a region page's VAO and VBO.
type gpuRegion struct{ vao, vbo uint32 }

func (r *Renderer) newGPURegion(initial int) gpuRegion {
	var g gpuRegion
	gl.GenVertexArrays(1, &g.vao)
	gl.BindVertexArray(g.vao)
	gl.GenBuffers(1, &g.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, g.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, initial*vertexBytes, nil, gl.STATIC_DRAW)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribIPointerWithOffset(0, 4, gl.UNSIGNED_INT, vertexBytes, 0)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.gpu.ebo)
	gl.BindVertexArray(0)
	return g
}

// upload writes vertices into a range no in-flight draw reads: freshly
// allocated spans only, since freed spans are retired for a few frames before
// reuse. That lets it map unsynchronized; glBufferSubData into a buffer the
// GPU is still drawing from makes Apple's driver stall the next draw call.
func (r *region) upload(off int, data []uint32) {
	if len(data) == 0 {
		return
	}
	gl.BindBuffer(gl.ARRAY_BUFFER, r.gpu.vbo)
	ptr := gl.MapBufferRange(gl.ARRAY_BUFFER, off*vertexBytes, len(data)*4,
		gl.MAP_WRITE_BIT|gl.MAP_UNSYNCHRONIZED_BIT|gl.MAP_INVALIDATE_RANGE_BIT)
	if ptr == nil {
		gl.BufferSubData(gl.ARRAY_BUFFER, off*vertexBytes, len(data)*4, gl.Ptr(data))
		return
	}
	copy(unsafe.Slice((*uint32)(ptr), len(data)), data)
	gl.UnmapBuffer(gl.ARRAY_BUFFER)
}

// overwrite rewrites a span that may still be in flight (synchronized).
func (r *region) overwrite(off int, data []uint32) {
	gl.BindBuffer(gl.ARRAY_BUFFER, r.gpu.vbo)
	gl.BufferSubData(gl.ARRAY_BUFFER, off*vertexBytes, len(data)*4, gl.Ptr(data))
}

func (g *gpuRegion) destroy() {
	gl.DeleteBuffers(1, &g.vbo)
	gl.DeleteVertexArrays(1, &g.vao)
}
