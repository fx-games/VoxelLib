package village

import (
	"math"
	"slices"
	"sort"
)

// Collection is VillageCollection: a world's villages, the villager
// positions waiting to be searched for doors and the doors found.
type Collection struct {
	w         World
	positions []Pos
	newDoors  []*Door
	Villages  []*Village
	tick      int
}

// New is a world's empty collection.
func New(w World) *Collection { return &Collection{w: w} }

// AddVillagerPosition is addVillagerPosition: at most 65 queued, no
// repeats.
func (c *Collection) AddVillagerPosition(x, y, z int) {
	if len(c.positions) <= 64 && !slices.Contains(c.positions, Pos{x, y, z}) {
		c.positions = append(c.positions, Pos{x, y, z})
	}
}

// Tick is VillageCollection.tick: the villages tick, the doorless go,
// the oldest villager position is searched for doors, and the new doors
// join the first village within 32 of its radius or found one each.
func (c *Collection) Tick() {
	c.tick++
	for _, v := range c.Villages {
		v.Tick(c.tick)
	}
	c.Villages = slices.DeleteFunc(c.Villages, (*Village).Annihilated)
	if len(c.positions) > 0 {
		p := c.positions[0]
		c.positions = c.positions[1:]
		c.findDoors(p)
	}
	for _, d := range c.newDoors {
		added := false
		for _, v := range c.Villages {
			j := int(v.Center.Dist2(d.X, d.Y, d.Z))
			k := 32 + float32(v.Radius)
			if float32(j) > k*k {
				continue
			}
			v.AddDoor(d)
			added = true
			break
		}
		if !added {
			v := &Village{w: c.w}
			v.AddDoor(d)
			c.Villages = append(c.Villages, v)
		}
	}
	c.newDoors = c.newDoors[:0]
}

// Nearest is findNearestVillage: the nearest village whose centre is
// within r of its radius, nil for none.
func (c *Collection) Nearest(x, y, z, r int) *Village {
	var best *Village
	f := float32(math.MaxFloat32)
	for _, v := range c.Villages {
		if f1 := v.Center.Dist2(x, y, z); f1 < f {
			if f2 := float32(r + v.Radius); f1 <= f2*f2 {
				best, f = v, f1
			}
		}
	}
	return best
}

// findDoors is addUnassignedWoodenDoorsAroundToNewDoorsList: the doors
// in x and z -16..15 and y -4..3 around p; a known one is seen again.
func (c *Collection) findDoors(p Pos) {
	for i := p.X - 16; i < p.X+16; i++ {
		for j := p.Y - 4; j < p.Y+4; j++ {
			for k := p.Z - 16; k < p.Z+16; k++ {
				if !c.w.Door(i, j, k) {
					continue
				}
				if d := c.doorAt(i, j, k); d != nil {
					d.TS = c.tick
				} else {
					c.newDoor(i, j, k)
				}
			}
		}
	}
}

func (c *Collection) doorAt(x, y, z int) *Door {
	for _, d := range c.newDoors {
		if d.X == x && d.Z == z && abs(d.Y-y) <= 1 {
			return d
		}
	}
	for _, v := range c.Villages {
		if d := v.DoorAt(x, y, z); d != nil {
			return d
		}
	}
	return nil
}

// newDoor is addDoorToNewListIfAppropriate: the sky seen over 5 blocks
// on either side across the door; the less open side is the inside, and
// a door as open on both sides is no village door.
func (c *Collection) newDoor(x, y, z int) {
	l := c.w.DoorFacing(x, y, z)
	n := 0
	for j := -5; j <= 5; j++ {
		if j == 0 {
			continue
		}
		var sky bool
		if l != 0 && l != 2 {
			sky = c.w.SeesSky(x, y, z+j)
		} else {
			sky = c.w.SeesSky(x+j, y, z)
		}
		if sky && j < 0 {
			n--
		} else if sky {
			n++
		}
	}
	if n == 0 {
		return
	}
	in := 2
	if n > 0 {
		in = -2
	}
	d := &Door{X: x, Y: y, Z: z, IDX: in, TS: c.tick}
	if l != 0 && l != 2 {
		d.IDX, d.IDZ = 0, in
	}
	c.newDoors = append(c.newDoors, d)
}

// MateRoom is EntityAIVillagerMate.checkSufficientDoorsPresentForNewVillager:
// the mating season, and fewer villagers than 35% of the doors.
func (v *Village) MateRoom() bool {
	return v.MatingSeason() && v.Villagers < int(float64(float32(len(v.Doors)))*0.35)
}

// Saved is the collection's villages.dat: writeToNBT's fields.
type Saved struct {
	Tick     int
	Villages []SavedVillage
}

// SavedVillage is writeVillageDataToNBT.
type SavedVillage struct {
	PopSize, Radius, Golems, Stable, Tick, MTick int
	CX, CY, CZ, ACX, ACY, ACZ                    int
	Doors                                        []SavedDoor
	Players                                      []SavedRep
}

type SavedDoor struct{ X, Y, Z, IDX, IDZ, TS int }

type SavedRep struct {
	Name string
	S    int
}

// Save is writeToNBT (the reputations in TreeMap order).
func (c *Collection) Save() Saved {
	s := Saved{Tick: c.tick}
	for _, v := range c.Villages {
		sv := SavedVillage{PopSize: v.Villagers, Radius: v.Radius, Golems: v.Golems, Stable: v.lastAdd, Tick: v.tick,
			MTick: v.noBreed, CX: v.Center.X, CY: v.Center.Y, CZ: v.Center.Z, ACX: v.helper.X, ACY: v.helper.Y, ACZ: v.helper.Z}
		for _, d := range v.Doors {
			sv.Doors = append(sv.Doors, SavedDoor{d.X, d.Y, d.Z, d.IDX, d.IDZ, d.TS})
		}
		for _, n := range sortedKeys(v.rep) {
			sv.Players = append(sv.Players, SavedRep{n, v.rep[n]})
		}
		s.Villages = append(s.Villages, sv)
	}
	return s
}

// Load is readFromNBT into a new collection of the world.
func Load(w World, s Saved) *Collection {
	c := &Collection{w: w, tick: s.Tick}
	for _, sv := range s.Villages {
		v := &Village{w: w, Villagers: sv.PopSize, Radius: sv.Radius, Golems: sv.Golems, lastAdd: sv.Stable, tick: sv.Tick,
			noBreed: sv.MTick, Center: Pos{sv.CX, sv.CY, sv.CZ}, helper: Pos{sv.ACX, sv.ACY, sv.ACZ}}
		for _, d := range sv.Doors {
			v.Doors = append(v.Doors, &Door{X: d.X, Y: d.Y, Z: d.Z, IDX: d.IDX, IDZ: d.IDZ, TS: d.TS})
		}
		for _, r := range sv.Players {
			if v.rep == nil {
				v.rep = map[string]int{}
			}
			v.rep[r.Name] = r.S
		}
		c.Villages = append(c.Villages, v)
	}
	return c
}

func sortedKeys(m map[string]int) []string {
	l := make([]string, 0, len(m))
	for k := range m {
		l = append(l, k)
	}
	sort.Strings(l)
	return l
}
