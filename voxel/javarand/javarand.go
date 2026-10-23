// Package javarand is java.util.Random, bit for bit, for effects Minecraft
// seeds by position or tick.
package javarand

import "math"

// Random is a java.util.Random; call SetSeed before use.
type Random struct {
	seed     int64
	gauss    float64
	hasGauss bool
}

const (
	mult = 0x5DEECE66D
	mask = 1<<48 - 1
)

// SetSeed is Random.setSeed.
func (r *Random) SetSeed(s int64) { r.seed, r.hasGauss = (s^mult)&mask, false }

// Next is Random.next(bits).
func (r *Random) Next(bits uint) int32 {
	r.seed = (r.seed*mult + 0xB) & mask
	return int32(r.seed >> (48 - bits))
}

// NextInt is Random.nextInt(n), n > 0.
func (r *Random) NextInt(n int32) int32 {
	if n&-n == n {
		return int32(int64(n) * int64(r.Next(31)) >> 31)
	}
	for {
		bits := r.Next(31)
		val := bits % n
		if bits-val+(n-1) >= 0 {
			return val
		}
	}
}

// NextLong is Random.nextLong.
func (r *Random) NextLong() int64 { return int64(r.Next(32))<<32 + int64(r.Next(32)) }

// NextFloat is Random.nextFloat.
func (r *Random) NextFloat() float32 { return float32(r.Next(24)) / (1 << 24) }

// NextDouble is Random.nextDouble.
func (r *Random) NextDouble() float64 {
	return float64(int64(r.Next(26))<<27+int64(r.Next(27))) / (1 << 53)
}

// NextGaussian is Random.nextGaussian (polar method, pairs cached).
func (r *Random) NextGaussian() float64 {
	if r.hasGauss {
		r.hasGauss = false
		return r.gauss
	}
	for {
		v1, v2 := 2*r.NextDouble()-1, 2*r.NextDouble()-1
		s := v1*v1 + v2*v2
		if s < 1 && s != 0 {
			m := math.Sqrt(-2 * math.Log(s) / s) // StrictMath in Java; equal for these inputs in practice
			r.gauss, r.hasGauss = v2*m, true
			return v1 * m
		}
	}
}
