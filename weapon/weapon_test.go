package weapon

import "testing"

func TestDraw(t *testing.T) {
	// 13 of 20 ticks is the 0.65 floor: (0.4225+1.3)/3.
	if c, ok := BowCharge(12, 20, 0.65); ok || c != 0 {
		t.Errorf("12 ticks shot %v", c)
	}
	if c, ok := BowCharge(13, 20, 0.65); !ok || c != Curve(13.0/20) {
		t.Errorf("13 ticks %v %v", c, ok)
	}
	if c, _ := BowCharge(40, 20, 0.65); c != 1 {
		t.Errorf("over-draw %v", c)
	}
	if c := ThrowCharge(1, 20); c != 0.05 {
		t.Errorf("short throw %v", c)
	}
	if c := ThrowCharge(10, 20); c != Curve(0.5) {
		t.Errorf("throw %v", c)
	}
	if Stage(0.9, 0.9, 0.65) != 2 || Stage(0.65, 0.9, 0.65) != 0 || Stage(0.66, 0.9, 0.65) != 1 || Stage(0, 0.9, 0.65) != -1 {
		t.Error("bow stages")
	}
}

func TestCrossbow(t *testing.T) {
	c := Crossbow{Draw: 50}
	if !c.LoadSound(50) || c.LoadSound(49) || c.Loads(49, false) || !c.Loads(50, false) || c.Loads(60, true) {
		t.Error("load")
	}
	if c.Stage(25, true, false) != 0 || c.Stage(26, true, false) != 1 || c.Stage(50, true, false) != 2 || c.Stage(0, false, true) != 2 {
		t.Error("stages")
	}
}

func TestZoomBlock(t *testing.T) {
	if z := Zoom(10, 20, false); z != 1-0.25*0.15 {
		t.Errorf("half draw %v", z)
	}
	if Zoom(30, 20, false) != 0.85 || Zoom(0, 20, true) != 0.85 {
		t.Error("full zoom")
	}
	var f FOV
	f.Cur = 1
	f.Tick(0.85)
	if f.Cur != 0.925 || f.At(0.5) != 0.9625 {
		t.Errorf("ease %+v", f)
	}
	if Blocked(9) != 5 || Blocked(0) != 0 {
		t.Error("block")
	}
}
