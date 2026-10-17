//go:build !wgpu

package render

import (
	"unsafe"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/platform/gl"
	"github.com/memmaker/voxellib/voxel"
)

// gpuRenderer is the GL state of a Renderer.
type gpuRenderer struct {
	terrain, sky, lines *program
	ebo                 uint32
	emptyVAO            uint32
	lineVAO, lineVBO    uint32
	ptrs                []unsafe.Pointer
}

func (r *Renderer) initGPU() error {
	if err := gl.Init(); err != nil {
		return err
	}
	g := &r.gpu
	var err error
	if g.terrain, err = newProgram(terrainVS, terrainFS); err != nil {
		return err
	}
	if g.sky, err = newProgram(skyVS, skyFS); err != nil {
		return err
	}
	if g.lines, err = newProgram(lineVS, lineFS); err != nil {
		return err
	}
	idx := quadIndices()
	gl.GenBuffers(1, &g.ebo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, g.ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(idx)*4, gl.Ptr(idx), gl.STATIC_DRAW)
	gl.GenVertexArrays(1, &g.emptyVAO)

	gl.GenVertexArrays(1, &g.lineVAO)
	gl.GenBuffers(1, &g.lineVBO)
	gl.BindVertexArray(g.lineVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, g.lineVBO)
	// Static: rewriting a buffer the GPU still reads stalls macOS.
	lv := lineVertices()
	gl.BufferData(gl.ARRAY_BUFFER, len(lv)*4, gl.Ptr(lv), gl.STATIC_DRAW)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, 12, 0)
	gl.BindVertexArray(0)
	return nil
}

// drawTerrain draws the sky and r.batches.
func (r *Renderer) drawTerrain(vp mgl32.Mat4) {
	g := &r.gpu
	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LEQUAL)
	gl.Enable(gl.CULL_FACE)
	gl.CullFace(gl.BACK)
	gl.FrontFace(gl.CCW)

	fog := r.CurrentFog()
	skyCol, fogCol := r.skyColors()
	gl.DepthMask(false)
	g.sky.use()
	inv := vp.Inv()
	gl.UniformMatrix4fv(g.sky.loc("uInvViewProj"), 1, false, &inv[0])
	gl.Uniform3fv(g.sky.loc("uFogColor"), 1, &fogCol[0])
	gl.Uniform3fv(g.sky.loc("uSkyColor"), 1, &skyCol[0])
	gl.Uniform3fv(g.sky.loc("uSunDir"), 1, &r.Opt.SunDir[0])
	gl.BindVertexArray(g.emptyVAO)
	gl.DrawArrays(gl.TRIANGLES, 0, 3)
	gl.DepthMask(true)

	p := g.terrain
	p.use()
	gl.UniformMatrix4fv(p.loc("uViewProj"), 1, false, &vp[0])
	gl.Uniform1f(p.loc("uDaylight"), r.Opt.Daylight)
	gl.Uniform1f(p.loc("uBlockBright"), r.Opt.BlockBrightness)
	gl.Uniform3fv(p.loc("uFogColor"), 1, &fogCol[0])
	gl.Uniform2f(p.loc("uFog"), fog.Start, fog.End)
	gl.Uniform1i(p.loc("uTex"), 0)
	gl.Uniform1i(p.loc("uLayerMap"), 1)
	gl.ActiveTexture(gl.TEXTURE1)
	gl.BindTexture(gl.TEXTURE_2D, r.tex.gpu.layerMap)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D_ARRAY, r.tex.gpu.id)

	gl.Disable(gl.BLEND)
	layer := voxel.LayerCount
	for _, b := range r.batches {
		if b.layer != layer {
			layer = b.layer
			cut := float32(0)
			switch layer {
			case voxel.LayerCutout:
				cut = 0.5
			case voxel.LayerTranslucent:
				// far to near, depth writes off
				cut = 0.01
				gl.Enable(gl.BLEND)
				gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
				gl.DepthMask(false)
			}
			gl.Uniform1f(p.loc("uAlphaCut"), cut)
		}
		r.multiDraw(b)
	}
	gl.DepthMask(true)
	gl.Disable(gl.BLEND)
	gl.BindVertexArray(0)
}

func (r *Renderer) multiDraw(b drawBatch) {
	g := &r.gpu
	if len(g.ptrs) < b.n {
		g.ptrs = make([]unsafe.Pointer, b.n*2)
	}
	origin := r.regionOrigin(b.reg)
	gl.Uniform3fv(g.terrain.loc("uRegionOrigin"), 1, &origin[0])
	gl.BindVertexArray(b.reg.gpu.vao)
	gl.MultiDrawElementsBaseVertex(gl.TRIANGLES, &r.counts[b.first], gl.UNSIGNED_INT, &g.ptrs[0], int32(b.n), &r.bases[b.first])
}

// DrawBlockOutline draws a wireframe box around a block (Minecraft's
// selection outline).
func (r *Renderer) DrawBlockOutline(cam *Camera, x, y, z int) {
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	r.drawLines(0, 24, outlineMVP(cam, x, y, z), mgl32.Vec4{0, 0, 0, 0.6})
	gl.Disable(gl.BLEND)
}

// DrawCrosshair draws a screen-centre cross; aspect = width/height.
func (r *Renderer) DrawCrosshair(aspect float32) {
	gl.Disable(gl.DEPTH_TEST)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.ONE_MINUS_DST_COLOR, gl.ZERO)
	r.drawLines(24, 4, crosshairMVP(aspect), mgl32.Vec4{1, 1, 1, 1})
	gl.Disable(gl.BLEND)
	gl.Enable(gl.DEPTH_TEST)
}

func (r *Renderer) drawLines(first, n int32, mvp mgl32.Mat4, col mgl32.Vec4) {
	g := &r.gpu
	g.lines.use()
	gl.UniformMatrix4fv(g.lines.loc("uMVP"), 1, false, &mvp[0])
	gl.Uniform4fv(g.lines.loc("uColor"), 1, &col[0])
	gl.BindVertexArray(g.lineVAO)
	gl.LineWidth(1)
	gl.DrawArrays(gl.LINES, first, n)
	gl.BindVertexArray(0)
}

func (r *Renderer) destroyGPU() { gl.DeleteBuffers(1, &r.gpu.ebo) }
