package main

// The slide track: TACZ default_state_machine's slide_states on the blending
// track line's SLIDE_TRACK (layer slideTrack). While the player crouches
// with a gun that can slide (Gun.Slide), the main track idle and not
// aiming, "slide" plays and holds, then "slide_idle" loops; leaving plays
// "slide_back" and holds.

import (
	"math"

	"github.com/memmaker/voxellib/app"
)

const (
	clipSlide     = "slide"
	clipSlideIdle = "slide_idle"
	clipSlideBack = "slide_back"
)

// slideState is slide_states' state and the clip's play mode.
type slideState struct {
	on, idle bool // in the slide state; slide_idle looping
}

// tickSlide runs the slide track for a tick (after tickGunTracks).
func (g *game) tickSlide(v *gunView) {
	l, s := &v.anim.Layers[slideTrack], &v.slide
	if c := l.Clip; c != nil && s.idle && l.Time >= c.Length && c.Length > 0 {
		l.Time = float32(math.Mod(float64(l.Time), float64(c.Length))) // LOOP
	}
	should := v.spec.Slide && g.Input.Down(app.ActSneak) && v.anim.Layers[mainTrack].Clip == nil &&
		!g.Input.Down(app.ActUse)
	run := func(name string, transition float32) {
		if c := v.m.Clips[name]; c != nil {
			l.Run(c, transition)
		}
	}
	switch {
	case !s.on && should: // entry
		s.on, s.idle = true, false
		run(clipSlide, 0.5)
	case s.on && !should: // exit
		s.on, s.idle = false, false
		run(clipSlideBack, 0.2)
	case s.on && !s.idle && l.Clip != nil && l.Time >= l.Clip.Length: // update: holding
		s.idle = true
		run(clipSlideIdle, 0.4)
	}
}
