package mp

import "testing"

func TestDaylight(t *testing.T) {
	for _, c := range []struct {
		time     int64
		min, max float32
	}{
		{TimeNoon, 1, 1},
		{TimeMid, 0.2, 0.2},
		{TimeDay, 0.9, 1},
		{TimeNight, 0.2, 0.5},
		{DayLength + TimeNoon, 1, 1}, // the next day
	} {
		if d := Daylight(c.time, 0); d < c.min || d > c.max {
			t.Errorf("Daylight(%d) = %v, want %v..%v", c.time, d, c.min, c.max)
		}
	}
	if a := CelestialAngle(TimeNoon, 0); a != 0 {
		t.Errorf("noon's celestial angle %v", a)
	}
}
