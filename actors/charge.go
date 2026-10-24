package actors

import (
	"math"

	"github.com/memmaker/voxellib/inventory"
)

// Charge is a player's attack strength (the attackStrengthTicker): it
// builds every tick and starts over after a blow, a swing at nothing or a
// change of the main-hand item type. A game turns the charged blow on by
// handing it to Blow.Charge.
type Charge struct {
	Ticker int
	item   *inventory.Item
}

// Tick is Player.tick's part: one more tick, back to 0 when the main hand
// holds another item type than the last tick (not a mere count or damage
// change).
func (c *Charge) Tick(main *inventory.Stack) {
	c.Ticker++
	var it *inventory.Item
	if main != nil {
		it = main.Item
	}
	if it != c.item {
		c.Ticker = 0
		c.item = it
	}
}

// Reset is resetAttackStrengthTicker: after a blow or a swing at air.
func (c *Charge) Reset() { c.Ticker = 0 }

// Delay is getCurrentItemAttackStrengthDelay: 20/attackSpeed ticks, the
// attack speed being 4 plus the held item's AttackSpeed modifier.
func (c *Charge) Delay() float32 {
	speed := 4.0
	if c.item != nil {
		speed += float64(c.item.AttackSpeed)
	}
	return float32(1 / speed * 20)
}

// Scale is getAttackStrengthScale(a): (Ticker+a)/Delay in 0..1. A blow
// reads it at a = 0.5, the HUD at 0.
func (c *Charge) Scale(a float32) float32 {
	return min(max((float32(c.Ticker)+a)/c.Delay(), 0), 1)
}

// Indicator is the crosshair attack indicator's state (Hud's
// extractCrosshair): full shows the 16x16 "full" sprite when the
// crosshair is on a living target (aimLiving), the charge is complete and
// the item is slower than 5 ticks; otherwise below a full charge it shows
// the 16x4 background with progress pixels of the progress sprite.
func (c *Charge) Indicator(aimLiving bool) (progress int, full, show bool) {
	s := c.Scale(0)
	if aimLiving && s >= 1 && c.Delay() > 5 {
		return 0, true, true
	}
	if s < 1 {
		return int(s * 17), false, true
	}
	return 0, false, false
}

// Attack sounds of the charged blow, as sound event names.
const (
	SoundAttackCrit      = "entity.player.attack.crit"
	SoundAttackKnockback = "entity.player.attack.knockback"
	SoundAttackStrong    = "entity.player.attack.strong"
	SoundAttackSweep     = "entity.player.attack.sweep"
	SoundAttackWeak      = "entity.player.attack.weak"
	SoundAttackNoDamage  = "entity.player.attack.nodamage"
)

// strikeCharged is Player.attack's charged blow; see Blow.Charge.
func (m *Manager) strikeCharged(p, t *Actor, b Blow) bool {
	s := b.Charge.Scale(0.5)
	bonus := s * float32(b.Bonus)
	d := float32(b.Damage) * (0.2 + s*s*0.8)
	b.Charge.Reset() // onAttack
	if d <= 0 && bonus <= 0 {
		return false
	}
	if b.Allow != nil && !b.Allow() {
		return false
	}
	sound := func(e string) {
		if b.Sound != nil {
			b.Sound(e)
		}
	}
	full := s > 0.9
	kb := p.Sprinting && full
	if kb {
		sound(SoundAttackKnockback)
	}
	crit := full && b.Crit && !p.Sprinting
	if crit {
		d *= 1.5
	}
	sweep := false
	if full && !crit && !kb && p.Body.OnGround && b.Sweep {
		speed := p.Speed
		if speed == 0 {
			speed = 0.1 // the player's movementSpeed
		}
		v := p.Body.VX*p.Body.VX + p.Body.VZ*p.Body.VZ
		sweep = v < speed*2.5*(speed*2.5)
	}
	lit := b.Fire > 0 && !t.OnFire()
	if lit {
		t.SetFire(1)
	}
	if !m.attack(p, t, float64(d+bonus), b.Knockback, kb) {
		if lit {
			t.Extinguish()
		}
		sound(SoundAttackNoDamage)
		return false
	}
	if sweep {
		m.sweep(p, t, s, b)
	}
	switch {
	case crit:
		sound(SoundAttackCrit)
	case !sweep:
		if full {
			sound(SoundAttackStrong)
		} else {
			sound(SoundAttackWeak)
		}
	}
	if b.Landed != nil {
		b.Landed()
	}
	if b.Fire > 0 {
		t.SetFire(b.Fire * 4)
	}
	return true
}

// sweep is doSweepAttack: 1 damage (no sweeping damage ratio) times the
// strength to every other living actor in t's box grown by 1, 0.25, 1,
// within 3 blocks of p and not allied, each knocked back 0.4 along p's
// facing.
func (m *Manager) sweep(p, t *Actor, s float32, b Blow) {
	if b.Sound != nil {
		b.Sound(SoundAttackSweep)
	}
	dmg := float64(1 * s)
	box := t.Box().Grow(1, 0.25, 1)
	fx, fz := math.Sin(p.Yaw), -math.Cos(p.Yaw)
	for _, o := range m.Actors {
		if o == p || o == t || o.Boat != nil || !o.Alive() || (b.Allied != nil && b.Allied(o)) ||
			!o.Box().Intersects(box) || o.Pos().dist2(p.Pos()) >= 9 {
			continue
		}
		if m.hurtAs(HurtMelee, o, p, dmg, false) {
			m.Knockback(o, 0.4, -fx, -fz)
		}
	}
	if b.Swept != nil {
		b.Swept(fx, fz)
	}
}
