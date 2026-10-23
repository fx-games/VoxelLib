package actors

import (
	"math"
	"testing"
)

// TestCNPCLook: a standing CustomNPC set to RotateBody looks around on its
// own (EntityAILook) and its still body follows the head (EntityBodyHelper);
// one set to NoRotation keeps its orientation.
func TestCNPCLook(t *testing.T) {
	m := NewManager(flat(8), nil, nil, 1)
	rot := New("rot", "npc", 0.5, 1, 0.5)
	rot.Brain = Brain{Mode: Stand, Home: rot.Pos(), Face: 1, Look: LookRotate}
	fix := New("fix", "npc", 6.5, 1, 6.5)
	fix.Brain = Brain{Mode: Stand, Home: fix.Pos(), Face: 1, Look: LookFixed}
	m.Add(rot)
	m.Add(fix)
	heads := map[int]bool{}
	bodyTurned := false
	for range 2000 {
		m.Tick()
		heads[int(math.Round(rot.HeadYaw*4))] = true
		if math.Abs(math.Remainder(rot.Yaw-1, 2*math.Pi)) > 0.5 {
			bodyTurned = true
		}
		if fix.Yaw != 1 || fix.HeadYaw != 1 {
			t.Fatalf("NoRotation NPC turned: yaw %.2f head %.2f", fix.Yaw, fix.HeadYaw)
		}
	}
	if len(heads) < 8 || !bodyTurned {
		t.Fatalf("RotateBody NPC: %d head directions, body turned %v", len(heads), bodyTurned)
	}
}
