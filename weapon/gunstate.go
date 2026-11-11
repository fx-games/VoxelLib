package weapon

// GunPhase is what a held gun is doing.
type GunPhase uint8

const (
	Drawing GunPhase = iota
	Ready
	Reloading
	Inspecting
	Bolting // manual action cycling after a shot
	PuttingAway
	Holstered
)

// Clip names of the TACZ gun animations the cycle plays.
const (
	ClipDraw           = "draw"
	ClipIdle           = "static_idle"
	ClipShoot          = "shoot"
	ClipBolt           = "bolt"
	ClipPutAway        = "put_away"
	ClipInspect        = "inspect"
	ClipInspectEmpty   = "inspect_empty"
	ClipReloadEmpty    = "reload_empty"
	ClipReloadTactical = "reload_tactical"
	ClipReloadIntro    = "reload_intro"
	ClipReloadLoop     = "reload_loop"
	ClipReloadEnd      = "reload_end"
)

// GunEvent is something the game must show or apply this tick.
type GunEvent struct {
	Clip string // clip to play now ("" for none)
	Shot bool   // a round left the barrel
	Dry  bool   // trigger pulled on an empty gun
}

// GunInput is the player's input for one tick.
type GunInput struct {
	Trigger  bool // held
	Pressed  bool // went down this tick
	Reload   bool
	Inspect  bool
	NextMode bool
}

// GunState runs one gun's cycle. Ammo counts the rounds in the gun,
// a chambered round included. ClipLen gives clip lengths (seconds) for the
// phases whose length is the animation's (bolt, tube reload, inspect).
type GunState struct {
	Gun     *Gun
	ClipLen func(clip string) float32
	Ammo    int
	Mode    int // index into Gun.Modes
	Phase   GunPhase
	T       float32 // seconds into the phase

	cooldown  float32 // seconds until the next shot may fire
	burstLeft int
	fed       bool // reload: magazine in / tube intro over
	empty     bool // the reload started on an empty gun
	ending    bool // tube reload: reload_end playing
}

// Draw starts taking the gun out.
func (s *GunState) Draw() []GunEvent {
	s.Phase, s.T, s.burstLeft = Drawing, 0, 0
	return []GunEvent{{Clip: ClipDraw}}
}

// PutAway starts putting the gun away; Phase becomes Holstered when done.
func (s *GunState) PutAway() []GunEvent {
	s.Phase, s.T = PuttingAway, 0
	return []GunEvent{{Clip: ClipPutAway}}
}

// FireMode returns the selected fire mode.
func (s *GunState) FireMode() FireMode {
	if len(s.Gun.Modes) == 0 {
		return Semi
	}
	return s.Gun.Modes[s.Mode%len(s.Gun.Modes)]
}

// Tick advances dt seconds. stock is the player's rounds of the gun's ammo;
// reloads take from it.
func (s *GunState) Tick(dt float32, in GunInput, stock *int) []GunEvent {
	g := s.Gun
	var ev []GunEvent
	s.T += dt
	if s.cooldown > 0 {
		s.cooldown -= dt // may go below 0: the credit lets fast guns fire twice in a tick
	}
	defer func() { s.cooldown = max(s.cooldown, 0) }() // no credit kept past a tick without fire
	switch s.Phase {
	case Drawing:
		if s.T >= g.Draw {
			s.ready(&ev)
		}
	case PuttingAway:
		if s.T >= g.PutAway {
			s.Phase = Holstered
		}
	case Inspecting:
		if s.T >= s.ClipLen(ClipInspect) || in.Pressed || in.Reload {
			s.ready(&ev)
		}
	case Bolting:
		if s.T >= s.ClipLen(ClipBolt) {
			s.ready(&ev)
		}
	case Reloading:
		s.tickReload(&ev, stock)
	}
	if s.Phase != Ready {
		return ev
	}
	switch {
	case in.Reload && g.Reload.Kind != Inventory && s.Ammo < s.capacity() && *stock > 0:
		s.startReload(&ev)
		return ev
	case in.Inspect:
		s.Phase, s.T = Inspecting, 0
		clip := ClipInspect
		if s.Ammo == 0 {
			clip = ClipInspectEmpty
		}
		return append(ev, GunEvent{Clip: clip})
	case in.NextMode && len(g.Modes) > 1:
		s.Mode = (s.Mode + 1) % len(g.Modes)
		s.burstLeft = 0
	}
	if in.Pressed {
		switch s.FireMode() {
		case Burst:
			s.burstLeft = max(g.Burst.Count, 1)
		case Semi:
			s.burstLeft = 1
		}
	}
	rounds := &s.Ammo
	if g.Reload.Kind == Inventory {
		rounds = stock
	}
	for s.cooldown <= 0 && (s.FireMode() == Auto && in.Trigger || s.burstLeft > 0) {
		if *rounds == 0 {
			s.burstLeft = 0
			if in.Pressed {
				ev = append(ev, GunEvent{Dry: true})
			}
			break
		}
		*rounds--
		ev = append(ev, GunEvent{Clip: ClipShoot, Shot: true})
		interval := 60 / g.RPM
		if s.burstLeft > 0 {
			s.burstLeft--
			if s.FireMode() == Burst && s.burstLeft > 0 && g.Burst.BPM > 0 {
				interval = 60 / g.Burst.BPM
			}
		}
		s.cooldown += interval
		if g.Bolt == ManualAction && s.Ammo > 0 {
			s.Phase, s.T, s.burstLeft = Bolting, 0, 0
			ev = append(ev, GunEvent{Clip: ClipBolt})
			break
		}
	}
	return ev
}

