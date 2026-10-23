package actors

import (
	"math"
	"testing"
)

func horseWorld(t *testing.T, s MountStats) (*Manager, *Actor, *Actor, *[]MountEvent) {
	t.Helper()
	m := NewManager(flat(40), nil, nil, 1)
	p := New("player", "player", 0.5, 1, 0.5)
	p.Player = true
	h := New("horse", "horse", 0.5, 1, 0.5)
	h.Body.Width, h.Body.Height = 1.4, 1.6
	h.Speed, h.Mount = 0.3, &s
	var evs []MountEvent
	h.Mount.OnEvent = func(_ *Actor, ev MountEvent) { evs = append(evs, ev) }
	m.Add(p)
	m.Add(h)
	if !m.Mount(p, h) {
		t.Fatal("mount failed")
	}
	return m, p, h, &evs
}

// TestBuck: an untamed horse throws a player rider and its temper rises
// 5; at full temper the first 1 in 50 roll tames it to the rider.
func TestBuck(t *testing.T) {
	m, p, h, evs := horseWorld(t, MountStats{RideHeight: 0.8, Horse: true, MaxTemper: 100})
	for i := 0; i < 2000 && p.Riding != nil; i++ {
		m.Tick()
	}
	if p.Riding != nil || h.Mount.Temper != 5 || h.Mount.Tame || len(*evs) != 1 || (*evs)[0] != MountBucked {
		t.Fatalf("riding %v temper %d tame %v events %v", p.Riding != nil, h.Mount.Temper, h.Mount.Tame, *evs)
	}
	h.Mount.Temper = 100
	m.Mount(p, h)
	for i := 0; i < 2000 && !h.Mount.Tame; i++ {
		m.Tick()
	}
	if !h.Mount.Tame || p.Riding != h || h.Mount.Owner != "player" || (*evs)[1] != MountTamed {
		t.Fatalf("not tamed: tame %v riding %v events %v", h.Mount.Tame, p.Riding != nil, *evs)
	}
}

// TestHorseFall: EntityHorse.fall deals ceil(d*0.5-3) to the horse and
// its rider; a tapped jump bar jumps with 0.4 of the jump strength.
func TestHorseFall(t *testing.T) {
	m, p, h, evs := horseWorld(t, MountStats{RideHeight: 0.8, Horse: true, Tame: true, Saddled: true, JumpStrength: 0.7})
	h.Body.Y, h.Body.PrevY = 13, 13 // 12 blocks above the floor
	for i := 0; i < 60 && !(h.Body.OnGround && h.Body.PrevY == h.Body.Y); i++ {
		m.Tick()
	}
	if h.Health != 17 || p.Health != 17 || len(*evs) == 0 || (*evs)[0] != MountLanded {
		t.Fatalf("health horse %.0f rider %.0f events %v, want 17 17 [landed]", h.Health, p.Health, *evs)
	}
	p.RideInput.Jump = true
	m.Tick()
	p.RideInput.Jump = false
	m.Tick() // released: setJumpPower(0) is 0.4, and the horse rears
	if v := h.Body.Y - h.Body.PrevY; math.Abs(v-0.7*0.4) > 1e-9 || !h.Rearing() {
		t.Fatalf("jump rise %.4f rearing %v", v, h.Rearing())
	}
}

// TestRearAnim: EntityHorse.onUpdate's rearing amount, 20 ticks up then
// easing back to 0.
func TestRearAnim(t *testing.T) {
	m := NewManager(flat(10), nil, nil, 1)
	h := New("horse", "horse", 0.5, 1, 0.5)
	h.Mount = &MountStats{Horse: true}
	m.Add(h)
	h.Rear()
	m.Tick()
	if r := h.Mount.Anim.Rear; math.Abs(r-0.45) > 1e-9 {
		t.Fatalf("rear after 1 tick %.4f, want 0.45", r)
	}
	for i := 0; i < 18; i++ {
		m.Tick()
	}
	if !h.Rearing() || h.Mount.Anim.Rear != 1 {
		t.Fatal("stopped rearing early")
	}
	m.Tick()
	if h.Rearing() || math.Abs(h.Mount.Anim.Rear-(1+(0.8-1)*0.6-0.05)) > 1e-9 {
		t.Fatalf("rearing %v amount %.4f after 20 ticks", h.Rearing(), h.Mount.Anim.Rear)
	}
}

// TestMountFall: a mount that is no horse takes ceil(d-3) and Entity.fall
// passes the same fall to its rider; sprinting speeds the ridden mount 1.3x.
func TestMountFall(t *testing.T) {
	m, p, h, _ := horseWorld(t, MountStats{RideHeight: 0.8, Tame: true, Saddled: true})
	h.Body.Y, h.Body.PrevY = 13, 13
	for i := 0; i < 60 && !(h.Body.OnGround && h.Body.PrevY == h.Body.Y); i++ {
		m.Tick()
	}
	if h.Health != 12 || p.Health != 12 { // the fall is 10.x blocks (TestHorseFall: ceil(d*0.5-3) = 3)
		t.Fatalf("health mount %.0f rider %.0f, want 12 12", h.Health, p.Health)
	}
	h.Sprinting = true
	p.RideInput.Forward = 1
	m.Tick()
	if math.Abs(h.Body.MoveSpeed-0.3*1.3) > 1e-12 {
		t.Fatalf("sprinting move speed %.4f, want 0.39", h.Body.MoveSpeed)
	}
}
