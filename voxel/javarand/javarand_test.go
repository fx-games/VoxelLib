package javarand

import "testing"

// Values printed by Java 8's java.util.Random.
func TestRandom(t *testing.T) {
	var r Random
	r.SetSeed(0)
	if got := r.Next(32); got != -1155484576 {
		t.Errorf("Random(0).nextInt() = %d", got)
	}
	r.SetSeed(42)
	if got := r.NextGaussian(); got != 1.1419053154730547 {
		t.Errorf("Random(42).nextGaussian() = %v", got)
	}
	r.SetSeed(42)
	if got := r.NextInt(10); got != 0 {
		t.Errorf("Random(42).nextInt(10) = %d", got)
	}
}
