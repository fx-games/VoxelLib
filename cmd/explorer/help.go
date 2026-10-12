package main

import (
	"image"
	"image/color"
	"image/draw"
	"log"

	"github.com/memmaker/voxellib/voxel/render"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// helpLines is the F1 help screen. Keep it in sync with input.go.
var helpLines = [][2]string{
	{"MOVEMENT", ""},
	{"W A S D", "walk"},
	{"Mouse", "look"},
	{"Space", "jump / swim up / fly up"},
	{"Shift", "sneak / sink / fly down"},
	{"Ctrl", "sprint (dive while swimming)"},
	{"", ""},
	{"BUILDING", ""},
	{"Left mouse", "break block"},
	{"Right mouse", "place block"},
	{"1-9 / wheel", "choose block"},
	{"", ""},
	{"ITEMS", ""},
	{"Walk over item", "take it (drops what you hold)"},
	{"Q", "throw held item"},
	{"", ""},
	{"DEBUG", ""},
	{"F / Space x2", "toggle flying"},
	{"G", "toggle no-clip"},
	{"N", "day / night"},
	{"R", "respawn"},
	{"K", "preview nearest creature's animations"},
	{"F3", "frame stats on stdout"},
	{"F1", "show / hide this help"},
	{"Esc", "release mouse (again: quit)"},
}

// toggleHelp shows or hides the help overlay, building it on first use.
func (g *game) toggleHelp() {
	g.showHelp = !g.showHelp
	if g.showHelp && g.help == nil {
		o, err := render.NewOverlay()
		if err != nil {
			log.Printf("help: %v", err)
			g.showHelp = false
			return
		}
		o.SetImage(helpImage())
		g.help = o
	}
}

// helpImage renders helpLines with the 7x13 bitmap font on a dark panel.
func helpImage() *image.NRGBA {
	const lineH, pad, keyW = 15, 10, 16 * 7
	face := basicfont.Face7x13
	w := 0
	for _, l := range helpLines {
		w = max(w, keyW+font.MeasureString(face, l[1]).Ceil())
	}
	img := image.NewNRGBA(image.Rect(0, 0, w+2*pad, len(helpLines)*lineH+2*pad+lineH))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.NRGBA{10, 12, 20, 200}), image.Point{}, draw.Src)
	d := &font.Drawer{Dst: img, Face: face}
	text := func(s string, x, y int, c color.Color) {
		d.Src = image.NewUniform(c)
		d.Dot = fixed.P(x, y)
		d.DrawString(s)
	}
	text("voxellib explorer - controls", pad, pad+11, color.NRGBA{255, 255, 255, 255})
	for i, l := range helpLines {
		y := pad + (i+2)*lineH - 4
		if l[1] == "" {
			text(l[0], pad, y, color.NRGBA{255, 210, 90, 255})
			continue
		}
		text(l[0], pad, y, color.NRGBA{140, 200, 255, 255})
		text(l[1], pad+keyW, y, color.NRGBA{230, 230, 230, 255})
	}
	return img
}

// drawHelp centres the help panel, scaled to the framebuffer.
func (g *game) drawHelp(fbW, fbH int) {
	if !g.showHelp || g.help == nil {
		return
	}
	w, h := g.help.Size()
	s := max(1, min(fbW*8/10/w, fbH*9/10/h))
	g.help.Draw((fbW-w*s)/2, (fbH-h*s)/2, s, fbW, fbH)
}
