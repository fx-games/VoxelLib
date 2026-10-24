package village

import "testing"

// flat is a world of air with doors and a roof over x < 0.
type flat struct {
	doors map[Pos]int // facing
	vill  int
	spawn []Vec
	n     int
}

func (f *flat) Door(x, y, z int) bool       { _, ok := f.doors[Pos{x, y, z}]; return ok }
func (f *flat) DoorFacing(x, y, z int) int  { return f.doors[Pos{x, y, z}] }
func (f *flat) SeesSky(x, y, z int) bool    { return x >= 0 }
func (f *flat) IntN(n int) int              { f.n++; return 0 }
func (f *flat) Villagers(Box) int           { return f.vill }
func (f *flat) Golems(Box) int              { return 0 }
func (f *flat) SolidTop(x, y, z int) bool   { return true }
func (f *flat) NormalCube(x, y, z int) bool { return false }
func (f *flat) SpawnGolem(x, y, z float64)  { f.spawn = append(f.spawn, Vec{x, y, z}) }
func (f *flat) Locate(string) (Vec, bool)   { return Vec{}, true }
func (f *flat) Player(string) (Vec, bool)   { return Vec{}, true }

func TestVillage(t *testing.T) {
	w := &flat{doors: map[Pos]int{{0, 64, 0}: 0, {0, 64, 10}: 1, {20, 64, 20}: 0}}
	c := New(w)
	c.AddVillagerPosition(0, 64, 0)
	c.AddVillagerPosition(0, 64, 0) // a repeat
	c.Tick()
	if len(c.Villages) != 1 {
		t.Fatalf("villages %d", len(c.Villages))
	}
	v := c.Villages[0]
	// (0,64,0) faces along x: sky on +x only, inside -x; (0,64,10) faces
	// along z, sky both ways: no village door; (20,64,20) is out of range.
	if len(v.Doors) != 1 || v.Doors[0].IDX != -2 || v.Doors[0].IDZ != 0 || v.Radius != 32 || v.Center != (Pos{0, 64, 0}) {
		t.Fatalf("doors %+v radius %d centre %v", v.Doors, v.Radius, v.Center)
	}
	if c.Nearest(0, 64, 64, 32) != v || c.Nearest(0, 64, 65, 32) != nil {
		t.Fatal("findNearestVillage range")
	}
	if v.AddRep("p", -20) != -20 || !v.RepTooLow("p") || v.AddRep("p", -20) != -30 || v.AddRep("q", 30) != 10 {
		t.Fatal("reputation")
	}
	// the mating gate: 1 door * 0.35 = 0 villagers allowed
	if v.MateRoom() {
		t.Fatal("one door makes room")
	}
	s := c.Save()
	c2 := Load(w, s)
	if v2 := c2.Villages[0]; v2.Rep("p") != -30 || v2.Center != v.Center || len(v2.Doors) != 1 || c2.tick != 1 {
		t.Fatalf("reload %+v", v2)
	}
	// a door gone leaves the village annihilated on the next tick
	delete(w.doors, Pos{0, 64, 0})
	c.Tick()
	if len(c.Villages) != 0 {
		t.Fatal("doorless village kept")
	}
}

func TestGolemSpawn(t *testing.T) {
	w := &flat{doors: map[Pos]int{}, vill: 10}
	c := New(w)
	v := &Village{w: w}
	for i := range 21 {
		w.doors[Pos{i, 64, 0}] = 0
		v.AddDoor(&Door{X: i, Y: 64, Z: 0, IDX: -2})
	}
	c.Villages = append(c.Villages, v)
	for range 20 {
		c.Tick()
	}
	// tick 20 counts 10 villagers; IntN always 0: a spawn one tick later
	if len(w.spawn) != 1 || v.Golems != 1 {
		t.Fatalf("golems %v", w.spawn)
	}
	if p := w.spawn[0]; p != (Vec{float64(v.Center.X - 8), 61, -8}) {
		t.Fatalf("spawned at %v", p)
	}
}
