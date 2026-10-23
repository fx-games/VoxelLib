package potion

import "testing"

// TestBrew: PotionHelper.applyIngredient on the classic chains (water +
// nether wart = awkward; + sugar = swiftness; + redstone extended, +
// glowstone II, + gunpowder splash; water + fermented spider eye =
// weakness; water + sugar = mundane 8192, as the &4 test zeroes and the
// rest still applies) and the stand's canBrew/brewPotions tests.
func TestBrew(t *testing.T) {
	const wart, sugar, red, glow, gun, fse = "+4", "-0+1-2-3&4-4+13", "-5+6-7", "+5-6-7", "+14&13-13", "-0+3-4+13"
	for _, c := range []struct {
		from int
		ing  string
		to   int
		can  bool
	}{
		{0, wart, 16, true}, {16, sugar, 8194, true}, {8194, red, 8258, true}, {8194, glow, 8226, true},
		{8194, gun, 16386, true}, {0, fse, 8200, true}, {0, sugar, 8192, true}, {0, red, 64, true},
		{8258, red, 8258, false}, {16386, gun, 0, false}, {16, wart, 16, false},
	} {
		k := Apply(c.from, c.ing)
		if k != c.to || CanBrew(c.from, k) != c.can {
			t.Errorf("%d %q: %d %v, want %d %v", c.from, c.ing, k, CanBrew(c.from, k), c.to, c.can)
		}
		if want := c.from; c.can && Brewed(c.from, k) != k || !c.can && Brewed(c.from, k) != want {
			t.Errorf("%d %q: brewed %d", c.from, c.ing, Brewed(c.from, k))
		}
	}
	// A potion whose effects stay the same does not brew: swiftness (8194)
	// and 8195 (bit 0 makes no other effect) differ in damage only.
	if CanBrew(8194, 8194|1<<11) {
		t.Error("same effects brewed")
	}
}
