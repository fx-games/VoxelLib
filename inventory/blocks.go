package inventory

import "math"

// BlocksAttacks is an item's blocking (a shield): declared, using the
// item raises it, and once it has been raised for DelayTicks it takes
// the blows from in front (actors.Manager.Hurt). The numbers are the
// item component's; Shield is the reference shield's.
type BlocksAttacks struct {
	// DelaySeconds is block_delay_seconds: how long the item must be
	// raised before it blocks.
	DelaySeconds float32
	// DisableScale is disable_cooldown_scale: a disabling weapon's
	// seconds are multiplied by it.
	DisableScale float32
	// Reductions are damage_reductions; their sum, at most the damage,
	// is what the item takes.
	Reductions []DamageReduction
	// ItemDamage is the wear a blocked blow gives the item.
	ItemDamage ItemDamageFunction
	// BlockSound plays when a blow is blocked, DisableSound when a weapon
	// disables the item, BreakSound (the item's break_sound) when it
	// breaks (sound events; "" none).
	BlockSound, DisableSound, BreakSound string
}

// DamageReduction is one of BlocksAttacks' reductions: within Angle
// degrees of the blocker's facing (horizontal_blocking_angle) it takes
// base + factor × damage, kept within 0..damage. The component's damage
// type filter is not part of it: a reduction applies to every blockable
// hurt.
type DamageReduction struct {
	Angle        float32
	Base, Factor float32
}

// ItemDamageFunction is the item's wear for a blocked amount: nothing
// below Threshold, else floor(Base + Factor × amount).
type ItemDamageFunction struct{ Threshold, Base, Factor float32 }

// Shield is the reference shield's BlocksAttacks: raised for 0.25 s
// (5 ticks), disabled for the weapon's full seconds, every blow from
// within 90° of its facing taken whole, worn floor(1 + amount) by blows
// of 3 or more, with the shield block and break sounds.
func Shield() *BlocksAttacks {
	return &BlocksAttacks{DelaySeconds: 0.25, DisableScale: 1,
		Reductions: []DamageReduction{{Angle: 90, Base: 0, Factor: 1}},
		ItemDamage: ItemDamageFunction{Threshold: 3, Base: 1, Factor: 1},
		BlockSound: "item.shield.block", DisableSound: "item.shield.break", BreakSound: "item.shield.break"}
}

// DelayTicks is blockDelayTicks: round(DelaySeconds × 20).
func (b *BlocksAttacks) DelayTicks() int { return round(b.DelaySeconds * 20) }

// DisableTicks is disableBlockingForTicks for a weapon's seconds: round
// of seconds × DisableScale × 20, 0 when that is not above 0.
func (b *BlocksAttacks) DisableTicks(seconds float32) int {
	if s := seconds * b.DisableScale; s > 0 {
		return round(s * 20)
	}
	return 0
}

// Resolve is resolveBlockedDamage: what the item takes of dealt from a
// source angle radians off its facing.
func (b *BlocksAttacks) Resolve(dealt float32, angle float64) float32 {
	var sum float32
	for _, r := range b.Reductions {
		sum += r.resolve(dealt, angle)
	}
	return clamp(sum, 0, dealt)
}

func (r DamageReduction) resolve(dealt float32, angle float64) float32 {
	if angle > float64(float32(math.Pi/180)*r.Angle) {
		return 0
	}
	return clamp(r.Base+r.Factor*dealt, 0, dealt)
}

// Apply is the wear for a blocked amount.
func (f ItemDamageFunction) Apply(dealt float32) int {
	if dealt < f.Threshold {
		return 0
	}
	return int(math.Floor(float64(f.Base + f.Factor*dealt)))
}

// Weapon is the weapon component: DisableSeconds
// (disable_blocking_for_seconds) is how long a blocked blow of it
// disables the blocker's item. AxeDisableSeconds is the axes'.
type Weapon struct {
	DisableSeconds float32
}

// AxeDisableSeconds is AXE_DISABLES_BLOCKING_FOR_SECONDS.
const AxeDisableSeconds = 5

// Hurt is ItemStack.hurtAndBreak's wear: n points (less what WearNegated
// spares); the stack breaks once its damage reaches MaxDamage (one item
// gone, the damage reset). It reports the break. Unlike AddDamage (1.7.10's
// damageItem, which breaks past MaxDamage) the last point breaks it.
func (s *Stack) Hurt(n int) bool {
	if !s.Damageable() || n <= 0 {
		return false
	}
	if WearNegated != nil {
		if n -= WearNegated(s, n); n <= 0 {
			return false
		}
	}
	if s.Damage += n; s.Damage < s.Item.MaxDamage {
		return false
	}
	s.Count = max(s.Count-1, 0)
	s.Damage = 0
	return true
}

// Cooldowns is ItemCooldowns: per item, the ticks it cannot be used for.
// The game ticks it with the player.
type Cooldowns struct {
	ticks int
	cool  map[*Item][2]int // start, end
}

// Tick is one tick; cooldowns that ran out end.
func (c *Cooldowns) Tick() {
	c.ticks++
	for it, t := range c.cool {
		if t[1] <= c.ticks {
			delete(c.cool, it)
		}
	}
}

// Add puts it on cooldown for n ticks (addCooldown).
func (c *Cooldowns) Add(it *Item, n int) {
	if c.cool == nil {
		c.cool = map[*Item][2]int{}
	}
	c.cool[it] = [2]int{c.ticks, c.ticks + n}
}

// Percent is getCooldownPercent at partial tick a: the part of the
// cooldown left, 0 when none (the hotbar's white overlay).
func (c *Cooldowns) Percent(it *Item, a float32) float32 {
	t, ok := c.cool[it]
	if !ok {
		return 0
	}
	return clamp((float32(t[1])-(float32(c.ticks)+a))/float32(t[1]-t[0]), 0, 1)
}

// On reports isOnCooldown.
func (c *Cooldowns) On(st *Stack) bool { return st != nil && c.Percent(st.Item, 0) > 0 }

func clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }

// round is Java's Math.round of a float.
func round(f float32) int { return int(math.Floor(float64(f) + 0.5)) }
