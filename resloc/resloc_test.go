package resloc

import "testing"

func TestQualify(t *testing.T) {
	if got := Qualify("stone"); got != Default+":stone" {
		t.Errorf("Qualify(stone) = %q", got)
	}
	if got := Qualify("mod:planks"); got != "mod:planks" {
		t.Errorf("Qualify(mod:planks) = %q", got)
	}
}
