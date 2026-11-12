package weapon

import "testing"

const tick = 1.0 / 20

func run(s *GunState, secs float32, in GunInput, stock *int) (shots int) {
	for range int(secs*20 + 0.5) {
		for _, e := range s.Tick(tick, in, stock) {
			if e.Shot {
				shots++
			}
		}
		in.Reload = false
	}
	return shots
}

func TestGunCycle(t *testing.T) {
	g := &Gun{Mag: 30, RPM: 600, Modes: []FireMode{Auto, Semi}, Draw: 0.35}
	g.Reload.FeedEmpty, g.Reload.DoneEmpty, g.Reload.FeedTactical, g.Reload.DoneTactical = 2.25, 2.6, 1.55, 2
	lens := func(string) float32 { return 0.5 }
	s := &GunState{Gun: g, ClipLen: lens, Ammo: 30}
	stock := 100
	s.Draw()
	if n := run(s, 0.3, GunInput{Trigger: true}, &stock); n != 0 {
		t.Fatalf("fired %d while drawing", n)
	}
	// a trigger held through the draw fires once ready (ShootKey.autoShoot)
	if n := run(s, 0.1, GunInput{Trigger: true}, &stock); n != 1 {
		t.Fatalf("held through draw: %d shots", n)
	}
	run(s, 0.1, GunInput{}, &stock)
	s.Ammo = 30
	if n := run(s, 1, GunInput{Trigger: true}, &stock); n != 10 {
		t.Fatalf("600 rpm for 1 s: %d shots", n)
	}
	// Tactical reload on a closed bolt: a full magazine plus the chambered round.
	run(s, 2.1, GunInput{Reload: true}, &stock)
	if s.Ammo != 31 || stock != 89 || s.Phase != Ready {
		t.Fatalf("tactical: ammo %d stock %d phase %d", s.Ammo, stock, s.Phase)
	}
	// Semi: one shot per press.
	s.NextMode()
	if n := run(s, 1, GunInput{Trigger: true}, &stock); n != 1 {
		t.Fatalf("semi: %d shots", n)
	}

	// Tube (m870_gun_logic, the m870's timings): an empty gun chambers a
	// round, then fills the tube: mag + 1.
	sg := &Gun{Mag: 5, RPM: 60, Modes: []FireMode{Semi}, Bolt: ManualAction}
	sg.Reload.Kind = Tube
	sg.Reload.Tube = &TubeTiming{IntroEmpty: 2.13, Intro: 0.37, Loop: 0.67, Ending: 0.17, IntroEmptyFeed: 1.35, LoopFeed: 0.25}
	s = &GunState{Gun: sg, ClipLen: lens, Phase: Ready, Ammo: 1}
	stock = 10
	if n := run(s, 0.1, GunInput{Trigger: true}, &stock); n != 1 || s.Phase != Ready && s.Ammo != 0 {
		t.Fatalf("pump shot: %d, phase %d", n, s.Phase)
	}
	run(s, 0.1, GunInput{Reload: true}, &stock)
	if run(s, 5.3, GunInput{}, &stock); s.Phase != Reloading || s.Ammo != 6 {
		t.Fatalf("tube at 5.4 s: ammo %d phase %d", s.Ammo, s.Phase)
	}
	if run(s, 0.4, GunInput{}, &stock); s.Phase != Ready || stock != 4 {
		t.Fatalf("tube done: ammo %d stock %d phase %d", s.Ammo, stock, s.Phase)
	}
	// tactical: the tube fills to 5 besides the chambered round; a shot
	// interrupts it, ending runs, then the held trigger fires
	s.Ammo = 3
	run(s, 0.05, GunInput{Reload: true}, &stock)
	run(s, 0.37+0.25+0.05, GunInput{}, &stock) // one round in
	if s.Ammo != 4 {
		t.Fatalf("tactical first round: %d", s.Ammo)
	}
	if n := run(s, 0.1, GunInput{Trigger: true}, &stock); n != 0 || s.Phase != Reloading {
		t.Fatalf("interrupted: %d shots, phase %d", n, s.Phase)
	}
	if n := run(s, 0.2, GunInput{Trigger: true}, &stock); n != 1 || s.Ammo != 3 {
		t.Fatalf("after ending: %d shots, ammo %d", n, s.Ammo)
	}

	// Inventory-fed: shots come from the stock.
	mg := &Gun{RPM: 1200, Modes: []FireMode{Auto}}
	mg.Reload.Kind = Inventory
	s = &GunState{Gun: mg, ClipLen: lens, Phase: Ready}
	stock = 7
	if n := run(s, 1, GunInput{Trigger: true}, &stock); n != 7 || stock != 0 {
		t.Fatalf("inventory: %d shots, stock %d", n, stock)
	}
}

