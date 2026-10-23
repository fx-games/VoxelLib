package actors

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/nav"
)

// testWorld: solid cells, doors (closed doors are solid), water.
type testWorld struct {
	solid map[nav.Pos]bool
	door  map[nav.Pos]bool // value: open
	water map[nav.Pos]bool
	opens int
}

func flat(n int) *testWorld {
	w := &testWorld{solid: map[nav.Pos]bool{}, door: map[nav.Pos]bool{}, water: map[nav.Pos]bool{}}
	for x := -n; x < n; x++ {
		for z := -n; z < n; z++ {
			w.solid[nav.Pos{X: x, Y: 0, Z: z}] = true
		}
	}
	return w
}

func (w *testWorld) Solid(x, y, z int) bool {
	p := nav.Pos{X: x, Y: y, Z: z}
	if open, ok := w.door[p]; ok {
		return !open
	}
	return w.solid[p]
}

func (w *testWorld) Liquid(x, y, z int) character.Liquid {
	if w.water[nav.Pos{X: x, Y: y, Z: z}] {
		return character.LiquidWater
	}
	return character.LiquidNone
}

func (w *testWorld) Door(x, y, z int) (bool, bool) {
	open, ok := w.door[nav.Pos{X: x, Y: y, Z: z}]
	return ok, open
}

func (w *testWorld) SetDoor(x, y, z int, open bool) {
	for _, dy := range []int{-1, 0, 1} { // both halves
		p := nav.Pos{X: x, Y: y + dy, Z: z}
		if _, ok := w.door[p]; ok {
			w.door[p] = open
		}
	}
	if open {
		w.opens++
	}
}

// wall across x=5 (z -20..20), 3 high, with a door at z=0.
func walled() *testWorld {
	w := flat(20)
	for z := -20; z < 20; z++ {
		for y := 1; y <= 3; y++ {
			w.solid[nav.Pos{X: 5, Y: y, Z: z}] = true
		}
	}
	delete(w.solid, nav.Pos{X: 5, Y: 1, Z: 0})
	delete(w.solid, nav.Pos{X: 5, Y: 2, Z: 0})
	w.door[nav.Pos{X: 5, Y: 1, Z: 0}] = false
	w.door[nav.Pos{X: 5, Y: 2, Z: 0}] = false
	return w
}

func TestRouteThroughDoor(t *testing.T) {
	w := walled()
	m := NewManager(w, nil, nil, 1)
	a := New("walker", "npc", 1.5, 1, -6.5)
	a.Brain = Brain{Mode: Route, Points: []Vec{{1.5, 1, -2.5}, {9.5, 1, 4.5}}}
	m.Add(a)
	for i := 0; i < 600 && !a.Brain.Done; i++ {
		m.Tick()
	}
	if !a.Brain.Done || a.Pos().hdist(Vec{9.5, 1, 4.5}) > 0.6 {
		t.Fatalf("did not arrive: at %+v done=%v", a.Pos(), a.Brain.Done)
	}
	if w.opens == 0 {
		t.Fatal("never opened the door")
	}
	for i := 0; i < 40; i++ {
		m.Tick()
	}
	if w.door[nav.Pos{X: 5, Y: 1, Z: 0}] {
		t.Fatal("left the door open")
	}
}

func TestDoorlessActorStaysOut(t *testing.T) {
	w := walled()
	m := NewManager(w, nil, nil, 1)
	a := New("cow", "animal", 1.5, 1, 0.5)
	a.OpensDoors = false
	a.Brain = Brain{Mode: Route, Points: []Vec{{9.5, 1, 0.5}}}
	m.Add(a)
	for i := 0; i < 300; i++ {
		m.Tick()
	}
	if a.Pos().X > 5 || w.opens > 0 {
		t.Fatalf("an actor that can't open doors got through: %+v opens=%d", a.Pos(), w.opens)
	}
}

