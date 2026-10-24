// Package village is the 1.7.10 village mechanics: a Collection of
// villages per world (VillageCollection), each a set of tracked doors
// with a centre and radius (Village, VillageDoorInfo), the players'
// reputation, the aggressors, the mating season, the golem spawn, and the
// saved form (villages.dat). The game answers the world questions
// through World.
package village

import "math"

// World is what the villages ask of the world.
type World interface {
	Door(x, y, z int) bool      // the village door block (isWoodenDoorAt, isBlockDoor)
	DoorFacing(x, y, z int) int // BlockDoor.func_150013_e: both halves' meta, & 3
	SeesSky(x, y, z int) bool   // canBlockSeeTheSky
	IntN(n int) int             // the world's rand.nextInt
	// Villagers and Golems count the villagers and golems whose boxes
	// meet the box (getEntitiesWithinAABB).
	Villagers(b Box) int
	Golems(b Box) int
	SolidTop(x, y, z int) bool   // World.doesBlockHaveSolidTopSurface
	NormalCube(x, y, z int) bool // Block.isNormalCube
	SpawnGolem(x, y, z float64)
	// Locate is an actor's position and isEntityAlive, Player a player's
	// by name (getPlayerEntityByName).
	Locate(id string) (p Vec, alive bool)
	Player(name string) (p Vec, ok bool)
}

// Box is an AABB, min then max.
type Box [6]float64

// Vec is an entity position.
type Vec struct{ X, Y, Z float64 }

func (a Vec) dist2(b Vec) float64 {
	dx, dy, dz := a.X-b.X, a.Y-b.Y, a.Z-b.Z
	return dx*dx + dy*dy + dz*dz
}

// Pos is a block position (ChunkCoordinates).
type Pos struct{ X, Y, Z int }

// Dist2 is ChunkCoordinates.getDistanceSquared, in float32.
func (p Pos) Dist2(x, y, z int) float32 {
	f, f1, f2 := float32(p.X-x), float32(p.Y-y), float32(p.Z-z)
	return f*f + f1*f1 + f2*f2
}

// Door is VillageDoorInfo: the lower door block, the inside direction
// (-2, 0 or 2 on one axis) and the tick it was last seen.
type Door struct {
	X, Y, Z    int
	IDX, IDZ   int
	TS         int
	Detached   bool // isDetachedFromVillageFlag
	restricted int  // doorOpeningRestrictionCounter
}

// Dist2 is getDistanceSquared.
func (d *Door) Dist2(x, y, z int) int {
	l, m, n := x-d.X, y-d.Y, z-d.Z
	return l*l + m*m + n*n
}

// InsideDist2 is getInsideDistanceSquare.
func (d *Door) InsideDist2(x, y, z int) int {
	l, m, n := x-d.X-d.IDX, y-d.Y, z-d.Z-d.IDZ
	return l*l + m*m + n*n
}

// Inside is getInsidePosX/Y/Z.
func (d *Door) Inside() Pos { return Pos{d.X + d.IDX, d.Y, d.Z + d.IDZ} }

// IsInside is isInside: x, z on the inside half.
func (d *Door) IsInside(x, z int) bool { return (x-d.X)*d.IDX+(z-d.Z)*d.IDZ >= 0 }

// Restrict is incrementDoorOpeningRestrictionCounter; Restricted reads it.
func (d *Door) Restrict()       { d.restricted++ }
func (d *Door) Restricted() int { return d.restricted }

type aggressor struct {
	id   string
	time int
}

// Village is one village.
type Village struct {
	w          World
	Doors      []*Door
	helper     Pos // centerHelper: the doors' summed positions
	Center     Pos
	Radius     int
	lastAdd    int // lastAddDoorTimestamp
	tick       int
	Villagers  int // numVillagers
	noBreed    int // noBreedTicks
	Golems     int // numIronGolems
	rep        map[string]int
	aggressors []aggressor
}