func TestHeat(t *testing.T) {
	g := &Gun{RPM: 600, Modes: []FireMode{Auto},
		Heat: &Heat{Max: 10, PerShot: 2, Cooling: 10, Delay: 0.5, Lock: 1, RPM: [2]float32{1, 2}, Inaccuracy: [2]float32{1, 1.5}}}
	s := &GunState{Gun: g, Ammo: 100, Phase: Ready}
	stock := 0
	shots := 0
	for i := 0; i < 100 && !s.Overheated; i++ {
		for _, e := range s.Tick(0.05, GunInput{Trigger: true}, &stock) {
			if e.Shot {
				shots++
			}
		}
	}
	if !s.Overheated || shots != 5 || s.SpreadFactor() != 1.5 {
		t.Fatalf("overheated %v after %d shots, spread ×%v", s.Overheated, shots, s.SpreadFactor())
	}
	if ev := s.Tick(0.05, GunInput{Trigger: true}, &stock); len(ev) != 1 || !ev[0].Dry {
		t.Fatalf("overheated trigger: %+v, want a dry click", ev)
	}
	for range 20 { // locked until 1 s after the last shot, then −10·t/10 per tick: 1.0 + 1.05
		s.Tick(0.05, GunInput{}, &stock)
	}
	if !s.Overheated || s.Heat > 7.96 || s.Heat < 7.94 {
		t.Fatalf("heat %v locked %v", s.Heat, s.Overheated)
	}
	for range 10 {
		s.Tick(0.05, GunInput{}, &stock)
	}
	if s.Overheated || s.Heat != 0 {
		t.Fatalf("heat %v locked %v after cooling", s.Heat, s.Overheated)
	}
}

// TestActionSounds: each action starts its display sound, a tube reload
// choosing reload_empty/reload_tactical by the barrel as a magazine does.
func TestActionSounds(t *testing.T) {
	lens := func(string) float32 { return 0.5 }
	sounds := func(ev []GunEvent) (out []GunSound) {
		for _, e := range ev {
			if e.Sound != "" {
				out = append(out, e.Sound)
			}
		}
		return out
	}
	stock := 10
	for _, kind := range []ReloadKind{Magazine, Tube} {
		g := &Gun{Mag: 5, RPM: 60, Modes: []FireMode{Semi, Auto}}
		g.Reload.Kind = kind
		s := &GunState{Gun: g, ClipLen: lens}
		if got := sounds(s.Draw()); len(got) != 1 || got[0] != SoundDraw {
			t.Fatalf("draw: %v", got)
		}
		s.Phase = Ready
		if got := sounds(s.Tick(tick, GunInput{Reload: true}, &stock)); len(got) != 1 || got[0] != SoundReloadEmpty {
			t.Fatalf("%v reload on empty: %v", kind, got)
		}
		s.Phase, s.Ammo = Ready, 1
		if got := sounds(s.Tick(tick, GunInput{Reload: true}, &stock)); len(got) != 1 || got[0] != SoundReloadTactical {
			t.Fatalf("%v reload: %v", kind, got)
		}
		s.Phase = Ready
		if got := sounds(s.Tick(tick, GunInput{Inspect: true}, &stock)); len(got) != 1 || got[0] != SoundInspect {
			t.Fatalf("inspect: %v", got)
		}
		s.Phase = Ready
		if got := sounds(s.NextMode()); len(got) != 1 || got[0] != SoundFireSelect {
			t.Fatalf("fire select: %v", got)
		}
	}
}

