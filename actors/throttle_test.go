package actors

import "testing"

// TestWanderThrottles: a wanderer rolls EntityAIWander only on the tasks'
// every third tick and not once 100 ticks from a player (entityAge); a
// player within 32 blocks keeps its age 0; an actor without its 32-block
// area loaded is not updated.
func TestWanderThrottles(t *testing.T) {
	walks := func(px float64, persist bool, loaded bool) (bool, int) {
		m := NewManager(flat(40), nil, nil, 1)
		p := New("player", "p", px, 1, 0.5)
		p.Player = true
		m.Add(p)
		a := New("w", "npc", 0.5, 1, 0.5)
		a.Brain = Brain{Mode: Wander, Home: Vec{0.5, 1, 0.5}, Range: 5}
		a.PersistReq = persist
		m.Add(a)
		if !loaded {
			m.Loaded = func(x, z, r int) bool { return false }
		}
		moved := false
		for range 3000 {
			m.Tick()
			moved = moved || a.Pos().hdist(Vec{0.5, 1, 0.5}) > 0.5
		}
		return moved, a.Ticks
	}
	if moved, _ := walks(3.5, false, true); !moved {
		t.Fatal("no wander near the player")
	}
	if moved, _ := walks(39.5, false, true); moved {
		t.Fatal("wandered 39 blocks from the player")
	}
	if moved, _ := walks(39.5, true, true); !moved {
		t.Fatal("persistenceRequired kept from wandering")
	}
	if moved, n := walks(3.5, false, false); moved || n != 0 {
		t.Fatal("updated without its area loaded:", moved, n)
	}
}
