package character

import "testing"

// TestStep: the first step after 1/0.6 blocks, then one per further
// 1/0.6; nothing over air; height counts only on a ladder.
func TestStep(t *testing.T) {
	var s Step
	n := 0
	for range 100 { // 10 blocks: walked 6 -> steps at >1, >2, ... >5 (6 is not past 6)
		if s.Moved(0.1, 0, 0, false, false) {
			n++
		}
	}
	if n != 5 {
		t.Fatal("steps", n)
	}
	if s.Moved(5, 0, 0, false, true) {
		t.Fatal("stepped on air")
	}
	var l Step
	if l.Moved(0, 2, 0, false, false) || !l.Moved(0, 2, 0, true, false) {
		t.Fatal("ladder climb")
	}
	if v := SwimVolume(0, 5, 0); v != 1 {
		t.Fatal("swim volume", v)
	}
}