func TestMeleeAndUnload(t *testing.T) {
	lens := func(string) float32 { return 0.3 }
	g := &Gun{Mag: 5, RPM: 600, Modes: []FireMode{Semi}, Unload: true,
		Melee: &Melee{Kind: MeleeBayonet, Cooldown: 0.7, Prep: 0.1}}
	s := &GunState{Gun: g, ClipLen: lens, Phase: Ready, Ammo: 5}
	stock := 0
	var clips []string
	hits := 0
	for i := range 40 { // 2 s: three melees at 0, 0.7, 1.4 s
		in := GunInput{Melee: true}
		for _, e := range s.Tick(tick, in, &stock) {
			if e.Melee {
				hits++
				if s.T < g.Melee.Prep {
					t.Errorf("tick %d: hit before prep", i)
				}
			}
			if e.Clip != "" && e.Clip != ClipIdle {
				clips = append(clips, e.Clip)
			}
		}
	}
	if hits != 3 || len(clips) != 3 || clips[0] != "melee_bayonet_1" || clips[2] != "melee_bayonet_3" {
		t.Fatalf("hits %d clips %v", hits, clips)
	}
	s.Phase, s.T = Inspecting, 0 // unload breaks out of inspecting
	ev := s.Tick(tick, GunInput{Unload: true}, &stock)
	if s.Ammo != 0 || stock != 5 || ev[len(ev)-1].Sound != SoundUnload || ev[len(ev)-1].Unloaded != 5 {
		t.Fatalf("unload: ammo %d stock %d ev %+v", s.Ammo, stock, ev)
	}
	if ev := s.Tick(tick, GunInput{Unload: true}, &stock); len(ev) != 0 {
		t.Fatalf("empty gun unloaded: %+v", ev)
	}
}

func TestMeleeReaches(t *testing.T) {
	m := &Melee{Distance: 2, Angle: 40}
	eye, look := [3]float64{0, 0, 0}, [3]float64{0, 0, -1}
	for _, c := range []struct {
		at   [3]float64
		want bool
	}{{[3]float64{0, 0, -1}, true}, {[3]float64{0, 0, 1}, false}, {[3]float64{3, 0, -1}, false}, {[3]float64{0.3, 0, -1.5}, true}} {
		if got := m.Reaches(eye, look, c.at); got != c.want {
			t.Errorf("%v: %v", c.at, got)
		}
	}
}

// Burst: count rounds at bpm, the next burst min_interval after the first
// shot; non-continuous waits for the trigger's release.
func TestBurst(t *testing.T) {
	g := &Gun{Mag: 30, RPM: 600, Modes: []FireMode{Burst}}
	g.Burst.Count, g.Burst.BPM, g.Burst.MinInterval = 3, 1200, 0.5
	s := &GunState{Gun: g, Ammo: 30, Phase: Ready}
	stock := 0
	if n := run(s, 2, GunInput{Trigger: true}, &stock); n != 3 {
		t.Fatalf("held: %d shots, want one burst", n)
	}
	g.Burst.Continuous = true
	run(s, 0.05, GunInput{}, &stock)
	if n := run(s, 1, GunInput{Trigger: true}, &stock); n != 6 {
		t.Fatalf("continuous 1 s at 0.5 s min interval: %d shots", n)
	}
}

