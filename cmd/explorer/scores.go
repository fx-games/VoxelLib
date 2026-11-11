package main

import (
	"fmt"
	"time"

	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/ui"
)

// drawMatch draws the multiplayer match state: a line at the top (team
// score, battle royale round and wall, zombie defense's zombies and
// nearest pickup), and the ranking while Tab is held
// (and while dead, like Q3).
func (g *game) drawMatch(ctx *ui.Context) {
	if g.net == nil {
		return
	}
	r := g.net.cl.Round()
	l := matchLine(r)
	if r.Mode == mp.ZombieDefense {
		l = g.defenseLine()
	}
	if l != "" {
		f := ctx.Font
		w := f.Width(l)
		ui.DrawRect(ctx.Canvas, ctx.W/2-w/2-3, 2, ctx.W/2+w/2+3, 14, 0x60000000)
		f.Draw(ctx.Canvas, l, ctx.W/2-w/2, 4, 0xFFFFFF, true)
	}
	if g.showScores || g.net.cl.Own().Dead {
		g.drawScores(ctx, r)
	}
}

// matchLine is the top line for a match state.
func matchLine(r mp.Round) string {
	secs := func(t int) string {
		d := time.Duration(t) * time.Second / mp.TickRate
		return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
	}
	switch r.Mode {
	case mp.TeamDeathmatch:
		return fmt.Sprintf("§cRed %d§r : §9%d Blue", r.RedKills, r.BlueKills)
	case mp.BattleRoyale:
		switch r.Phase {
		case mp.PhaseWaiting:
			return "§eBattle royale: waiting for a second player"
		case mp.PhaseCountdown:
			return fmt.Sprintf("§eThe round starts in %d", (r.Ticks+mp.TickRate-1)/mp.TickRate)
		case mp.PhaseOver:
			return "§eRound over"
		}
		if r.Shrinking {
			return fmt.Sprintf("%d alive  §cThe wall is closing in! %s", r.Alive, secs(r.Ticks))
		}
		return fmt.Sprintf("%d alive  The wall closes in %s", r.Alive, secs(r.Ticks))
	}
	return ""
}

// drawScores draws the ranking: the players by kills (rounds won first in
// battle royale) with their deaths, by team in team deathmatch, this
// player's line highlighted.
func (g *game) drawScores(ctx *ui.Context, r mp.Round) {
	ranks := g.net.cl.Ranking()
	f := ctx.Font
	const rowH, nameW, numW, pad = 11, 110, 46, 6
	cols := []string{"Kills", "Deaths"}
	if r.Mode == mp.BattleRoyale {
		cols = append([]string{"Wins"}, cols...)
	}
	// Rows: the header, then per team a team line and its players.
	type row struct {
		team int // a team line
		rank mp.Rank
		n    int
	}
	var rows []row
	if r.Mode == mp.TeamDeathmatch {
		for _, t := range []int{mp.TeamRed, mp.TeamBlue} {
			rows = append(rows, row{team: t})
			n := 0
			for _, rk := range ranks {
				if rk.Team == t {
					n++
					rows = append(rows, row{rank: rk, n: n})
				}
			}
		}
	} else {
		for i, rk := range ranks {
			rows = append(rows, row{rank: rk, n: i + 1})
		}
	}
	w := 18 + nameW + len(cols)*numW + 2*pad
	x0, y0 := ctx.W/2-w/2, 30
	h := (len(rows)+1)*rowH + 3 + 2*pad - 2
	ui.DrawRect(ctx.Canvas, x0, y0, x0+w, y0+h, 0xA0000000)
	y := y0 + pad
	text := func(s string, x int, right bool, c uint32) {
		if right {
			x -= f.Width(s)
		}
		f.Draw(ctx.Canvas, s, x, y, c, true)
	}
	colX := func(i int) int { return x0 + w - pad - (len(cols)-1-i)*numW }
	text("#", x0+pad, false, 0xA0A0A0)
	text(r.Mode.String(), x0+pad+18, false, 0xA0A0A0)
	for i, c := range cols {
		text(c, colX(i), true, 0xA0A0A0)
	}
	y += rowH + 3
	me := g.net.cl.Slot()
	for _, rw := range rows {
		if rw.team != 0 {
			kills := r.RedKills
			if rw.team == mp.TeamBlue {
				kills = r.BlueKills
			}
			text(mp.TeamColor(rw.team)+"Team "+mp.TeamName(rw.team), x0+pad, false, 0xFFFFFF)
			text(mp.TeamColor(rw.team)+fmt.Sprint(kills), colX(0), true, 0xFFFFFF)
			y += rowH
			continue
		}
		rk := rw.rank
		c := uint32(0xFFFFFF)
		if rk.Slot == me {
			ui.DrawRect(ctx.Canvas, x0+2, y-1, x0+w-2, y+rowH-2, 0x40FFFFFF)
			c = 0xFFFF55
		}
		name := f.TrimToWidth(rk.Name, nameW, false)
		if r.Mode == mp.BattleRoyale && r.Phase == mp.PhasePlaying && g.out(rk.Slot) {
			name, c = "§m"+name, 0x808080 // out of the round
		}
		text(fmt.Sprint(rw.n), x0+pad, false, c)
		text(name, x0+pad+18, false, c)
		vals := []int{rk.Kills, rk.Deaths}
		if r.Mode == mp.BattleRoyale {
			vals = append([]int{rk.Wins}, vals...)
		}
		for i, v := range vals {
			text(fmt.Sprint(v), colX(i), true, c)
		}
		y += rowH
	}
}

// out reports whether the player in slot is dead (in battle royale: out
// of the round).
func (g *game) out(slot int) bool {
	if slot == g.net.cl.Slot() {
		return g.net.cl.Own().Dead
	}
	for _, v := range g.net.cl.Players() {
		if v.Slot == slot {
			return v.Dead
		}
	}
	return false
}
