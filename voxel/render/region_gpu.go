package render

import "github.com/gogpu/wgpu"

// gpuRegion is a region page's vertex buffer.
type gpuRegion struct{ vbo *wgpu.Buffer }

func (r *Renderer) newGPURegion(initial int) gpuRegion {
	return gpuRegion{newBuffer("region page", wgpu.BufferUsageVertex|wgpu.BufferUsageCopySrc, nil, initial*vertexBytes)}
}

// grow moves the page into a buffer of newCap vertices with a GPU copy in
// its own submission: earlier writes land in the old buffer first, later
// ones go to the new one. Frames in flight keep the old buffer alive (wgpu
// refcounts it).
func (r *region) grow(newCap int) {
	if r.gpu.vbo != nil {
		old := r.gpu.vbo
		r.gpu.vbo = newBuffer("region page", wgpu.BufferUsageVertex|wgpu.BufferUsageCopySrc, nil, newCap*vertexBytes)
		enc, err := dev.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "region grow"})
		if err != nil {
			panic(err)
		}
		enc.CopyBufferToBuffer(old, 0, r.gpu.vbo, 0, uint64(r.arena.cap*vertexBytes))
		cmd, err := enc.Finish()
		if err != nil {
			panic(err)
		}
		if _, err := dev.queue.Submit(cmd); err != nil {
			panic(err)
		}
		old.Release()
	}
	r.arena.grow(newCap)
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
