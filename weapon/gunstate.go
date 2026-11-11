package weapon

import "strconv"

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
	Meleeing // a melee attack: the hit lands at Melee.Prep
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
	ClipMeleePush      = "melee_push"
	ClipMeleeStock     = "melee_stock"
	ClipMeleeBayonet   = "melee_bayonet_" // + 1..meleeBayonetClips in turn
)

// meleeBayonetClips is how many bayonet clips the state machine cycles through.
const meleeBayonetClips = 3

// GunEvent is something the game must show or apply this tick.
type GunEvent struct {
	Clip string // clip to play now ("" for none)
	Shot bool   // a round left the barrel
	Dry  bool   // trigger pulled on an empty gun
	// Melee: the melee attack's hit lands now (Gun.Melee).
	Melee bool
	// Unloaded is the rounds unloading took out (they went back to the stock).
	Unloaded int
	// Sound is the display sound an action starts (draw, put away, reload,
	// inspect, bolt, fire select; "" for none). Shots and Dry have their own.
	Sound GunSound
}

// GunInput is the player's input for one tick.
type GunInput struct {
	Trigger  bool // held
	Pressed  bool // went down this tick
	Reload   bool
	Inspect  bool
	NextMode bool
	Melee    bool // went down this tick
	Unload   bool // went down this tick
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
	// Heat is the gun's heat (Gun.Heat), Overheated its lock.
	Heat       float32
	Overheated bool

	cooldown  float32 // seconds until the next shot may fire
	burstLeft int
	fed       bool    // reload: magazine in / tube intro over
	empty     bool    // the reload started on an empty gun
	ending    bool    // tube reload: reload_end playing
	sinceShot float32 // seconds since the last shot
	interval  float32 // the last shot's interval (0: none fired yet)
	meleeHit  bool    // the melee's hit has landed
	meleeLen  float32 // seconds the melee phase lasts
	bayonet   int     // bayonet clips played (the next is bayonet%meleeBayonetClips+1)
}

// firingGrace is how late a shot may come and still continue a stream of
// fire (the minigun script's 100 ms against game stutter).
const firingGrace = 0.1

// Firing reports whether the gun is firing continuously: the next shot is
// due (minigun_state_machine spin_states.fire).
func (s *GunState) Firing() bool {
	return s.interval > 0 && s.sinceShot <= s.interval+firingGrace
}

// CoolingDown reports whether the next shot must still wait (TACZ's
// getShootCoolDown() > 0).
func (s *GunState) CoolingDown() bool { return s.cooldown > 0 }

// Draw starts taking the gun out.
func (s *GunState) Draw() []GunEvent {
	s.Phase, s.T, s.burstLeft = Drawing, 0, 0
	return []GunEvent{{Clip: ClipDraw, Sound: SoundDraw}}
}

// PutAway starts putting the gun away; Phase becomes Holstered when done.
func (s *GunState) PutAway() []GunEvent {
	s.Phase, s.T = PuttingAway, 0
	return []GunEvent{{Clip: ClipPutAway, Sound: SoundPutAway}}
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
	s.sinceShot += dt
	s.cool(dt)
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
	case Meleeing:
		if !s.meleeHit && s.T >= g.Melee.Prep {
			s.meleeHit = true
			ev = append(ev, GunEvent{Melee: true})
		}
		if s.meleeHit && s.T >= s.meleeLen {
			s.ready(&ev)
		}
	}
	// Melee and unload break out of inspecting (INPUT_INSPECT_BREAK_OUT).
	if s.Phase == Inspecting && (in.Melee && g.Melee != nil || in.Unload && s.canUnload()) {
		s.ready(&ev)
	}
	if s.Phase != Ready {
		return ev
	}
	switch {
	case in.Melee && g.Melee != nil:
		return append(ev, s.melee())
	case in.Unload && s.canUnload():
		n := s.Ammo
		*stock += n
		s.Ammo, s.burstLeft = 0, 0
		return append(ev, GunEvent{Sound: SoundUnload, Unloaded: n})
	case in.Reload && g.Reload.Kind != Inventory && s.Ammo < s.capacity() && *stock > 0:
		s.startReload(&ev)
		return ev
	case in.Inspect:
		s.Phase, s.T = Inspecting, 0
		clip, sound := ClipInspect, SoundInspect
		if s.Ammo == 0 {
			clip, sound = ClipInspectEmpty, SoundInspectEmpty
		}
		return append(ev, GunEvent{Clip: clip, Sound: sound})
	case in.NextMode && len(g.Modes) > 1:
		s.Mode = (s.Mode + 1) % len(g.Modes)
		s.burstLeft = 0
		ev = append(ev, GunEvent{Sound: SoundFireSelect})
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
		if s.Overheated {
			s.burstLeft = 0
			break
		}
		if *rounds == 0 {
			s.burstLeft = 0
			if in.Pressed {
				ev = append(ev, GunEvent{Dry: true})
			}
			break
		}
		*rounds--
		ev = append(ev, GunEvent{Clip: ClipShoot, Shot: true})
		interval := 60 / (g.RPM * s.heatFactor(func(h *Heat) [2]float32 { return h.RPM }))
		s.heat()
		if s.burstLeft > 0 {
			s.burstLeft--
			if s.FireMode() == Burst && s.burstLeft > 0 && g.Burst.BPM > 0 {
				interval = 60 / g.Burst.BPM
			}
		}
		s.cooldown += interval
		s.sinceShot, s.interval = 0, interval
		if g.Bolt == ManualAction && s.Ammo > 0 {
			s.Phase, s.T, s.burstLeft = Bolting, 0, 0
			ev = append(ev, GunEvent{Clip: ClipBolt, Sound: SoundBolt})
			break
		}
	}
	return ev
}

