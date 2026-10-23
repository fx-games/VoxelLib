package collide

import (
	"math"
	"testing"
)

func wallAt(x int) SolidsFunc {
	return func(bx, by, bz int) bool { return bx == x }
}

func mover(w Solids, b *Bodies) *Mover {
	if b == nil {
		b = NewBodies()
	}
	return &Mover{Shapes: b.Shapes(w, -1, nil)}
}

var unitBox = AABB{0, 0, 0, 1, 1, 1}

func TestSlideStopsAtWall(t *testing.T) {
	got, cs := mover(wallAt(2), nil).Slide(unitBox, [3]float64{5, 0, 0})
	if math.Abs(got[0]-1) > 0.01 || len(cs) != 1 || cs[0].Normal != ([3]float64{-1, 0, 0}) || cs[0].Body != -1 {
		t.Fatalf("got %v %+v", got, cs)
	}
}

func TestSlideSlidesAlongWall(t *testing.T) {
	got, _ := mover(wallAt(2), nil).Slide(unitBox, [3]float64{5, 0, 3})
	if math.Abs(got[0]-1) > 0.01 || math.Abs(got[2]-3) > 0.01 {
		t.Fatalf("got %v, want x stopped at 1 and z all the way to 3", got)
	}
}

func TestSlideInCornerTerminates(t *testing.T) {
	corner := SolidsFunc(func(x, y, z int) bool { return x == 2 || z == 2 })
	got, _ := mover(corner, nil).Slide(unitBox, [3]float64{5, 0, 5})
	if got[0] > 1.01 || got[2] > 1.01 {
		t.Fatalf("went through the corner to %v", got)
	}
}

func TestSlideMeetsATurnedBodyAtItsSurface(t *testing.T) {
	b := NewBodies()
	c, s := math.Cos(math.Pi/4), math.Sin(math.Pi/4)
	plank := OBB{Local: AABB{-0.1, 0, -0.5, 0.1, 1, 0.5}, Axes: [3][3]float64{{c, 0, -s}, {0, 1, 0}, {s, 0, c}}, Pos: [3]float64{3, 0, 0.5}}
	id := b.Add(Body{Box: AABB{2.5, 0, 0, 3.5, 1, 1}, OBB: &plank, Solid: true})
	got, cs := mover(nil, b).Slide(unitBox, [3]float64{5, 0, 0})
	if len(cs) == 0 || cs[0].Body != id {
		t.Fatalf("missed the plank: %v %+v", got, cs)
	}
	if got[0] < 1.5 || got[0] > 2.4 { // further than the enclosing box allows
		t.Fatalf("stopped at %v, not at the turned surface", got[0])
	}
}

func TestSlidePassesNonSolidBodies(t *testing.T) {
	b := NewBodies()
	door := b.Add(Body{Box: AABB{2, 0, 0, 3, 2, 1}, Solid: true})
	if got, _ := mover(nil, b).Slide(unitBox, [3]float64{5, 0, 0}); got[0] > 1.01 {
		t.Fatalf("walked through the shut door to %v", got)
	}
	open := *b.Get(door)
	open.Solid = false
	b.Set(door, open)
	if got, _ := mover(nil, b).Slide(unitBox, [3]float64{5, 0, 0}); got[0] != 5 {
		t.Fatalf("stopped at %v by an open door", got)
	}
}

func TestOnContactCanEndTheMove(t *testing.T) {
	m := mover(wallAt(2), nil)
	m.OnContact = func(Contact) bool { return false }
	got, cs := m.Slide(unitBox, [3]float64{5, 0, 3})
	if len(cs) != 1 || math.Abs(got[2]-cs[0].Point[2]) > 0.01 {
		t.Fatalf("kept sliding to %v after the contact said stop", got)
	}
}

func ledge(h float64) *Bodies {
	b := NewBodies()
	b.Add(Body{Box: AABB{-10, -1, -10, 10, 0, 10}, Solid: true})
	b.Add(Body{Box: AABB{2, 0, -10, 10, h, 10}, Solid: true})
	return b
}

func TestStepSlideClimbsLowLedgesOnly(t *testing.T) {
	box := AABB{0, 0.001, 0, 1, 1.001, 1}
	got, cs := mover(nil, ledge(0.25)).StepSlide(box, [3]float64{1.5, 0, 0}, 0.3)
	if got[0] < 1.4 || got[1] < 0.24 || !Grounded(cs) {
		t.Fatalf("did not step onto the 0.25 ledge: %v %+v", got, cs)
	}
	got, _ = mover(nil, ledge(0.5)).StepSlide(box, [3]float64{1.5, 0, 0}, 0.3)
	if got[0] > 1.01 || got[1] > 0.01 {
		t.Fatalf("climbed a 0.5 wall: %v", got)
	}
}

func TestBodiesIndex(t *testing.T) {
	b := NewBodies()
	id := b.Add(Body{Box: AABB{0.5, 0, 0.5, 2.5, 1, 1}, Bounds: AABB{0.5, 0, 0.5, 2.5, 1.5, 1}, Solid: true})
	found := 0
	b.Query(AABB{1, 1, 0, 2, 2, 1}, func(got int32, _ *Body) {
		if got == id {
			found++
		}
	})
	if found != 1 {
		t.Fatalf("found the body %d times by its bounds", found)
	}
	if n := len(b.Boxes(AABB{1, 1.2, 0, 2, 2, 1}, nil)); n != 0 {
		t.Fatalf("Boxes gave %d boxes above the collision box", n)
	}
	if n := len(b.BoxesExcept(id, AABB{0, 0, 0, 3, 1, 1}, nil)); n != 0 {
		t.Fatal("BoxesExcept kept the excluded body")
	}
	b.Set(id, Body{Box: AABB{10.2, 0.2, 10.2, 10.8, 0.8, 10.8}, Solid: true})
	if b.Cells() != 1 {
		t.Fatalf("moving left %d cells listed", b.Cells())
	}
	b.Remove(id)
	if b.Cells() != 0 {
		t.Fatal("Remove left the body listed")
	}
	if again := b.Add(Body{Box: unitBox}); again != id {
		t.Fatalf("id %d not reused, got %d", id, again)
	}
}
