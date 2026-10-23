package actors

import (
	"math"
	"testing"
)

// rangedPair sets up a shooter at x 0.5 and a standing target at x tx
// (flat ground, hostile factions) and records the ticks it fires.
func rangedPair(t *testing.T, r *Ranged, tx float64, sees bool) (*Manager, *Actor, *Actor, *[]int, *[]float64) {
	t.Helper()
	f := NewFactions(&Faction{Name: "orcs", Enemies: []string{"men"}}, &Faction{Name: "men"})
	m := NewManager(flat(40), f, nil, 3)
	a := New("archer", "archer", 0.5, 1, 0.5)
	a.Faction, a.Ranged = "orcs", r
	tg := New("man", "man", tx, 1, 0.5)
	tg.Faction, tg.Health, tg.MaxHealth = "men", 500, 500
	tg.Brain = Brain{Mode: Stand, Home: tg.Pos()}
	m.Add(a)
	m.Add(tg)
	a.Target = tg
	var ticks []int
	var fs []float64
	n := 0
	m.Launch = func(s, at *Actor, f float64) {
		if s != a || at != tg {
			t.Error("Launch", s.ID, at.ID)
		}
		ticks, fs = append(ticks, n), append(fs, f)
	}
	m.Sees = func(*Actor, *Actor) bool { return sees }
	for n = 1; n <= 300; n++ {
		m.Tick()
	}
	return m, a, tg, &ticks, &fs
}

// TestLOTRRangedTiming: LOTREntityAIRangedAttack with min 30, max 50,
// range 16 against a target 10 blocks away: the timer starts at -1, is
// set to floor(10/16*20+30) = 42 on the first tick and fires every 42
// ticks with power 10/16; unseen targets are not shot.
func TestLOTRRangedTiming(t *testing.T) {
	r := &Ranged{AI: RangedLOTR, MinInterval: 30, MaxInterval: 50, MaxRange: 16, MoveSpeed: 1.25}
	_, _, _, ticks, fs := rangedPair(t, r, 10.5, true)
	want := []int{43, 85, 127, 169, 211, 253, 295}
	if len(*ticks) != len(want) {
		t.Fatalf("shots at %v, want %v", *ticks, want)
	}
	for i, w := range want {
		if (*ticks)[i] != w || math.Abs((*fs)[i]-10.0/16) > 1e-6 {
			t.Errorf("shot %d at %d power %v, want %d %v", i, (*ticks)[i], (*fs)[i], w, 10.0/16)
		}
	}
	if _, _, _, ticks, _ := rangedPair(t, r, 10.5, false); len(*ticks) != 0 {
		t.Errorf("shot unseen targets at %v", *ticks)
	}
	// Closer than 5: it backs off (findPositionAwayFrom 8..16, speed 1.5).
	_, a, tg, _, _ := rangedPair(t, r, 3.5, true)
	if d := math.Sqrt(a.Pos().dist2(tg.Pos())); d < 5 {
		t.Errorf("archer stayed %.1f from the target", d)
	}
}

// TestCNPCRangedBurst: CustomNPC EntityAIRangedAttack with minDelay 20,
// maxDelay 40, FireRate 5. The timer starts at minDelay/2 = 10; the burst
// counter fires from its second step, so BurstCount 1 shoots once per
// cycle: first at tick 15, then every 10 + (40 - floor(rand*20)) ticks;
// BurstCount 2 shoots twice, 5 ticks apart.
func TestCNPCRangedBurst(t *testing.T) {
	r := &Ranged{AI: RangedCNPC, MinInterval: 20, MaxInterval: 40, MaxRange: 15, FireRate: 5, Burst: 1, DistanceToMelee: 4, DirectLOS: true}
	_, _, _, ticks, fs := rangedPair(t, r, 10.5, true)
	if len(*ticks) < 5 || (*ticks)[0] != 15 {
		t.Fatalf("shots at %v", *ticks)
	}
	for i := 1; i < len(*ticks); i++ {
		if d := (*ticks)[i] - (*ticks)[i-1]; d < 31 || d > 50 {
			t.Errorf("interval %d: %d", i, d)
		}
		if (*fs)[i] != 0 {
			t.Errorf("indirect shot %d", i)
		}
	}
	r2 := *r
	r2.Burst = 2
	_, _, _, ticks, _ = rangedPair(t, &r2, 10.5, true)
	if len(*ticks) < 4 || (*ticks)[0] != 15 || (*ticks)[1] != 20 || (*ticks)[3]-(*ticks)[2] != 5 {
		t.Errorf("burst 2 shots at %v", *ticks)
	}
	// FireIndirect 1: beyond half the range² the shot arcs (f = 1).
	r3 := *r
	r3.Indirect, r3.DirectLOS = 1, false // no walking up to it
	_, _, _, _, fs = rangedPair(t, &r3, 12.5, true)
	if len(*fs) == 0 || (*fs)[0] != 1 {
		t.Errorf("indirect: %v", *fs)
	}
}
