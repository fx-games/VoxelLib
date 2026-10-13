package anvil

import "testing"

func TestLegacyState(t *testing.T) {
	if n, p := legacyState(35, 14); n != "red_wool" || p != nil {
		t.Fatal(n)
	}
	if n, p := legacyState(53, 4|2); n != "oak_stairs" || p["facing"] != "south" || p["half"] != "top" {
		t.Fatal(n, p)
	}
	if n, p := legacyState(50, 1); n != "wall_torch" || p["facing"] != "east" {
		t.Fatal(n, p)
	}
}
