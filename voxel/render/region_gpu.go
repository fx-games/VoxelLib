package render

import "github.com/gogpu/wgpu"

// gpuRegion is a region page's vertex buffer.
type gpuRegion struct{ vbo *wgpu.Buffer }

func (r *Renderer) newGPURegion(initial int) gpuRegion {
	return gpuRegion{newBuffer("region page", wgpu.BufferUsageVertex, nil, initial*vertexBytes)}
}

// upload writes vertices; Queue.WriteBuffer is ordered after earlier GPU
// work, so in-flight frames are never disturbed.
func (r *region) upload(off int, data []uint32) {
	if len(data) > 0 {
		writeBuffer(r.gpu.vbo, off*vertexBytes, bytesOf(data))
	}
}

func (r *region) overwrite(off int, data []uint32) { r.upload(off, data) }

func (g *gpuRegion) destroy() { g.vbo.Release() }
