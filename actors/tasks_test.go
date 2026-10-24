package actors

import "testing"

// TestTasks: EntityAITasks' scheduling - starts only on every third run,
// a higher priority task with a shared mutex bit pre-empts a lower one
// (reset in the same pass), compatible tasks run together, Continue
// is asked on the runs between.
func TestTasks(t *testing.T) {
	var log []string
	want, cont := map[string]bool{}, map[string]bool{}
	mk := func(name string, prio, mutex int) *Task {
		return &Task{Priority: prio, Mutex: mutex,
			Should:   func() bool { return want[name] },
			Continue: func() bool { return cont[name] },
			Start:    func() { log = append(log, "start "+name) },
			Reset:    func() { log = append(log, "reset "+name) },
			Update:   func() { log = append(log, "up "+name) }}
	}
	var ts Tasks
	hi, lo, side := mk("hi", 1, 3), mk("lo", 5, 1), mk("side", 6, 4)
	ts.Add(hi)
	ts.Add(lo)
	ts.Add(side)
	want["lo"], cont["lo"], want["side"], cont["side"] = true, true, true, true
	ts.Run() // tick 0: lo and side start
	if !ts.Running(lo) || !ts.Running(side) || len(log) != 4 {
		t.Fatalf("first run: %v", log)
	}
	want["hi"], cont["hi"] = true, true
	ts.Run() // tick 1: no starts
	ts.Run() // tick 2
	if ts.Running(hi) {
		t.Fatal("started off the tick rate")
	}
	log = nil
	ts.Run() // tick 3: hi starts; lo, after it in the list, is reset in the same pass
	if !ts.Running(hi) || ts.Running(lo) || log[0] != "reset lo" || log[1] != "start hi" {
		t.Fatalf("tick 3: %v", log)
	}
	ts.Run()
	ts.Run()
	ts.Run() // tick 6: a full pass
	cont["side"] = false
	log = nil
	ts.Run() // tick 7: side stops by its Continue
	if ts.Running(side) || log[0] != "reset side" {
		t.Fatalf("tick 7: %v", log)
	}
}

// TestKeeps: a target the game set on a non-hostile actor is dropped at
// once unless Keeps holds it.
func TestKeeps(t *testing.T) {
	m := NewManager(flat(10), nil, nil, 1)
	a, b := New("a", "a", 0.5, 1, 0.5), New("b", "b", 3.5, 1, 0.5)
	a.Melee.Damage = 2
	m.Add(a)
	m.Add(b)
	a.Target = b
	m.fight(a)
	if a.Target != nil {
		t.Fatal("a non-hostile target stayed")
	}
	m.Keeps = func(x, y *Actor) bool { return x == a && y == b }
	a.Target = b
	m.fight(a)
	if a.Target != b {
		t.Fatal("Keeps did not keep the target")
	}
}
