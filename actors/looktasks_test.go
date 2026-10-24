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

// TestFlierLooks: a flier's look tasks run though it flies: an idle look
// is not dropped by the motion, the head turns to a watched player and,
// moving, stays within 75 of the body, which keeps the flight's yaw.
func TestFlierLooks(t *testing.T) {
	m := NewManager(flat(16), nil, nil, 1)
	p := New("p", "player", 0.5, 1, -6.5) // straight ahead (yaw 0 faces -z)
	p.Player = true
	b := New("b", "bird", 0.5, 4, 0.5)
	b.Fly = &Flight{Hover: 1}
	b.Looks = []LookTask{{Priority: 0, Kind: LookWatch, Dist: 12, Chance: 1}}
	m.Add(p)
	m.Add(b)
	for range 30 {
		m.Tick()
	}
	if !b.Watching() || math.Abs(math.Remainder(b.HeadYaw-yawTo(b.Pos(), p.Pos()), 2*math.Pi)) > 0.01 {
		t.Fatal("still bird not watching", b.lt, b.HeadYaw)
	}
	p.Body.Teleport(-6.5, 1, 0.5) // to the side: 90 off the body
	b.Yaw = 0
	b.Body.VX, b.Body.VZ = 0, -0.2 // flying ahead
	for range 15 {
		b.Body.VZ = -0.2
		m.Tick()
		b.Yaw = 0
	}
	if d := math.Remainder(b.HeadYaw-b.Yaw, 2*math.Pi); math.Abs(math.Abs(d)-75*math.Pi/180) > 0.01 {
		t.Fatalf("flying head %.1f deg off the body, want 75", d*180/math.Pi)
	}
	b.Looks = []LookTask{{Kind: LookIdle, Chance: 1}}
	b.lt = lookRun{}
	ran := false
	for range 600 {
		b.Body.VZ = -0.2
		m.Tick()
		ran = ran || b.Watching()
	}
	if !ran {
		t.Error("idle look dropped while flying")
	}
}

// TestLookHeld: a held look bit stops the running watch and keeps it from
// starting again until released.
func TestLookHeld(t *testing.T) {
	m := NewManager(flat(16), nil, nil, 1)
	p := New("p", "player", 4.5, 1, 0.5)
	p.Player = true
	h := New("h", "npc", 0.5, 1, 0.5)
	h.Brain = Brain{Mode: Stand, Home: h.Pos(), Face: 0}
	h.Looks = []LookTask{{Priority: 12, Kind: LookWatch2, Dist: 8, Chance: 1}}
	m.Add(p)
	m.Add(h)
	for range 3 {
		m.Tick()
	}
	if !h.Watching() {
		t.Fatal("not watching")
	}
	h.LookHeld = true
	for range 6 {
		if m.Tick(); h.Watching() {
			t.Fatal("watching while held")
		}
	}
	h.LookHeld = false
	for range 3 {
		m.Tick()
	}
	if !h.Watching() {
		t.Fatal("not watching after release")
	}
}

// TestLookPos: a task's setLookPosition turns the head by 10 a tick and
// the pitch by at most 40 toward the cell, over the watch task.
func TestLookPos(t *testing.T) {
	m := NewManager(flat(16), nil, nil, 1)
	h := New("h", "npc", 0.5, 1, 0.5)
	h.Brain = Brain{Mode: Stand, Home: h.Pos(), Face: 0}
	h.Looks = []LookTask{{Priority: 8, Kind: LookWatch, Dist: 8, Chance: 1}}
	m.Add(h)
	m.Tick()
	h.HeadYaw, h.Yaw = 0, 0
	goal := Vec{4.5, 1, 0.5}
	want := yawTo(Vec{h.Body.X, h.Body.Y + h.Body.EyeHeight, h.Body.Z}, goal)
	h.LookPos = &goal
	m.Tick()
	if h.LookPos != nil || math.Abs(math.Abs(h.HeadYaw)-10*math.Pi/180) > 1e-9 || h.Pitch >= 0 {
		t.Fatal("first tick", h.HeadYaw, h.Pitch, h.LookPos)
	}
	for range 20 {
		h.LookPos = &goal
		m.Tick()
	}
	if math.Abs(math.Remainder(h.HeadYaw-want, 2*math.Pi)) > 1e-9 {
		t.Fatal("head", h.HeadYaw, want)
	}
}
