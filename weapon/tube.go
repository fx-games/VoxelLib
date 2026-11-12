package weapon

// tubeReload is a tube reload in progress: the rounds as m870_gun_logic
// loads them, the clips as m870_unload_state_machine plays them.
type tubeReload struct {
	needed   int     // the tube's free rounds at the start (getNeededAmmoAmount)
	reloaded int     // rounds loaded, the chambered one included (cache.reloaded_count)
	stopAt   float32 // seconds into the reload it was ended or interrupted; <0: not yet
	animEnd  float32 // when the playing intro or loop clip ends
	loops    int     // reload_loop clips played (loaded_ammo)
	animDone bool    // reload_end played
	unloaded bool    // revolver: the cylinder emptied
}

// startTube starts a tube reload, returning its first clip: reload_intro,
// or reload_intro_empty with no round chambered.
func (s *GunState) startTube() string {
	inTube := max(s.Ammo-1, 0) // getCurrentAmmoCount: the barrel's round apart
	s.tube = tubeReload{needed: s.Gun.Mag - inTube, stopAt: -1}
	if tm := s.Gun.Reload.Tube; tm != nil && tm.Unload != nil {
		s.tube.needed = s.Gun.Mag // getMaxAmmoCount until unloaded
	}
	clip := ClipReloadIntro
	if s.empty && s.ClipLen(ClipReloadIntroEmpty) > 0 {
		clip = ClipReloadIntroEmpty
	}
	s.tube.animEnd = s.ClipLen(clip)
	return clip
}

// interruptTube is interrupt_reload and the state machine's
// INPUT_CANCEL_RELOAD: Ending runs from now, reload_end plays.
func (s *GunState) interruptTube(ev *[]GunEvent) {
	if s.tube.stopAt < 0 || s.tube.stopAt > s.T {
		s.tube.stopAt = s.T
	}
	s.endTubeClip(ev)
}

func (s *GunState) endTubeClip(ev *[]GunEvent) {
	if !s.tube.animDone {
		s.tube.animDone = true
		*ev = append(*ev, GunEvent{Clip: ClipReloadEnd})
	}
}

// tickTube is m870_gun_logic.tick_reload, with reload_loop clips played
// back to back while rounds are needed (reload_state.update).
func (s *GunState) tickTube(ev *[]GunEvent, stock *int) {
	tm, r := s.Gun.Reload.Tube, &s.tube
	if tm == nil { // no timings: nothing loads (tick_reload returns NOT_RELOADING)
		s.Phase, s.T = Ready, 0
		s.endTubeClip(ev)
		return
	}
	if !r.animDone && s.T >= r.animEnd {
		if r.loops > r.needed || *stock == 0 {
			s.endTubeClip(ev)
		} else {
			r.loops++
			r.animEnd += s.ClipLen(ClipReloadLoop)
			*ev = append(*ev, GunEvent{Clip: ClipReloadLoop})
		}
	}
	if r.stopAt >= 0 {
		if s.T-r.stopAt >= tm.Ending {
			// ponytail: the state machine ends its clips on its own count;
			// here reload_end plays by the end at the latest
			s.endTubeClip(ev)
			s.Phase, s.T = Ready, 0
		}
		return
	}
	if tm.Unload != nil {
		s.tickRevolver(tm, stock)
		return
	}
	if *stock == 0 { // !hasAmmoToConsume
		r.stopAt = s.T
	}
	introEmpty, introEmptyFeed := tm.IntroEmpty, tm.IntroEmptyFeed
	if tm.IntroEmptyBurst > 0 && s.FireMode() == Burst {
		introEmpty, introEmptyFeed = tm.IntroEmptyBurst, tm.IntroEmptyFeedBurst
	}
	if r.reloaded == 0 {
		if !s.empty {
			r.reloaded = 1
		} else if s.T > introEmptyFeed {
			s.take(1, stock) // into the barrel
			r.reloaded = 1
		}
	}
	if r.reloaded > 0 {
		intro := tm.Intro
		if s.empty {
			intro = introEmpty
		}
		for at := intro + tm.nextFeed(r.reloaded, r.needed); at < s.T && r.reloaded <= r.needed; {
			if tm.Loop2 > 0 && r.needed-r.reloaded >= 1 { // a pair
				r.reloaded += 2
				at += tm.Loop2
				s.take(2, stock)
				continue
			}
			r.reloaded++
			at += tm.Loop
			s.take(1, stock)
		}
	}
	if r.reloaded > r.needed {
		r.stopAt = s.T - tm.LoopFeed + tm.Loop
	}
}

// tickRevolver is taurus943_gun_logic.tick_reload: unload at Unload.Time,
// then a round every Loop.
func (s *GunState) tickRevolver(tm *TubeTiming, stock *int) {
	r := &s.tube
	if !r.unloaded {
		if s.T < tm.Unload.Time {
			return
		}
		n := s.Ammo
		if tm.Unload.Amount != UnloadAll {
			n = min(n, tm.Unload.Amount)
		}
		s.Ammo -= n
		*stock += n // unloadAmmo: back to the player
		r.unloaded = true
		r.needed = s.Gun.Mag - s.Ammo // getNeededAmmoAmount
		if r.needed <= 0 {
			r.stopAt = s.T
			return
		}
	}
	for *stock > 0 && r.reloaded < r.needed && s.T >= tm.Intro+tm.LoopFeed+float32(r.reloaded)*tm.Loop {
		r.reloaded++
		s.take(1, stock)
	}
	var finish float32
	switch {
	case r.reloaded >= r.needed:
		finish = tm.Intro + float32(r.needed)*tm.Loop
	case *stock == 0:
		finish = tm.Intro + float32(r.reloaded)*tm.Loop
	default:
		return
	}
	r.stopAt = max(s.T, finish)
}

// UnloadAll is TubeUnload.Amount for the whole cylinder.
const UnloadAll = -1

// nextFeed is the time after the intro the next round goes in, reloaded
// counting the chambered one (m870: a round per Loop; m1014: pairs per
// Loop2 while two or more are needed).
func (tm *TubeTiming) nextFeed(reloaded, needed int) float32 {
	switch {
	case tm.Loop2 <= 0:
		return float32(reloaded-1)*tm.Loop + tm.LoopFeed
	case needed == 1:
		return tm.LoopFeed
	case needed < 1:
		return 0
	case reloaded%2 == 0:
		return float32((reloaded-2)/2)*tm.Loop2 + tm.LoopFeed2
	}
	return float32((reloaded-1)/2)*tm.Loop2 + tm.LoopFeed
}
