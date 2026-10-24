package actors

import "github.com/memmaker/voxellib/inventory"

// Endless is getMaxItemUseDuration for an item held until it is released
// (a raised blade, a drawn bow): 72000 ticks.
const Endless = 72000

// ItemUse is a player's item in use (itemInUse) and its countdown
// (itemInUseCount). What the item does on a tick, a release or at the end
// is the game's; a "block" item gives its actor a raised blade
// (isBlocking), which Hurt reads.
type ItemUse struct {
	Using    *inventory.Stack
	UseCount int
}

// Begin is setItemInUse for n ticks. Using the same stack again keeps the
// count; it reports whether the use started.
func (u *ItemUse) Begin(st *inventory.Stack, n int) bool {
	if u.Using == st {
		return false
	}
	u.Using, u.UseCount = st, n
	return true
}

// Raise begins an endless use that blocks (ItemSword.onItemRightClick).
func (u *ItemUse) Raise(a *Actor, st *inventory.Stack) {
	if u.Begin(st, Endless) {
		a.Blocking = true
	}
}

// Clear is clearItemInUse: nothing in use, the blade down.
func (u *ItemUse) Clear(a *Actor) { u.Using, u.UseCount, a.Blocking = nil, 0, false }

// Held is how long an endless use has been held (72000 - itemInUseCount).
func (u *ItemUse) Held() int { return Endless - u.UseCount }

// Keep is onUpdate's check: the use stops (Clear) once the held stack is
// another or the player is dead. It reports whether it goes on.
func (u *ItemUse) Keep(a *Actor, held *inventory.Stack, dead bool) bool {
	if u.Using == nil {
		return false
	}
	if held != u.Using || dead {
		u.Clear(a)
		return false
	}
	return true
}

// Crumbs is updateItemUse's beat: every 4th tick of the last 25 the eat
// or drink sound and 5 crumbs.
func (u *ItemUse) Crumbs() bool { return u.UseCount <= 25 && u.UseCount%4 == 0 }

// Tick counts one tick down and reports the use run out
// (onItemUseFinish is due).
func (u *ItemUse) Tick() bool {
	u.UseCount--
	return u.UseCount <= 0
}

// Hold is one tick of a held use with no finish (a blade, a bow): the
// blade is up while blocks, and the use clears when its count runs out.
func (u *ItemUse) Hold(a *Actor, blocks bool) {
	a.Blocking = blocks
	if u.Tick() {
		u.Clear(a)
	}
}

// Blow is a player's melee blow (attackTargetEntityWithCurrentItem) as the
// game prices it.
type Blow struct {
	Damage    float64 // the attackDamage attribute (weapon.AttackDamage)
	Bonus     float64 // the enchantments against the target (getEnchantmentModifierLiving)
	Knockback int     // the knockback level (sprinting adds one in Attack)
	Fire      int     // the fire aspect level
	Crit      bool    // a falling blow (Critical): Damage ×1.5
	// Allow is the game's veto, asked once the blow is worth striking
	// (nil allows).
	Allow func() bool
	// Landed is the game's work after a blow that hurt: hitEntity, the
	// enchantment crit, item wear. It runs before the fire takes hold.
	Landed func()
}

// Critical is attackTargetEntityWithCurrentItem's falling blow: p has
// fallen (fall, its fallDistance, above 0) and is not on the ground, on a
// ladder, in water or riding, and not blind. Sprinting does not matter.
func Critical(p *Actor, fall float32, blind bool) bool {
	c := p.Body
	return fall > 0 && !c.OnGround && !c.OnLadder && !c.InWater && !blind && p.Riding == nil
}

// Strike lands b on t from p: nothing when neither damage nor bonus is
// above 0 or Allow says no; a Crit blow's damage (before the bonus) is
// ×1.5; fire sets t alight for the blow itself (so a
// kill burns) and puts it out again when the blow fails, and after Landed
// sets fire*4 seconds. It reports whether the blow hurt.
func (m *Manager) Strike(p, t *Actor, b Blow) bool {
	if b.Damage <= 0 && b.Bonus <= 0 {
		return false
	}
	if b.Allow != nil && !b.Allow() {
		return false
	}
	lit := b.Fire > 0 && !t.OnFire()
	if lit {
		t.SetFire(1)
	}
	d := float32(b.Damage)
	if b.Crit && d > 0 {
		d *= 1.5
	}
	if !m.Attack(p, t, float64(d+float32(b.Bonus)), b.Knockback) {
		if lit {
			t.Extinguish()
		}
		return false
	}
	if b.Landed != nil {
		b.Landed()
	}
	if b.Fire > 0 {
		t.SetFire(b.Fire * 4)
	}
	return true
}