// melee starts the melee attack: its clip and sound by kind
// (LocalPlayerMelee), the bayonet's three clips in turn. It lasts the
// cooldown, or the clip if longer (the idle pose would cut it).
func (s *GunState) melee() GunEvent {
	m := s.Gun.Melee
	s.Phase, s.T, s.meleeHit, s.burstLeft = Meleeing, 0, false, 0
	e := GunEvent{Clip: ClipMeleePush, Sound: SoundMeleePush}
	switch m.Kind {
	case MeleeStock:
		e = GunEvent{Clip: ClipMeleeStock, Sound: SoundMeleeStock}
	case MeleeBayonet:
		e = GunEvent{Clip: ClipMeleeBayonet + strconv.Itoa(s.bayonet%meleeBayonetClips+1), Sound: SoundMeleeBayonet}
		s.bayonet++
	}
	s.meleeLen = max(m.Cooldown, m.Prep, s.ClipLen(e.Clip))
	return e
}

// canUnload: the gun unloads (Gun.Unload) and holds a round.
func (s *GunState) canUnload() bool { return s.Gun.Unload && s.Ammo > 0 }

// heatCoolingScale: calcHeatReduction takes ms/10000, i.e. seconds/10.
const heatCoolingScale = 10

// heat adds a shot's heat (sp_heat.lua handle_shoot_heat).
func (s *GunState) heat() {
	h := s.Gun.Heat
	if h == nil {
		return
	}
	s.Heat = min(s.Heat+h.PerShot, h.Max)
	s.Overheated = s.Overheated || s.Heat >= h.Max
}

// cool lets the heat fall, once per Tick (sp_heat.lua tick_heat, run per
// entity tick): past the wait, by Cooling·(seconds since the shot)/10 each
// tick (ModernKineticGunScriptAPI.calcHeatReduction).
func (s *GunState) cool(dt float32) {
	h := s.Gun.Heat
	if h == nil {
		return
	}
	wait := h.Delay
	if s.Overheated {
		wait = h.Lock
	}
	if s.Heat > 0 && s.sinceShot >= wait {
		s.Heat = max(s.Heat-s.sinceShot/heatCoolingScale*h.Cooling, 0)
		s.Overheated = s.Overheated && s.Heat > 0
	}
}

// heatFactor lerps the factor pick chooses by the heat fraction (1 without heat).
func (s *GunState) heatFactor(pick func(*Heat) [2]float32) float32 {
	h := s.Gun.Heat
	if h == nil || h.Max <= 0 {
		return 1
	}
	f := pick(h)
	return f[0] + (f[1]-f[0])*s.Heat/h.Max
}

// SpreadFactor scales Gun.Spread for the heat.
func (s *GunState) SpreadFactor() float32 {
	return s.heatFactor(func(h *Heat) [2]float32 { return h.Inaccuracy })
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
	clip, sound := ClipReloadTactical, SoundReloadTactical
	if s.empty {
		clip, sound = ClipReloadEmpty, SoundReloadEmpty
	}
	if s.Gun.Reload.Kind == Tube {
		clip = ClipReloadIntro
	}
	*ev = append(*ev, GunEvent{Clip: clip, Sound: sound})
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
