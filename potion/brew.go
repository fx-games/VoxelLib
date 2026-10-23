package potion

import "slices"

// Require adds (or replaces) an effect's requirement and amplifier
// strings, as mods extend the reference's tables ("" for no amplifier).
func Require(id int, req, amp string) {
	requirements[id] = req
	if amp == "" {
		delete(amplifiers, id)
	} else {
		amplifiers[id] = amp
	}
}

// IsSplash is ItemPotion.isSplash.
func IsSplash(damage int) bool { return damage&SplashBit != 0 }

// Apply is PotionHelper.applyIngredient: an ingredient's brewing string
// ("+n" sets bit n, "-n" clears it, "!n" toggles it, "&n" ends the brew
// unless bit n is set) applied to a potion's damage value.
func Apply(damage int, ingredient string) int {
	set := func(v, n int, clear, toggle, need bool) int {
		switch {
		case need:
			if !flag(v, n) {
				return 0
			}
		case clear:
			v &^= 1 << n
		case toggle:
			v ^= 1 << n
		default:
			v |= 1 << n
		}
		return v
	}
	digits, toggle, clear, need, n := false, false, false, false, 0
	flush := func() {
		if digits {
			damage = set(damage, n, clear, toggle, need)
			digits, toggle, clear, need, n = false, false, false, false, 0
		}
	}
	for i := 0; i < len(ingredient); i++ {
		switch c := ingredient[i]; {
		case c >= '0' && c <= '9':
			n, digits = n*10+int(c-'0'), true
		case c == '!':
			flush()
			toggle = true
		case c == '-':
			flush()
			clear = true
		case c == '+':
			flush()
		case c == '&':
			flush()
			need = true
		}
	}
	flush()
	return damage & 32767
}

// differ is the reference's list identity test between two potions'
// cached effect lists: the same damage shares its list, and two potions
// without effects both have none.
func differ(j, k int) bool { return j != k && (Effects(j) != nil || Effects(k) != nil) }

// changes is the stand's test of what an ingredient does to a potion j
// brewing into k: its effects change (a new, different, non-empty list)
// for a potion that is not plain water or that has a new list.
func changes(j, k int) bool {
	l6, l7 := Effects(j), Effects(k)
	return (j <= 0 || differ(j, k)) && (l6 == nil || !slices.Equal(l6, l7) && l7 != nil)
}

// CanBrew is the brewing stand's canBrew test for one potion slot holding
// damage j with an ingredient brewing it into k.
func CanBrew(j, k int) bool {
	return !IsSplash(j) && IsSplash(k) || changes(j, k) && j != k
}

// Brewed is the potion's damage after brewPotions: k when its effects
// change or it turns splash, else j.
func Brewed(j, k int) int {
	if changes(j, k) || !IsSplash(j) && IsSplash(k) {
		return k
	}
	return j
}