// Tick is Village.tick: drop the doors gone or unseen for 1200 ticks and
// the aggressors dead or 300 ticks old, recount the villagers every 20
// ticks and the golems every 30, and with more than 20 doors and fewer
// golems than a tenth of the villagers, one in 7000 ticks try to spawn a
// golem near the centre.
func (v *Village) Tick(t int) {
	v.tick = t
	v.dropDoors()
	v.dropAggressors()
	if t%20 == 0 {
		v.Villagers = v.w.Villagers(v.box())
		if v.Villagers == 0 {
			clear(v.rep)
		}
	}
	if t%30 == 0 {
		v.Golems = v.w.Golems(v.box())
	}
	if v.Golems < v.Villagers/10 && len(v.Doors) > 20 && v.w.IntN(7000) == 0 {
		if p, ok := v.golemSpot(v.Center.X, v.Center.Y, v.Center.Z, 2, 4, 2); ok {
			v.w.SpawnGolem(float64(p.X), float64(p.Y), float64(p.Z))
			v.Golems++
		}
	}
}

func (v *Village) box() Box {
	c, r := v.Center, float64(v.Radius)
	return Box{float64(c.X) - r, float64(c.Y - 4), float64(c.Z) - r, float64(c.X) + r, float64(c.Y + 4), float64(c.Z) + r}
}

// golemSpot is tryGetIronGolemSpawningLocation: ten tries within 8 of x,
// 3 of y, 8 of z, in range, on a solid top, the w*h*d room free of
// normal cubes.
func (v *Village) golemSpot(x, y, z, w, h, d int) (Pos, bool) {
	for range 10 {
		i := x + v.w.IntN(16) - 8
		j := y + v.w.IntN(6) - 3
		k := z + v.w.IntN(16) - 8
		if v.InRange(i, j, k) && v.golemFits(i, j, k, w, h, d) {
			return Pos{i, j, k}, true
		}
	}
	return Pos{}, false
}

func (v *Village) golemFits(x, y, z, w, h, d int) bool {
	if !v.w.SolidTop(x, y-1, z) {
		return false
	}
	x0, z0 := x-w/2, z-d/2
	for i := x0; i < x0+w; i++ {
		for j := y; j < y+h; j++ {
			for k := z0; k < z0+d; k++ {
				if v.w.NormalCube(i, j, k) {
					return false
				}
			}
		}
	}
	return true
}

// SinceDoorAdded is getTicksSinceLastDoorAdding.
func (v *Village) SinceDoorAdded() int { return v.tick - v.lastAdd }

// InRange is isInRange: strictly within the radius of the centre.
func (v *Village) InRange(x, y, z int) bool {
	return v.Center.Dist2(x, y, z) < float32(v.Radius*v.Radius)
}

// NearestDoor is findNearestDoor.
func (v *Village) NearestDoor(x, y, z int) *Door {
	var best *Door
	l := math.MaxInt32
	for _, d := range v.Doors {
		if i := d.Dist2(x, y, z); i < l {
			best, l = d, i
		}
	}
	return best
}

// NearestDoorUnrestricted is findNearestDoorUnrestricted: among the doors
// within 16 the least restricted, else the nearest.
func (v *Village) NearestDoorUnrestricted(x, y, z int) *Door {
	var best *Door
	l := math.MaxInt32
	for _, d := range v.Doors {
		i := d.Dist2(x, y, z)
		if i > 256 {
			i *= 1000
		} else {
			i = d.restricted
		}
		if i < l {
			best, l = d, i
		}
	}
	return best
}

// DoorAt is getVillageDoorAt: a door at x, z within a block of y, if the
// point is in the radius.
func (v *Village) DoorAt(x, y, z int) *Door {
	if v.Center.Dist2(x, y, z) > float32(v.Radius*v.Radius) {
		return nil
	}
	for _, d := range v.Doors {
		if d.X == x && d.Z == z && abs(d.Y-y) <= 1 {
			return d
		}
	}
	return nil
}

// AddDoor is addVillageDoorInfo.
func (v *Village) AddDoor(d *Door) {
	v.Doors = append(v.Doors, d)
	v.helper.X += d.X
	v.helper.Y += d.Y
	v.helper.Z += d.Z
	v.update()
	v.lastAdd = d.TS
}

