package actors

import (
	"math"
	"testing"
)

// TestLookTasks: a standing mob's WatchClosest2 starts on the player
// within its range and turns the head (10 a tick) and the pitch (at most
// 40) at the eyes; out of range it stops; a higher-priority watch takes
// over a lower one; moving drops WatchClosest2 but keeps WatchClosest.
func TestLookTasks(t *testing.T) {
	m := NewManager(flat(16), nil, nil, 1)
	p := New("p", "player", 4.5, 1, 0.5)
	p.Player = true
	h := New("h", "npc", 0.5, 1, 0.5)
	h.Brain = Brain{Mode: Stand, Home: h.Pos(), Face: 0}
	o := New("o", "npc", 0.5, 1, 3.5)
	o.Brain = Brain{Mode: Stand, Home: o.Pos(), Face: 0}
	h.Looks = []LookTask{{Priority: 12, Kind: LookWatch2, Dist: 8, Chance: 1},
		{Priority: 13, Kind: LookWatch, Of: func(a *Actor) bool { return a.Kind == "npc" }, Dist: 8, Chance: 1}}
	m.Add(p)
	m.Add(h)
	m.Add(o)
	for range 3 {
		m.Tick()
	}
	if h.lt.on != 1 || h.lt.target != p {
		t.Fatal("not watching the player", h.lt)
	}
	for range 20 {
		m.Tick()
	}
	if d := math.Remainder(h.HeadYaw-yawTo(h.Pos(), p.Pos()), 2*math.Pi); math.Abs(d) > 0.01 || h.Pitch != 0 {
		t.Fatal("head off the player", d, h.Pitch)
	}
	p.Body.Teleport(20.5, 1, 0.5)
	m.Tick()
	if h.lt.on == 1 {
		t.Fatal("watching beyond 8")
	}
	for range 3 {
		m.Tick()
	}
	if h.lt.on != 2 || h.lt.target != o {
		t.Fatal("WatchClosest on the npc", h.lt)
	}
	m.lookTasks(h, false, true)
	if h.lt.on != 2 {
		t.Fatal("moving dropped WatchClosest")
	}
	h.lt = lookRun{on: 1, target: p, time: 50}
	m.lookTasks(h, false, true)
	if h.lt.on == 1 {
		t.Fatal("moving kept WatchClosest2")
	}
}
