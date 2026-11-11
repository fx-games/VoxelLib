// Package nametag draws name tags over entities as vanilla
// EntityRenderer.renderNameTag: the text in the GUI font (ui.Font), centred
// half a block over the head, turned to the camera, 0.025 blocks per font
// pixel, lit by the entity's light. A tag shows through walls dimmed
// (0x20FFFFFF on a 25% black background) with the plain white text over it
// where it is in sight; a sneaking ("discrete") entity's tag is only the dim
// pass, hidden by walls.
//
//	nt, err := nametag.New(app.Models, assets) // assets holds the GUI font
//	...
//	nt.Draw(&app.Cam, tags) // each frame
package nametag

import (
	"image"
	"io/fs"
	"log"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxellib/voxel/render"
)

const (
	pixel = 0.025 // blocks per font pixel (poseStack.scale(-0.025, -0.025, 0.025))
	// Lift is how far over an entity's height its tag sits (getNameTagOffsetY).
	Lift = 0.5
	// Range is how far a tag shows, DiscreteRange a sneaking entity's
	// (LivingEntityRenderer.shouldShowName).
	Range         = 64
	DiscreteRange = 32
	// Font.drawInBatch colours: the see-through text, the plain text and
	// the background (Options.getBackgroundOpacity(0.25)).
	seeThroughColor = 0x20FFFFFF
	plainColor      = 0xFFFFFFFF
	backgroundColor = 0x40000000
	height          = 10  // the background: one pixel over the 9 px line
	textCutoff      = 0.1 // the rendertype_text shaders discard below this alpha
)

// Tag is one name tag: Text over the entity whose top is at X, Y, Z (feet
// plus its height).
type Tag struct {
	Text       string
	X, Y, Z    float64
	Discrete   bool  // sneaking: dim, and hidden by walls
	Sky, Block uint8 // light levels at the entity
}

// Renderer draws name tags with one ModelRenderer (one per engine session).
type Renderer struct {
	font   *ui.Font
	canvas *ui.SoftCanvas
	models *render.ModelRenderer
	cache  map[string]handles // by text
	used   map[string]bool
	dim    []render.ModelInstance
	plain  []render.ModelInstance
	turn   mgl32.Mat4
}

// handles are a text's two quads: the dim see-through one with its
// background, and the plain text.
type handles struct{ dim, plain int }

// New makes a Renderer drawing with models in the GUI font read from assets
// (ui.FontASCII and ui.FontGlyphSizes).
func New(models *render.ModelRenderer, assets fs.FS) (*Renderer, error) {
	c := ui.NewSoftCanvas(1, 1, 1)
	f, err := ui.LoadFont(assets, c.NewTexture)
	if err != nil {
		return nil, err
	}
	return &Renderer{font: f, canvas: c, models: models, cache: map[string]handles{}, used: map[string]bool{}}, nil
}

// Draw draws tags for cam; texts no longer shown are freed.
func (r *Renderer) Draw(cam *render.Camera, tags []Tag) {
	r.dim, r.plain = r.dim[:0], r.plain[:0]
	clear(r.used)
	// cameraOrientation: the quads lie in the view plane.
	r.turn = mgl32.LookAtV(mgl32.Vec3{}, cam.Forward(), mgl32.Vec3{0, 1, 0}).Inv()
	for _, t := range tags {
		dx, dy, dz := t.X-cam.X, t.Y-cam.Y, t.Z-cam.Z
		rng := float64(Range)
		if t.Discrete {
			rng = DiscreteRange
		}
		if t.Text == "" || dx*dx+dy*dy+dz*dz >= rng*rng {
			continue
		}
		h, ok := r.cache[t.Text]
		if !ok {
			h = r.add(t.Text)
			r.cache[t.Text] = h
		}
		r.used[t.Text] = true
		in := render.ModelInstance{X: t.X, Y: t.Y + Lift, Z: t.Z, Transform: &r.turn, Sky: t.Sky, Block: t.Block, LightOnly: true, AlphaCutoff: textCutoff}
		if t.Discrete {
			in.Model, in.Blend = h.dim, render.BlendAlpha // Font.DisplayMode.NORMAL
			r.dim = append(r.dim, in)
			continue
		}
		in.Model, in.Blend = h.dim, render.BlendOver // SEE_THROUGH
		r.dim = append(r.dim, in)
		in.Model, in.Blend = h.plain, render.BlendNone
		r.plain = append(r.plain, in)
	}
	for text, h := range r.cache {
		if !r.used[text] {
			r.models.Remove(h.dim)
			r.models.Remove(h.plain)
			delete(r.cache, text)
		}
	}
	r.models.Draw(cam, r.dim)
	r.models.Draw(cam, r.plain)
}

// add rasterises text's two quads (-1 where one fails).
func (r *Renderer) add(text string) handles {
	w := r.font.Width(text)
	h := handles{-1, -1}
	for _, q := range []struct {
		to         *int
		background bool
		color      uint32
	}{{&h.dim, true, seeThroughColor}, {&h.plain, false, plainColor}} {
		c := ui.NewSoftCanvas(w+1, height, 1)
		if q.background {
			ui.DrawRect(c, 0, 0, w+1, height, backgroundColor)
		}
		r.font.Draw(c, text, 1, 1, q.color, false)
		var err error
		if *q.to, err = r.models.Add(quad(c.Img, -(w / 2))); err != nil {
			log.Printf("nametag %q: %v", text, err)
			*q.to = -1
		}
	}
	return h
}

// quad is img on a quad facing +Z in view space, its font pixels at x
// (Font.drawInBatch's x, -width/2 truncated) and y 0, one pixel of
// background to the left and above.
func quad(img image.Image, x int) *model.Model {
	b := img.Bounds()
	x0, x1 := float32(x-1)*pixel, float32(x-1+b.Dx())*pixel
	y0, y1 := -float32(b.Dy()-1)*pixel, float32(pixel) // font y grows down
	return model.Quad("nametag", img, x0, y0, x1, y1)
}
