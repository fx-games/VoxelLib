package app

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/voxel/collide"
)

func TestRayBox(t *testing.T) {
	b := collide.Box(0, 0, 5, 1, 2) // x -0.5..0.5, y 0..2, z 4.5..5.5
	if d, ok := rayBox(0, 1, 0, 0, 0, 1, b); !ok || math.Abs(d-4.5) > 1e-9 {
		t.Fatalf("hit = %v %v, want 4.5", d, ok)
	}
	if _, ok := rayBox(0, 1, 0, 0, 0, -1, b); ok {
		t.Fatal("hit behind the ray")
	}
	if _, ok := rayBox(2, 1, 0, 0, 0, 1, b); ok {
		t.Fatal("hit beside the box")
	}
}

func TestEntities(t *testing.T) {
	var es Entities
	e := es.Add(&Entity{X: 1, Yaw: math.Pi - 0.1, Model: -1})
	e.Tick = func(e *Entity) { e.X += 1; e.Yaw += 0.2 } // crosses ±π
	es.tick()
	x, _, _, yaw, _ := e.Lerp(0.5)
	if x != 1.5 || math.Abs(yaw-math.Pi) > 1e-9 {
		t.Fatalf("lerp = %v %v, want 1.5 π (short way round)", x, yaw)
	}
	es.Add(&Entity{Model: -1}).Removed = true
	es.tick()
	if len(es.List) != 1 || es.ByID(e.ID) != e {
		t.Fatalf("registry = %v", es.List)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	a := &App{}
	a.Player = &Player{Character: character.New(1, 2, 3), a: a, SpawnX: 7}
	a.Player.Flying = true
	a.Cam.Yaw = 0.5
	a.Hooks.Save = func() []byte { return []byte("game") }
	b, err := a.encodeSave()
	if err != nil {
		t.Fatal(err)
	}
	c := &App{}
	c.Player = &Player{Character: character.New(0, 0, 0), a: c, findGround: true}
	game, err := c.decodeSave(b)
	p := c.Player
	if err != nil || string(game) != "game" || p.X != 1 || p.Z != 3 || !p.Flying || p.SpawnX != 7 || c.Cam.Yaw != 0.5 || p.findGround {
		t.Fatalf("decoded %q %v: %+v", game, err, p)
	}
	var ps save // a save from before dimensions: no Dim
	if json.Unmarshal(b, &ps); ps.Player.Dim != nil {
		t.Fatal("no engine, yet a dimension was saved")
	}
	if game, err := c.decodeSave(nil); game != nil || err != nil {
		t.Fatal("new game must decode to nil")
	}
}

func TestHotbarAction(t *testing.T) {
	a := &App{}
	a.Input = newInput(a)
	a.Player = &Player{Character: character.New(0, 0, 0), a: a, HotbarSize: 3}
	a.Input.dispatch(ActHotbar+"3", true)
	a.Input.dispatch(ActHotbar+"9", true) // beyond the hotbar: ignored
	if a.Player.Selected != 2 {
		t.Fatalf("selected %d, want 2", a.Player.Selected)
	}
	a.Hooks.Action = func(name string, pressed bool) bool { return name == ActAttack }
	a.Input.dispatch(ActAttack, true)
	if !a.Input.suppress[ActAttack] || a.Player.clicks[Attack] {
		t.Fatal("a consumed attack must suppress breaking")
	}
	a.Input.dispatch(ActAttack, false)
	if a.Input.suppress[ActAttack] {
		t.Fatal("release must clear the suppression")
	}
}