func TestRoutePatterns(t *testing.T) {
	w := flat(20)
	pts := []Vec{{0.5, 1, 0.5}, {6.5, 1, 0.5}, {6.5, 1, 6.5}}
	for _, pat := range []RoutePattern{Loop, BackForth} {
		m := NewManager(w, nil, nil, 1)
		a := New("w", "npc", 0.5, 1, 0.5)
		a.Brain = Brain{Mode: Route, Points: pts, Pattern: pat, Pause: 5}
		m.Add(a)
		var seen []int
		for i := 0; i < 1500 && len(seen) < 6; i++ {
			m.Tick()
			if n := a.Brain.Next; len(seen) == 0 || seen[len(seen)-1] != n {
				seen = append(seen, n)
			}
		}
		want := map[RoutePattern][]int{Loop: {1, 2, 0, 1, 2, 0}, BackForth: {1, 2, 1, 0, 1, 2}}[pat]
		for i := range want {
			if i >= len(seen) || seen[i] != want[i] {
				t.Fatalf("pattern %d: points %v, want %v", pat, seen, want)
			}
		}
	}
}

// A leader walks 30 blocks; twelve followers with formation slots follow,
// spread out and don't overlap.
func TestFollowerFormation(t *testing.T) {
	w := flat(60)
	m := NewManager(w, nil, nil, 1)
	p := New("player", "player", 0.5, 1, 0.5)
	p.Player = true
	m.Add(p)
	var fs []*Actor
	for i := 0; i < 12; i++ {
		f := New(string(rune('a'+i)), "dwarf", 0.5+float64(i%4)*1.2, 1, 4.5+float64(i/4)*1.2)
		f.Brain = Brain{Mode: Follow, Leader: "player", Offset: FormationSlot(i, 1.6), StopDist: 0.8}
		m.Add(f)
		fs = append(fs, f)
	}
	p.Yaw = math.Pi / 2 // +X
	for i := 0; i < 400; i++ {
		in := character.Input{Yaw: p.Yaw}
		if p.Body.X < 30 {
			in.Forward = 1
		}
		p.Body.Tick(w, in)
		m.Tick()
	}
	for i, a := range fs {
		if d := a.Pos().hdist(p.Pos()); d > 9 {
			t.Errorf("follower %d is %.1f blocks from the leader", i, d)
		}
		for _, b := range fs[i+1:] {
			if d := a.Pos().hdist(b.Pos()); d < 0.5 {
				t.Errorf("followers %s and %s overlap (%.2f apart)", a.ID, b.ID, d)
			}
		}
	}
}

func TestMeleeTimingAndHurtWindow(t *testing.T) {
	w := flat(10)
	f := NewFactions(&Faction{Name: "orcs", Enemies: []string{"men"}}, &Faction{Name: "men"})
	m := NewManager(w, f, nil, 1)
	orc := New("orc", "orc", 0.5, 1, 0.5)
	orc.Faction, orc.Melee = "orcs", Melee{Damage: 3, Cooldown: 20}
	man := New("man", "man", 1.5, 1, 0.5)
	man.Faction, man.Health, man.MaxHealth, man.KnockbackResist = "men", 1000, 1000, 1
	man.Brain = Brain{Mode: Stand, Home: Vec{1.5, 1, 0.5}}
	m.Add(orc)
	m.Add(man)
	hits := 0
	man.OnHurt = func(a, src *Actor, n float64) { hits++ }
	for i := 0; i < 210; i++ {
		m.Tick()
	}
	// target pick on the first tick divisible by 10 for this ID, then a blow every 20 ticks
	if hits < 9 || hits > 11 {
		t.Fatalf("%d hits in 210 ticks, want about 10 (one per 20)", hits)
	}
	if man.Health != 1000-3*float64(hits) {
		t.Fatalf("health %.1f after %d hits of 3", man.Health, hits)
	}

	// The invulnerability window: within 10 ticks only a stronger blow
	// counts, by the difference.
	v := New("v", "x", 5, 1, 5)
	m.Add(v)
	if !m.Hurt(v, nil, 4, true) || m.Hurt(v, nil, 4, true) || !m.Hurt(v, nil, 6, true) {
		t.Fatal("hurt window: want hit, block equal, allow stronger")
	}
	if v.Health != 20-6 || v.HurtTime != MaxHurtTime {
		t.Fatalf("health %.1f hurt %d, want 14 and %d", v.Health, v.HurtTime, MaxHurtTime)
	}
	for i := 0; i < 11; i++ {
		m.Tick()
	}
	if !m.Hurt(v, nil, 1, true) {
		t.Fatal("a blow after the window must count")
	}

	// Armour 20 takes 80%; knockback launches 0.4 up.
	k := New("k", "x", -5, 1, -5)
	k.Armor = 20
	m.Add(k)
	m.Hurt(k, orc, 10, false)
	if math.Abs(k.Health-18) > 1e-9 || math.Abs(k.Body.VY-0.4) > 1e-9 {
		t.Fatalf("armour/knockback: health %.2f vy %.3f, want 18 and 0.4", k.Health, k.Body.VY)
	}
}

