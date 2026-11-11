package mp

// Zombie defense on the server (the rules: zdefense.go): spawning, the
// zombies' chase and blows, their wounds, and the pickups.

import (
	"fmt"
	"math"
	"time"

	"github.com/memmaker/voxellib/actors"
	"github.com/memmaker/voxellib/mobs"
	"github.com/memmaker/voxellib/voxel/collide"
)

// firstEntityID is the first id Server.nextID hands out: above every
// player's snapshot id (slot+1).
const firstEntityID = 16

// defense is the server's zombie defense state.
type defense struct {
	loot    Loot
	spawn   int // ticks until the next spawn
	extra   int // more zombies in the next wave (ServerConfig.Zombies)
	zombies []*sZombie
	pickups []*sPickup
	mobBuf  []*mobs.Mob
}

type sZombie struct {
	id     uint32
	m      *mobs.Mob
	health float64
	dead   int // ticks since dying (0: alive)
}

type sPickup struct {
	id uint32
	Pickup
	pos [3]float64
	age int
}

// views are the players' view frustums now.
func (s *Server) views() []View {
	var vs []View
	for _, p := range s.players {
		if p != nil {
			v := p.view
			ex, ey, ez := p.ch.Eye()
			v.Eye, v.Look = [3]float64{ex, ey, ez}, Look{Yaw: p.yaw, Pitch: p.pitch}
			vs = append(vs, v)
		}
	}
	return vs
}

// living are the players alive now.
func (s *Server) living() []*sPlayer {
	var ps []*sPlayer
	for _, p := range s.players {
		if p != nil && p.dead == 0 {
			ps = append(ps, p)
		}
	}
	return ps
}

// tickDefense runs zombie defense for a tick.
func (s *Server) tickDefense() {
	if s.cfg.Mode != ZombieDefense {
		return
	}
	d := &s.def
	live := s.living()
	for _, p := range live {
		for range loadPerTick {
			if !s.world.LoadNext(p.ch.X, p.ch.Z, ZombieMaxDist) {
				break
			}
		}
	}
	if d.spawn--; d.spawn <= 0 && len(live) > 0 {
		d.spawn = SpawnInterval
		s.spawnWave(live)
	}
	s.tickZombies(live)
	s.tickPickups(live)
}

// spawnWave spawns a zombie, a gun and an ammunition pickup out of sight,
// each round a random living player.
func (s *Server) spawnWave(live []*sPlayer) {
	views := s.views()
	d := &s.def
	spot := func(what string, minD, maxD, h float64) ([3]float64, bool) {
		p := live[s.rng.Intn(len(live))]
		feet := [3]float64{p.ch.X, p.ch.Y, p.ch.Z}
		at, ok := PickSpot(s.rng, feet, minD, maxD, h, views, s.world)
		if !ok {
			s.cfg.Logf("mp: zd: no %s spot out of sight %.0f..%.0f blocks from %s in %d tries; trying next interval", what, minD, maxD, p.name, SpotTries)
			return at, false
		}
		s.cfg.Logf("mp: zd: %s at %.1f,%.1f,%.1f, %.1f blocks from %s, in view %v", what, at[0], at[1], at[2],
			math.Hypot(at[0]-feet[0], at[2]-feet[2]), p.name, Seen(views, at, h))
		return at, true
	}
	n := 1 + d.extra
	d.extra = 0
	for range n {
		if at, ok := spot("zombie", ZombieMinDist, ZombieMaxDist, ZombieHeight); ok {
			m := mobs.New(mobs.Walker, at[0], at[1], at[2], ZombieWidth, ZombieHeight)
			m.Yaw = s.rng.Float64() * 2 * math.Pi
			m.Reach, m.Cooldown, m.AttackTicks, m.HitTick = ZombieReach, ZombieCooldown, ZombieAttackTicks, 1
			s.nextID++
			d.zombies = append(d.zombies, &sZombie{id: s.nextID, m: m, health: ZombieHealth})
		}
	}
	for _, pick := range []func() (Pickup, bool){func() (Pickup, bool) { return d.loot.Gun(s.rng) }, func() (Pickup, bool) { return d.loot.RandomAmmo(s.rng) }} {
		pk, ok := pick()
		if !ok {
			continue // a server without a gun pack spawns none
		}
		if at, ok := spot(pk.Sprite(), PickupMinDist, PickupMaxDist, 2*pickupRadius); ok {
			s.addPickup(pk, at)
		}
	}
}

