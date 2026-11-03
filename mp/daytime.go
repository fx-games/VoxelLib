package mp

import (
	"math"

	"github.com/memmaker/voxellib/fmath"
)

// The day: the server's world time, a tick a tick, in the reference's
// units (the server tick rate is the reference's, so a day lasts 20
// minutes). Only the server changes it; snapshots carry it.
const (
	DayLength = 24000
	TimeDay   = 1000 // the reference's /time set day
	TimeNoon  = 6000
	TimeNight = 13000
	TimeMid   = 18000 // midnight
)

// CelestialAngle is WorldProvider.calculateCelestialAngle: the sun's
// position in the day, 0..1 (0: noon, 0.5: midnight).
func CelestialAngle(time int64, partial float64) float32 {
	f := (float32(time%DayLength)+float32(partial))/DayLength - 0.25
	if f < 0 {
		f++
	}
	if f > 1 {
		f--
	}
	f1 := f
	f = 1 - float32((math.Cos(float64(f)*math.Pi)+1)/2)
	return f1 + (f-f1)/3
}

// Daylight is World.getSunBrightness without weather: the sky's brightness
// for the renderer's Daylight, 0.2 at night to 1 by day.
func Daylight(time int64, partial float64) float32 {
	a := CelestialAngle(time, partial)
	f := 1 - (fmath.Cos(a*math.Pi*2)*2 + 0.2)
	f = 1 - min(max(f, 0), 1)
	return f*0.8 + 0.2
}
