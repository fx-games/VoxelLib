package audio

import (
	"math"
	"math/rand"
	"slices"
)

// MusicType is the current musical situation, like 1.7.10's MusicTicker.MusicType.
type MusicType struct {
	Name               string                  // a change of name stops the playing track
	MinDelay, MaxDelay int                     // ticks between two tracks
	Pick               func(*rand.Rand) string // the event to play now; "" waits RetryDelay ticks
}

// Track is what the ticker plays; *Voice implements it.
type Track interface {
	Playing() bool
	Stop()
}

// MusicTicker is 1.7.10's MusicTicker: after a track ends it waits a random
// [MinDelay, MaxDelay] ticks, then plays the type's next track. Call Tick every client tick
// that is not paused (the main menu never pauses).
type MusicTicker struct {
	Play  func(event string) Track // starts a track; nil when nothing plays
	Muted func() bool              // music volume is zero: stop and wait (optional)
	Rand  *rand.Rand
	// ChangeDelay is the wait after the type changes. < 0 is vanilla: random [0, MinDelay/2]
	// ticks and only if a track was playing; >= 0 applies on every change.
	ChangeDelay int
	RetryDelay  int // wait after Pick returned "" or Play returned nil

	timer   int
	cur     Track
	curType string
	started bool
}

// NewMusicTicker returns a vanilla ticker: the first track comes after 100 ticks.
func NewMusicTicker(play func(string) Track, rng *rand.Rand) *MusicTicker {
	return &MusicTicker{Play: play, Rand: rng, ChangeDelay: -1, RetryDelay: 400, timer: 100}
}

// Music returns a ticker that plays through s (category from each event, usually "music").
func (s *Sounds) Music() *MusicTicker {
	t := NewMusicTicker(func(ev string) Track {
		if v := s.Play(ev, UI(1, 1)); v != nil {
			return v
		}
		return nil
	}, s.Rand)
	t.Muted = func() bool { return s.Mixer.Volume("music") <= 0 }
	return t
}

// randRange is MathHelper.getRandomIntegerInRange.
func randRange(r *rand.Rand, lo, hi int) int {
	if lo >= hi {
		return lo
	}
	return lo + r.Intn(hi-lo+1)
}

// Current returns the playing track, or nil.
func (t *MusicTicker) Current() Track { return t.cur }

// Tick advances the ticker by one tick in situation mt.
func (t *MusicTicker) Tick(mt MusicType) {
	if !t.started {
		t.started, t.curType = true, mt.Name
	}
	if mt.Name != t.curType {
		t.curType = mt.Name
		if t.ChangeDelay >= 0 {
			if t.cur != nil {
				t.cur.Stop()
				t.cur = nil
			}
			t.timer = t.ChangeDelay
		} else if t.cur != nil {
			t.cur.Stop()
			t.timer = randRange(t.Rand, 0, mt.MinDelay/2)
		}
	}
	if t.cur != nil {
		if t.Muted != nil && t.Muted() {
			t.cur.Stop()
		}
		if !t.cur.Playing() {
			t.cur = nil
			t.timer = min(randRange(t.Rand, mt.MinDelay, mt.MaxDelay), t.timer)
		}
	}
	if t.Muted != nil && t.Muted() {
		return
	}
	if t.cur == nil {
		t.timer--
		if t.timer < 0 {
			t.timer = t.RetryDelay
			if ev := mt.Pick(t.Rand); ev != "" {
				if tr := t.Play(ev); tr != nil {
					t.cur, t.timer = tr, math.MaxInt
				}
			}
		}
	}
}

// MusicTrack is one entry of a weighted track pool.
type MusicTrack struct {
	Event  string   `json:"event"`
	Title  string   `json:"title,omitempty"`
	Weight float64  `json:"weight"`
	Tags   []string `json:"tags,omitempty"` // situations it may play in (e.g. day, night, cave)
}

// MusicPool is a list of tracks to pick from.
type MusicPool []MusicTrack

// Pick returns a random track's event among those tagged tag ("" = any), weighted.
// It returns "" if none fits.
func (p MusicPool) Pick(r *rand.Rand, tag string) string {
	var fit []MusicTrack
	total := 0.0
	for _, tr := range p {
		if tag == "" || slices.Contains(tr.Tags, tag) {
			fit = append(fit, tr)
			total += tr.Weight
		}
	}
	if len(fit) == 0 {
		return ""
	}
	x := r.Float64() * total
	for _, tr := range fit {
		if x -= tr.Weight; x < 0 {
			return tr.Event
		}
	}
	return fit[len(fit)-1].Event
}
