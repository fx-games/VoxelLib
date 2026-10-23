package ui

import (
	"testing"
	"time"
)

// The window's top-left as LOTRGuiNotificationDisplay.updateWindow places
// a 190x32 window on a 427 wide GUI, 3000 ms long, values worked by hand
// from the reference arithmetic.
func TestNotifications(t *testing.T) {
	ctx, _ := testContext(t)
	var got [][2]int
	rec := func(ctx *Context, x, y int) { got = append(got, [2]int{x, y}) }
	t0 := time.Unix(1000, 0)
	var q Notifications
	q.Add(&Notification{Sheet: "minecraft:textures/gui/widgets.png", W: 190, H: 32, Duration: 3 * time.Second, Draw: rec}, t0)
	q.Add(&Notification{Sheet: "minecraft:textures/gui/widgets.png", W: 190, H: 32, Duration: 6 * time.Second, Draw: rec}, t0)
	for _, c := range []struct {
		ms   int
		want [][2]int
	}{
		{0, [][2]int{{237, -36}, {237, 4}}},
		{187, [][2]int{{237, -2}, {237, 29}}}, // 0.0632*36 = 2; the 6 s window: 0.3175*36 = 11
		{1500, [][2]int{{237, 0}, {237, 40}}},
		{2900, [][2]int{{237, -10}, {237, 40}}}, // 1-4(2-2*0.9667) = 0.7333, ^4 = 0.289 -> 10
		{3001, [][2]int{{237, 40}}},             // the first one ends this frame: its place still counts
		{3002, [][2]int{{237, 0}}},              // then the second moves up
		{6001, nil},
	} {
		got = nil
		q.Draw(ctx, t0.Add(time.Duration(c.ms)*time.Millisecond))
		if len(got) != len(c.want) {
			t.Fatalf("%d ms: %v, want %v", c.ms, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%d ms: %v, want %v", c.ms, got, c.want)
			}
		}
	}
	if q.Len() != 0 {
		t.Fatal(q.Len())
	}
}
