package main

// The slide track: TACZ default_state_machine's slide_states on the blending
// track line's SLIDE_TRACK (layer slideTrack). While the player crouches
// with a gun that can slide (Gun.Slide), the main track idle and not
// aiming, "slide" plays and holds, then "slide_idle" loops; leaving plays
// "slide_back" and holds.

import (
	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/model"
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
	should := v.spec.Slide && g.Input.Down(app.ActSneak) && v.anim.Layers[mainTrack].Clip == nil &&
		!g.Input.Down(app.ActUse)
	run := func(name string, play model.Loop, transition float32) {
		if c := v.m.Clips[name]; c != nil {
			l.Run(c, play, transition)
		}
	}
	switch {
	case !s.on && should: // entry
		s.on, s.idle = true, false
		run(clipSlide, model.LoopHold, 0.5)
	case s.on && !should: // exit
		s.on, s.idle = false, false
		run(clipSlideBack, model.LoopHold, 0.2)
	case s.on && !s.idle && l.Ended(): // update: holding
		s.idle = true
		run(clipSlideIdle, model.LoopRepeat, 0.4)
	}
}