func (s *Server) addPickup(pk Pickup, at [3]float64) {
	s.nextID++
	s.def.pickups = append(s.def.pickups, &sPickup{id: s.nextID, Pickup: pk, pos: at})
}

// tickStatsPeriod is how often ServerConfig.Zombies logs tick times.
const tickStatsPeriod = 5 * TickRate

// tickStats sums the server's tick times for ServerConfig.Zombies.
type tickStats struct {
	n        int
	sum, max time.Duration
}

func (t *tickStats) add(s *Server, d time.Duration) {
	t.n++
	t.sum += d
	t.max = max(t.max, d)
	if t.n < tickStatsPeriod {
		return
	}
	alive := 0
	for _, z := range s.def.zombies {
		if z.dead == 0 {
			alive++
		}
	}
	s.cfg.Logf("mp: zd: %d zombies alive, tick mean %v max %v (budget %v)", alive,
		(t.sum / time.Duration(t.n)).Round(time.Microsecond), t.max.Round(time.Microsecond), time.Second/TickRate)
	*t = tickStats{}
}

// nearest is the living player closest to m (nil: none).
func nearest(live []*sPlayer, m *mobs.Mob) *sPlayer {
	var best *sPlayer
	bd := math.Inf(1)
	for _, p := range live {
		if d := sq(p.ch.X-m.X) + sq(p.ch.Y-m.Y) + sq(p.ch.Z-m.Z); d < bd {
			best, bd = p, d
		}
	}
	return best
}

// tickZombies moves the zombies after the nearest player, lands their
// blows and removes the dead once they have fallen.
func (s *Server) tickZombies(live []*sPlayer) {
	d := &s.def
	all := d.mobBuf[:0]
	keep := d.zombies[:0]
	for _, z := range d.zombies {
		m := z.m
		if z.dead > 0 {
			if z.dead++; z.dead > ZombieDeathTicks {
				continue
			}
			m.PrevX, m.PrevY, m.PrevZ, m.PrevYaw = m.X, m.Y, m.Z, m.Yaw
			keep = append(keep, z)
			continue
		}
		t := nearest(live, m)
		m.Hostile = t != nil
		if t != nil {
			m.GoalX, m.GoalY, m.GoalZ = t.ch.X, t.ch.Y, t.ch.Z
		}
		m.Tick(s.world, s.rng)
		if m.Strike && t != nil && t.protect == 0 && math.Hypot(t.ch.X-m.X, t.ch.Z-m.Z) <= ZombieHitReach && math.Abs(t.ch.Y-m.Y) < 2 {
			s.zombieBlow(z, t)
		}
		all = append(all, m)
		keep = append(keep, z)
	}
	clear(d.zombies[len(keep):])
	d.zombies = keep
	// Players shove zombies aside but aren't shoved back.
	for _, p := range live {
		all = append(all, &mobs.Mob{X: p.ch.X, Y: p.ch.Y, Z: p.ch.Z, Width: PlayerHalfWidth * 2, Height: ZombieHeight, Static: true})
	}
	mobs.Separate(all, s.world)
	d.mobBuf = all[:0]
}

// zombieBlow is a zombie's blow landing on t.
func (s *Server) zombieBlow(z *sZombie, t *sPlayer) {
	t.health -= ZombieDamage
	push(t, actors.HurtKnockback, t.ch.X-z.m.X, t.ch.Z-z.m.Z)
	h := Hit{Attacker: t.slot, Victim: t.slot, Zombie: z.id, ByZombie: true, Damage: ZombieDamage}
	if t.health <= 0 {
		h.Killed = true
		s.kill(t, nil, "was eaten by a zombie", "")
	}
	s.broadcast(h.encode())
}

