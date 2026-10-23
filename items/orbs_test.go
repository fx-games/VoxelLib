package items

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/memmaker/voxellib/voxel/collide"
)

func TestOrbs(t *testing.T) {
	floor := collide.SolidsFunc(func(x, y, z int) bool { return y < 0 })
	w := &Orbs{Rand: rand.New(rand.NewPCG(1, 2))}
	w.Drop(0.5, 0.5, 0.5, 30) // 17 + 7 + 3 + 3
	var got []int
	for _, o := range w.List {
		got = append(got, o.Value)
	}
	if len(got) != 4 || got[0] != 17 || got[1] != 7 || got[2] != 3 || got[3] != 3 {
		t.Fatalf("split %v", got)
	}
	if w.List[0].Icon() != 3 || w.List[3].Icon() != 1 {
		t.Fatal("icon")
	}
	// Free fall from rest, far from the player: vy -0.03, ×0.98.
	o := &Orb{X: 0.5, Y: 5.5, Z: 0.5, Health: 5, Value: 1}
	w.List = []*Orb{o}
	far := Player{X: 100, Y: 0, Z: 100, Eye: 1.62, Alive: true}
	w.Tick(floor, far)
	if math.Abs(o.Y-(5.5-0.029999999329447746)) > 1e-12 || math.Abs(o.VY-(-0.029999999329447746*0.9800000190734863)) > 1e-12 {
		t.Fatalf("fall y %v vy %v", o.Y, o.VY)
	}
	// Landing: on ground, vertical motion zeroed, ground friction 0.6×0.98.
	for range 60 {
		w.Tick(floor, far)
	}
	if !o.OnGround || math.Abs(o.Y-0.25) > 1e-9 {
		t.Fatalf("ground %v %v", o.OnGround, o.Y)
	}
	o.VX = 0.1
	w.Tick(floor, far)
	if math.Abs(o.VX-0.1*float64(float32(0.6)*0.98)) > 1e-9 {
		t.Fatalf("friction %v", o.VX)
	}
	// Attraction toward the eyes of a player within 8 (id 0: the target is looked up once xpColor passes the last lookup by 20).
	o.VX, o.Color = 0, 100
	near := Player{X: 4.5, Y: 0, Z: 0.5, Eye: 1.62, Alive: true}
	x0, y0 := o.X, o.Y
	w.Tick(floor, near)
	dx, dy := (4.5-x0)/8, (1.62-y0)/8
	d := math.Sqrt(dx*dx + dy*dy)
	want := dx / d * (1 - d) * (1 - d) * 0.1 * float64(float32(0.6)*0.98) // still on the ground
	if math.Abs(o.VX-want) > 1e-9 {
		t.Fatalf("pull %v want %v", o.VX, want)
	}
	// Pickup: one orb per cooldown of 2.
	w.Spawn(0.5, 0.25, 0.5, 7)
	body := collide.Box(0.5, 0, 0.5, 0.6, 1.8)
	cd := 0
	if v := w.Collect(body, &cd); v == 0 || cd != 2 || len(w.List) != 1 {
		t.Fatalf("collect %v %v %v", v, cd, len(w.List))
	}
	if w.Collect(body, &cd) != 0 {
		t.Fatal("cooldown")
	}
	// Stuck inside a block: kicked up by 0.1..0.3.
	stone := collide.SolidsFunc(func(x, y, z int) bool { return y < 2 && x == 0 && z == 0 || y < 0 })
	in := &Orb{X: 0.5, Y: 1.5, Z: 0.5, Health: 5}
	w.pushOut(in, stone)
	if in.VX > -0.1 || in.VX < -0.3 || in.VY != 0 { // -x is nearest first
		t.Fatal("push")
	}
	// Despawn and lava.
	w.List[0].Age = OrbLife - 1
	w.Tick(floor, far)
	if len(w.List) != 0 {
		t.Fatal("despawn")
	}
	w.Lava = func(x, y, z int) bool { return y == 0 }
	l := w.Spawn(0.5, 0.5, 0.5, 1)
	w.Tick(floor, far)
	w.Tick(floor, far)
	if len(w.List) != 0 || l.Health > 0 {
		t.Fatalf("lava %v", l.Health)
	}
}