// capacity is how many rounds a full reload holds: a closed bolt keeps one
// chambered on top of a full magazine when reloaded before running dry.
func (s *GunState) capacity() int {
	if s.Gun.Bolt == ClosedBolt && s.Gun.Reload.Kind == Magazine && (s.Phase == Reloading && !s.empty || s.Phase != Reloading && s.Ammo > 0) {
		return s.Gun.Mag + 1
	}
	return s.Gun.Mag
}

func (s *GunState) ready(ev *[]GunEvent) {
	s.Phase, s.T = Ready, 0
	*ev = append(*ev, GunEvent{Clip: ClipIdle})
}

func (s *GunState) startReload(ev *[]GunEvent) {
	s.Phase, s.T, s.fed, s.empty, s.ending, s.burstLeft = Reloading, 0, false, s.Ammo == 0, false, 0
	clip := ClipReloadTactical
	switch {
	case s.Gun.Reload.Kind == Tube:
		clip = ClipReloadIntro
	case s.empty:
		clip = ClipReloadEmpty
	}
	*ev = append(*ev, GunEvent{Clip: clip})
}

// tickReload: a magazine goes in at the feed time; a tube reload runs
// reload_intro, then reload_loop once per round, then reload_end.
func (s *GunState) tickReload(ev *[]GunEvent, stock *int) {
	g := s.Gun
	if g.Reload.Kind == Magazine {
		feed, done := g.Reload.FeedTactical, g.Reload.DoneTactical
		if s.empty {
			feed, done = g.Reload.FeedEmpty, g.Reload.DoneEmpty
		}
		if !s.fed && s.T >= feed {
			s.fed = true
			s.take(s.capacity()-s.Ammo, stock)
		}
		if s.T >= done {
			s.ready(ev)
		}
		return
	}
	switch intro, loop := s.ClipLen(ClipReloadIntro), s.ClipLen(ClipReloadLoop); {
	case s.ending:
		if s.T >= s.ClipLen(ClipReloadEnd) {
			s.ready(ev)
		}
	case !s.fed:
		if s.T >= intro {
			s.fed, s.T = true, s.T-intro
			*ev = append(*ev, GunEvent{Clip: ClipReloadLoop})
		}
	case s.T >= loop:
		s.T -= loop
		s.take(1, stock)
		if s.Ammo >= g.Mag || *stock == 0 {
			s.ending, s.T = true, 0
			*ev = append(*ev, GunEvent{Clip: ClipReloadEnd})
		} else {
			*ev = append(*ev, GunEvent{Clip: ClipReloadLoop})
		}
	}
}

// take moves up to n rounds from stock into the gun.
func (s *GunState) take(n int, stock *int) {
	n = max(min(n, *stock), 0)
	*stock -= n
	s.Ammo += n
}

// DamageAt is the damage of one pellet hitting at dist blocks.
func (g *Gun) DamageAt(dist float32) float32 {
	for _, f := range g.Bullet.Falloff {
		if f.Dist == 0 || dist <= f.Dist {
			return f.Damage
		}
	}
	return g.Bullet.Damage
}

// Spread is the cone half-angle (degrees) of a shot.
func (g *Gun) Spread(aiming, moving, sneaking bool) float32 {
	switch {
	case aiming:
		return g.Inaccuracy.Aim
	case moving:
		return g.Inaccuracy.Move
	case sneaking:
		return g.Inaccuracy.Sneak
	}
	return g.Inaccuracy.Stand
}
