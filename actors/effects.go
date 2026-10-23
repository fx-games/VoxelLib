package actors

// Status effects on non-player actors (the reference's potion effects):
// the game lists the effect kinds in Manager.Effects; an actor's running
// effects tick with it (each acts when ready, then counts down and ends
// at 0) and put their modifiers on it while they last. A player's
// effects are the game's.

// Effect is one running effect: the ticks left and the amplifier (level-1).
type Effect struct {
	Ticks int `json:"t"`
	Amp   int `json:"a"`
}

// EffectRule is one effect kind.
type EffectRule struct {
	Name string
	// Period: the effect acts when the remaining ticks are a multiple of
	// Period>>amp (isReady; 0 never, a period shifted to 0 every tick).
	Period int
	// Damage is the armour-bypassing hurt each time it acts, only while
	// Health > Floor; Heal the healing, up to MaxHealth.
	Damage, Floor, Heal float64
	// Speed is the movement speed modifier per level (operation 2: the
	// speed times 1 + Speed*level).
	Speed float64
	// Resist sets Resistance to the level, FireProof sets FireProof.
	Resist, FireProof bool
	// Color is the liquid colour 0xRRGGBB, mixed by EffectColor.
	Color int
}

func (m *Manager) rule(name string) *EffectRule {
	for i := range m.Effects {
		if m.Effects[i].Name == name {
			return &m.Effects[i]
		}
	}
	return nil
}

// AddEffect is addPotionEffect: unless a.Refuse turns it down, a running
// effect is replaced only by a higher amplifier or, at the same one, by
// more ticks. It reports whether a has the effect now.
func (m *Manager) AddEffect(a *Actor, name string, ticks, amp int) bool {
	if a.Refuse != nil && a.Refuse(a, name) {
		return false
	}
	if a.Effects == nil {
		a.Effects = map[string]*Effect{}
	}
	if e := a.Effects[name]; e != nil {
		if amp > e.Amp {
			e.Amp, e.Ticks = amp, ticks
		} else if amp == e.Amp && e.Ticks < ticks {
			e.Ticks = ticks
		}
	} else {
		a.Effects[name] = &Effect{Ticks: ticks, Amp: amp}
	}
	m.applyEffects(a)
	return true
}

// effectTick is updatePotionEffects' server half, in Effects order.
func (m *Manager) effectTick(a *Actor) {
	if len(a.Effects) == 0 {
		return
	}
	for i := range m.Effects {
		r := &m.Effects[i]
		e := a.Effects[r.Name]
		if e == nil {
			continue
		}
		if e.Ticks > 0 {
			if k := r.Period >> max(e.Amp, 0); r.Period > 0 && (k <= 0 || e.Ticks%k == 0) && a.Alive() {
				if r.Heal > 0 && a.Health < a.MaxHealth {
					a.Health = min(a.MaxHealth, a.Health+r.Heal)
				}
				if r.Damage > 0 && a.Health > r.Floor {
					m.Hurt(a, nil, r.Damage, true)
				}
			}
			e.Ticks--
		}
		if e.Ticks <= 0 {
			delete(a.Effects, r.Name)
			m.applyEffects(a)
		}
	}
}

// applyEffects puts the running effects' modifiers on a.
func (m *Manager) applyEffects(a *Actor) {
	mul := 1.0
	a.Resistance, a.FireProof = 0, false
	for i := range m.Effects {
		r := &m.Effects[i]
		e := a.Effects[r.Name]
		if e == nil {
			continue
		}
		l := float64(e.Amp + 1)
		if r.Speed != 0 {
			mul *= 1 + r.Speed*l
		}
		if r.Resist {
			a.Resistance = e.Amp + 1
		}
		if r.FireProof {
			a.FireProof = true
		}
	}
	a.speedOff = mul - 1
}

// EffectColor is the running effects' mixed colour (calcPotionLiquidColor:
// each effect counted amp+1 times), 0 with no effect at level 1 or more.
func (m *Manager) EffectColor(a *Actor) int {
	var r, g, b, n float32
	for i := range m.Effects {
		e := a.Effects[m.Effects[i].Name]
		if e == nil {
			continue
		}
		c := m.Effects[i].Color
		for k := 0; k <= e.Amp; k++ {
			r += float32(c>>16&255) / 255
			g += float32(c>>8&255) / 255
			b += float32(c&255) / 255
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return int(r/n*255)<<16 | int(g/n*255)<<8 | int(b/n*255)
}
