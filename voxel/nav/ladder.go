package nav

// Ladders returns a Links func for climb (a World's Climbable): a walker
// standing at the foot of a ladder is linked to where it steps off at the
// top (the ground beside, or on, the cell above the top rung), and back.
// A body follows the link by walking into the ladder toward the top cell
// (character climbs what it walks into) and down by stepping onto it.
func (g *Graph) Ladders(climb func(x, y, z int) bool) func(p Pos, buf []Pos) []Pos {
	return func(p Pos, buf []Pos) []Pos {
		if climb(p.X, p.Y, p.Z) && g.W.Solid(p.X, p.Y-1, p.Z) { // the foot: up
			top := p.Y
			for climb(p.X, top+1, p.Z) {
				top++
			}
			if top > p.Y {
				buf = g.ladderExits(Pos{p.X, top + 1, p.Z}, buf)
			}
		}
		// The top: down every ladder whose top rung is just below beside p.
		for _, d := range ladderSides {
			x, y, z := p.X+d[0], p.Y-1, p.Z+d[1]
			if !climb(x, y, z) || climb(x, y+1, z) {
				continue
			}
			for climb(x, y-1, z) {
				y--
			}
			if foot := (Pos{x, y, z}); foot != p && g.W.Solid(x, y-1, z) && g.Walkable(foot) {
				buf = append(buf, foot)
			}
		}
		return buf
	}
}

var ladderSides = [5][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// ladderExits appends the places to step off a ladder whose cell above
// the top rung is above: that cell or one beside it with ground under it.
func (g *Graph) ladderExits(above Pos, buf []Pos) []Pos {
	for _, d := range ladderSides {
		if q := (Pos{above.X + d[0], above.Y, above.Z + d[1]}); g.Walkable(q) {
			buf = append(buf, q)
		}
	}
	return buf
}
