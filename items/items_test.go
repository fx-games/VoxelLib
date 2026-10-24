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

// TestHazards: lava (4, then the burning contact's 1) ends a drop in its
// first tick; fire contact takes 1 a tick, so 5 ticks; water keeps a drop
// a cactus pricks 1 a tick and puts a drop's fire at -1; health is an int.
func TestHazards(t *testing.T) {
	at := func(cx, cy, cz int) func(x, y, z int) bool {
		return func(x, y, z int) bool { return x == cx && y == cy && z == cz }
	}
	lava := &World{Lava: at(0, 0, 0), Burning: at(0, 0, 0)}
	lava.Spawn("a", 0.5, 0.3, 0.5)
	lava.Tick(floor)
	if len(lava.Drops) != 0 {
		t.Fatal("a drop survived lava", lava.Drops[0].Wounds)
	}
	fire := &World{Burning: at(0, 0, 0)}
	d := fire.Spawn("a", 0.5, 0, 0.5)
	for i := 1; i <= 4; i++ {
		fire.Tick(floor)
		if len(fire.Drops) != 1 || d.Wounds != i {
			t.Fatalf("tick %d: wounds %d", i, d.Wounds)
		}
	}
	// a new drop's fire is 0, so ++fire never reaches 0: setFire(8) waits
	// for a drop that was once out of fire (fire -1)
	if fire.Tick(floor); len(fire.Drops) != 0 || d.Fire != 1 {
		t.Fatal("fire: not gone after 5 ticks", d.Fire)
	}
	cactus := &World{Prick: func(x, y, z int) float64 { return 1 }}
	d = cactus.Spawn("a", 0.5, 0, 0.5)
	cactus.Tick(floor)
	if d.Wounds != 1 || d.Fire != -1 {
		t.Fatal("cactus", d.Wounds, d.Fire)
	}
	if !d.Hurt(3.9) { // (int)(4 - 3.9) = 0
		t.Fatal("health truncates")
	}
}
