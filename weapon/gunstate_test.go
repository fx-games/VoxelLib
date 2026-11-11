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
		in.Pressed, in.Reload = false, false
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
	if n := run(s, 0.3, GunInput{Trigger: true, Pressed: true}, &stock); n != 0 {
		t.Fatalf("fired %d while drawing", n)
	}
	run(s, 0.1, GunInput{}, &stock)
	if n := run(s, 1, GunInput{Trigger: true, Pressed: true}, &stock); n != 10 {
		t.Fatalf("600 rpm for 1 s: %d shots", n)
	}
	// Tactical reload on a closed bolt: a full magazine plus the chambered round.
	run(s, 2.1, GunInput{Reload: true}, &stock)
	if s.Ammo != 31 || stock != 89 || s.Phase != Ready {
		t.Fatalf("tactical: ammo %d stock %d phase %d", s.Ammo, stock, s.Phase)
	}
	// Semi: one shot per press.
	s.Tick(tick, GunInput{NextMode: true}, &stock)
	if n := run(s, 1, GunInput{Trigger: true, Pressed: true}, &stock); n != 1 {
		t.Fatalf("semi: %d shots", n)
	}

	// Tube: one round per reload_loop, then reload_end.
	sg := &Gun{Mag: 5, RPM: 60, Modes: []FireMode{Semi}, Bolt: ManualAction}
	sg.Reload.Kind = Tube
	s = &GunState{Gun: sg, ClipLen: lens, Phase: Ready, Ammo: 1}
	stock = 10
	if n := run(s, 0.1, GunInput{Trigger: true, Pressed: true}, &stock); n != 1 || s.Phase != Ready && s.Ammo != 0 {
		t.Fatalf("pump shot: %d, phase %d", n, s.Phase)
	}
	run(s, 0.5+5*0.5+0.5+0.2, GunInput{Reload: true}, &stock)
	if s.Ammo != 5 || stock != 5 || s.Phase != Ready {
		t.Fatalf("tube: ammo %d stock %d phase %d", s.Ammo, stock, s.Phase)
	}

	// Inventory-fed: shots come from the stock.
	mg := &Gun{RPM: 1200, Modes: []FireMode{Auto}}
	mg.Reload.Kind = Inventory
	s = &GunState{Gun: mg, ClipLen: lens, Phase: Ready}
	stock = 7
	if n := run(s, 1, GunInput{Trigger: true, Pressed: true}, &stock); n != 7 || stock != 0 {
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
	for range 21 { // locked until 1 s after the last shot, then −10·t/10 per tick: 1.0 + 1.05
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