func TestDeathRemovesAfterAnimation(t *testing.T) {
	m := NewManager(flat(5), nil, nil, 1)
	a := New("a", "x", 0.5, 1, 0.5)
	m.Add(a)
	var killed, removed bool
	m.OnDeath = func(d, k *Actor) { killed = true }
	m.OnRemove = func(*Actor) { removed = true }
	m.Hurt(a, nil, 50, true)
	for i := 0; i < DeathTicks-1; i++ {
		m.Tick()
	}
	if !killed || removed {
		t.Fatalf("killed %v removed %v before %d ticks", killed, removed, DeathTicks)
	}
	m.Tick()
	if !removed || m.Get("a") != nil {
		t.Fatal("not removed after the death animation")
	}
}

func TestHorseJumpCharge(t *testing.T) {
	var j jumpCharge
	got := 0.0
	for i := 0; i < 6; i++ { // press, then hold 5 ticks: power 0.5
		got = j.update(true)
	}
	got = j.update(false)
	if want := 0.4 + 0.4*50/90.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("jump power %.4f, want %.4f", got, want)
	}
	for i := 0; i < 12; i++ { // cooldown, then a full charge
		j.update(false)
	}
	for i := 0; i < 11; i++ {
		got = j.update(true)
	}
	if got = j.update(false); got != 1 {
		t.Fatalf("full charge gives %.2f, want 1", got)
	}
}

