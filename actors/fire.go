package actors

import "github.com/memmaker/voxellib/voxel/collide"

// FireWorld is the optional part of a World that sets actors on fire and
// puts them out (Entity.moveEntity's fire checks). Without it actors only
// burn from SetFire and StruckByLightning.
type FireWorld interface {
	// Burning is World.isBoundingBoxBurning: fire or lava in the box.
	Burning(box collide.AABB) bool
	// Wet is Entity.isWet: in water, or rain at the feet or the head.
	Wet(a *Actor) bool
}

// fireResist is Entity.fireResistance: ticks in fire before catching it.
func (a *Actor) fireResist() int {
	if a.FireResist > 0 {
		return a.FireResist
	}
	return 1
}

// OnFire is Entity.isBurning.
func (a *Actor) OnFire() bool { return !a.FireImmune && a.fire > 0 }

// Extinguish is Entity.extinguish: stop burning.
func (a *Actor) Extinguish() { a.fire = 0 }

// SetFire is Entity.setFire: burn for at least sec seconds.
func (a *Actor) SetFire(sec int) {
	if j := sec * 20; a.fire < j {
		a.fire = j
	}
}

// fireDamage is Entity.dealFireDamage (DamageSource.inFire: armour counts;
// fire resistance ignores fire damage).
func (m *Manager) fireDamage(a *Actor, n float64) {
	if !a.FireImmune && !a.FireProof {
		m.hurtAs(HurtFire, a, nil, n, false)
	}
}

// StruckByLightning is Entity.onStruckByLightning: 5 fire damage, and an
// actor not burning yet catches fire for 8 seconds.
func (m *Manager) StruckByLightning(a *Actor) {
	m.fireDamage(a, 5)
	if a.fire++; a.fire == 0 {
		a.SetFire(8)
	}
}

// fireTick is Entity.onEntityUpdate's burning (1 unblockable damage every
// 20 ticks, fire-immune actors cool 4 a tick) and moveEntity's fire and
// water checks.
func (m *Manager) fireTick(a *Actor) {
	if a.fire > 0 {
		if a.FireImmune {
			a.fire = max(a.fire-4, 0)
		} else {
			if a.fire%20 == 0 && !a.FireProof {
				m.hurtAs(HurtFire, a, nil, 1, true) // DamageSource.onFire
			}
			a.fire--
		}
	}
	fw, ok := m.W.(FireWorld)
	if !ok || !a.Alive() {
		return
	}
	wet := fw.Wet(a)
	b := a.Box()
	b.MinX, b.MinY, b.MinZ, b.MaxX, b.MaxY, b.MaxZ = b.MinX+0.001, b.MinY+0.001, b.MinZ+0.001, b.MaxX-0.001, b.MaxY-0.001, b.MaxZ-0.001
	if fw.Burning(b) {
		m.fireDamage(a, 1)
		if !wet {
			if a.fire++; a.fire == 0 {
				a.SetFire(8)
			}
		}
	} else if a.fire <= 0 {
		a.fire = -a.fireResist()
	}
	if wet && a.fire > 0 {
		if m.OnExtinguish != nil {
			m.OnExtinguish(a) // random.fizz
		}
		a.fire = -a.fireResist()
	}
}
