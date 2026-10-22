package survival

import "math"

// XP is the player's experience (EntityPlayer experience fields).
type XP struct {
	Level    int
	Progress float32 // 0..1 within the level
	Total    int
}

// BarCap is xpBarCap: points needed for the next level.
func (x *XP) BarCap() int {
	switch {
	case x.Level >= 30:
		return 62 + (x.Level-30)*7
	case x.Level >= 15:
		return 17 + (x.Level-15)*3
	}
	return 17
}

// Add is addExperience.
func (x *XP) Add(n int) {
	n = min(n, math.MaxInt32-x.Total)
	x.Progress += float32(n) / float32(x.BarCap())
	x.Total += n
	for x.Progress >= 1 {
		x.Progress = (x.Progress - 1) * float32(x.BarCap())
		x.Level++
		x.Progress /= float32(x.BarCap())
	}
}

// DeathDrop is the XP a dying player drops: level×7, at most 100.
func (x *XP) DeathDrop() int { return min(x.Level*7, 100) }

// XPSplit is EntityXPOrb.getXPSplit: the largest orb value up to n.
func XPSplit(n int) int {
	for _, v := range []int{2477, 1237, 617, 307, 149, 73, 37, 17, 7, 3} {
		if n >= v {
			return v
		}
	}
	return 1
}
