package actors

import (
	"math"

	"github.com/memmaker/voxellib/projectile"
)

// 1.7.10 EntityLivingBase timings.
const (
	MaxHurtResist = 20 // ticks of invulnerability after a hit (half of it blocks equal or weaker blows)
	MaxHurtTime   = 10 // ticks of the red hurt flash
	DeathTicks    = 20 // ticks of the death animation before removal
	SwingTicks    = 6  // ticks of an arm swing
)

// fight runs target selection and the attack for one tick. It reports
// whether the actor is busy fighting (the mode is skipped then).
func (m *Manager) fight(a *Actor) bool {
	if (a.Melee.Damage <= 0 && a.Ranged == nil) || a.Brain.Mode == Flee {
		return false
	}
	if t := a.Target; t != nil {
		r := a.AggroRange * 1.5
		if !t.Alive() || t.Pos().dist2(a.Pos()) > r*r ||
			(m.Factions.Sees(a, t) != Hostile && t != a.revenge) {
			a.Target = nil
		}
	}
	b := &a.Brain
	if b.Mode == Guard && b.Range > 0 && a.Pos().hdist(b.Home) > b.Range {
		a.Target = nil // leashed: back home first
		return false
	}
	if b.Mode == Follow {
		if l := m.byID[b.Leader]; l != nil && l.Pos().dist2(a.Pos()) > 32*32 {
			a.Target = nil // never wander off from the leader
			return false
		}
	}
	if a.Target == nil && (m.ticks+hash(a.ID))%10 == 0 {
		a.Target = m.pickTarget(a)
	}
	t := a.Target
	if t == nil {
		m.rangedIdle(a)
		return false
	}
	a.age = 0
	if r := a.Ranged; r != nil && r.AI == RangedLOTR {
		m.lotrFight(a, t)
		return true
	}
	if r := a.Ranged; r != nil && r.AI == RangedCNPC {
		m.cnpcFight(a, t)
		return true
	}
	if r := a.Ranged; r != nil && (r.MeleeInRange == 0 || t.Pos().dist2(a.Pos()) > r.MeleeInRange*r.MeleeInRange || a.Melee.Damage <= 0) {
		m.shoot(a, t)
	} else {
		m.melee(a, t)
	}
	return true
}

func hash(s string) int {
	h := 0
	for i := 0; i < len(s); i++ {
		h = h*31 + int(s[i])
	}
	if h < 0 {
		h = -h
	}
	return h
}

// pickTarget: the one who hurt us, else (for a defending follower) whoever
// threatens the leader or the company, else the nearest hostile in range.
func (m *Manager) pickTarget(a *Actor) *Actor {
	if r := a.revenge; r != nil && r.Alive() && a.revengeTicks > 0 && m.Factions.Sees(a, r) != Friendly {
		return r
	}
	b := &a.Brain
	if b.Mode == Follow && b.Defend {
		if l := m.byID[b.Leader]; l != nil {
			var best *Actor
			bd := math.Inf(1)
			for _, o := range m.Actors {
				if o == a || !o.Alive() || o.Pos().dist2(l.Pos()) > 16*16 {
					continue
				}
				threat := m.Factions.Sees(l, o) == Hostile && (o.Melee.Damage > 0 || o.Ranged != nil)
				if t := o.Target; t != nil && (t == l || (t.Brain.Mode == Follow && t.Brain.Leader == l.ID)) {
					threat = true
				}
				if threat {
					if d := o.Pos().dist2(a.Pos()); d < bd {
						best, bd = o, d
					}
				}
			}
			if best != nil {
				return best
			}
		}
	}
	return m.nearestHostile(a, a.AggroRange)
}

func (m *Manager) nearestHostile(a *Actor, r float64) *Actor {
	var best *Actor
	bd := r * r
	for _, o := range m.Actors {
		if o == a || !o.Alive() || o == a.Riding || o == a.Rider {
			continue
		}
		if d := o.Pos().dist2(a.Pos()); d < bd && m.Factions.Sees(a, o) == Hostile {
			best, bd = o, d
		}
	}
	return best
}

