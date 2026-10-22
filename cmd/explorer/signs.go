package main

// Sign text: each sign's four lines are rasterised once into a texture with
// a pixel font and drawn as a quad on the board's front, laid out like
// vanilla SignRenderer (1/96 block per font pixel, 10px lines, centred).

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
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
	signW, signLine = 90, 10 // text box width and line height, font pixels
	signPx          = 1.0 / 96
)

// signFontTTF is the default sign font: IBM CGA thin from VileR's Ultimate
// Oldschool PC Font Pack (CC BY-SA 4.0, Px437_LICENSE.TXT).
//
//go:embed Px437_IBM_CGAthin.ttf
var signFontTTF []byte

type signText struct {
	face    font.Face
	handles map[voxel.BlockPos]int // ModelRenderer handle per sign, -1 = blank
	insts   []render.ModelInstance
}

// loadSignFont opens a pixel TTF at its native 8px size (” = built in).
func (g *game) loadSignFont(path string) error {
	b := signFontTTF
	if path != "" {
		var err error
		if b, err = os.ReadFile(path); err != nil {
			return err
		}
	}
	f, err := opentype.Parse(b)
	if err != nil {
		return err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 8, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return err
	}
	g.signs = &signText{face: face, handles: map[voxel.BlockPos]int{}}
	return nil
}

// glyphAdvance trims the monospace cell to the glyph's ink plus one pixel of
// spacing, like vanilla's proportional font; space stays 4px.
func (s *signText) glyphAdvance(r rune) int {
	if r == ' ' {
		return 4
	}
	b, _, ok := s.face.GlyphBounds(r)
	if !ok || b.Max.X <= b.Min.X {
		return 4
	}
	return b.Max.X.Ceil() + 1
}

func (s *signText) lineWidth(line string) int {
	w := 0
	for _, r := range line {
		w += s.glyphAdvance(r)
	}
	return w
}

// signImage renders the lines black on transparent; ok is false for a blank sign.
func (s *signText) signImage(lines [4]string) (img *image.NRGBA, ok bool) {
	img = image.NewNRGBA(image.Rect(0, 0, signW, 4*signLine))
	d := &font.Drawer{Dst: img, Src: image.NewUniform(color.NRGBA{0, 0, 0, 255}), Face: s.face}
	asc := s.face.Metrics().Ascent.Ceil()
	for i, line := range lines {
		if line == "" {
			continue
		}
		ok = true
		x := (signW - s.lineWidth(line)) / 2
		for _, r := range line {
			// Draw each glyph at its trimmed position; a cell wider than the
			// box simply clips.
			d.Dot = fixed.P(x, i*signLine+1+asc)
			d.DrawString(string(r))
			x += s.glyphAdvance(r)
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

// signModel is a unit quad (font pixels → blocks) facing +Z, centred on the
// text box.
func signModel(img image.Image) *model.Model {
	w, h := float32(signW*signPx)/2, float32(4*signLine*signPx)/2
	m := &model.Model{Name: "sign", Textures: []image.Image{img}, Clips: map[string]*model.Clip{}, Tags: map[string]bool{},
		Bones: []model.Bone{{Name: "sign", Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}}}
	n := mgl32.Vec3{0, 0, 1}
	m.Meshes = []model.Mesh{{Texture: 0, Indices: []uint32{0, 1, 2, 0, 2, 3}, Verts: []model.Vertex{
		{Pos: mgl32.Vec3{-w, -h, 0}, Normal: n, UV: mgl32.Vec2{0, 1}, Weights: [4]float32{1}},
		{Pos: mgl32.Vec3{w, -h, 0}, Normal: n, UV: mgl32.Vec2{1, 1}, Weights: [4]float32{1}},
		{Pos: mgl32.Vec3{w, h, 0}, Normal: n, UV: mgl32.Vec2{1, 0}, Weights: [4]float32{1}},
		{Pos: mgl32.Vec3{-w, h, 0}, Normal: n, UV: mgl32.Vec2{0, 0}, Weights: [4]float32{1}},
	}}}
	return m
}

// drawSigns draws sign text within 16 blocks (vanilla's sign text
// distance) and frees the textures of signs left behind.
func (g *game) drawSigns() {
	s := g.signs
	if s == nil {
		return
	}
	s.insts = s.insts[:0]
	w := g.Eng.World
	cx, cz := int(math.Floor(g.Cam.X))>>4, int(math.Floor(g.Cam.Z))>>4
	for dz := -1; dz <= 1; dz++ {
		for dx := -1; dx <= 1; dx++ {
			ch := w.Chunk(voxel.ChunkPos{X: cx + dx, Z: cz + dz})
			if ch == nil {
				continue
			}
			for p, lines := range ch.Signs {
				x, y, z := float64(p.X)+0.5, float64(p.Y), float64(p.Z)+0.5
				if ddx, ddy, ddz := x-g.Cam.X, y-g.Cam.Y, z-g.Cam.Z; ddx*ddx+ddy*ddy+ddz*ddz > 16*16 {
					continue
				}
				id := w.Block(p.X, p.Y, p.Z)
				wall := strings.HasSuffix(w.Reg.Info(id).Name, "_wall_sign")
				if !wall && !strings.HasSuffix(w.Reg.Info(id).Name, "_sign") {
					continue
				}
				h, ok := s.handles[p]
				if !ok {
					h = -1
					if img, any := s.signImage(lines); any {
						var err error
						if h, err = g.Models.Add(signModel(img)); err != nil {
							fmt.Fprintln(os.Stderr, "sign:", err)
							h = -1
						}
					}
					s.handles[p] = h
				}
				if h < 0 {
					continue
				}
				props := w.Reg.StateProps(id)
				// Yaw 0 faces south (+Z), turning clockwise from above. Standing
				// board front at z = 26/3 px, text centre 40/3 px up; a wall
				// sign sits 5 px lower and 7 px further back.
				ty, tz := 40.0/3, 26.0/3
				if wall {
					ty, tz = ty-5, tz-7
				}
				t := mgl32.HomogRotate3DY(-voxel.SignYaw(props["facing"], props["rotation"]) * math.Pi / 180).
					Mul4(mgl32.Translate3D(0, float32(ty/16), float32((tz-8)/16+0.002)))
				sky, blk := g.Light(x, y+0.8, z)
				s.insts = append(s.insts, render.ModelInstance{Model: h, X: x, Y: y, Z: z, Transform: &t, Sky: sky, Block: blk})
			}
		}
	}
	// Free signs well past the draw distance, so walking back and forth
	// across the edge doesn't re-render them every time.
	for p, h := range s.handles {
		dx, dz := float64(p.X)+0.5-g.Cam.X, float64(p.Z)+0.5-g.Cam.Z
		if dx*dx+dz*dz > 32*32 {
			g.Models.Remove(h)
			delete(s.handles, p)
		}
	}
	g.Models.Draw(&g.Cam, s.insts)
}
