// Package actors runs NPCs, monsters and mounts: living bodies on the
// character controller's physics from the reference, with behaviours (stand and look,
// wander, follow a leader in formation, walk routes through doors, guard,
// flee), 1.7.10 combat (melee timing, hurt and invulnerability ticks,
// knockback, armour, arrows through package projectile), faction relations,
// riding, and spawning rules.
//
// Rules of the package: actors only ever move physically. Nothing here
// teleports an actor. Spawning and despawning (Spawner) only
// happen where the game's Visible hook says no player can see.
//
// It knows nothing about models, rendering or game content: the game
// supplies the World, the faction table, spawn tables and actor templates.
// Tick at 20 Hz.
package actors

import (
	"math"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/nav"
)

// World is what actors need to know about the terrain. A closed door or
// gate is Solid; Door reports doors and gates whatever their state, so
// paths go through them, and SetDoor opens or closes one (the game sets
// both halves of a door, plays the sound, and so on).
type World interface {
	character.World
	Door(x, y, z int) (door, open bool)
	SetDoor(x, y, z int, open bool)
}

// BodySolids is an optional World method: the cells bodies collide with
// as whole cubes. A World whose Solid is the path finder's cell test
// (true for slabs, fences and doors too) implements it so bodies meet those
// blocks' real boxes (World.Boxes) instead, as the reference entities do.
type BodySolids interface {
	BodySolid(x, y, z int) bool
}

// bodyWorld is a World seen by bodies: BodySolid as Solid.
type bodyWorld struct {
	World
	b BodySolids
}

func (w bodyWorld) Solid(x, y, z int) bool { return w.b.BodySolid(x, y, z) }

// Boxes passes the World's collide.Boxer on (an embedded interface
// promotes only World's own methods).
func (w bodyWorld) Boxes(area collide.AABB, out []collide.AABB) []collide.AABB {
	if b, ok := w.World.(collide.Boxer); ok {
		return b.Boxes(area, out)
	}
	return out
}

// body is the world bodies move in.
func (m *Manager) body() character.World {
	if b, ok := m.W.(BodySolids); ok {
		return bodyWorld{m.W, b}
	}
	return m.W
}

// Fits reports whether a's body overlaps no block box it collides with
// (EntityLiving.getCanSpawnHere: getCollidingBoundingBoxes(boundingBox)
// is empty). A body placed overlapping blocks ignores those boxes but can
// be shut in by the next ones, as in the reference.
func (m *Manager) Fits(a *Actor) bool { return collide.Fits(m.body(), a.Body.Box()) }

// Vec is a world position.
type Vec struct{ X, Y, Z float64 }

func (v Vec) dist2(o Vec) float64 {
	return (v.X-o.X)*(v.X-o.X) + (v.Y-o.Y)*(v.Y-o.Y) + (v.Z-o.Z)*(v.Z-o.Z)
}

func (v Vec) hdist(o Vec) float64 { return math.Hypot(v.X-o.X, v.Z-o.Z) }

// State is what the actor is doing, for picking an animation.
type State uint8

const (
	Idle State = iota
	Walk
	Run
	Jump
	Swim
	Ride
	Dead
)

func (s State) String() string {
	return [...]string{"Idle", "Walk", "Run", "Jump", "Swim", "Ride", "Dead"}[s]
}

// Melee is a close attack. Zero Damage means the actor never strikes.
type Melee struct {
	Damage float64 // half hearts (attackDamage attribute: zombie 3)
	// Cooldown is the ticks between blows (EntityAIAttackOnCollide: 20).
	Cooldown int
	// Reach in blocks from feet to the target's feet; 0 uses 1.7.10's
	// sqrt((2*width)^2 + target width).
	Reach float64
	// Knockback is the weapon's knockback level (enchantment).
	Knockback int
	// Speed is the AI speed factor while chasing (0 = 1).
	Speed float64
}

// Ranged is a bow or other launcher, as 1.7.10's EntityAIArrowAttack.
type Ranged struct {
	Damage       float64 // arrow base damage (2)
	MinInterval  int     // ticks between shots at point blank (skeleton 20)
	MaxInterval  int     // ... at MaxRange (skeleton 60)
	MaxRange     float64 // (skeleton 15)
	Speed        float64 // launch speed in blocks/tick (skeleton 1.6)
	Inaccuracy   float64 // (skeleton 14 - 4*difficulty)
	MeleeInRange float64 // switch to melee closer than this (0 = never)

	// AI picks the task: RangedSkeleton (the fields above), RangedLOTR
	// (MinInterval, MaxInterval, MaxRange, MoveSpeed, Switch) or
	// RangedCNPC (MinInterval/MaxInterval as minDelay/maxDelay, MaxRange
	// as MaxFiringRange, and the fields below).
	AI int
	// MoveSpeed is LOTREntityAIRangedAttack's path speed factor toward a
	// target out of range; Switch marks a LOTR class with both a melee
	// and a ranged task that its attack mode swaps.
	MoveSpeed float64
	Switch    bool
	// CustomNPC DataStats/DataAI: FireRate and BurstCount, RangeAndMelee
	// (0 ranged only, 1 melee within DistanceToMelee, 2 both),
	// FireIndirect (0 never, 1 beyond half the range, 2 when unseen),
	// DirectLOS and TacticalVariant (EnumNavType ordinal).
	FireRate, Burst int
	UseRangeMelee   int
	DistanceToMelee float64
	Indirect        int
	DirectLOS       bool
	Tactic          int
}

