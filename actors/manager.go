package actors

import (
	"math"
	"math/rand"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/nav"
)

// Manager owns the actors of a world and ticks them.
//
// Each tick it runs every NPC's brain, then moves every body that is not a
// player's and not riding (a mount carries its rider), seats riders, pushes
// overlapping actors apart and closes doors behind walkers. The game ticks
// player bodies itself, except while the player rides: then it sets the
// player actor's Yaw and RideInput and leaves the body to the Manager.
type Manager struct {
	W          World
	Actors     []*Actor
	Factions   *Factions
	Arrows     *projectile.World // for ranged attacks; may be nil
	Rand       *rand.Rand
	Difficulty int // 0 peaceful .. 3 hard, as Minecraft (arrow damage)

	// OnDeath is called once when an actor's health reaches 0, with the
	// actor that dealt the last blow (nil for falls and the like).
	OnDeath func(dead, killer *Actor)
	// OnRemove is called when an actor leaves the Manager (after its death
	// animation, by Remove, or by a despawn).
	OnRemove func(a *Actor)
	// ArrowDamage, if set, tells who shot an arrow the Manager did not
	// fire (the player's bow) and its base damage and launch speed.
	ArrowDamage func(*projectile.Arrow) (from *Actor, damage, speed float64)

	byID     map[string]*Actor
	occupied map[nav.Pos]bool
	tracks   map[string][]Vec
	shots    map[*projectile.Arrow]shot
	prevHit  func(*projectile.Arrow, projectile.Target)
	graph    nav.Graph
	ticks    int
}

// NewManager returns a Manager. If arrows is not nil, the Manager installs
// itself as its Hit callback (chaining any callback already there for
// targets that are not actors).
func NewManager(w World, f *Factions, arrows *projectile.World, seed int64) *Manager {
	m := &Manager{W: w, Factions: f, Arrows: arrows, Rand: rand.New(rand.NewSource(seed)), Difficulty: 2,
		byID: map[string]*Actor{}, occupied: map[nav.Pos]bool{}, tracks: map[string][]Vec{},
		shots: map[*projectile.Arrow]shot{}}
	if arrows != nil {
		m.prevHit = arrows.Hit
		arrows.Hit = m.arrowHit
	}
	return m
}

// Add puts an actor into the world. An actor without a Home gets its
// current position. Spawning is the game's call: only where nobody sees.
func (m *Manager) Add(a *Actor) {
	if a.Brain.Home == (Vec{}) {
		a.Brain.Home = a.Pos()
	}
	if a.Tags == nil {
		a.Tags = map[string]string{}
	}
	m.Actors = append(m.Actors, a)
	m.byID[a.ID] = a
}

// Get returns the actor with that ID, or nil.
func (m *Manager) Get(id string) *Actor { return m.byID[id] }

// Remove takes an actor out of the world. Story rule: only off-stage.
func (m *Manager) Remove(a *Actor) {
	if m.byID[a.ID] != a {
		return
	}
	if a.Riding != nil {
		m.Dismount(a)
	}
	if a.Rider != nil {
		m.Dismount(a.Rider)
	}
	delete(m.byID, a.ID)
	delete(m.tracks, a.ID)
	for i, o := range m.Actors {
		if o == a {
			m.Actors = append(m.Actors[:i], m.Actors[i+1:]...)
			break
		}
	}
	for _, o := range m.Actors {
		if o.Target == a {
			o.Target = nil
		}
		if o.revenge == a {
			o.revenge = nil
		}
	}
	if m.OnRemove != nil {
		m.OnRemove(a)
	}
}

