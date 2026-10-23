package app

import (
	"testing"
)

// TestHeldUse drives the use action through Input.Hold: a press uses the
// item once, a use in progress stays while the button is held and does not
// repeat, the release ends it, and an idle held button repeats every 4
// ticks.
func TestHeldUse(t *testing.T) {
	a := Headless()
	calls, using, start := 0, false, true
	a.Hooks.UseItem = func() bool { calls++; using = start; return start }
	a.Hooks.UsingItem = func() bool { return using }
	a.Hooks.Action = func(name string, pressed bool) bool {
		if name == ActUse && !pressed {
			using = false
		}
		return false
	}
	a.Input.Hold(ActUse, true)
	for i := 0; i < 20; i++ {
		a.TickInput()
	}
	if calls != 1 || !using {
		t.Fatalf("held use: %d calls, using %v; want 1, true", calls, using)
	}
	a.Input.Hold(ActUse, false)
	a.TickInput()
	if using || calls != 1 {
		t.Fatalf("release: using %v, %d calls", using, calls)
	}
	start, calls = false, 0 // an item with no use: retried every 4 ticks
	a.Input.Hold(ActUse, true)
	for i := 0; i < 9; i++ {
		a.TickInput()
	}
	if calls != 3 { // ticks 0, 4, 8
		t.Fatalf("idle repeat: %d calls, want 3", calls)
	}
}
