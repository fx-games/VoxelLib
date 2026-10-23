// Package potion is the reference's brewed potion data: which effects a
// potion's damage value carries (the requirement and amplifier strings
// read against its bits), their durations, and the liquid colour.
package potion

import (
	"math"
	"strings"
)

// Effect is one potion effect: the effect id, duration in ticks and
// amplifier; Splash marks one from a splash potion.
type Effect struct {
	ID, Duration, Amp int
	Splash            bool
}

// SplashBit is the damage bit of a splash potion.
const SplashBit = 16384

// Type is one effect type: liquid colour, bad (isBadEffect), instant
// (heal, harm, saturation) and effectiveness (the duration factor).
type Type struct {
	Color         int
	Bad, Instant  bool
	Effectiveness float64
}

// Types are the reference's effect types by id (1..23).
var Types = [24]Type{
	1: {8171462, false, false, 1}, 2: {5926017, true, false, 1}, 3: {14270531, false, false, 1.5},
	4: {4866583, true, false, 1}, 5: {9643043, false, false, 1}, 6: {16262179, false, true, 1},
	7: {4393481, true, true, 1}, 8: {7889559, false, false, 1}, 9: {5578058, true, false, 0.25},
	10: {13458603, false, false, 0.25}, 11: {10044730, false, false, 1}, 12: {14981690, false, false, 1},
	13: {3035801, false, false, 1}, 14: {8356754, false, false, 1}, 15: {2039587, true, false, 0.25},
	16: {2039713, false, false, 1}, 17: {5797459, true, false, 1}, 18: {4738376, true, false, 1},
	19: {5149489, true, false, 0.25}, 20: {3484199, true, false, 0.25}, 21: {16284963, false, false, 1},
	22: {2445989, false, false, 1}, 23: {16262179, false, true, 1},
}

var requirements = map[int]string{
	10: "0 & !1 & !2 & !3 & 0+6", 1: "!0 & 1 & !2 & !3 & 1+6", 12: "0 & 1 & !2 & !3 & 0+6",
	6: "0 & !1 & 2 & !3", 19: "!0 & !1 & 2 & !3 & 2+6", 18: "!0 & !1 & !2 & 3 & 3+6",
	7: "!0 & !1 & 2 & 3", 2: "!0 & 1 & !2 & 3 & 3+6", 5: "0 & !1 & !2 & 3 & 3+6",
	16: "!0 & 1 & 2 & !3 & 2+6", 14: "!0 & 1 & 2 & 3 & 2+6", 13: "0 & !1 & 2 & 3 & 2+6",
}

var amplifiers = map[int]string{1: "5", 3: "5", 5: "5", 10: "5", 7: "5", 6: "5", 11: "5", 19: "5"}

// Effects is PotionHelper.getPotionEffects(damage, false), in id order.
func Effects(damage int) []Effect {
	var out []Effect
	for id := 1; id < len(Types); id++ {
		req, ok := requirements[id]
		if !ok {
			continue
		}
		l := parse(req, 0, len(req), damage)
		if l <= 0 {
			continue
		}
		amp := 0
		if a, ok := amplifiers[id]; ok {
			amp = max(parse(a, 0, len(a), damage), 0)
		}
		t := Types[id]
		if t.Instant {
			l = 1
		} else {
			l = 1200 * (l*3 + (l-1)*2)
			l >>= amp
			l = int(math.Floor(float64(l)*t.Effectiveness + 0.5)) // Math.round
			if damage&SplashBit != 0 {
				l = int(math.Floor(float64(l)*0.75 + 0.5 + 0.5)) // Math.round(l*0.75+0.5)
			}
		}
		out = append(out, Effect{ID: id, Duration: l, Amp: amp, Splash: damage&SplashBit != 0})
	}
	return out
}

// Color is calcPotionLiquidColor: the effects' colours averaged, each
// counted amplifier+1 times; 3694022 (water) for none.
func Color(effects []Effect) int {
	if len(effects) == 0 {
		return 3694022
	}
	var r, g, b, n float32
	for _, e := range effects {
		c := 0
		if e.ID > 0 && e.ID < len(Types) {
			c = Types[e.ID].Color
		}
		for range e.Amp + 1 {
			r += float32(c>>16&255) / 255
			g += float32(c>>8&255) / 255
			b += float32(c&255) / 255
			n++
		}
	}
	return int(r/n*255)<<16 | int(g/n*255)<<8 | int(b/n*255)
}

func flag(v, bit int) bool { return v&(1<<bit) != 0 }

func countSetFlags(v int) int {
	n := 0
	for ; v > 0; n++ {
		v &= v - 1
	}
	return n
}

// term is func_77904_a: one bit test (! negates, *k scales, - subtracts,
// =/</> compare the set bit count).
func term(not, scaled, neg bool, cmp, n, k, v int) int {
	i := 0
	switch {
	case not:
		if !flag(v, n) {
			i = 1
		}
	case cmp != -1:
		c := countSetFlags(v)
		if cmp == 0 && c == n || cmp == 1 && c > n || cmp == 2 && c < n {
			i = 1
		}
	default:
		if flag(v, n) {
			i = 1
		}
	}
	if scaled {
		i *= k
	}
	if neg {
		i = -i
	}
	return i
}

// parse is parsePotionEffects over s[from:to] against the damage bits v:
// '|' alternatives, '&' all of (the largest), else a sum of terms.
func parse(s string, from, to, v int) int {
	if from >= len(s) || to < 0 || from >= to {
		return 0
	}
	if l := index(s, '|', from); l >= 0 && l < to {
		if i := parse(s, from, l-1, v); i > 0 {
			return i
		}
		return max(parse(s, l+1, to, v), 0)
	}
	if a := index(s, '&', from); a >= 0 && a < to {
		j := parse(s, from, a-1, v)
		if j <= 0 {
			return 0
		}
		k := parse(s, a+1, to, v)
		if k <= 0 {
			return 0
		}
		return max(j, k)
	}
	star, scaled, digits, not, neg := false, false, false, false, false
	cmp, n, k, sum := -1, 0, 0, 0
	flush := func() {
		if digits {
			sum += term(not, scaled, neg, cmp, n, k, v)
			not, neg, star, scaled, digits = false, false, false, false, false
			k, n, cmp = 0, 0, -1
		}
	}
	for i := from; i < to; i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			if star {
				k, scaled = int(c-'0'), true
			} else {
				n, digits = n*10+int(c-'0'), true
			}
		case c == '*':
			star = true
		case c == '!':
			flush()
			not = true
		case c == '-':
			flush()
			neg = true
		case c == '=' || c == '<' || c == '>':
			flush()
			cmp = map[byte]int{'=': 0, '<': 2, '>': 1}[c]
		case c == '+':
			flush()
		}
	}
	flush()
	return sum
}

func index(s string, c byte, from int) int {
	if i := strings.IndexByte(s[from:], c); i >= 0 {
		return from + i
	}
	return -1
}