// MeleeReach2 is the squared reach of a's melee against t, from feet to
// feet (EntityAIAttackOnCollide: (2*width)^2 + target width).
func MeleeReach2(a, t *Actor) float64 {
	if a.Melee.Reach > 0 {
		return a.Melee.Reach * a.Melee.Reach
	}
	w := a.carrier().Body.Width * 2
	return w*w + t.Body.Width
}

func (m *Manager) face(a, t *Actor) {
	want := yawTo(a.Pos(), t.Pos())
	a.HeadYaw = turn(a.HeadYaw, want, turnRate)
	a.Yaw = turn(a.Yaw, want, turnRate)
}

func (m *Manager) melee(a, t *Actor) {
	mul := a.Melee.Speed
	if mul <= 0 {
		mul = 1
	}
	d2 := a.Pos().dist2(t.Pos())
	r2 := MeleeReach2(a, t)
	if d2 > r2*0.6 {
		m.moveTo(a, t.Pos(), mul)
	} else {
		m.stop(a)
	}
	m.face(a, t)
	if d2 <= r2 && a.attackCool <= 0 {
		a.attackCool = max(1, a.Melee.Cooldown)
		m.Attack(a, t, a.Melee.Damage, a.Melee.Knockback)
	}
}

// Attack is a melee blow of src on t (EntityMob.attackEntityAsMob, or the
// player's attackTargetEntityWithCurrentItem): t is hurt by damage, and a
// knockback level (enchantment; +1 for a sprinting player) pushes it on
// along src's facing while src slows down. It reports whether the blow
// counted.
func (m *Manager) Attack(src, t *Actor, damage float64, knockback int) bool {
	src.Swing = 0
	if src.Sprinting {
		knockback++
	}
	if !m.Hurt(t, src, damage, false) {
		return false
	}
	if knockback > 0 {
		k := float64(knockback) * 0.5
		tb := t.carrier().Body
		tb.VX += math.Sin(src.Yaw) * k // along the attacker's facing
		tb.VY += 0.1
		tb.VZ += -math.Cos(src.Yaw) * k
		src.Body.VX *= 0.6
		src.Body.VZ *= 0.6
	}
	return true
}

// Hurt applies damage as 1.7.10's EntityLivingBase.attackEntityFrom: during
// the second half of the invulnerability window only a stronger blow counts,
// and only by the difference; a fresh hit starts the window, the hurt flash
// and knockback away from src. Armour reduces it by armor/25 unless
// unblockable (falls, drowning, magic). It reports whether damage was done.
func (m *Manager) Hurt(t, src *Actor, amount float64, unblockable bool) bool {
	if !t.Alive() || amount <= 0 {
		return false
	}
	fresh := true
	if float64(t.hurtResist) > MaxHurtResist/2 {
		if amount <= t.lastDamage {
			return false
		}
		amount, t.lastDamage = amount-t.lastDamage, amount
		fresh = false
	} else {
		t.lastDamage = amount
		t.hurtResist = MaxHurtResist
		t.HurtTime = MaxHurtTime
	}
	dealt := amount
	if !unblockable {
		dealt = amount * (25 - math.Min(20, t.Armor)) / 25
	}
	t.Health -= dealt
	t.age = 0
	if src != nil && src != t {
		t.revenge, t.revengeTicks = src, 100
		if t.FleeWhenHurt {
			t.panic = 60
		}
		if fresh {
			m.knockBack(t, src.Pos().X-t.Pos().X, src.Pos().Z-t.Pos().Z)
		}
	}
	if t.OnHurt != nil {
		t.OnHurt(t, src, dealt)
	}
	if t.Health <= 0 {
		t.Health = 0
		t.set(Dead)
		if m.OnDeath != nil {
			m.OnDeath(t, src)
		}
	}
	return true
}

