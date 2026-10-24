package actors

import (
	"github.com/memmaker/voxellib/voxel/collide"
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
	Difficulty int // 0 peaceful .. 3 hard, as the reference (arrow damage)

	// Loaded, if set, is World.checkChunksExist for the columns within r
	// blocks of x, z: an actor without its 32-block area loaded is not
	// updated (updateEntityWithOptionalForce), it neither thinks nor moves.
	Loaded func(x, z, r int) bool

	// OnDeath is called once when an actor's health reaches 0, with the
	// actor that dealt the last blow (nil for falls and the like).
	OnDeath func(dead, killer *Actor)
	// OnRemove is called when an actor leaves the Manager (after its death
	// animation, by Remove, or by a despawn).
	OnRemove func(a *Actor)
	// OnFire is called when an actor's ranged attack launches an arrow at
	// t (attackEntityWithRangedAttack: the shot's sound).
	OnFire func(a, t *Actor)
	// Launch, if set, launches the projectile of a's ranged attack at t
	// (attackEntityWithRangedAttack) instead of an Arrows arrow; f is the
	// task's power argument (LOTR and skeleton: the draw, 0.1..1; CNPC:
	// 1 for an indirect, arcing shot, else 0). OnFire is not called then.
	Launch func(a, t *Actor, f float64)
	// Sees, if set, is EntitySenses.canSee (the eye-to-eye block ray);
	// nil sees everything.
	Sees func(a, t *Actor) bool
	// CanTarget, if set, is a target task's isSuitableTarget: whether a
	// may pick t as its nearest hostile (to fight, or to flee from in
	// Flee mode); revenge is not asked.
	CanTarget func(a, t *Actor) bool
	// AlsoTargets, if set, lets a pick t as its nearest hostile though it
	// does not see t as hostile (a target task's own rule, asked last,
	// after CanTarget: it may roll a chance).
	AlsoTargets func(a, t *Actor) bool
	// AttackerClears, if set, reports a non-player source whose hurt
	// clears the target's Attacker (a tame wolf's).
	AttackerClears func(src *Actor) bool
	// Keeps, if set, keeps an attack target the game set although a does
	// not see it as hostile (a target task's own pick: a wolf's sheep, a
	// skirmish); a dead or out-of-range target still goes.
	Keeps func(a, t *Actor) bool
	// PathWeight, if set, is a's getBlockPathWeight at a cell (the random
	// spots a CustomNPC's tasks pick); nil weighs all alike.
	PathWeight func(a *Actor, x, y, z int) float32
	// Sky answers the CustomNPC shelter tasks (CNPCAI.FindShelter); nil:
	// they never run.
	Sky SkyWorld
	// Doors is the world as the CustomNPC door buster sees it
	// (CNPCAI.DoorInteract 0); nil: it never runs.
	Doors DoorBuster
	// Griefing is the mobGriefing rule: without it the door buster never
	// starts (EntityAIBustDoor.shouldExecute).
	Griefing bool
	// Opaque is Block.isOpaqueCube and RayHits World.rayTraceBlocks (a
	// block meets the segment): the cover a CustomNPC ambushes or stalks
	// from. Without both it finds none.
	Opaque  func(x, y, z int) bool
	RayHits func(from, to Vec) bool
	// OnExtinguish is called when water or rain puts a burning actor out
	// (random.fizz 0.7, 1.6 + (r - r) * 0.4).
	OnExtinguish func(a *Actor)
	// FellOn, if set, is called when an actor lands with a fall distance
	// (the block's onFallenUpon), before OnLand and the fall damage.
	FellOn func(a *Actor, fall float64)
	// ArrowDamage, if set, tells who shot an arrow the Manager did not
	// fire (the player's bow) and its base damage and launch speed.
	ArrowDamage func(*projectile.Arrow) (from *Actor, damage, speed float64)
	// Protection, if set, is t's summed enchantment protection (EPF)
	// against a hurt from src (nil: no attacker) of the kind how; Hurt
	// reduces the damage by it as applyPotionDamageCalculations.
	Protection func(t, src *Actor, how HurtKind) int
	// ProtectionAdjust, if set, changes the randomised protection i
	// (getEnchantmentModifierDamage's result) before the cap of 20, as a
	// hook on its return would.
	ProtectionAdjust func(t, src *Actor, how HurtKind, i int) int
	// Effects are the status effect kinds, in the order they tick.
	Effects []EffectRule

	byID     map[string]*Actor
	occupied map[nav.Pos]bool
	tracks   map[string][]Vec
	wrecks   []*Actor // boats broken this tick
	shots    map[*projectile.Arrow]shot
	prevHit  func(*projectile.Arrow, projectile.Target)
	graph    nav.Graph
	ticks    int
	hurtKind HurtKind // of the Hurt in progress

	// path searches (pump): one runs on search at a time, the rest queue
	search          nav.Graph
	searching       *Actor
	queue           []*Actor
	budget          int // expansions left this tick
	doorFn, waterFn func(x, y, z int) bool
	costFn          func(nav.Pos) int
	half            float64 // half the width of the walker the graph is set for

	bodyW bodyWorld // body's

	pushGrid map[pushCell][]pushEntry // push's, reused
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
	m.cancelSearch(a)
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
	m.budget = pathBudget // after occupied: searches cost occupied cells
	m.pump()
	for id := range leaders {
		if l := m.byID[id]; l != nil {
			m.record(l)
		}
	}
	var gone []*Actor
	for _, a := range m.Actors {
		a.frozen = !a.Player && m.Loaded != nil && !m.Loaded(int(math.Floor(a.Body.X)), int(math.Floor(a.Body.Z)), 32)
		if a.frozen {
			continue
		}
		a.PrevYaw, a.PrevHeadYaw = a.Yaw, a.HeadYaw
		a.Ticks++
		if a.Boat != nil {
			continue
		}
		a.StateTicks++
		a.age++
		if a.PersistReq || m.playerWithin(a, 32) { // despawnEntity
			a.age = 0
		}
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
		if a.attackerTicks > 0 {
			a.attackerTicks--
		} else {
			a.attacker = nil
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
		m.fireTick(a)
		if !a.Player {
			m.effectTick(a)
		}
		if !a.Alive() || a.Player {
			continue
		}
		if a.Mount != nil {
			m.mountTick(a)
		}
		if r := a.Rider; r != nil {
			// an NPC rider steers; a player steers a saddled mount, an
			// untamed one bucks, any other carries the player on its own
			if !r.Player || a.Controlled() {
				continue
			}
			a.Target = nil
			if !a.Mount.Tame {
				m.buck(a)
				continue
			}
			if m.steering(a) {
				continue
			}
		}
		if a.Blocked {
			a.nav.in = character.Input{}
			continue
		}
		m.think(a)
	}
	for _, a := range m.Actors {
		if a.Riding != nil || (a.Player && a.Riding == nil) || a.frozen {
			continue
		}
		if a.Boat != nil {
			if !a.Boat.broken {
				m.boatTick(a)
			}
			continue
		}
		if a.Move != nil {
			a.Move(a)
			continue
		}
		in := a.nav.in
		a.Body.StepHeight, a.Body.AirAccel = 0, 0
		if s := a.Mount; s != nil && a.Alive() {
			if a.Controlled() {
				in = m.rideInput(a)
			} else if s.Steer != nil && s.Steer.running {
				in = m.steerInput(a)
			} else if s.Eating || s.rearing { // isMovementBlocked
				in.Forward, in.Strafe, in.Jump = 0, 0, false
			}
		}
		in.Yaw = a.Yaw
		if a.BeforeMove != nil {
			a.BeforeMove(a, in)
		}
		m.moveBody(a, in)
		if s := a.Mount; s != nil && a.Alive() && !a.Controlled() && s.Steer != nil && s.Steer.running {
			// onLivingUpdate's own step after the task's, with no input
			m.moveBody(a, character.Input{Yaw: a.Yaw, Jump: s.Steer.jump && a.Body.OnGround})
		}
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
	for _, a := range append(gone, m.wrecks...) {
		m.Remove(a)
	}
	m.wrecks = m.wrecks[:0]
}

// moveBody ticks one body with the 1.7.10 living physics, applies fall
// damage and picks the animation state.
func (m *Manager) moveBody(a *Actor, in character.Input) {
	b := a.Body
	f := a.Fly
	if f != nil && b.OnGround {
		in.Forward, in.Strafe = 0, 0
	}
	b.Tick(m.body(), in)
	if f != nil {
		if f.Still {
			b.VX, b.VY, b.VZ = 0, 0, 0
		} else {
			b.VY *= f.Hover
		}
	}
	if s := a.Mount; s != nil && s.SwimUp && b.InWater && b.VY < 0 && a.Rider != nil && a.Rider.Player &&
		collide.Fits(m.body(), collide.Box(b.X, b.Y-1, b.Z, b.Width, b.Height+1)) && m.Rand.Float64() < 0.55 {
		b.VY += 0.05
	}
	if b.OnLadder { // moveEntityWithHeading zeroes fallDistance before the move
		a.fall = 0
	}
	if b.InWater || b.InLava || f != nil {
		a.fall = 0
	} else if b.OnGround {
		if m.FellOn != nil && a.fall > 0 {
			m.FellOn(a, a.fall)
		}
		if a.OnLand != nil && a.fall > 0 {
			a.fall = a.OnLand(a, a.fall)
		}
		if s := a.Mount; s != nil && s.Horse { // EntityHorse.fall
			if a.fall > 1 {
				s.event(a, MountLanded)
			}
			if d := math.Ceil(a.fall*0.5 - 3); d > 0 {
				m.Hurt(a, nil, d, true)
				if r := a.Rider; r != nil {
					m.Hurt(r, nil, d, true)
				}
			}
		} else if a.fall > 3 { // EntityLivingBase.fall; Entity.fall passes it to the rider
			if !a.NoFallHurt {
				m.Hurt(a, nil, math.Ceil(a.fall-3), true)
			}
			if r := a.Rider; r != nil {
				m.Hurt(r, nil, math.Ceil(a.fall-3), true)
			}
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
	if f != nil && len(a.Looks) > 0 && a.Brain.LookAt == "" {
		// a flier's look tasks: its flight is no AI task, so all of them
		// run aloft; EntityBodyHelper by the horizontal motion
		m.lookRun(a, false, false, speed*speed > 2.5e-7)
		return
	}
	if len(a.Looks) > 0 && a.Brain.LookAt == "" && a.Fly == nil && !a.lt.ran && a.State != Idle {
		m.lookTasks(a, a.lt.busy, true) // moving: WatchClosest keeps the head
		if !a.lt.busy {
			return
		}
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

// push is the reference's per-entity push loops: each living actor's own
// collideWithNearbyEntities (dying ones too: they run onLivingUpdate until
// removed) and each boat's loop over boats of its own Kind push every
// pushable actor whose box overlaps theirs grown by 0.2 sideways, through
// applyEntityCollision, which moves both. A pair is so pushed once per
// loop that reaches it: two living actors twice a tick, a living actor and
// a boat once. A rider and its mount don't push each other; a rider pushes
// but its own velocity is the mount's business (updateRidden zeroes it).
// Fliers neither push nor are pushed.
// pushCell and pushEntry are push's grid of bodies by 2×2 column.
type pushCell struct{ x, z int }
type pushEntry struct {
	a *Actor
	i int
}

func (m *Manager) push() {
	type cell = pushCell
	type entry = pushEntry
	grid := m.pushGrid
	if grid == nil || len(grid) > 4*len(m.Actors)+64 {
		grid = map[cell][]entry{}
		m.pushGrid = grid
	}
	for c, l := range grid { // reused tick to tick: keep the slices
		grid[c] = l[:0]
	}
	for i, a := range m.Actors {
		if (a.Fly == nil || a.Fly.Pushed) && (a.Boat == nil || !a.Boat.broken) {
			c := cell{int(math.Floor(a.Body.X / 2)), int(math.Floor(a.Body.Z / 2))}
			grid[c] = append(grid[c], entry{a, i})
		}
	}
	loops := func(a, b *Actor) int { // does a's loop push b?
		if a.Boat == nil || b.Boat != nil && a.Kind == b.Kind {
			return 1
		}
		return 0
	}
	for c, list := range grid {
		for _, e := range list {
			box := e.a.Body.Box().Grow(0.2, 0, 0.2)
			for dx := -1; dx <= 1; dx++ {
				for dz := -1; dz <= 1; dz++ {
					for _, f := range grid[cell{c.x + dx, c.z + dz}] {
						a, b := e.a, f.a
						if f.i <= e.i || a.Riding == b || b.Riding == a || !box.Intersects(b.Body.Box()) {
							continue
						}
						if pa, pb := vehiclePush(a), vehiclePush(b); pa != nil || pb != nil {
							if pa != nil {
								pa(a, b)
							}
							if pb != nil {
								pb(b, a)
							}
							continue
						}
						for range loops(a, b) + loops(b, a) {
							collidePair(a, b)
						}
					}
				}
			}
		}
	}
}

// vehiclePush is a vehicle's own Push, or nil.
func vehiclePush(a *Actor) func(a, o *Actor) {
	if a.Boat != nil {
		return a.Boat.Push
	}
	return nil
}

// collidePair is applyEntityCollision: below 0.01 apart (the larger axis)
// nothing happens; else both get 0.05 apart, scaled by min(1, 1/sqrt d),
// along the offset over sqrt d (MathHelper's float square root).
func collidePair(a, b *Actor) {
	dx, dz := b.Body.X-a.Body.X, b.Body.Z-a.Body.Z
	d := math.Max(math.Abs(dx), math.Abs(dz))
	if d < 0.009999999776482582 {
		return
	}
	d = float64(float32(math.Sqrt(d)))
	k := math.Min(1, 1/d)
	dx, dz = dx/d*k*0.05000000074505806, dz/d*k*0.05000000074505806
	if a.Riding == nil && !a.Unpushed {
		a.Body.VX -= dx
		a.Body.VZ -= dz
	}
	if b.Riding == nil && !b.Unpushed {
		b.Body.VX += dx
		b.Body.VZ += dz
	}
}

// SkyWorld is the world's sky and light as the shelter tasks ask them.
type SkyWorld interface {
	SeesSky(x, y, z int) bool       // World.canBlockSeeTheSky
	FullLight(x, y, z int) int      // World.getFullBlockLightValue
	Brightness(x, y, z int) float32 // World.getLightBrightness
	Daytime() bool                  // World.isDaytime
	Raining() bool                  // World.isRaining
}

// DoorBuster is what EntityAIBustDoor does to the world.
type DoorBuster interface {
	Bustable(x, y, z int) bool          // a wooden door (func_151503_a)
	Hit(a *Actor, x, y, z int)          // playAuxSFX 1010
	Crack(a *Actor, x, y, z, stage int) // destroyBlockInWorldPartially (-1 clears)
	Bust(a *Actor, x, y, z int)         // setBlock air, playAuxSFX 1012 and 2001
}

// playerWithin reports whether the player nearest a is closer than r
// blocks (getClosestPlayerToEntity, a 3-D distance).
func (m *Manager) playerWithin(a *Actor, r float64) bool {
	for _, o := range m.Actors {
		if o.Player && o.Pos().dist2(a.Pos()) < r*r {
			return true
		}
	}
	return false
}
