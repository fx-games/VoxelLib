// Package signtext draws the text of signs (voxel.Chunk.Signs): each
// sign's four lines are rasterised once into a texture with a pixel font
// and drawn as a quad on the board's front through a render.ModelRenderer,
// laid out like vanilla SignRenderer (1/96 block per font pixel, 10px
// lines, centred).
//
//	st, err := signtext.New(app.Models, nil) // nil: the built-in font
//	...
//	st.Draw(&app.Cam, app.Eng.World, app.Light) // each frame
package signtext

import (
	_ "embed"
	"image"
	"image/color"
	"log"
	"math"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/render"
)

const (
	boxW, lineH = 90, 10 // text box width and line height, font pixels
	px          = 1.0 / 96
	// Distance is vanilla's sign text distance in blocks.
	Distance = 16
)

// defaultTTF is the default sign font: IBM CGA thin from VileR's Ultimate
// Oldschool PC Font Pack (CC BY-SA 4.0, Px437_LICENSE.TXT).
//
//go:embed Px437_IBM_CGAthin.ttf
var defaultTTF []byte

// Renderer draws sign text with one ModelRenderer (one per engine session).
type Renderer struct {
	// Block names the Java block of a state and its properties (a sign's
	// facing or rotation). It defaults to the registry's name and
	// properties; a VoxelMap passes its Map.JavaState, whose keys
	// ("oak_wall_sign#1") hold no properties.
	Block func(w *voxel.World, id voxel.StateID) (name string, props map[string]string)

	face    font.Face
	models  *render.ModelRenderer
	handles map[voxel.BlockPos]int // ModelRenderer handle per sign, -1 = blank
	insts   []render.ModelInstance
}

// New makes a Renderer drawing with models in the TrueType pixel font ttf
// (nil: the built-in IBM CGA thin).
func New(models *render.ModelRenderer, ttf []byte) (*Renderer, error) {
	if ttf == nil {
		ttf = defaultTTF
	}
	f, err := opentype.Parse(ttf)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 8, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	return &Renderer{Block: registryBlock, face: face, models: models, handles: map[voxel.BlockPos]int{}}, nil
}

func registryBlock(w *voxel.World, id voxel.StateID) (string, map[string]string) {
	return w.Reg.Info(id).Name, w.Reg.StateProps(id)
}

// glyphAdvance trims the monospace cell to the glyph's ink plus one pixel of
// spacing, like vanilla's proportional font; space stays 4px.
func (s *Renderer) glyphAdvance(r rune) int {
	if r == ' ' {
		return 4
	}
	b, _, ok := s.face.GlyphBounds(r)
	if !ok || b.Max.X <= b.Min.X {
		return 4
	}
	return b.Max.X.Ceil() + 1
}

func (s *Renderer) lineWidth(line string) int {
	w := 0
	for _, r := range line {
		w += s.glyphAdvance(r)
	}
	return w
}

// Image renders the lines black on transparent; ok is false for a blank sign.
// A line wider than the box (the pixel font runs wider than vanilla's,
// which the lines were written for) is squeezed to fit, not clipped.
func (s *Renderer) Image(lines [4]string) (img *image.NRGBA, ok bool) {
	img = image.NewNRGBA(image.Rect(0, 0, boxW, 4*lineH))
	asc := s.face.Metrics().Ascent.Ceil()
	for i, line := range lines {
		if line == "" {
			continue
		}
		ok = true
		lw := s.lineWidth(line)
		row := image.NewNRGBA(image.Rect(0, 0, max(boxW, lw), lineH))
		d := &font.Drawer{Dst: row, Src: image.NewUniform(color.NRGBA{0, 0, 0, 255}), Face: s.face}
		x := max(boxW-lw, 0) / 2
		for _, r := range line {
			// Each glyph at its trimmed position.
			d.Dot = fixed.P(x, 1+asc)
			d.DrawString(string(r))
			x += s.glyphAdvance(r)
		}
		rw := row.Bounds().Dx()
		for y := range lineH {
			for x := range boxW {
				copy(img.Pix[img.PixOffset(x, i*lineH+y):][:4], row.Pix[row.PixOffset(x*rw/boxW, y):][:4])
			}
		}
	}
	// Font antialiasing would leave grey edges; the model shader discards
	// below half alpha, so harden it.
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] >= 128 {
			img.Pix[i] = 255
		} else {
			img.Pix[i] = 0
		}
	}
	return img, ok
}

// quad is the text box facing +Z, centred (font pixels → blocks).
func quad(img image.Image) *model.Model {
	w, h := float32(boxW*px)/2, float32(4*lineH*px)/2
	return model.Quad("sign", img, -w, -h, w, h)
}

// Draw draws the text of w's signs within Distance of the camera, lit by
// light (sky and block level at a point), and frees the textures of signs
// left behind.
func (s *Renderer) Draw(cam *render.Camera, w *voxel.World, light func(x, y, z float64) (sky, block uint8)) {
	s.insts = s.insts[:0]
	cx, cz := int(math.Floor(cam.X))>>4, int(math.Floor(cam.Z))>>4
	for dz := -1; dz <= 1; dz++ {
		for dx := -1; dx <= 1; dx++ {
			ch := w.Chunk(voxel.ChunkPos{X: cx + dx, Z: cz + dz})
			if ch == nil {
				continue
			}
			for p, lines := range ch.Signs {
				x, y, z := float64(p.X)+0.5, float64(p.Y), float64(p.Z)+0.5
				if ddx, ddy, ddz := x-cam.X, y-cam.Y, z-cam.Z; ddx*ddx+ddy*ddy+ddz*ddz > Distance*Distance {
					continue
				}
				name, props := s.Block(w, w.Block(p.X, p.Y, p.Z))
				wall := strings.HasSuffix(name, "_wall_sign")
				if !wall && !strings.HasSuffix(name, "_sign") {
					continue
				}
				h, ok := s.handles[p]
				if !ok {
					h = -1
					if img, any := s.Image(lines); any {
						var err error
						if h, err = s.models.Add(quad(img)); err != nil {
							log.Printf("sign: %v", err)
							h = -1
						}
					}
					s.handles[p] = h
				}
				if h < 0 {
					continue
				}
				// Yaw 0 faces south (+Z), turning clockwise from above. Standing
				// board front at z = 26/3 px, text centre 40/3 px up; a wall
				// sign sits 5 px lower and 7 px further back.
				ty, tz := 40.0/3, 26.0/3
				if wall {
					ty, tz = ty-5, tz-7
				}
				t := mgl32.HomogRotate3DY(-voxel.SignYaw(props["facing"], props["rotation"]) * math.Pi / 180).
					Mul4(mgl32.Translate3D(0, float32(ty/16), float32((tz-8)/16+0.002)))
				sky, blk := light(x, y+0.8, z)
				s.insts = append(s.insts, render.ModelInstance{Model: h, X: x, Y: y, Z: z, Transform: &t, Sky: sky, Block: blk})
			}
		}
	}
	// Free signs well past the draw distance, so walking back and forth
	// across the edge doesn't re-render them every time.
	for p, h := range s.handles {
		dx, dz := float64(p.X)+0.5-cam.X, float64(p.Z)+0.5-cam.Z
		if dx*dx+dz*dz > 4*Distance*Distance {
			if h >= 0 {
				s.models.Remove(h)
			}
			delete(s.handles, p)
		}
	}
	s.models.Draw(cam, s.insts)
}