// knockBack is EntityLivingBase.knockBack: dx,dz point from the victim to
// the attacker.
func (m *Manager) knockBack(t *Actor, dx, dz float64) {
	for dx*dx+dz*dz < 1e-4 {
		dx, dz = (m.Rand.Float64()-m.Rand.Float64())*0.01, (m.Rand.Float64()-m.Rand.Float64())*0.01
	}
	if m.Rand.Float64() < t.KnockbackResist {
		return
	}
	b := t.carrier().Body
	f := math.Hypot(dx, dz)
	const k = 0.4
	b.VX, b.VY, b.VZ = b.VX/2-dx/f*k, b.VY/2+k, b.VZ/2-dz/f*k
	if b.VY > 0.4 {
		b.VY = 0.4
	}
}

// shoot is EntityAIArrowAttack.
func (m *Manager) shoot(a, t *Actor) {
	r := a.Ranged
	d2 := a.Pos().dist2(t.Pos())
	max2 := r.MaxRange * r.MaxRange
	a.seeTime++ // ponytail: no line-of-sight test; add a World raycast hook when archers shoot through walls
	if d2 <= max2 && a.seeTime >= 20 {
		m.stop(a)
	} else {
		m.moveTo(a, t.Pos(), 1)
	}
	m.face(a, t)
	f := math.Sqrt(d2) / r.MaxRange
	a.rangedCool--
	switch {
	case a.rangedCool == 0:
		if d2 > max2 {
			return
		}
		m.fire(a, t, math.Max(0.1, math.Min(1, f)))
		a.rangedCool = int(math.Floor(f*float64(r.MaxInterval-r.MinInterval) + float64(r.MinInterval)))
	case a.rangedCool < 0:
		a.rangedCool = int(math.Floor(f*float64(r.MaxInterval-r.MinInterval) + float64(r.MinInterval)))
	}
}

type shot struct {
	from   *Actor
	damage float64
	speed  float64
}

// fire launches an arrow from a's eyes at t (EntitySkeleton
// attackEntityWithRangedAttack and the aiming EntityArrow constructor).
func (m *Manager) fire(a, t *Actor, power float64) {
	if m.Launch != nil {
		a.Swing = 0
		m.Launch(a, t, power)
		return
	}
	if m.Arrows == nil {
		return
	}
	r := a.Ranged
	a.Swing = 0
	x, y, z := a.Body.X, a.Body.Y+a.Body.EyeHeight-0.1, a.Body.Z
	dx, dy, dz := t.Body.X-x, t.Body.Y+t.Body.Height/3-y, t.Body.Z-z
	dy += math.Hypot(dx, dz) * 0.2
	l := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if l < 1e-6 {
		return
	}
	g := 0.0075 * r.Inaccuracy
	hx, hy, hz := dx/l+m.Rand.NormFloat64()*g, dy/l+m.Rand.NormFloat64()*g, dz/l+m.Rand.NormFloat64()*g
	yaw, pitch := math.Atan2(hx, -hz), math.Atan2(hy, math.Hypot(hx, hz))
	arrow := m.Arrows.Shoot(x, y, z, yaw, pitch, r.Speed)
	dmg := power*r.Damage + m.Rand.NormFloat64()*0.25 + float64(m.Difficulty)*0.11
	m.shots[arrow] = shot{a, dmg, r.Speed}
	if m.OnFire != nil {
		m.OnFire(a, t)
	}
}

// arrowHit is installed as the projectile world's Hit callback: an arrow
// that strikes an actor hurts it by ceil(speed * damage) and is gone, as in
// 1.7.10. Other targets go to the callback that was there before.
func (m *Manager) arrowHit(ar *projectile.Arrow, tg projectile.Target) {
	s, ours := m.shots[ar]
	delete(m.shots, ar)
	t, isActor := tg.(*Actor)
	if !isActor {
		if m.prevHit != nil {
			m.prevHit(ar, tg)
		}
		return
	}
	if !ours {
		s = shot{damage: 2, speed: projectile.Speed} // the game's own arrows (the player's bow)
		if m.ArrowDamage != nil {
			s.from, s.damage, s.speed = m.ArrowDamage(ar)
		}
	}
	v := s.speed * math.Pow(projectile.Drag, float64(ar.Flown))
	m.Hurt(t, s.from, math.Ceil(v*s.damage), false)
	ar.Discard()
}
