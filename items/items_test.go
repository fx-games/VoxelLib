package items

import (
	"testing"

	"github.com/memmaker/voxellib/voxel/collide"
)

var floor = collide.SolidsFunc(func(x, y, z int) bool { return y < 0 })

func TestFallAndRest(t *testing.T) {
	var w World
	d := w.Spawn("a", 0.5, 5, 0.5)
	for i := 0; i < 100; i++ {
		w.Tick(floor)
	}
	if !d.OnGround || d.Y != 0 {
		t.Fatalf("drop at y=%v onGround=%v", d.Y, d.OnGround)
	}
}

func TestSwap(t *testing.T) {
	var w World
	w.Spawn("sword", 0.5, 0, 0.5)
	held := "axe"
	body := collide.Box(0.5, 0, 0.5, 0.6, 1.8)
	if got := w.Collect(body, &held, 0.5, 0, 0.5); got != "sword" || held != "sword" {
		t.Fatalf("got %q, held %q", got, held)
	}
	if len(w.Drops) != 1 || w.Drops[0].Item != "axe" {
		t.Fatalf("old item not dropped: %+v", w.Drops)
	}
	// Standing still must not swap back.
	for i := 0; i < 10; i++ {
		if got := w.Collect(body, &held, 0.5, 0, 0.5); got != "" {
			t.Fatalf("picked %q while standing on the dropped item", got)
		}
	}
	// Leave, come back: swaps again.
	far := body.Offset(10, 0, 0)
	w.Collect(far, &held, 10.5, 0, 0.5)
	if got := w.Collect(body, &held, 0.5, 0, 0.5); got != "axe" {
		t.Fatalf("got %q after returning", got)
	}
}
