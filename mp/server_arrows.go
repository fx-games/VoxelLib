package mp

import (
	"fmt"
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/worldgen/flat"
	"github.com/memmaker/voxellib/worldgen/overworld"
)

// NewGenerator makes the terrain generator a server names in its Welcome.
func NewGenerator(reg *voxel.Registry, name string, seed int64) (Generator, error) {
	switch name {
	case "overworld":
		return overworld.New(reg, seed), nil
	case "flat":
		return flat.New(reg), nil
	}
	if n := MapName(name); n != "" {
		m, err := OpenMap(reg, n)
		if err != nil {
			return nil, err
		}
		return NewMapGen(m), nil
	}
	return nil, fmt.Errorf("mp: unknown generator %q", name)
}

// rewound is a player as a shooter saw them: their pose lag ticks ago,
// between the two recorded ticks around that time. Arrows test the posed
// body exactly, as the shooter's screen drew it (unlagged hit detection,
// applied over an arrow's whole flight).
type rewound struct {
	p       *sPlayer
	x, y, z float64
	yaw     float64
	anim    Anim
	body    *Body
	skin    []mgl32.Mat4
	pose    *model.Pose
	posed   bool
}

func (r *rewound) Box() collide.AABB {
	return collide.Box(r.x, r.y, r.z, character.PlayerWidth, character.PlayerHeight)
}

func (r *rewound) Frame() (x, y, z, yaw float64) { return r.x, r.y, r.z, r.yaw }

// Reach is the box the posed body can occupy: raised arms reach forward.
func (r *rewound) Reach() collide.AABB {
	return collide.Box(r.x, r.y, r.z, 2.2, character.PlayerHeight+0.4)
}

func (r *rewound) Intersect(x0, y0, z0, x1, y1, z1 float64) (x, y, z float64, part int, ok bool) {
	if !segNearBox(r.Reach(), x0, y0, z0, x1, y1, z1) {
		return 0, 0, 0, -1, false
	}
	if !r.posed {
		r.skin = r.body.Skin(r.anim, r.skin)
		r.posed = true
	}
	h, bone, ok := r.body.Raycast(r.skin, r.pose, r.x, r.y, r.z, r.yaw, [3]float64{x0, y0, z0}, [3]float64{x1, y1, z1})
	if !ok {
		return 0, 0, 0, -1, false
	}
	return h[0], h[1], h[2], bone, true
}

// segNearBox is a cheap reject: the segment's bounds miss the box.
func segNearBox(b collide.AABB, x0, y0, z0, x1, y1, z1 float64) bool {
	return math.Max(x0, x1) >= b.MinX && math.Min(x0, x1) <= b.MaxX &&
		math.Max(y0, y1) >= b.MinY && math.Min(y0, y1) <= b.MaxY &&
		math.Max(z0, z1) >= b.MinZ && math.Min(z0, z1) <= b.MaxZ
}

// rewind returns p as seen lag ticks before tick, or false if p was not
// alive then.
func (s *Server) rewind(p *sPlayer, tick uint32, lag float64) (*rewound, bool) {
	t := float64(tick) - lag
	t0 := uint32(math.Floor(t))
	f := t - float64(t0)
	a, b := p.hist[t0%historyTicks], p.hist[(t0+1)%historyTicks]
	// Ticks start at 1: an empty history entry (tick 0) is no pose.
	has := func(q pose, t uint32) bool { return q.tick == t && t != 0 }
	switch {
	case !has(a, t0) && has(b, t0+1):
		a, f = b, 0
	case !has(a, t0):
		a, f = p.hist[tick%historyTicks], 0 // no history (just joined): now
	case !has(b, t0+1):
		b = a
	}
	if !a.alive || !b.alive {
		return nil, false
	}
	return &rewound{p: p, body: s.body, pose: &model.Pose{},
		x: a.x + (b.x-a.x)*f, y: a.y + (b.y-a.y)*f, z: a.z + (b.z-a.z)*f,
		yaw: lerpAngle(a.yaw, b.yaw, f), anim: a.anim.Lerp(b.anim, f)}, true
}

func lerpAngle(a, b, f float64) float64 {
	d := math.Remainder(b-a, 2*math.Pi)
	return a + d*f
}

// targets are p's enemies as p saw them at tick (lag compensated). The
// dead and the protected have no body: shots pass through them.
func (s *Server) targets(p *sPlayer, tick uint32) []projectile.Target {
	var targets []projectile.Target
	for _, o := range s.players {
		if o == nil || !enemies(p, o) || o.dead > 0 || o.protect > 0 {
			continue
		}
		if r, ok := s.rewind(o, tick, p.lag); ok {
			targets = append(targets, r)
		}
	}
	return targets
}

