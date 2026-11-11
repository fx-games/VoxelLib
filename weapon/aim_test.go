package weapon

import (
	"testing"
	"time"
)

// TestAimProgress raises the sights over the aim time in ticks of
// (50+1) ms as LivingEntityAim does, clamps at 0 and 1, and an aim time of
// 0 raises at once.
func TestAimProgress(t *testing.T) {
	const tick = 50 * time.Millisecond
	var p AimProgress
	n := 0
	for ; p < Aimed; n++ {
		p = p.Tick(true, 0.2, tick)
	}
	if n != 4 { // 51/200 per tick: 0.255, 0.51, 0.765, 1
		t.Errorf("0.2 s aim took %d ticks, want 4", n)
	}
	if p = p.Tick(false, 0.2, tick); p != 1-AimProgress(51.0/200) {
		t.Errorf("lowering: %v", p)
	}
	if p = AimProgress(0).Tick(true, 0, tick); p != Aimed {
		t.Errorf("0 s aim: %v", p)
	}
	if p = AimProgress(0.1).Tick(false, 0.2, tick); p != 0 {
		t.Errorf("clamp at 0: %v", p)
	}
}
