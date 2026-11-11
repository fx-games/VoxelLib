package mp

import (
	"math"
	"sync/atomic"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
)

// GunRange is how far a bullet reaches, in blocks.
const GunRange = 256

// shoot runs a round of p's held gun: each pellet is a hitscan ray from
// p's eye against the players as p saw them; damage falls off with
// distance and head hits multiply it. Everyone else hears it.
//
// ponytail: the client's ammo, fire rate and spread are trusted (a LAN
// game); validate them here against the gun's spec if cheating matters.
func (s *Server) shoot(p *sPlayer, sh Shot) {
	gun := p.gun
	if gun == nil || sh.Gun != p.held.Gun || p.dead > 0 || len(sh.Pellets) > max(gun.Bullet.Pellets, 1) {
		return
	}
	targets := s.targets(p, atomic.LoadUint32(&s.tick))
	boxes := func(x, y, z int, id voxel.StateID, buf []collide.AABB) []collide.AABB {
		return s.world.W.StateBoxes(id, x, y, z, buf)
	}
	ex, ey, ez := p.ch.Eye()
	for _, l := range sh.Pellets {
		d := l.Dir()
		h, ok := projectile.Hitscan(s.world, boxes, [3]float64{ex, ey, ez}, d, GunRange, targets)
		if !ok || h.Block {
			continue
		}
		r, ok := h.Target.(*rewound)
		if !ok || r.p.dead > 0 {
			continue // killed by an earlier pellet
		}
		dmg := gun.DamageAt(float32(h.Dist))
		head := IsHead(h.Part)
		if head && gun.Bullet.Headshot > 0 {
			dmg *= gun.Bullet.Headshot
		}
		s.damage(Hit{Attacker: p.slot, Victim: r.p.slot, Damage: max(1, int(math.Round(float64(dmg)))), Head: head, Gun: true}, d[0], d[2], float64(gun.Bullet.Knockback))
	}
	for _, o := range s.players {
		if o != nil && o != p {
			s.net.SendReliable(o.slot, []byte{msgFired, uint8(p.slot)})
		}
	}
}

// melee lands p's held gun's melee hit along l (ModernKineticGunItem.doMelee)
// on the players as p saw them.
//
// ponytail: the client's melee cooldown is trusted, like its fire rate.
func (s *Server) melee(p *sPlayer, gun string, l Look) {
	g := p.gun
	if g == nil || g.Melee == nil || gun != p.held.Gun || p.dead > 0 {
		return
	}
	m := g.Melee
	boxes := func(x, y, z int, id voxel.StateID, buf []collide.AABB) []collide.AABB {
		return s.world.W.StateBoxes(id, x, y, z, buf)
	}
	ex, ey, ez := p.ch.Eye()
	eye, d := [3]float64{ex, ey, ez}, l.Dir()
	dist := float64(m.Distance)
	reach := p.ch.Box().Grow(dist, dist, dist)
	for _, t := range s.targets(p, atomic.LoadUint32(&s.tick)) {
		r := t.(*rewound)
		te := [3]float64{r.x, r.y + character.PlayerEyeHeight, r.z}
		if !reach.Intersects(r.Box()) || !m.Reaches(eye, d, te) || !projectile.InSight(s.world, boxes, eye, te) {
			continue
		}
		s.damage(Hit{Attacker: p.slot, Victim: r.p.slot, Damage: int(math.Round(float64(m.Damage))), Melee: true},
			d[0], d[2], float64(m.Knockback))
	}
}
