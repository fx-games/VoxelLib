package mp

import (
	"math"
	"sync/atomic"

	"github.com/memmaker/voxellib/actors"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/weapon"
)

// sBullet is a flying bullet and who fired it.
type sBullet struct {
	id      uint32
	shooter int // slot
	gun     string
	b       *projectile.Bullet
}

// shoot fires a round of p's held gun (ModernKineticGunScriptAPI.shootOnce):
// bullet_amount bullets from p's eye along the shot's look, the spread
// rolled here from p's state as the server sees it (InaccuracyType), each
// spawned to every client (they fly and draw it) and flown by tickBullets.
// Everyone else hears it.
//
// ponytail: the client's ammo, fire rate and heat are trusted (a LAN
// game); validate them here against the gun's spec if cheating matters.
func (s *Server) shoot(p *sPlayer, sh Shot) {
	gun := p.gun
	if gun == nil || sh.Gun != p.held.Gun || p.dead > 0 {
		return
	}
	tick := atomic.LoadUint32(&s.tick)
	prev := s.lastPos(p, tick)
	at := projectile.BulletSpawn(prev, [3]float64{p.ch.X, p.ch.Y, p.ch.Z}, p.ch.EyeHeight)
	moving := math.Sqrt(sq(p.ch.X-prev[0])+sq(p.ch.Y-prev[1])+sq(p.ch.Z-prev[2])) > weapon.MoveThreshold
	inacc := float64(gun.Spread(p.aim == weapon.Aimed, moving, p.anim.Sneak) * heatFactor(gun, sh.Heat))
	// applyRecoilPush on the server, before the bullets spawn; the
	// snapshot carries the new motion to the shooter
	push := gun.RecoilPush.Push(sh.Look.Dir(), p.ch.OnGround, p.anim.Sneak, gun.Bullet.Pellets)
	p.ch.VX, p.ch.VY, p.ch.VZ = p.ch.VX+push[0], p.ch.VY+push[1], p.ch.VZ+push[2]
	for _, b := range projectile.Fire(at, sh.Look.Dir(), gun, inacc, &p.tracers, s.rng) {
		s.nextID++
		sb := &sBullet{id: s.nextID, shooter: p.slot, gun: gun.ID, b: b}
		s.bullets = append(s.bullets, sb)
		s.broadcast(BulletSpawn{ID: sb.id, Shooter: p.slot, Gun: gun.ID, Pos: b.Pos, Vel: b.Vel,
			Gravity: float32(b.Gravity), Friction: float32(b.Friction), Life: b.Life, Tracer: b.Tracer}.encode())
	}
	for _, o := range s.players {
		if o != nil && o != p {
			s.net.SendReliable(o.slot, []byte{msgFired, uint8(p.slot)})
		}
	}
}

// heatFactor is the client's heat inaccuracy factor, kept within the gun's range.
func heatFactor(g *weapon.Gun, f float32) float32 {
	h := g.Heat
	if h == nil || h.Max <= 0 {
		return 1
	}
	lo, hi := min(h.Inaccuracy[0], h.Inaccuracy[1]), max(h.Inaccuracy[0], h.Inaccuracy[1])
	return min(max(f, lo), hi)
}

// lastPos is p's feet at the last tick (xOld), or now if there is none.
func (s *Server) lastPos(p *sPlayer, tick uint32) [3]float64 {
	if q := p.hist[tick%historyTicks]; q.tick == tick && tick != 0 {
		return [3]float64{q.x, q.y, q.z}
	}
	return [3]float64{p.ch.X, p.ch.Y, p.ch.Z}
}

// hitTarget is who a bullet target is: an enemy player or a zombie.
type hitTarget struct {
	player *sPlayer
	zombie *sZombie
}

// bulletTargets are p's enemies as bullet boxes (HitboxHelper's lag
// compensation: each box as p saw it, with the velocity it had then) and
// the living zombies (as they are now), and who each is.
func (s *Server) bulletTargets(p *sPlayer, tick uint32) ([]projectile.BulletTarget, []hitTarget) {
	var ts []projectile.BulletTarget
	var who []hitTarget
	for _, o := range s.players {
		if o == nil || !s.enemies(p, o) || o.dead > 0 || o.protect > 0 {
			continue
		}
		r, ok := s.rewind(o, tick, p.lag)
		if !ok {
			continue
		}
		vel := [3]float64{}
		if q, ok := s.rewind(o, tick, p.lag+1); ok {
			vel = [3]float64{r.x - q.x, r.y - q.y, r.z - q.z}
		}
		ts = append(ts, projectile.BulletTarget{Box: projectile.HitBox(r.Box(), vel, true, r.anim.Sneak),
			Feet: [3]float64{r.x, r.y, r.z}, EyeHeight: character.PlayerEye(r.anim.Sneak)})
		who = append(who, hitTarget{player: o})
	}
	for _, z := range s.def.zombies {
		if z.dead > 0 {
			continue
		}
		m := z.m
		ts = append(ts, projectile.BulletTarget{Box: projectile.HitBox(m.Box(), [3]float64{m.X - m.PrevX, m.Y - m.PrevY, m.Z - m.PrevZ}, false, false),
			Feet: [3]float64{m.X, m.Y, m.Z}, EyeHeight: ZombieEyeHeight})
		who = append(who, hitTarget{zombie: z})
	}
	return ts, who
}