// MountStats make an actor rideable, as 1.7.10's EntityHorse.
type MountStats struct {
	JumpStrength float64 // horse 0.4..1.0
	RideHeight   float64 // rider's feet above the mount's feet
}

// Actor is one living thing. The player is an actor too (Player set), so
// NPCs can follow it, fight it and carry it; its body is ticked outside.
type Actor struct {
	ID       string // stable, unique in a Manager (save key)
	Kind     string // the game's template key
	Model    any    // the renderer's handle; opaque here
	Category string // spawn category (caps), "" for placed actors

	Name, Title string // name tag lines (i18n keys or plain text: the game decides)
	ShowName    bool

	Body                 *character.Character
	Yaw, Pitch, HeadYaw  float64 // radians, as character.Input
	PrevYaw, PrevHeadYaw float64
	Player               bool // body driven by the game (input), not by a brain
	Sprinting            bool // set by the game for a sprinting player (+1 knockback)
	Persistent           bool // never despawns
	Faction              string
	Health, MaxHealth    float64
	Armor                float64 // armour points 0..20
	KnockbackResist      float64 // 0..1
	Speed                float64 // movementSpeed attribute (player 0.1, zombie 0.23)
	CanSwim, OpensDoors  bool
	FleeWhenHurt         bool // panics when hit (animals)
	FireImmune           bool // isImmuneToFire
	FireResist           int  // fireResistance: 1 (0 means 1), player 20
	// Potion effects the game keeps up to date: Resistance is the
	// resistance amplifier+1 (0 none), FireProof fireResistance.
	Resistance             int
	FireProof              bool
	AggroRange             float64
	Melee                  Melee
	Ranged                 *Ranged
	Mount                  *MountStats // non-nil: rideable
	Brain                  Brain
	Tags                   map[string]string // game data, saved
	State                  State
	StateTicks             int
	Swing                  int // ticks since the last swing started (MC swing: 6)
	HurtTime, DeathTime    int // HurtTime counts 10 → 0 after a hit (red flash)
	Target                 *Actor
	Riding, Rider          *Actor
	RideInput              RideInput // the rider's steering, set each tick for a player rider
	OnHurt                 func(a, src *Actor, amount float64)
	hurtResist             int
	fire                   int // Entity.fire: > 0 burning ticks left, < 0 contact ticks to catch
	lastDamage             float64
	fall                   float64
	attackCool, rangedCool int
	seeTime                int
	rs                     rangedState
	revenge                *Actor
	revengeTicks           int
	panic                  int
	age                    int // ticks since spawn or last fight (despawn)
	nav                    navigator
	jump                   jumpCharge
	jumpPower              float64
	routeWait              int
	look                   lookState
}

// New returns an actor with a humanoid body and 1.7.10 defaults (20 health,
// zombie-like speed 0.25, 16 blocks follow range, swims, opens doors).
func New(id, kind string, x, y, z float64) *Actor {
	return &Actor{ID: id, Kind: kind, Body: character.New(x, y, z), Health: 20, MaxHealth: 20,
		Speed: 0.25, AggroRange: 16, CanSwim: true, OpensDoors: true, Persistent: true,
		Melee: Melee{Cooldown: 20}, Swing: 100, Tags: map[string]string{}}
}

// Pos returns the feet position.
func (a *Actor) Pos() Vec { return Vec{a.Body.X, a.Body.Y, a.Body.Z} }

// Box implements projectile.Target.
func (a *Actor) Box() collide.AABB { return a.Body.Box() }

// Frame implements projectile.Target.
func (a *Actor) Frame() (x, y, z, yaw float64) { return a.Body.X, a.Body.Y, a.Body.Z, a.Yaw }

// Alive reports whether the actor has health left.
func (a *Actor) Alive() bool { return a.Health > 0 }

// Lerp interpolates position and yaw for rendering.
func (a *Actor) Lerp(alpha float64) (x, y, z, yaw float64) {
	x, y, z = a.Body.Lerp(alpha)
	d := math.Remainder(a.Yaw-a.PrevYaw, 2*math.Pi)
	return x, y, z, a.PrevYaw + d*alpha
}

// NameTagRange is how far a name tag shows (RendererLivingEntity: 64).
const NameTagRange = 64

// NameTag returns the tag's lines and where to draw them (half a block
// over the head), ok when it shows to a viewer at v.
func (a *Actor) NameTag(v Vec) (name, title string, at Vec, ok bool) {
	at = Vec{a.Body.X, a.Body.Y + a.Body.Height + 0.5, a.Body.Z}
	ok = a.ShowName && a.Name != "" && a.Alive() && a.Pos().dist2(v) < NameTagRange*NameTagRange
	return a.Name, a.Title, at, ok
}

// carrier is the body that moves for the actor: its mount while riding.
func (a *Actor) carrier() *Actor {
	if a.Riding != nil {
		return a.Riding
	}
	return a
}

func (a *Actor) set(s State) {
	if a.State != s {
		a.State, a.StateTicks = s, 0
	}
}

func yawTo(from, to Vec) float64 { return math.Atan2(to.X-from.X, -(to.Z - from.Z)) }

// turn moves yaw toward want by at most max radians.
func turn(yaw, want, max float64) float64 {
	d := math.Remainder(want-yaw, 2*math.Pi)
	return yaw + math.Max(-max, math.Min(max, d))
}

func (a *Actor) cell() nav.Pos {
	return nav.Pos{X: int(math.Floor(a.Body.X)), Y: int(math.Floor(a.Body.Y + 0.01)), Z: int(math.Floor(a.Body.Z))}
}
