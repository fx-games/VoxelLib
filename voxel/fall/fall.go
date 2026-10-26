// Package fall drops a set of blocks out of a world as one rigid piece: a
// wall or balcony whose supports broke, Dark Messiah style. Detach lifts
// the blocks out, Tick lets the piece fall under gravity until it rests
// on something, and Settle puts the blocks back down where it landed.
package fall

import "math"

// World is what a piece falls through; S is its block state (a name, an id).
type World[S comparable] interface {
	Block(x, y, z int) S
	SetBlock(x, y, z int, s S)
	Solid(x, y, z int) bool
}

// Gravity and terminal speed, in blocks per tick (the reference's falling
// sand: 0.04 per tick, up to 2).
const (
	Gravity  = 0.04
	Terminal = 2.0
)

// Piece is blocks falling as one. Cells are where they were; each is
// Drop blocks lower now.
type Piece[S comparable] struct {
	Cells       [][3]int
	States      []S
	Drop, Speed float64
	PrevDrop    float64 // last tick's Drop, for drawing between ticks
	Landed      bool
}

// Detach lifts the given cells out of w (those that are air, as air, are
// skipped) as a piece about to fall.
func Detach[S comparable](w World[S], cells [][3]int, air S) *Piece[S] {
	p := &Piece[S]{}
	for _, c := range cells {
		if s := w.Block(c[0], c[1], c[2]); s != air {
			p.Cells = append(p.Cells, c)
			p.States = append(p.States, s)
		}
	}
	for _, c := range p.Cells {
		w.SetBlock(c[0], c[1], c[2], air)
	}
	return p
}

// Tick falls one tick; it reports whether the piece landed this tick. It
// lands on the first solid cell below any of its blocks that is not one
// of its own.
func (p *Piece[S]) Tick(w World[S]) bool {
	if p.Landed {
		return false
	}
	p.PrevDrop = p.Drop
	p.Speed = math.Min(Terminal, p.Speed+Gravity)
	own := map[[3]int]bool{}
	for _, c := range p.Cells {
		own[c] = true
	}
	// The most it can fall: down to just above the highest floor under it.
	room := p.Drop + p.Speed
	for _, c := range p.Cells {
		if own[[3]int{c[0], c[1] - 1, c[2]}] {
			continue // another of its blocks is under this one
		}
		for d := math.Floor(p.Drop) + 1; d <= math.Ceil(p.Drop+p.Speed); d++ {
			if y := c[1] - int(d); w.Solid(c[0], y, c[2]) {
				room = math.Min(room, d-1)
				break
			}
		}
	}
	p.Drop = room
	if room < p.PrevDrop+p.Speed {
		p.Drop, p.Speed, p.Landed = math.Floor(room), 0, true
		return true
	}
	return false
}

// At is block i's lower corner now (Drop blocks below its cell).
func (p *Piece[S]) At(i int) (x, y, z float64) {
	c := p.Cells[i]
	return float64(c[0]), float64(c[1]) - p.Drop, float64(c[2])
}

// Settle places a landed piece's blocks where they came to rest, but for
// those keep refuses (a body lies there) and those whose cell is not empty.
func (p *Piece[S]) Settle(w World[S], air S, keep func(x, y, z int) bool) {
	d := int(p.Drop)
	for i, c := range p.Cells {
		x, y, z := c[0], c[1]-d, c[2]
		if w.Block(x, y, z) == air && (keep == nil || keep(x, y, z)) {
			w.SetBlock(x, y, z, p.States[i])
		}
	}
}
