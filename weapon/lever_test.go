package weapon

import (
	"slices"
	"testing"
)

// TestLever: m1866 loads into the magazine from the first loop and ends
// with ending_empty; kar98 empty loads a stripper clip, barrel included.
func TestLever(t *testing.T) {
	g := &Gun{Mag: 5, RPM: 60, Modes: []FireMode{Semi}, Bolt: ClosedBolt}
	g.Reload.Kind = Tube
	g.Reload.Tube = &TubeTiming{IntroEmpty: 0.6, Intro: 0.6, Loop: 0.53, LoopFeed: 0.2, Ending: 0.27, EndingEmpty: 1.05, Lever: true}
	s := &GunState{Gun: g, ClipLen: func(string) float32 { return 0.5 }, Phase: Ready}
	stock := 20
	run(s, 0.05, GunInput{Reload: true}, &stock)
	if run(s, 0.8, GunInput{}, &stock); s.Ammo != 1 {
		t.Fatalf("first round at 0.85 s: ammo %d", s.Ammo)
	}
	if run(s, 2.5, GunInput{}, &stock); s.Ammo != 5 || s.Phase != Reloading {
		t.Fatalf("full by 2.92 s, still ending: ammo %d phase %v", s.Ammo, s.Phase)
	}
	if run(s, 1.2, GunInput{}, &stock); s.Phase != Ready {
		t.Fatalf("ending_empty done: phase %v", s.Phase)
	}
	g.Reload.Tube = &TubeTiming{IntroEmpty: 0.6, Intro: 0.6, Loop: 0.53, LoopFeed: 0.2, Ending: 0.27, Lever: true, ChamberFirst: true, Clip: &ClipLoad{Feed: 2.78, Done: 3}}
	s = &GunState{Gun: g, ClipLen: func(string) float32 { return 0.5 }, Phase: Ready}
	run(s, 0.05, GunInput{Reload: true}, &stock)
	if run(s, 2.8, GunInput{}, &stock); s.Ammo != 6 {
		t.Fatalf("stripper clip by 2.85 s: ammo %d", s.Ammo)
	}
	if run(s, 0.3, GunInput{}, &stock); s.Phase != Ready {
		t.Fatalf("clip done at 3 s: phase %v", s.Phase)
	}
}

// TestLeverClips: m1866_state_machine's loop clip sequence.
func TestLeverClips(t *testing.T) {
	s := &GunState{ClipLen: func(string) float32 { return 0.5 }}
	s.tube.animNeed = 3
	var got []string
	for c := s.loopClip(); c != ""; c = s.loopClip() {
		got = append(got, c)
		s.tube.loops++
	}
	if want := []string{ClipReloadLoopIntro, ClipReloadLoopNext, ClipReloadLoopEnd}; !slices.Equal(got, want) {
		t.Errorf("clips %v, want %v", got, want)
	}
	s.tube.animNeed, s.tube.loops = 1, 0
	if c := s.loopClip(); c != ClipReloadLoop1Load {
		t.Errorf("one round: %s", c)
	}
}

// TestCanAim: shouldBlockAim keeps the sights up through a reload unless
// force_aim_exit_on_reload.
func TestCanAim(t *testing.T) {
	s := &GunState{Gun: &Gun{}, Phase: Reloading}
	if !s.CanAim() {
		t.Error("reloading blocks aim without force_aim_exit_on_reload")
	}
	s.Gun.AimExitOnReload = true
	if s.CanAim() {
		t.Error("force_aim_exit_on_reload: aimed while reloading")
	}
	if s.Phase = PuttingAway; s.CanAim() {
		t.Error("aimed while putting away")
	}
}