// hurtZombie deals p's hit to z, pushing it knock along (dx, dz); a kill
// counts for p and drops a magazine of p's gun's ammunition.
func (s *Server) hurtZombie(p *sPlayer, z *sZombie, h Hit, dmg float64, dx, dz, knock float64) {
	if z.dead > 0 {
		return
	}
	m := z.m
	z.health -= dmg
	m.VX, m.VY, m.VZ = actors.KnockbackMotion(m.VX, m.VY, m.VZ, m.OnGround, knock, -dx, -dz)
	h.Attacker, h.Zombie, h.Damage = p.slot, z.id, max(1, int(math.Round(dmg)))
	if z.health <= 0 {
		h.Killed = true
		z.dead = 1
		m.VX, m.VY, m.VZ = 0, 0, 0
		p.kills++
		s.broadcast(p.score().encode())
		if p.gun != nil {
			s.addPickup(s.def.loot.Magazine(p.gun.Ammo), [3]float64{m.X, m.Y, m.Z})
		}
		s.cfg.Logf("mp: zd: %s killed zombie %d", p.name, z.id)
	}
	s.broadcast(h.encode())
}

// tickPickups gives touched pickups to their players and lets old ones go.
func (s *Server) tickPickups(live []*sPlayer) {
	d := &s.def
	keep := d.pickups[:0]
	for _, pk := range d.pickups {
		if pk.age++; pk.age > pickupLifespan {
			continue
		}
		box := collide.Box(pk.pos[0], pk.pos[1], pk.pos[2], 2*pickupRadius, 2*pickupRadius)
		var taker *sPlayer
		for _, p := range live {
			if p.ch.Box().Grow(pickupGrowXZ, pickupGrowY, pickupGrowXZ).Intersects(box) {
				taker = p
				break
			}
		}
		if taker == nil {
			keep = append(keep, pk)
			continue
		}
		b := wbuf{msgPickup}
		pk.Pickup.encode(&b)
		s.net.SendReliable(taker.slot, b)
		s.cfg.Logf("mp: zd: %s took %s", taker.name, describe(pk.Pickup))
	}
	clear(d.pickups[len(keep):])
	d.pickups = keep
}

// cheatMagazines is how many magazines of each ammunition the gun cheat gives.
const cheatMagazines = 3

// giveAllGuns is the gun cheat: every gun and cheatMagazines of each
// ammunition, given like pickups.
func (s *Server) giveAllGuns(p *sPlayer) {
	if s.cfg.Mode != ZombieDefense {
		return
	}
	l := s.def.loot
	var pks []Pickup
	for _, id := range l.Guns {
		pks = append(pks, Pickup{Kind: PickupGun, ID: id})
	}
	for _, id := range l.Ammo {
		m := l.Magazine(id)
		m.Rounds *= cheatMagazines
		pks = append(pks, m)
	}
	for _, pk := range pks {
		b := wbuf{msgPickup}
		pk.encode(&b)
		s.net.SendReliable(p.slot, b)
	}
	s.cfg.Logf("mp: zd: %s used the gun cheat", p.name)
}

func describe(p Pickup) string {
	if p.Kind == PickupGun {
		return "gun " + p.ID
	}
	return fmt.Sprintf("%d rounds of %s", p.Rounds, p.ID)
}

// snapshot adds the zombies and pickups to a snapshot.
func (d *defense) snapshot(snap map[uint32][]byte) {
	for _, z := range d.zombies {
		m := z.m
		attack := -1
		if m.State == mobs.Attack {
			attack = m.StateTicks
		}
		snap[z.id] = ZombieView{X: m.X, Y: m.Y, Z: m.Z, Yaw: m.Yaw, Hostile: m.Hostile, Dead: z.dead > 0, Attack: attack}.encode()
	}
	for _, pk := range d.pickups {
		snap[pk.id] = PickupView{Pickup: pk.Pickup, X: pk.pos[0], Y: pk.pos[1], Z: pk.pos[2]}.encode()
	}
}
