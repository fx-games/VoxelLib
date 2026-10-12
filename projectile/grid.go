package projectile

import (
	"math"

	"github.com/memmaker/voxellib/voxel/collide"
)

// Reacher is a Target whose shape can reach outside its Box (e.g. an
// animated mesh swinging its limbs). The broad phase files it under Reach
// instead of Box.
type Reacher interface {
	Reach() collide.AABB
}

// cellSize is the broad-phase grid's cell edge in blocks: about a mob's
// size, so a target spans few cells and a sub-step's segment one or two.
const cellSize = 4

type cell struct{ x, y, z int32 }

// grid is a uniform spatial hash over the targets, rebuilt every tick (they
// move between ticks, not during one).
type grid struct {
	cells   map[cell][]int32
	targets []Target
	seen    []uint32 // per target: the query that last returned it
	query   uint32
	out     []Target
}

func cellOf(v float64) int32 { return int32(math.Floor(v / cellSize)) }

func (g *grid) build(targets []Target) {
	if g.cells == nil {
		g.cells = map[cell][]int32{}
	}
	for k, v := range g.cells {
		g.cells[k] = v[:0]
	}
	g.targets = targets
	if cap(g.seen) < len(targets) {
		g.seen = make([]uint32, len(targets))
	}
	g.seen = g.seen[:len(targets)]
	clear(g.seen)
	g.query = 0
	for i, t := range targets {
		b := t.Box()
		if r, ok := t.(Reacher); ok {
			b = r.Reach()
		}
		for x := cellOf(b.MinX); x <= cellOf(b.MaxX); x++ {
			for y := cellOf(b.MinY); y <= cellOf(b.MaxY); y++ {
				for z := cellOf(b.MinZ); z <= cellOf(b.MaxZ); z++ {
					k := cell{x, y, z}
					g.cells[k] = append(g.cells[k], int32(i))
				}
			}
		}
	}
}

// near returns the targets filed in any cell the segment's box touches,
// each once. The slice is reused by the next call.
func (g *grid) near(x0, y0, z0, x1, y1, z1 float64) []Target {
	g.query++
	g.out = g.out[:0]
	for x := cellOf(min(x0, x1)); x <= cellOf(max(x0, x1)); x++ {
		for y := cellOf(min(y0, y1)); y <= cellOf(max(y0, y1)); y++ {
			for z := cellOf(min(z0, z1)); z <= cellOf(max(z0, z1)); z++ {
				for _, i := range g.cells[cell{x, y, z}] {
					if g.seen[i] != g.query {
						g.seen[i] = g.query
						g.out = append(g.out, g.targets[i])
					}
				}
			}
		}
	}
	return g.out
}
