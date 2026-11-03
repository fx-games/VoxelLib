package dungeon

import (
	"math/rand"
	"testing"

	"github.com/memmaker/voxellib/grid"
)

// GetJPSPath finds shortest straight-step paths: as long as a BFS's.
func TestGetJPSPath(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	for range 50 {
		m := NewDungeonMap(30, 20)
		for i := range m.tiles {
			if rnd.Float64() < 0.7 {
				m.tiles[i] = Room
			}
		}
		a := grid.Point{X: rnd.Intn(30), Y: rnd.Intn(20)}
		b := grid.Point{X: rnd.Intn(30), Y: rnd.Intn(20)}
		got, want := len(m.GetJPSPath(a, b)), bfs(m, a, b)
		if got != want {
			t.Fatalf("%v to %v: path of %d cells, BFS %d", a, b, got, want)
		}
	}
}

// bfs is the cell count of a shortest 4-connected path a..b, 0 for none.
func bfs(m *DungeonMap, a, b grid.Point) int {
	if !m.IsWalkable(a) || !m.IsWalkable(b) {
		return 0
	}
	dist := map[grid.Point]int{a: 1}
	q := []grid.Point{a}
	for len(q) > 0 {
		p := q[0]
		q = q[1:]
		if p == b {
			return dist[p]
		}
		for _, d := range []grid.Point{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}} {
			n := p.Add(d)
			if _, seen := dist[n]; !seen && m.IsWalkable(n) {
				dist[n] = dist[p] + 1
				q = append(q, n)
			}
		}
	}
	return 0
}
