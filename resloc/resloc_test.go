package resloc

import "testing"

func TestQualify(t *testing.T) {
	if got := Qualify("stone"); got != Default+":stone" {
		t.Errorf("Qualify(stone) = %q", got)
	}
	if got := Qualify("lotr:planks"); got != "lotr:planks" {
		t.Errorf("Qualify(lotr:planks) = %q", got)
	}
}
