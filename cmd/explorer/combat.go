package main

// Health: the player's hearts, the hurt flash and its invulnerability.
// In zombie defense the server deals the damage (mp.Hit) and the snapshot
// carries the health; drawHUD after the world, tickPlayerHealth each tick.

import (
	"image"
	"image/color"

	"github.com/memmaker/voxellib/voxel/render"
)

const (
	maxHealth = 20 // half-hearts, as the reference
	hurtTicks = 10 // flashing red after a hit
)

type combatState struct {
	health    int
	hurt      int // ticks left of the hurt flash
	hud       *render.Overlay
	hudHealth int // health drawn into hud
	flash     *render.Overlay
}

func (g *game) tickPlayerHealth() {
	if g.hurt > 0 {
		g.hurt--
	}
}

// drawHUD draws the health hearts and the hurt flash.
func (g *game) drawHUD(fbW, fbH int) {
	if g.hud == nil {
		var err error
		if g.hud, err = render.NewOverlay(); err != nil {
			return
		}
		g.flash, _ = render.NewOverlay()
		if g.flash != nil {
			img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
			img.Set(0, 0, color.NRGBA{200, 0, 0, 90})
			g.flash.SetImage(img)
		}
	}
	if g.hudHealth != g.health {
		g.hudHealth = g.health
		g.hud.SetImage(heartsImage(g.health))
	}
	if g.hurt > 0 && g.flash != nil {
		g.flash.Draw(0, 0, max(fbW, fbH), fbW, fbH)
	}
	w, h := g.hud.Size()
	s := max(2, fbH/360)
	g.hud.Draw((fbW-w*s)/2, fbH-h*s-fbH/12, s, fbW, fbH)
}

// heartsImage draws ten reference-style hearts for health half-hearts.
func heartsImage(health int) image.Image {
	const hw = 9
	shape := []string{
		".XX...XX.",
		"XOOX.XOOX",
		"XOOOXOOOX",
		"XOOOOOOOX",
		".XOOOOOX.",
		"..XOOOX..",
		"...XOX...",
		"....X....",
	}
	img := image.NewNRGBA(image.Rect(0, 0, 10*hw+9, len(shape)))
	outline := color.NRGBA{20, 0, 0, 255}
	full := color.NRGBA{220, 20, 30, 255}
	empty := color.NRGBA{60, 20, 20, 200}
	for i := 0; i < 10; i++ {
		x0 := i * (hw + 1)
		for y, row := range shape {
			for x, ch := range row {
				var c color.NRGBA
				switch {
				case ch == 'X':
					c = outline
				case ch != 'O':
					continue
				case 2*i+2 <= health, 2*i+1 == health && x < hw/2:
					c = full
				default:
					c = empty
				}
				img.Set(x0+x, y, c)
			}
		}
	}
	return img
}
