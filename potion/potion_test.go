package potion

import (
	"reflect"
	"testing"
)

// TestEffects: the reference's potions by damage value (swiftness 8194,
// extended 8258, II 8226; splash regeneration 16385; splash instant
// health II 16421; awkward 16 none; invisibility 8238) and their colours.
func TestEffects(t *testing.T) {
	for _, c := range []struct {
		d    int
		want []Effect
	}{
		{8194, []Effect{{1, 3600, 0, false}}},
		{8258, []Effect{{1, 9600, 0, false}}},
		{8226, []Effect{{1, 1800, 1, false}}},
		{16385, []Effect{{10, 676, 0, true}}},
		{16421, []Effect{{6, 1, 1, true}}},
		{16, nil},
		{8238, []Effect{{14, 3600, 0, false}}},
	} {
		if got := Effects(c.d); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%d: %v, want %v", c.d, got, c.want)
		}
	}
	if Color(nil) != 3694022 || Color(Effects(8194)) != 8171462 {
		t.Fatalf("colours %d %d", Color(nil), Color(Effects(8194)))
	}
}