// taurus943_gun_logic: the cylinder empties back into the stock, then
// refills a round per loop.
func TestRevolverReload(t *testing.T) {
	g := &Gun{Mag: 8, RPM: 300, Modes: []FireMode{Semi}, Bolt: OpenBolt}
	g.Reload.Kind = Tube
	g.Reload.Tube = &TubeTiming{Intro: 1.45, Loop: 0.45, LoopFeed: 0.1, Ending: 0.75, Unload: &TubeUnload{Time: 0.8, Amount: 8}}
	s := &GunState{Gun: g, ClipLen: func(string) float32 { return 0.5 }, Phase: Ready, Ammo: 3}
	stock := 10
	run(s, 0.05, GunInput{Reload: true}, &stock)
	if run(s, 0.8, GunInput{}, &stock); s.Ammo != 0 || stock != 13 {
		t.Fatalf("unloaded: ammo %d stock %d", s.Ammo, stock)
	}
	if run(s, 1.45+8*0.45+0.75-0.8, GunInput{}, &stock); s.Ammo != 8 || stock != 5 || s.Phase != Ready {
		t.Fatalf("refilled: ammo %d stock %d phase %d", s.Ammo, stock, s.Phase)
	}
}

// defaultTickBolt: the bolt starts once the shot's cooldown is over and
// lasts bolt_action_time; the hip time applies under half aimed.
func TestBoltTiming(t *testing.T) {
	g := &Gun{Mag: 5, RPM: 60, Modes: []FireMode{Semi}, Bolt: ManualAction, BoltTime: 0.85, BoltHipTime: 0.2}
	s := &GunState{Gun: g, ClipLen: func(string) float32 { return 5 }, Phase: Ready, Ammo: 5}
	stock := 0
	run(s, 0.05, GunInput{Trigger: true, Aimed: true}, &stock)
	if run(s, 0.95+0.8, GunInput{Aimed: true}, &stock); s.Phase != Bolting {
		t.Fatalf("bolt over before cooldown + bolt_time: phase %d", s.Phase)
	}
	if run(s, 0.1, GunInput{Aimed: true}, &stock); s.Phase != Ready {
		t.Fatalf("bolt not over at 1.9 s: phase %d", s.Phase)
	}
	run(s, 0.05, GunInput{Trigger: true}, &stock)
	if run(s, 0.95+0.25, GunInput{}, &stock); s.Phase != Ready {
		t.Fatalf("hip bolt not over: phase %d", s.Phase)
	}
}

// LivingEntityShoot rejects a shot until sprintTimeS has come down to 0;
// a sprint attempt interrupts a tube reload.
func TestSprint(t *testing.T) {
	g := &Gun{Mag: 5, RPM: 600, Modes: []FireMode{Semi}, Sprint: 0.2}
	s := &GunState{Gun: g, ClipLen: func(string) float32 { return 0.5 }, Phase: Ready, Ammo: 5}
	stock := 0
	run(s, 1, GunInput{Sprinting: true}, &stock)
	if n := run(s, 0.15, GunInput{Trigger: true}, &stock); n != 0 {
		t.Fatalf("fired %d coming out of a sprint", n)
	}
	if n := run(s, 0.1, GunInput{Trigger: true}, &stock); n != 1 {
		t.Fatalf("fired %d after sprint_time", n)
	}
	if s.StopsSprint() {
		t.Fatal("ready gun stops sprint")
	}
}

// db_short_gun_logic: a burst shot fires both barrels at once.
func TestVolley(t *testing.T) {
	g := &Gun{Mag: 2, RPM: 150, Modes: []FireMode{Burst}}
	g.Burst.Count, g.Burst.Volley = 1, 2
	s := &GunState{Gun: g, ClipLen: func(string) float32 { return 0.5 }, Phase: Ready, Ammo: 2}
	stock := 0
	ev := s.Tick(0.05, GunInput{Trigger: true}, &stock)
	if len(ev) != 1 || ev[0].Rounds != 2 || s.Ammo != 0 {
		t.Fatalf("volley %+v, ammo %d", ev, s.Ammo)
	}
}
