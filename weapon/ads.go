package weapon

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Ease is an AdsCurve easing (AdsCurve.Ease).
type Ease int

const (
	EaseLinear Ease = iota
	EaseInSine
	EaseOutSine
	EaseInOutSine
	EaseInQuad
	EaseOutQuad
	EaseInOutQuad
	EaseInCubic
	EaseOutCubic
	EaseInOutCubic
	EaseInQuart
	EaseOutQuart
	EaseInOutQuart
	EaseInQuint
	EaseOutQuint
	EaseInOutQuint
	EaseInExpo
	EaseOutExpo
	EaseInOutExpo
	EaseInCirc
	EaseOutCirc
	EaseInOutCirc
	EaseInBack
	EaseOutBack
	EaseInOutBack
	EaseSmoothstep
)

var easeNames = map[string]Ease{
	"linear": EaseLinear, "smoothstep": EaseSmoothstep,
	"ease_in_sine": EaseInSine, "ease_out_sine": EaseOutSine, "ease_in_out_sine": EaseInOutSine,
	"ease_in_quad": EaseInQuad, "ease_out_quad": EaseOutQuad, "ease_in_out_quad": EaseInOutQuad,
	"ease_in_cubic": EaseInCubic, "ease_out_cubic": EaseOutCubic, "ease_in_out_cubic": EaseInOutCubic,
	"ease_in_quart": EaseInQuart, "ease_out_quart": EaseOutQuart, "ease_in_out_quart": EaseInOutQuart,
	"ease_in_quint": EaseInQuint, "ease_out_quint": EaseOutQuint, "ease_in_out_quint": EaseInOutQuint,
	"ease_in_expo": EaseInExpo, "ease_out_expo": EaseOutExpo, "ease_in_out_expo": EaseInOutExpo,
	"ease_in_circ": EaseInCirc, "ease_out_circ": EaseOutCirc, "ease_in_out_circ": EaseInOutCirc,
	"ease_in_back": EaseInBack, "ease_out_back": EaseOutBack, "ease_in_out_back": EaseInOutBack,
}

// UnmarshalJSON reads an ease by its name or the camelCase alternate
// (easeInOutSine).
func (e *Ease) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	for name, v := range easeNames {
		if s == name || strings.EqualFold(s, strings.ReplaceAll(name, "_", "")) {
			*e = v
			return nil
		}
	}
	return fmt.Errorf("unknown ease %q", s)
}

// AdsCurve maps the aiming progress through an easing over [Start, End]
// (TACZ's AdsCurve).
type AdsCurve struct {
	Start, End float32
	Ease       Ease
	Clamp      bool
}

// UnmarshalJSON applies AdsCurve's defaults: 0..1, linear, clamped.
func (c *AdsCurve) UnmarshalJSON(b []byte) error {
	type raw AdsCurve
	r := raw{End: 1, Clamp: true}
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	*c = AdsCurve(r)
	return nil
}

// Valid is AdsCurve.isValid.
func (c AdsCurve) Valid() bool { return c.Start >= 0 && c.End <= 1 && c.Start < c.End }

