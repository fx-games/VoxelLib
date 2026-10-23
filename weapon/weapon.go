// Package weapon is the reference's charge-and-release and blocking maths:
// the draw curve of bows and thrown spears, the pull stages their icons
// follow, the crossbow's load-then-fire state, the view zoom while drawing
// and the damage a raised blade lets through. Which items are which kind,
// their draw times and damage factors are the game's data.
package weapon

// Curve is the draw curve: a raw charge c (ticks held over the draw time)
// becomes (c² + 2c) / 3, at most 1.
func Curve(c float32) float32 { return min((c*c+c*2)/3, 1) }

// Raw is the raw charge: ticks held over the draw time.
func Raw(ticks, draw int) float32 { return float32(ticks) / float32(draw) }

// BowCharge is a bow's release: ok false below the minimum raw charge
// (0.65 for bows: no shot), else the curved charge. A full charge (1) is
// a critical shot.
func BowCharge(ticks, draw int, minRaw float32) (charge float32, ok bool) {
	c := Raw(ticks, draw)
	if c < minRaw {
		return 0, false
	}
	return Curve(c), true
}

// ThrowCharge is a thrown spear's release: the raw charge, curved from
// 0.1 up (below it is thrown as is).
func ThrowCharge(ticks, draw int) float32 {
	c := Raw(ticks, draw)
	if c >= 0.1 {
		c = Curve(c)
	}
	return c
}

// Stage is the pull icon for a raw charge f: 2 at or above hi, 1 above
// mid, 0 above 0, -1 not drawn (bows: hi 0.9, mid 0.65; crossbows 1, 0.5).
func Stage(f, hi, mid float32) int {
	switch {
	case f >= hi:
		return 2
	case f > mid:
		return 1
	case f > 0:
		return 0
	}
	return -1
}

// Crossbow loads while use is held for Draw ticks and fires on the next
// use; the game keeps the loaded state on the item.
type Crossbow struct{ Draw int }

// LoadSound reports whether the load sound plays at this held tick.
func (c Crossbow) LoadSound(ticks int) bool { return ticks == c.Draw }

// Loads reports whether letting go after ticks loads an unloaded crossbow.
func (c Crossbow) Loads(ticks int, loaded bool) bool { return !loaded && ticks >= c.Draw }

// Stage is the crossbow's pull icon: full when loaded.
func (c Crossbow) Stage(ticks int, using, loaded bool) int {
	if loaded {
		return 2
	}
	if !using {
		return -1
	}
	return Stage(Raw(ticks, c.Draw), 1, 0.5)
}

// Zoom is the field-of-view factor while drawing: usage = ticks / draw,
// squared below 1, 1 from there on or when loaded; the view narrows by
// 15% of it.
func Zoom(ticks, draw int, loaded bool) float32 {
	u := Raw(ticks, draw)
	if loaded || u > 1 {
		u = 1
	} else {
		u *= u
	}
	return 1 - u*0.15
}

// FOV eases the view's field-of-view factor towards the target, half the
// way per tick, kept within 0.1..1.5.
type FOV struct{ Prev, Cur float32 }

// Tick moves Cur half way to target.
func (f *FOV) Tick(target float32) {
	f.Prev = f.Cur
	f.Cur = min(max(f.Cur+(target-f.Cur)*0.5, 0.1), 1.5)
}

// At is the factor at partial tick pt.
func (f FOV) At(pt float32) float32 { return f.Prev + (f.Cur-f.Prev)*pt }

// Blocked is the damage a raised blade lets through: (1 + d) / 2. The
// game skips it for damage that ignores armour.
func Blocked(d float32) float32 {
	if d <= 0 {
		return d
	}
	return (1 + d) * 0.5
}