func TestRidingSteersMount(t *testing.T) {
	w := flat(40)
	m := NewManager(w, nil, nil, 1)
	p := New("player", "player", 0.5, 1, 0.5)
	p.Player = true
	pony := New("pony", "pony", 0.5, 1, 0.5)
	pony.Body.Width, pony.Body.Height = 1.4, 1.6
	pony.Speed, pony.Mount = 0.3, &MountStats{JumpStrength: 0.7, RideHeight: 1}
	m.Add(p)
	m.Add(pony)
	if !m.Mount(p, pony) {
		t.Fatal("mount failed")
	}
	p.Yaw = math.Pi / 2
	p.RideInput = RideInput{Forward: 1}
	for i := 0; i < 40; i++ {
		m.Tick()
	}
	if pony.Body.X < 15 || math.Abs(p.Body.X-pony.Body.X) > 1e-9 || math.Abs(p.Body.Y-pony.Body.Y-1) > 1e-9 {
		t.Fatalf("pony at %.1f, rider at %+v", pony.Body.X, p.Pos())
	}
	// 0.3 speed: terminal ground speed 0.3*0.98/(1-0.546) ≈ 0.65 blocks/tick
	if v := pony.Body.X - pony.Body.PrevX; math.Abs(v-0.3*0.98/(1-0.546)) > 0.02 {
		t.Fatalf("pony speed %.3f", v)
	}
	m.Dismount(p)
	if p.Riding != nil || pony.Rider != nil || p.Pos().hdist(pony.Pos()) < 0.9 {
		t.Fatal("dismount left the rider on the pony")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	w := flat(10)
	f := NewFactions(&Faction{Name: "men", Default: 5})
	m := NewManager(w, f, nil, 1)
	p := New("player", "player", 0.5, 1, 0.5)
	p.Player = true
	m.Add(p)
	a := New("gandalf", "wizard", 2.5, 1, 3.5)
	a.Name, a.ShowName, a.Faction, a.Health = "npc.gandalf", true, "men", 13
	a.Brain = Brain{Mode: Route, Points: []Vec{{1, 1, 1}, {4, 1, 4}}, Next: 1, Lead: "player"}
	a.Tags["dialog"] = "d42"
	pony := New("pony", "pony", 2.5, 1, 3.5)
	pony.Mount = &MountStats{JumpStrength: 0.5, RideHeight: 1}
	m.Add(a)
	m.Add(pony)
	m.Mount(a, pony)
	f.Add("men", 20)
	data, err := json.Marshal(m.Save())
	if err != nil {
		t.Fatal(err)
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	m2 := NewManager(w, NewFactions(&Faction{Name: "men", Default: 5}), nil, 1)
	m2.Add(p)
	m2.Load(s, func(id, kind string) *Actor {
		n := New(id, kind, 0, 0, 0)
		if kind == "pony" {
			n.Mount = &MountStats{JumpStrength: 0.5, RideHeight: 1}
		}
		return n
	})
	b := m2.Get("gandalf")
	if b == nil || b.Health != 13 || b.Name != "npc.gandalf" || !b.ShowName || b.Brain.Next != 1 ||
		b.Brain.Lead != "player" || len(b.Brain.Points) != 2 || b.Tags["dialog"] != "d42" || b.Riding != m2.Get("pony") {
		t.Fatalf("round trip lost state: %+v", b)
	}
	if m2.Factions.Points("men") != 25 {
		t.Fatalf("standing %d, want 25", m2.Factions.Points("men"))
	}
}

func TestFactionStances(t *testing.T) {
	f := NewFactions(&Faction{Name: "elves", FriendlyAt: 1500, NeutralAt: 500, Default: 1000, Enemies: []string{"orcs"}},
		&Faction{Name: "orcs"})
	if f.Relation("orcs", "elves") != Hostile || f.Relation("elves", "elves") != Friendly {
		t.Fatal("relations")
	}
	if f.PlayerStance("elves") != Neutral {
		t.Fatal("default standing should be neutral")
	}
	f.Add("elves", 600)
	if f.PlayerStance("elves") != Friendly {
		t.Fatal("1600 should be friendly")
	}
	f.Add("elves", -1200)
	if f.PlayerStance("elves") != Hostile {
		t.Fatal("400 should be hostile")
	}
}

func TestArcherShootsWithInterval(t *testing.T) {
	w := flat(30)
	f := NewFactions(&Faction{Name: "undead", Enemies: []string{"men"}}, &Faction{Name: "men"})
	arrows := &projectile.World{}
	m := NewManager(w, f, arrows, 3)
	sk := New("sk", "skeleton", 0.5, 1, 0.5)
	sk.Faction = "undead"
	sk.Ranged = &Ranged{Damage: 2, MinInterval: 20, MaxInterval: 60, MaxRange: 15, Speed: 1.6}
	man := New("man", "man", 10.5, 1, 0.5)
	man.Faction, man.Health, man.MaxHealth = "men", 500, 500
	man.Brain = Brain{Mode: Stand, Home: man.Pos()}
	m.Add(sk)
	m.Add(man)
	hits := 0
	man.OnHurt = func(a, src *Actor, n float64) {
		if src != sk || n < 1 || n > 6 {
			t.Errorf("hit by %v for %.1f", src, n)
		}
		hits++
	}
	for i := 0; i < 400; i++ {
		var ts []projectile.Target
		for _, a := range m.Actors {
			ts = append(ts, a)
		}
		m.Tick()
		arrows.Tick(w, ts)
	}
	// The archer closes in for the first second (seeTime < 20), then
	// shoots every floor(d/15*40+20) ticks: about 40 at 7.5 blocks.
	if hits < 7 || hits > 10 {
		t.Fatalf("%d arrow hits in 400 ticks, want about 9", hits)
	}
}

type spawnWorld struct {
	*testWorld
	light int
}

func (spawnWorld) Biome(x, z int) string   { return "shire" }
func (s spawnWorld) Light(x, y, z int) int { return s.light }
func (spawnWorld) Surface(x, z int) (int, bool) {
	return 1, x >= -60 && x < 60 && z >= -60 && z < 60
}

func TestSpawnerCapsLightAndDespawn(t *testing.T) {
	w := flat(60)
	sw := spawnWorld{w, 4}
	m := NewManager(w, nil, nil, 1)
	sp := &Spawner{W: sw, Caps: map[string]int{"monster": 6}, Radius: 50,
		Rules: []SpawnRule{{Kind: "orc", Category: "monster", Weight: 1, Min: 1, Max: 3, MaxLight: 7},
			{Kind: "rabbit", Category: "monster", Weight: 5, Min: 1, Max: 1, MinLight: 9}},
		Make: func(r *SpawnRule, id string, x, y, z float64) *Actor { return New(id, r.Kind, x, y, z) }}
	for i := 0; i < 200; i++ {
		sp.Tick(m, Vec{})
	}
	n := 0
	for _, a := range m.Actors {
		n++
		if a.Kind != "orc" || a.Pos().hdist(Vec{}) < 24 {
			t.Fatalf("bad spawn %s at %+v", a.Kind, a.Pos())
		}
	}
	if n < 6 || n > 8 { // a group may overshoot the cap by its size - 1
		t.Fatalf("%d spawned, cap 6", n)
	}
	sp.Tick(m, Vec{X: 1000}) // the player left: all far beyond Radius
	if len(m.Actors) != 0 {
		t.Fatalf("%d left after the player went away", len(m.Actors))
	}
	sp.Visible = func(x, y, z float64) bool { return true } // everything in view: nothing spawns
	for i := 0; i < 50; i++ {
		sp.Tick(m, Vec{})
	}
	if len(m.Actors) != 0 {
		t.Fatal("spawned in view")
	}
}

func TestNPCRidesRoute(t *testing.T) {
	w := flat(40)
	m := NewManager(w, nil, nil, 1)
	d := New("dwarf", "dwarf", 0.5, 1, 0.5)
	d.Brain = Brain{Mode: Route, Points: []Vec{{20.5, 1, 10.5}}}
	pony := New("pony", "pony", 0.5, 1, 0.5)
	pony.Body.Width, pony.Body.Height = 1.4, 1.6
	pony.Speed, pony.Mount = 0.3, &MountStats{JumpStrength: 0.7, RideHeight: 1}
	m.Add(d)
	m.Add(pony)
	m.Mount(d, pony)
	for i := 0; i < 300 && !d.Brain.Done; i++ {
		m.Tick()
	}
	if !d.Brain.Done || pony.Pos().hdist(Vec{20.5, 1, 10.5}) > 0.7 || d.Riding != pony || d.State != Ride {
		t.Fatalf("pony at %+v, done %v", pony.Pos(), d.Brain.Done)
	}
}

// pocket: flat ground with a 3-high U-shaped wall around the start, open
// toward -x, too big for one 6000-node search to leave (70 x 120 cells).
func pocket() *testWorld {
	w := flat(90)
	for y := 1; y <= 3; y++ {
		for z := -60; z <= 60; z++ {
			w.solid[nav.Pos{X: 10, Y: y, Z: z}] = true
		}
		for x := -60; x <= 10; x++ {
			w.solid[nav.Pos{X: x, Y: y, Z: -60}] = true
			w.solid[nav.Pos{X: x, Y: y, Z: 60}] = true
		}
	}
	return w
}

// TestStuckBehindWallDetours: the walker's partial plans end at the wall;
// after sticking there it takes the long search out of the pocket.
func TestStuckBehindWallDetours(t *testing.T) {
	m := NewManager(pocket(), nil, nil, 1)
	a := New("walker", "npc", 0.5, 1, 0.5)
	goal := Vec{40.5, 1, 0.5}
	a.Brain = Brain{Mode: Route, Points: []Vec{goal}}
	m.Add(a)
	for i := 0; i < 6000 && !a.Brain.Done; i++ {
		m.Tick()
	}
	if !a.Brain.Done {
		t.Fatalf("stuck at %+v (tries %d)", a.Pos(), a.nav.tries)
	}
	if a.nav.tries == 0 {
		t.Error("arrived without a detour: the pocket no longer tests one")
	}
}

// TestBigWalkerTakesWideGap: a troll-sized walker (1.6 wide -> 2x2 cells)
// takes the far 2-wide opening, not the near 1-wide one.
func TestBigWalkerTakesWideGap(t *testing.T) {
	w := flat(30)
	for z := -30; z < 30; z++ {
		if z != 0 && z != 10 && z != 11 {
			for y := 1; y <= 5; y++ {
				w.solid[nav.Pos{X: 5, Y: y, Z: z}] = true
			}
		}
	}
	m := NewManager(w, nil, nil, 1)
	a := New("troll", "troll", 1.5, 1, 0.5)
	a.Body.Width, a.Body.Height = 1.6, 3.2
	goal := Vec{10, 1, 1}
	a.Brain = Brain{Mode: Route, Points: []Vec{goal}}
	m.Add(a)
	for i := 0; i < 1200 && !a.Brain.Done; i++ {
		m.Tick()
	}
	if !a.Brain.Done {
		t.Fatalf("troll stuck at %+v", a.Pos())
	}
}
