package main

// Hit feedback on the HUD: TACZ's RenderCrosshairEvent hit marker (four
// corners of hit_marker.png around the crosshair, red after a head shot,
// pushed apart after a kill, fading over hitMarkKeep) and KillAmountOverlay
// (the kills in a streak, bottom right). hitMark marks both.

import (
	"fmt"
	"image"
	"time"

	"github.com/memmaker/voxellib/ui"
)

const (
	hitMarkKeep    = 300 * time.Millisecond // RenderCrosshairEvent.KEEP_TIME
	hitMarkStart   = 4                      // RenderConfig HitMarketStartPosition (GUI pixels)
	hitMarkKillGap = 4                      // a kill pushes the corners this much further over the keep time
	// KillAmountOverlay: the streak shows killStreakKeep (KillAmountDurationSecond),
	// fading over its last third; its hue runs from killHueMax to red over killHueCount kills.
	killStreakKeep = 3 * time.Second
	killHueCount   = 30
	killHueMax     = 0.15
	killSkullSize  = 7 // kill_skull.png, font pixels
)

// hitMarks are the hit feedback timestamps (zero: never).
type hitMarks struct {
	hit, kill, head time.Time
	streak          int // kills in the current streak
}

// mark records a hit as ClientHitMark does.
func (h *hitMarks) mark(head, killed bool) {
	now := time.Now()
	h.hit = now
	if head {
		h.head = now
	}
	if killed {
		if now.Sub(h.kill) >= killStreakKeep {
			h.streak = 0
		}
		h.kill = now
		h.streak++
	}
}

// drawHitMarks draws the hit marker and kill streak (in GUI pixels).
func (g *game) drawHitMarks(ctx *ui.Context) {
	h := &g.hits
	now := time.Now()
	c := ctx.Canvas
	if t := g.gunHUDTex(ctx, "hud:hit_marker", func() (image.Image, error) { return readHUDPNG("hit_marker") }); t != nil {
		offset, since := float32(hitMarkStart), now.Sub(h.hit)
		if k := now.Sub(h.kill); k < hitMarkKeep {
			offset += float32(k) * hitMarkKillGap / float32(hitMarkKeep)
			since = k
		}
		if since < hitMarkKeep {
			a := 1 - float32(since)/float32(hitMarkKeep)
			tint := argb(a, 1, 1, 1)
			if now.Sub(h.head) < hitMarkKeep {
				tint = argb(a, 1, 0, 0)
			}
			x, y := float32(int(float32(ctx.W)/2-8)), float32(int(float32(ctx.H)/2-8))
			o := float32(int(offset))
			for _, q := range [4][2]float32{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
				px, py := x-o+q[0]*(8+2*o), y-o+q[1]*(8+2*o)
				ui.DrawTexScaled(c, t, px, py, 8, 8, q[0]*8, q[1]*8, 8, 8, 16, 16, tint)
			}
		}
	}
	since := now.Sub(h.kill)
	if since >= killStreakKeep {
		return
	}
	// "☠ x 07": the skull is an icon (the UI font has no ☠ glyph).
	text := fmt.Sprintf(" x %02d", h.streak)
	alpha := float32(1)
	if fade := killStreakKeep * 2 / 3; since > fade {
		alpha = 1 - float32(since-fade)/float32(killStreakKeep-fade)*0xF0/0xFF
	}
	r, gg, b := hsv((1-min(float32(h.streak)/killHueCount, 1))*killHueMax, 0.75, 1)
	f, color := ctx.Font, argb(alpha, r, gg, b)
	const scale = 0.5
	x := float32(ctx.W) - float32(killSkullSize+f.Width(text))/2 // in scaled units
	y := float32((ctx.H-45)*2 - 1)
	if t := g.gunHUDTex(ctx, "hud:kill_skull", func() (image.Image, error) { return readHUDPNG("kill_skull") }); t != nil {
		ui.DrawTexScaled(c, t, x*scale, y*scale, killSkullSize*scale, killSkullSize*scale, 0, 0, 1, 1, 1, 1, color)
	}
	f.DrawScaled(c, text, 0, 0, scale, x+killSkullSize, y, color, false)
}

// argb packs a colour (components 0..1).
func argb(a, r, g, b float32) uint32 {
	c := func(v float32) uint32 { return uint32(max(0, min(1, v))*255 + 0.5) }
	return c(a)<<24 | c(r)<<16 | c(g)<<8 | c(b)
}

// hsv is Mth.hsvToRgb (all 0..1).
func hsv(h, s, v float32) (r, g, b float32) {
	i := int(h*6) % 6
	f := h*6 - float32(int(h*6))
	p, q, t := v*(1-s), v*(1-f*s), v*(1-(1-f)*s)
	switch i {
	case 0:
		return v, t, p
	case 1:
		return q, v, p
	case 2:
		return p, v, t
	case 3:
		return p, q, v
	case 4:
		return t, p, v
	}
	return v, p, q
}