// Annihilated is isAnnihilated: no door left.
func (v *Village) Annihilated() bool { return len(v.Doors) == 0 }

// AddAggressor is addOrRenewAgressor.
func (v *Village) AddAggressor(id string) {
	for i := range v.aggressors {
		if v.aggressors[i].id == id {
			v.aggressors[i].time = v.tick
			return
		}
	}
	v.aggressors = append(v.aggressors, aggressor{id, v.tick})
}

// NearestAggressor is findNearestVillageAggressor: the last of the
// nearest ("" for none).
func (v *Village) NearestAggressor(from Vec) string {
	best, bd := "", math.MaxFloat64
	for _, a := range v.aggressors {
		p, _ := v.w.Locate(a.id)
		if d := p.dist2(from); d <= bd {
			best, bd = a.id, d
		}
	}
	return best
}

// NearestBadPlayer is func_82685_c: the nearest player in this world
// whose reputation is too low ("" for none), the names in TreeMap order.
func (v *Village) NearestBadPlayer(from Vec) string {
	best, bd := "", math.MaxFloat64
	for _, n := range sortedKeys(v.rep) {
		if !v.RepTooLow(n) {
			continue
		}
		if p, ok := v.w.Player(n); ok {
			if d := p.dist2(from); d <= bd {
				best, bd = n, d
			}
		}
	}
	return best
}

func (v *Village) dropAggressors() {
	l := v.aggressors[:0]
	for _, a := range v.aggressors {
		if _, alive := v.w.Locate(a.id); alive && abs(v.tick-a.time) <= 300 {
			l = append(l, a)
		}
	}
	v.aggressors = l
}

func (v *Village) dropDoors() {
	gone := false
	reset := v.w.IntN(50) == 0
	l := v.Doors[:0]
	for _, d := range v.Doors {
		if reset {
			d.restricted = 0
		}
		if !v.w.Door(d.X, d.Y, d.Z) || abs(v.tick-d.TS) > 1200 {
			v.helper.X -= d.X
			v.helper.Y -= d.Y
			v.helper.Z -= d.Z
			gone, d.Detached = true, true
			continue
		}
		l = append(l, d)
	}
	clear(v.Doors[len(l):])
	v.Doors = l
	if gone {
		v.update()
	}
}

// update is updateVillageRadiusAndCenter: the doors' mean (Java's
// integer division) and at least 32 or one past the farthest door.
func (v *Village) update() {
	n := len(v.Doors)
	if n == 0 {
		v.Center, v.Radius = Pos{}, 0
		return
	}
	v.Center = Pos{v.helper.X / n, v.helper.Y / n, v.helper.Z / n}
	j := 0
	for _, d := range v.Doors {
		j = max(d.Dist2(v.Center.X, v.Center.Y, v.Center.Z), j)
	}
	v.Radius = max(32, int(math.Sqrt(float64(j)))+1)
}

// Rep is getReputationForPlayer.
func (v *Village) Rep(name string) int { return v.rep[name] }

// AddRep is setReputationForPlayer: the reputation moves by d, held to
// -30..10.
func (v *Village) AddRep(name string, d int) int {
	k := min(max(v.rep[name]+d, -30), 10)
	if v.rep == nil {
		v.rep = map[string]int{}
	}
	v.rep[name] = k
	return k
}

// RepTooLow is isPlayerReputationTooLow: -15 or less.
func (v *Village) RepTooLow(name string) bool { return v.Rep(name) <= -15 }

// EndMatingSeason is endMatingSeason.
func (v *Village) EndMatingSeason() { v.noBreed = v.tick }

// MatingSeason is isMatingSeason: never ended, or 3600 ticks since.
func (v *Village) MatingSeason() bool { return v.noBreed == 0 || v.tick-v.noBreed >= 3600 }

// AddRepAll is setDefaultPlayerReputation: every known player's moves by d.
func (v *Village) AddRepAll(d int) {
	for _, n := range sortedKeys(v.rep) {
		v.AddRep(n, d)
	}
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}