// tickBullets flies every bullet a tick against its shooter's enemies as
// the shooter saw them (EntityKineticBullet.onBulletTick, hits dealt by
// Server.damage) and tells everyone which are gone.
func (s *Server) tickBullets(tick uint32) {
	boxes := func(x, y, z int, id voxel.StateID, buf []collide.AABB) []collide.AABB {
		return s.world.W.StateBoxes(id, x, y, z, buf)
	}
	type shooterTargets struct {
		ts  []projectile.BulletTarget
		who []hitTarget
	}
	bySlot := map[int]shooterTargets{}
	live := s.bullets[:0]
	for _, sb := range s.bullets {
		p := s.players[sb.shooter]
		if p == nil { // the shooter left
			s.broadcast(bulletGone(sb.id))
			continue
		}
		t, ok := bySlot[sb.shooter]
		if !ok {
			t.ts, t.who = s.bulletTargets(p, tick)
			bySlot[sb.shooter] = t
		}
		sb.b.Tick(s.world, &projectile.BulletCollider{Boxes: boxes, Targets: t.ts,
			Hit: func(b *projectile.Bullet, h projectile.BulletHit) {
				dmg, knock := float64(b.Damage(h.Pos, h.Head)), float64(b.Gun.Bullet.Knockback)
				// LivingEntity.hurt pushes away from the source's entity, the shooter.
				if z := t.who[h.Target].zombie; z != nil {
					s.hurtZombie(p, z, Hit{Head: h.Head, Gun: true}, dmg, z.m.X-p.ch.X, z.m.Z-p.ch.Z, knock)
					return
				}
				v := t.who[h.Target].player
				if v.dead > 0 {
					return // killed by an earlier bullet
				}
				s.damage(Hit{Attacker: p.slot, Victim: v.slot, Damage: max(1, int(math.Round(dmg))), Head: h.Head, Gun: true},
					v.ch.X-p.ch.X, v.ch.Z-p.ch.Z, knock)
			}})
		if sb.b.Gone {
			s.broadcast(bulletGone(sb.id))
			continue
		}
		live = append(live, sb)
	}
	clear(s.bullets[len(live):])
	s.bullets = live
}

func bulletGone(id uint32) []byte {
	b := wbuf{msgBulletGone}
	b.u32(id)
	return b
}

// melee lands p's held gun's melee hit along l (ModernKineticGunItem.doMelee)
// on the players as p saw them and on the zombies.
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
		te := [3]float64{r.x, r.y + character.PlayerEye(r.anim.Sneak), r.z}
		if !reach.Intersects(r.Box()) || !m.Reaches(eye, d, te) || !projectile.InSight(s.world, boxes, eye, te) {
			continue
		}
		// doPerLivingHurt: knockback(melee knockback) along the look, then
		// hurt's own vanilla knockback away from the attacker.
		push(r.p, float64(m.Knockback), d[0], d[2])
		s.damage(Hit{Attacker: p.slot, Victim: r.p.slot, Damage: int(math.Round(float64(m.Damage))), Melee: true},
			r.p.ch.X-p.ch.X, r.p.ch.Z-p.ch.Z, actors.HurtKnockback)
	}
	for _, z := range s.def.zombies {
		zm := z.m
		te := [3]float64{zm.X, zm.Y + ZombieEyeHeight, zm.Z}
		if z.dead > 0 || !reach.Intersects(zm.Box()) || !m.Reaches(eye, d, te) || !projectile.InSight(s.world, boxes, eye, te) {
			continue
		}
		zm.VX, zm.VY, zm.VZ = actors.KnockbackMotion(zm.VX, zm.VY, zm.VZ, zm.OnGround, float64(m.Knockback), -d[0], -d[2])
		s.hurtZombie(p, z, Hit{Melee: true}, float64(m.Damage), zm.X-p.ch.X, zm.Z-p.ch.Z, actors.HurtKnockback)
	}
}
