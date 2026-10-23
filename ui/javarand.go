package ui

// javaRandom is java.util.Random, for the HUD's heart and hunger jitter
// that the reference seeds per tick.
type javaRandom struct{ seed int64 }

func (r *javaRandom) SetSeed(s int64) { r.seed = (s ^ 0x5DEECE66D) & (1<<48 - 1) }

func (r *javaRandom) next(bits uint) int32 {
	r.seed = (r.seed*0x5DEECE66D + 0xB) & (1<<48 - 1)
	return int32(r.seed >> (48 - bits))
}

// NextInt is Random.nextInt(n).
func (r *javaRandom) NextInt(n int32) int32 {
	if n&-n == n {
		return int32((int64(n) * int64(r.next(31))) >> 31)
	}
	for {
		bits := r.next(31)
		val := bits % n
		if bits-val+(n-1) >= 0 {
			return val
		}
	}
}
