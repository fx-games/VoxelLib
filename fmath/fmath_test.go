package fmath

import "testing"

func TestTable(t *testing.T) {
	if Sin(0) != 0 || Cos(0) != 1 || Sin(Pi/2) != 1 {
		t.Error("table")
	}
	// The table rounds down to its 65536 steps.
	if Sin(0.00009) != 0 {
		t.Errorf("sin(9e-5) %v", Sin(0.00009))
	}
	if I(-1.9) != -1 || I(3e9) != 2147483647 {
		t.Error("cast")
	}
}