// At is AdsCurve.evaluate.
func (c AdsCurve) At(progress float32) float32 {
	t := float64((progress - c.Start) / (c.End - c.Start))
	if c.Clamp {
		t = clamp01(t)
	}
	const back, backIO = 1.70158, 1.70158 * 1.525
	var r float64
	switch c.Ease {
	case EaseLinear:
		r = t
	case EaseInQuad:
		r = t * t
	case EaseOutQuad:
		r = 1 - (1-t)*(1-t)
	case EaseInOutQuad:
		r = inOut(t, func(t float64) float64 { return 2 * t * t }, 2)
	case EaseInCubic:
		r = t * t * t
	case EaseOutCubic:
		r = 1 - math.Pow(1-t, 3)
	case EaseInOutCubic:
		r = inOut(t, func(t float64) float64 { return 4 * t * t * t }, 3)
	case EaseInQuart:
		r = math.Pow(t, 4)
	case EaseOutQuart:
		r = 1 - math.Pow(1-t, 4)
	case EaseInOutQuart:
		r = inOut(t, func(t float64) float64 { return 8 * math.Pow(t, 4) }, 4)
	case EaseInQuint:
		r = math.Pow(t, 5)
	case EaseOutQuint:
		r = 1 - math.Pow(1-t, 5)
	case EaseInOutQuint:
		r = inOut(t, func(t float64) float64 { return 16 * math.Pow(t, 5) }, 5)
	case EaseInSine:
		r = 1 - math.Cos(t*math.Pi/2)
	case EaseOutSine:
		r = math.Sin(t * math.Pi / 2)
	case EaseInOutSine:
		r = (1 - math.Cos(t*math.Pi)) / 2
	case EaseInExpo:
		r = EaseInExpoAt(t)
	case EaseOutExpo:
		switch {
		case t <= 0:
			r = 0
		case t >= 1:
			r = 1
		default:
			r = 1 - math.Pow(2, -10*t)
		}
	case EaseInOutExpo:
		switch {
		case t <= 0:
			r = 0
		case t >= 1:
			r = 1
		case t < 0.5:
			r = math.Pow(2, 20*t-10) / 2
		default:
			r = (2 - math.Pow(2, -20*t+10)) / 2
		}
	case EaseInCirc:
		r = 1 - math.Sqrt(1-math.Pow(clamp01(t), 2))
	case EaseOutCirc:
		r = math.Sqrt(1 - math.Pow(clamp01(t)-1, 2))
	case EaseInOutCirc:
		u := clamp01(t)
		if u < 0.5 {
			r = (1 - math.Sqrt(1-math.Pow(2*u, 2))) / 2
		} else {
			r = (math.Sqrt(1-math.Pow(-2*u+2, 2)) + 1) / 2
		}
	case EaseInBack:
		r = (back+1)*t*t*t - back*t*t
	case EaseOutBack:
		r = 1 + (back+1)*math.Pow(t-1, 3) + back*math.Pow(t-1, 2)
	case EaseInOutBack:
		if t < 0.5 {
			r = math.Pow(2*t, 2) * ((backIO+1)*2*t - backIO) / 2
		} else {
			r = math.Pow(2*t-2, 2)*((backIO+1)*(2*t-2)+backIO)/2 + 1
		}
	case EaseSmoothstep:
		r = t * t * (3 - 2*t)
	}
	return float32(r)
}

// inOut is the in-out easings' shape: in(t) below half, mirrored above
// as 1 - (-2t+2)^pow/2.
func inOut(t float64, in func(float64) float64, pow float64) float64 {
	if t < 0.5 {
		return in(t)
	}
	return 1 - math.Pow(-2*t+2, pow)/2
}

func clamp01(t float64) float64 { return math.Max(0, math.Min(1, t)) }

// EaseInExpoAt is TACZ's Easing.easeInExpo.
func EaseInExpoAt(t float64) float64 {
	switch {
	case t <= 0:
		return 0
	case t >= 1:
		return 1
	}
	return math.Pow(2, 10*t-10)
}

// AdsPath eases the hand's move to the sight per axis (AdsPath): X, Y, Z
// the translation's, Rotation the rotation's (and the rest of the move).
type AdsPath struct {
	X, Y, Z, Rotation AdsCurve
}

// UnmarshalJSON applies AdsPath's defaults: every axis a default curve.
func (p *AdsPath) UnmarshalJSON(b []byte) error {
	type raw AdsPath
	def := AdsCurve{End: 1, Clamp: true}
	r := raw{def, def, def, def}
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	*p = AdsPath(r)
	return nil
}

// Valid is AdsPath.isValid.
func (p AdsPath) Valid() bool {
	return p.X.Valid() && p.Y.Valid() && p.Z.Valid() && p.Rotation.Valid()
}

// AdsFovChannel is a FOV's curve (AdsFov.Channel); without Smooth the FOV
// follows it directly instead of through the FOV dynamics.
type AdsFovChannel struct {
	AdsCurve
	Smooth bool
}

// UnmarshalJSON applies the defaults: a default curve, smoothed.
func (c *AdsFovChannel) UnmarshalJSON(b []byte) error {
	if err := c.AdsCurve.UnmarshalJSON(b); err != nil {
		return err
	}
	s := struct{ Smooth *bool }{}
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	c.Smooth = s.Smooth == nil || *s.Smooth
	return nil
}

// FovProgress is CameraSetupEvent.getFovProgress: the curve's value, or
// easeInExpo without one.
func (c *AdsFovChannel) FovProgress(aim float32) float32 {
	if c == nil {
		return float32(EaseInExpoAt(float64(aim)))
	}
	return c.At(aim)
}

// Smoothed reports whether the FOV goes through the dynamics.
func (c *AdsFovChannel) Smoothed() bool { return c == nil || c.Smooth }
