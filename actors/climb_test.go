package actors

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel/nav"
)

// climbWorld has BodySolids (so bodies see it through bodyWorld) and a
// ladder against a wall.
type climbWorld struct{ *testWorld }

func (w climbWorld) BodySolid(x, y, z int) bool { return w.Solid(x, y, z) }
func (w climbWorld) Climbable(x, y, z int) bool { return x == 0 && z == 1 && y >= 1 && y <= 6 }

// A body walking into the wall behind a ladder climbs it
// (EntityLivingBase.isOnLadder through the World's Climber), with no
// fall counted on the way down.
func TestActorClimbsLadder(t *testing.T) {
	w := climbWorld{flat(8)}
	for y := 1; y <= 7; y++ {
		w.solid[nav.Pos{X: 0, Y: y, Z: 2}] = true
	}
	m := NewManager(w, nil, nil, 1)
	a := New("climber", "npc", 0.5, 1, 0.5)
	m.Add(a)
	for range 60 {
		m.moveBody(a, character.Input{Forward: 1, Yaw: math.Pi}) // +Z
	}
	if a.Body.Y < 4 {
		t.Fatal("did not climb", a.Body.Y)
	}
	for range 60 {
		m.moveBody(a, character.Input{})
	}
	if a.fall > 0.2 || a.Body.Y > 1.01 {
		t.Fatal("came down with fall", a.fall, a.Body.Y)
	}
}

// A web touched in the last move (Entity.setInWeb) zeroes the fall
// distance, so a body dropped into one lands unhurt.
func TestWebClearsFall(t *testing.T) {
	m := NewManager(flat(8), nil, nil, 1)
	a := New("faller", "npc", 0.5, 12, 0.5)
	m.Add(a)
	a.fall = 10
	a.Body.InWeb = true
	m.moveBody(a, character.Input{})
	if a.fall > 0.1 {
		t.Fatal("fall kept", a.fall)
	}
}