// Tick advances every actor by one 50 ms step.
func (m *Manager) Tick() {
	m.ticks++
	clear(m.occupied)
	leaders := map[string]bool{}
	for _, a := range m.Actors {
		if a.Body.VX*a.Body.VX+a.Body.VZ*a.Body.VZ < 1e-4 {
			m.occupied[a.cell()] = true
		}
		if a.Brain.Mode == Follow {
			leaders[a.Brain.Leader] = true
		}
	}
	for id := range leaders {
		if l := m.byID[id]; l != nil {
			m.record(l)
		}
	}
	var gone []*Actor
	for _, a := range m.Actors {
		a.PrevYaw, a.PrevHeadYaw = a.Yaw, a.HeadYaw
		a.StateTicks++
		a.age++
		a.Swing++
		if a.hurtResist > 0 {
			a.hurtResist--
		}
		if a.HurtTime > 0 {
			a.HurtTime--
		}
		if a.attackCool > 0 {
			a.attackCool--
		}
		if a.revengeTicks > 0 {
			a.revengeTicks--
		}
		if !a.Alive() {
			a.DeathTime++
			a.set(Dead)
			a.nav.in.Forward, a.nav.in.Strafe, a.nav.in.Jump = 0, 0, false
			if a.DeathTime >= DeathTicks {
				gone = append(gone, a)
			}
			continue
		}
		if a.Player || (a.Rider != nil) {
			continue
		}
		m.think(a)
	}
	for _, a := range m.Actors {
		if a.Riding != nil || (a.Player && a.Riding == nil) {
			continue
		}
		in := a.nav.in
		if r := a.Rider; r != nil && r.Player && a.Alive() {
			in = m.rideInput(a)
		}
		in.Yaw = a.Yaw
		m.moveBody(a, in)
	}
	for _, a := range m.Actors {
		if a.Riding != nil {
			m.seat(a)
			if !a.Player {
				a.set(Ride)
			}
		}
	}
	m.push()
	for _, a := range m.Actors {
		if len(a.nav.opened) > 0 {
			m.closeDoors(a)
		}
	}
	for _, a := range gone {
		m.Remove(a)
	}
}

// moveBody ticks one body with the 1.7.10 living physics, applies fall
// damage and picks the animation state.
func (m *Manager) moveBody(a *Actor, in character.Input) {
	b := a.Body
	b.Tick(m.W, in)
	if b.InWater || b.InLava {
		a.fall = 0
	} else if b.OnGround {
		if a.fall > 3 {
			m.Hurt(a, nil, math.Ceil(a.fall-3), true)
		}
		a.fall = 0
	} else if dy := b.Y - b.PrevY; dy < 0 {
		a.fall -= dy
	}
	if !a.Alive() {
		return
	}
	speed := math.Hypot(b.X-b.PrevX, b.Z-b.PrevZ)
	switch {
	case b.InWater && !b.OnGround:
		a.set(Swim)
	case !b.OnGround && b.VY > 0.05:
		a.set(Jump)
	case speed > 0.2:
		a.set(Run)
	case speed > 0.01:
		a.set(Walk)
	default:
		a.set(Idle)
	}
	if a.State != Idle {
		a.HeadYaw = turn(a.HeadYaw, a.Yaw, turnRate)
	}
}

// record keeps a leader's track: a point every 4 blocks, at most 400; a
// jump of more than 64 blocks starts it afresh.
func (m *Manager) record(l *Actor) {
	tr := m.tracks[l.ID]
	p := l.carrier().Pos()
	if n := len(tr); n > 0 {
		d := tr[n-1].dist2(p)
		if d > 64*64 {
			tr = tr[:0]
		} else if d < 16 {
			return
		}
	}
	tr = append(tr, p)
	if len(tr) > 400 {
		tr = tr[1:]
	}
	m.tracks[l.ID] = tr
}

// push is EntityLivingBase.collideWithNearbyEntities/applyEntityCollision:
// actors whose boxes (grown by 0.2 sideways) overlap push each other apart
// through their velocity. A rider and its mount don't push.
func (m *Manager) push() {
	type cell struct{ x, z int }
	type entry struct {
		a *Actor
		i int
	}
	grid := map[cell][]entry{}
	for i, a := range m.Actors {
		if a.Riding == nil && a.Alive() {
			c := cell{int(math.Floor(a.Body.X / 2)), int(math.Floor(a.Body.Z / 2))}
			grid[c] = append(grid[c], entry{a, i})
		}
	}
	for c, list := range grid {
		for _, e := range list {
			box := e.a.Body.Box().Grow(0.2, 0, 0.2)
			for dx := -1; dx <= 1; dx++ {
				for dz := -1; dz <= 1; dz++ {
					for _, f := range grid[cell{c.x + dx, c.z + dz}] {
						if f.i > e.i && box.Intersects(f.a.Body.Box()) {
							collidePair(e.a, f.a)
						}
					}
				}
			}
		}
	}
}

func collidePair(a, b *Actor) {
	dx, dz := b.Body.X-a.Body.X, b.Body.Z-a.Body.Z
	d := math.Max(math.Abs(dx), math.Abs(dz))
	if d < 0.01 {
		dx, dz, d = 0.01, 0.005, 0.01 // exactly stacked: pick a side
	}
	d = math.Sqrt(d)
	dx, dz = dx/d, dz/d
	k := math.Min(1, 1/d) * 0.05
	dx, dz = dx*k, dz*k
	a.Body.VX -= dx
	a.Body.VZ -= dz
	b.Body.VX += dx
	b.Body.VZ += dz
}
