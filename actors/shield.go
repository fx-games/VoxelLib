package actors

import (
	"math"

	"github.com/memmaker/voxellib/inventory"
)

// Guard begins the endless use of a blocker (an item with Blocks, a
// shield) held in hand: a's Use is u, and once it has been raised for the
// item's DelayTicks BlockingWith returns it (LivingEntity.startUsingItem
// with getItemBlockingWith). A blocker on cooldown (u.Inv.Cool) is not
// raised. It reports whether the use started.
func (u *ItemUse) Guard(a *Actor, hand int, st *inventory.Stack) bool {
	if st == nil || st.Item.Blocks == nil || u.Inv != nil && u.Inv.Cool.On(st) {
		return false
	}
	if !u.BeginIn(hand, st, Endless) {
		return false
	}
	a.Use = u
	return true
}

// BlockingWith is getItemBlockingWith: the blocker in use once it has
// been raised for its block delay, else nil.
func (a *Actor) BlockingWith() *inventory.Stack {
	u := a.Use
	if u == nil || u.Using == nil || u.Using.Item.Blocks == nil {
		return nil
	}
	if u.Held() >= u.Using.Item.Blocks.DelayTicks() {
		return u.Using
	}
	return nil
}

// IsBlocking is isBlocking: a raised blocker (BlockingWith) or a raised
// blade (Blocking).
func (a *Actor) IsBlocking() bool { return a.Blocking || a.BlockingWith() != nil }

// SourceAngle is applyItemBlocking's angle: between t's facing (its head
// yaw, the view vector at pitch 0) and the horizontal direction from t to
// the source at x, z, in radians.
func SourceAngle(t *Actor, x, z float64) float64 {
	p := t.Pos()
	dx, dz := x-p.X, z-p.Z
	l := math.Hypot(dx, dz)
	if l < 1e-4 {
		dx, dz = 0, 0 // Vec3.normalize of a zero vector
	} else {
		dx, dz = dx/l, dz/l
	}
	yaw := t.HeadYaw
	return math.Acos(dx*math.Sin(yaw) + dz*-math.Cos(yaw))
}

// applyBlocking is applyItemBlocking: what t's raised blocker takes of
// amount from src (none without a source: the angle is pi). The blocker
// wears (hurtBlockingItem, players only) and a blow from a living
// attacker (not a projectile) runs blockUsingItem: the attacker's
// blockedByItem knockback while the blow was not fully taken and, for a
// player, the attacker's disabling weapon.
func (m *Manager) applyBlocking(t, src *Actor, amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	st := t.BlockingWith()
	if st == nil {
		return 0
	}
	b, u := st.Item.Blocks, t.Use
	angle := math.Pi
	if src != nil && src != t {
		p := src.Pos()
		angle = SourceAngle(t, p.X, p.Z)
	}
	blocked := float64(b.Resolve(float32(amount), angle))
	if t.Player { // hurtBlockingItem
		if n := b.ItemDamage.Apply(float32(blocked)); n > 0 && (u.Inv == nil || !u.Inv.Creative) && st.Hurt(n) {
			u.broken(m, t, st)
		}
	}
	if blocked > 0 && !m.hurtKind.Is(HurtProjectile) && src != nil && src != t {
		if blocked < amount { // blockedByItem
			p, q := t.Pos(), src.Pos()
			m.Knockback(t, 0.5, p.X-q.X, p.Z-q.Z)
		}
		if t.Player && m.Weapon != nil {
			if w := m.Weapon(src); w != nil && w.Item.Weapon != nil {
				u.disable(m, t, st, w.Item.Weapon.DisableSeconds)
			}
		}
	}
	return blocked
}

// disable is BlocksAttacks.disable: the blocker on cooldown for the
// weapon's seconds, the use stopped and the disable sound.
func (u *ItemUse) disable(m *Manager, a *Actor, st *inventory.Stack, seconds float32) {
	b := st.Item.Blocks
	n := b.DisableTicks(seconds)
	if n <= 0 {
		return
	}
	if u.Inv != nil {
		u.Inv.Cool.Add(st.Item, n)
	}
	u.Clear(a)
	if u.OnDisabled != nil {
		u.OnDisabled(a, st, n)
		return
	}
	u.sound(b.DisableSound, 0.8, 0.8+m.Rand.Float32()*0.4)
}

// blocked is BlocksAttacks.onBlocked: the block sound.
func (u *ItemUse) blocked(m *Manager, a *Actor, st *inventory.Stack, amount float64) {
	if u.OnBlocked != nil {
		u.OnBlocked(a, st, amount)
		return
	}
	u.sound(st.Item.Blocks.BlockSound, 1, 0.8+m.Rand.Float32()*0.4)
}

// broken is hurtAndBreak's break: the hand emptied, the use stopped and
// the break sound (breakItem; the game's OnBroken adds the crumbs).
func (u *ItemUse) broken(m *Manager, a *Actor, st *inventory.Stack) {
	if u.Inv != nil && st.Count <= 0 {
		u.Inv.SetInHand(u.Hand, nil)
	}
	u.Clear(a)
	if u.OnBroken != nil {
		u.OnBroken(a, st)
		return
	}
	u.sound(st.Item.Blocks.BreakSound, 0.8, 0.8+m.Rand.Float32()*0.4)
}

func (u *ItemUse) sound(ev string, vol, pitch float32) {
	if ev != "" && u.Sound != nil {
		u.Sound(ev, vol, pitch)
	}
}

// Knockback is the modern LivingEntity.knockback: power less the
// knockback resistance pushes t against xd, zd (pointing from t to the
// source), halving its motion; on the ground it is lifted by power, at
// most to 0.4.
func (m *Manager) Knockback(t *Actor, power, xd, zd float64) {
	if power *= 1 - t.KnockbackResist; power <= 0 {
		return
	}
	for xd*xd+zd*zd < float64(float32(1e-5)) {
		xd, zd = (m.Rand.Float64()-m.Rand.Float64())*0.01, (m.Rand.Float64()-m.Rand.Float64())*0.01
	}
	b := t.carrier().Body
	b.VX, b.VY, b.VZ = KnockbackMotion(b.VX, b.VY, b.VZ, b.OnGround, power, xd, zd)
}

// HurtKnockback is the knockback power LivingEntity.hurt gives a hit from
// an attacker (TACZ's KnockBackModifier replaces it for a bullet).
const HurtKnockback = 0.4

// knockbackMaxLift caps the upward motion a knockback gives on the ground.
const knockbackMaxLift = 0.4

// KnockbackMotion is the motion after LivingEntity.knockback of power
// (resistance already taken off) against xd, zd (pointing from the target
// to the source, not both 0): the old motion halves and is pushed power
// away; on the ground it is lifted by power, at most to 0.4. A power of 0
// or less leaves the motion as it is. Bodies outside the actors (creatures,
// multiplayer players) call this directly.
func KnockbackMotion(vx, vy, vz float64, onGround bool, power, xd, zd float64) (float64, float64, float64) {
	l := math.Hypot(xd, zd)
	if power <= 0 || l == 0 {
		return vx, vy, vz
	}
	if onGround {
		vy = min(knockbackMaxLift, vy/2+power)
	}
	return vx/2 - xd/l*power, vy, vz/2 - zd/l*power
}