// tickArrows flies each shooter's arrows against the other players as that
// shooter saw them.
func (s *Server) tickArrows(tick uint32) {
	for _, p := range s.players {
		if p == nil {
			continue
		}
		targets := s.targets(p, tick)
		shooter := p
		p.arrows.Hit = func(a *projectile.Arrow, t projectile.Target) {
			if r, ok := t.(*rewound); ok {
				s.hit(shooter, r.p, a)
			}
		}
		p.arrows.Tick(s.world, targets)
	}
	// Forget arrows that are gone; keep the total bounded.
	live := map[*projectile.Arrow]bool{}
	var stuck []*projectile.Arrow
	for _, p := range s.players {
		if p == nil {
			continue
		}
		for _, a := range p.arrows.Arrows {
			live[a] = true
			if a.Stuck {
				stuck = append(stuck, a)
			}
		}
	}
	for a := range s.arrows {
		if !live[a] {
			delete(s.arrows, a)
		}
	}
	if extra := len(s.arrows) - maxArrows; extra > 0 {
		for i := 0; i < extra && i < len(stuck); i++ {
			stuck[i].Discard() // gone next tick
		}
	}
}

// hit deals an arrow's damage: a head shot kills.
func (s *Server) hit(shooter, victim *sPlayer, a *projectile.Arrow) {
	if victim.dead > 0 || victim.protect > 0 {
		return // no one is killed twice, or on arrival (tickArrows skips them)
	}
	a.Discard()
	h := Hit{Attacker: shooter.slot, Victim: victim.slot, Damage: ArrowDamage, Head: IsHead(a.Part)}
	if h.Head {
		h.Damage = HeadDamage
	}
	s.damage(h, a.VX, a.VZ, hitKnockback)
}

// damage deals a hit (arrow or bullet) to a live, unprotected victim and
// knocks them back along dx,dz, like an arrow's knockback in the reference.
// hitKnockback is the push (blocks/tick) of an arrow hit (a bullet pushes by its gun's Bullet.Knockback).
const hitKnockback = 0.4

// damage applies a hit, pushing the victim knock blocks/tick along (dx, dz).
func (s *Server) damage(h Hit, dx, dz, knock float64) {
	shooter, victim := s.players[h.Attacker], s.players[h.Victim]
	victim.health -= h.Damage
	if l := math.Hypot(dx, dz); l > 0 {
		victim.ch.VX += dx / l * knock
		victim.ch.VZ += dz / l * knock
		victim.ch.VY = math.Max(victim.ch.VY, 0.3)
	}
	if victim.health <= 0 {
		h.Killed = true
		how := "shot"
		switch {
		case h.Melee:
			how = "struck down"
		case h.Head:
			how = "head-shot"
		}
		dist := math.Sqrt(sq(victim.ch.X-shooter.ch.X) + sq(victim.ch.Y-shooter.ch.Y) + sq(victim.ch.Z-shooter.ch.Z))
		s.kill(victim, shooter, how, fmt.Sprintf(" (%.0f m)", dist))
	}
	s.broadcast(h.encode())
}

func sq(v float64) float64 { return v * v }

// sendSnapshot sends one player their state and everyone else's, delta
// compressed against the last snapshot they acknowledged.
func (s *Server) sendSnapshot(p *sPlayer, tick uint32) {
	snap := make(map[uint32][]byte, len(s.players)+len(s.arrows))
	for _, o := range s.players {
		if o == nil || o == p {
			continue
		}
		snap[uint32(o.slot+1)] = PlayerView{Slot: o.slot, X: o.ch.X, Y: o.ch.Y, Z: o.ch.Z, Yaw: o.yaw, Pitch: o.pitch,
			Sneak: o.anim.Sneak, Dead: o.dead > 0, Protected: o.protect > 0, Health: o.health, Shots: o.shots}.encode()
	}
	for a, m := range s.arrows {
		snap[arrowBase+m.id] = ArrowView{X: a.X, Y: a.Y, Z: a.Z, VX: a.VX, VY: a.VY, VZ: a.VZ, Yaw: a.Yaw, Pitch: a.Pitch,
			Stuck: a.Stuck, Shooter: m.shooter, Shot: m.shot}.encode()
	}
	var b wbuf
	b.u32(tick)
	b.u32(p.last.Num)
	b.f64(float64(s.time))
	b.u8(flag(s.frozen, 1))
	s.m.round.encode(&b)
	st := stateOf(p.ch)
	st.Health, st.Dead, st.Reload, st.Shots = p.health, p.dead > 0, p.reload, p.shots
	st.Protected = p.protect > 0
	st.encode(&b)
	seq := s.net.NextSeq(p.slot)
	payload := p.enc.Encode(b, seq, s.net.Acked(p.slot), snap)
	if _, err := s.net.Send(p.slot, payload); err != nil {
		s.cfg.Logf("mp: snapshot to %s: %v", p.name, err)
	}
}
