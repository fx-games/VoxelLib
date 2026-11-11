package main

import (
	"slices"
	"testing"
)

// TestArsenal: a picked-up gun is kept once and drawn only into empty
// hands; V cycles the guns in pickup order, then the bare hand.
func TestArsenal(t *testing.T) {
	var a arsenal
	if !a.add("a", false) {
		t.Error("a gun into empty hands is not drawn")
	}
	if a.add("b", true) {
		t.Error("a gun is drawn over the one in hand")
	}
	a.add("a", true)
	if !slices.Equal(a.owned, []string{"a", "b"}) {
		t.Fatalf("owned %v", a.owned)
	}
	for cur, want := range map[string]string{"": "a", "a": "b", "b": ""} {
		if got := a.next(cur); got != want {
			t.Errorf("next(%q) = %q, want %q", cur, got, want)
		}
	}
}
