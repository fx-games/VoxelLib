// Package guicanvas is ui.Canvas (and ui.Targets) over render.GUIBatch,
// for a game with its own render loop: draw the HUD and screens through
// it after the world, then Flush.
//
//	c := guicanvas.New()
//	ctx, _ := ui.NewContext(c, assets, lang)
//	... draw ...
//	c.Flush(fbW, fbH, px) // px: framebuffer pixels per GUI pixel
//
// ui.PlayerGUI.Draw flushes it itself.
package guicanvas

import (
	"image"

	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxellib/voxel/render"
)

// Canvas queues the GUI's quads for one GPU flush.
type Canvas struct{ batch render.GUIBatch }

// New returns an empty canvas.
func New() *Canvas { return &Canvas{} }

// Flush draws what was queued onto the fbW x fbH framebuffer, px
// framebuffer pixels to the GUI pixel.
func (c *Canvas) Flush(fbW, fbH int, px float32) { c.batch.Flush(fbW, fbH, px) }

func (c *Canvas) NewTexture(img image.Image) ui.Texture { return render.NewGUITexture(img) }

func (c *Canvas) UpdateTexture(t ui.Texture, img image.Image) bool {
	g, ok := t.(*render.GUITexture)
	return ok && g.Update(img)
}

func (c *Canvas) Quad(q ui.Quad) {
	t, _ := q.Tex.(*render.GUITexture)
	c.batch.Linear(q.Linear)
	if q.Rot != 0 {
		c.batch.QuadRot(t, q.X0, q.Y0, q.X1, q.Y1, q.U0, q.V0, q.U1, q.V1, q.Skew, q.Top, q.Bottom, q.Rot, q.RX, q.RY)
		return
	}
	c.batch.Quad(t, q.X0, q.Y0, q.X1, q.Y1, q.U0, q.V0, q.U1, q.V1, q.Skew, q.Top, q.Bottom)
}

func (c *Canvas) SetScissor(r image.Rectangle) {
	if r.Empty() {
		c.batch.Scissor(0, 0, 0, 0)
		return
	}
	c.batch.Scissor(r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}

func (c *Canvas) SetInvert(on bool) { c.batch.Invert(on) }

func (c *Canvas) SetOrReverse(on bool) { c.batch.OrReverse(on) }

func (c *Canvas) NewTarget(w, h int) ui.Texture { return render.NewGUITarget(w, h) }

func (c *Canvas) SetTarget(t ui.Texture, clear uint32) {
	g, _ := t.(*render.GUITexture)
	c.batch.Target(g, clear)
}
