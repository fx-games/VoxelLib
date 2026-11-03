package main

import (
	"fmt"

	"github.com/memmaker/voxellib/ui"
)

// drawScores draws the multiplayer ranking while Tab is held (and while
// dead, like Q3): the players by kills, with their deaths, this player's
// line highlighted.
func (g *game) drawScores(ctx *ui.Context) {
	if g.net == nil || !g.showScores && !g.net.cl.Own().Dead {
		return
	}
	ranks := g.net.cl.Ranking()
	f := ctx.Font
	const rowH, nameW, numW, pad = 11, 110, 46, 6
	w := 18 + nameW + 2*numW + 2*pad
	x0, y0 := ctx.W/2-w/2, 30
	h := (len(ranks)+1)*rowH + 3 + 2*pad - 2
	ui.DrawRect(ctx.Canvas, x0, y0, x0+w, y0+h, 0xA0000000)
	y := y0 + pad
	col := func(s string, x int, right bool, c uint32) {
		if right {
			x -= f.Width(s)
		}
		f.Draw(ctx.Canvas, s, x, y, c, true)
	}
	kx, dx := x0+pad+18+nameW+numW, x0+w-pad
	col("#", x0+pad, false, 0xA0A0A0)
	col("Player", x0+pad+18, false, 0xA0A0A0)
	col("Kills", kx, true, 0xA0A0A0)
	col("Deaths", dx, true, 0xA0A0A0)
	y += rowH + 3
	me := g.net.cl.Slot()
	for i, r := range ranks {
		c := uint32(0xFFFFFF)
		if r.Slot == me {
			ui.DrawRect(ctx.Canvas, x0+2, y-1, x0+w-2, y+rowH-2, 0x40FFFFFF)
			c = 0xFFFF55
		}
		col(fmt.Sprint(i+1), x0+pad, false, c)
		col(f.TrimToWidth(r.Name, nameW, false), x0+pad+18, false, c)
		col(fmt.Sprint(r.Kills), kx, true, c)
		col(fmt.Sprint(r.Deaths), dx, true, c)
		y += rowH
	}
}
