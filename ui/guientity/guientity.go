// Package guientity draws an entity inside a GUI the way
// GuiInventory.func_147046_a does (the player in its inventory, a mount in
// its inventory): models posed in its lighting frame, rasterized on the
// CPU (model/raster) at the GUI scale factor's resolution and drawn as
// one textured quad.
package guientity

import (
	"image"
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/model/raster"
	"github.com/memmaker/voxellib/ui"
)

// Angles are the entity angles func_147046_a sets from the mouse offsets
// dx = x - mouseX and dy = y - 50 - mouseY (degrees): renderYawOffset,
// rotationYaw (= rotationYawHead) and rotationPitch.
func Angles(dx, dy float32) (yawOffset, yaw, pitch float32) {
	ax := float32(math.Atan(float64(dx / 40)))
	ay := float32(math.Atan(float64(dy / 40)))
	return ax * 20, ax * 40, -ay * 20
}

// Lights are RenderHelper.enableStandardItemLighting's two lights and
// global ambient, in the frame current at that call (after glRotatef(135,
// 0,1,0)), which is the lighting frame of Matrices.
var Lights = raster.Lighting{Ambient: 0.4, Lights: []raster.Light{
	{Dir: mgl32.Vec3{0.2, 1, -0.7}.Normalize(), Diffuse: 0.6},
	{Dir: mgl32.Vec3{-0.2, 1, 0.7}.Normalize(), Diffuse: 0.6},
}}

// Matrices splits func_147046_a's GL chain at the lighting call: view
// maps the lighting frame to GUI pixels (z towards the viewer) and body
// maps a model's world matrices (cuboid.Pose.World) to the lighting frame,
// adding doRender's yaw turn (World holds its Ry(180)).
func Matrices(x, y, scale, dx, dy float32) (view, body mgl32.Mat4) {
	yawOffset, _, _ := Angles(dx, dy)
	view = cuboid.Translate(x, y, 50).Mul4(cuboid.Scale(-scale, scale, scale)).
		Mul4(cuboid.Rotate(180, 0, 0, 1)).Mul4(cuboid.Rotate(135, 0, 1, 0))
	pitchX := -float32(math.Atan(float64(dy/40))) * 20
	// translate(0, yOffset) and RenderPlayer's y - yOffset cancel.
	body = cuboid.Rotate(-135, 0, 1, 0).Mul4(cuboid.Rotate(pitchX, 1, 0, 0)).
		Mul4(cuboid.Rotate(-yawOffset, 0, 1, 0))
	return view, body
}

// Draw is one model draw of func_147046_a's entity, its skin in the
// lighting frame; Additive blends GL_ONE, GL_ONE (glowing eyes).
type Draw struct {
	Model    *model.Model
	Skin     []mgl32.Mat4
	Tex      *image.NRGBA
	Color    [4]float32
	Additive bool
}

// Region is the GUI pixel box the draws cover under view (lighting frame
// to GUI pixels), clipped only by the w x h screen as GL clips them; its
// size is rounded up to 32 so the target is not remade every frame.
func Region(view mgl32.Mat4, ds []Draw, w, h int) (l, t, r, b int, ok bool) {
	var x0, y0, x1, y1 float32
	for _, d := range ds {
		a, c, e, f, in := raster.Bounds(d.Model, d.Skin, view)
		if !in {
			continue
		}
		if !ok {
			x0, y0, x1, y1, ok = a, c, e, f, true
		}
		x0, y0, x1, y1 = min(x0, a), min(y0, c), max(x1, e), max(y1, f)
	}
	l, t = max(int(math.Floor(float64(x0))), 0), max(int(math.Floor(float64(y0))), 0)
	r, b = min(int(math.Ceil(float64(x1))), w), min(int(math.Ceil(float64(y1))), h)
	if !ok || r <= l || b <= t {
		return 0, 0, 0, 0, false
	}
	r, b = min(l+(r-l+31)/32*32, w), min(t+(b-t+31)/32*32, h)
	return l, t, r, b, true
}

// Target rasterizes func_147046_a's draws at the GUI scale factor's
// resolution over their region; the zero value is ready.
type Target struct {
	target *raster.Target
	tex    ui.Texture
}

// Draw rasterizes ds under view (lighting frame to GUI pixels) and draws
// them on ctx's canvas.
func (gt *Target) Draw(ctx *ui.Context, view mgl32.Mat4, ds []Draw) {
	l, t, r, b, ok := Region(view, ds, ctx.W, ctx.H)
	if !ok {
		return
	}
	fb := max(ctx.Scale, 1)
	w, h := (r-l)*fb, (b-t)*fb
	if gt.target == nil || gt.target.Img.Rect.Dx() != w || gt.target.Img.Rect.Dy() != h {
		gt.target = raster.NewTarget(w, h)
	}
	gt.target.Clear()
	v := cuboid.Scale(float32(fb), float32(fb), 1).Mul4(cuboid.Translate(-float32(l), -float32(t), 0)).Mul4(view)
	for _, d := range ds {
		gt.target.Additive = d.Additive
		gt.target.Draw(d.Model, d.Skin, v, d.Tex, d.Color, Lights)
	}
	gt.target.Additive = false
	gt.tex = ui.UpdateTexture(ctx.Canvas, gt.tex, gt.target.Img)
	ctx.Canvas.Quad(ui.Quad{Tex: gt.tex, X0: float32(l), Y0: float32(t), X1: float32(r), Y1: float32(b),
		U1: 1, V1: 1, Top: 0xFFFFFFFF, Bottom: 0xFFFFFFFF})
}
