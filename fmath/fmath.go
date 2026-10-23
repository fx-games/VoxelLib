// Package fmath is the reference's float maths: the 65536-entry sine table
// and float-to-int casts, so ported formulas round as the reference does.
package fmath

import "math"

var sinTable = func() []float32 {
	t := make([]float32, 65536)
	for i := range t {
		t[i] = float32(math.Sin(float64(i) * math.Pi * 2 / 65536))
	}
	return t
}()

// Sin is the table sine of f radians.
func Sin(f float32) float32 { return sinTable[I(f*10430.378)&65535] }

// Cos is the table cosine of f radians.
func Cos(f float32) float32 { return sinTable[I(f*10430.378+16384)&65535] }

// Pi is pi as a float.
const Pi float32 = 3.1415927

// I is a float-to-int cast: truncated, NaN 0, saturating.
func I(f float32) int32 {
	switch {
	case f != f:
		return 0
	case f >= math.MaxInt32:
		return math.MaxInt32
	case f <= math.MinInt32:
		return math.MinInt32
	}
	return int32(f)
}
