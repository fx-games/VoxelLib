package grid

import "testing"

func TestJPSPathAroundWall(t *testing.T) {
	pr := NewPathRange(NewRect(0, 0, 10, 10))
	wall := func(p Point) bool { return !(p.X == 5 && p.Y < 9) }
	path := pr.JPSPath(nil, Point{X: 1, Y: 1}, Point{X: 8, Y: 1}, wall, false)
	if len(path) == 0 || path[0] != (Point{X: 8, Y: 1}) && path[len(path)-1] != (Point{X: 8, Y: 1}) {
		t.Fatalf("no path to target: %v", path)
	}
	for _, p := range path {
		if !wall(p) {
			t.Fatalf("path crosses wall at %v", p)
		}
	}
}
