package mp

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/weapon"
)

func TestViewSees(t *testing.T) {
	v := View{Eye: [3]float64{0, 0, 0}, Look: Look{}, FOV: math.Pi / 2, Aspect: 1} // looking -Z, 45° each side
	for _, c := range []struct {
		p    [3]float64
		want bool
	}{
		{[3]float64{0, 0, -10}, true},
		{[3]float64{9, 0, -10}, true},
		{[3]float64{0, 0, 10}, false},    // behind
		{[3]float64{12, 0, -10}, false},  // right of the frustum
		{[3]float64{0, 12, -10}, false},  // above
		{[3]float64{10.5, 0, -10}, true}, // a sphere reaching in
	} {
		if got := v.Sees(c.p, 1); got != c.want {
			t.Errorf("Sees(%v) = %v, want %v", c.p, got, c.want)
		}
	}
	v.Look.Yaw = math.Pi / 2 // looking +X
	if !v.Sees([3]float64{10, 0, 0}, 0) || v.Sees([3]float64{0, 0, -10}, 0) {
		t.Error("turned view")
	}
}

type flatGround struct{}

func (flatGround) StandY(x, z int) (int, bool) { return 4, x < 1000 } // columns past x 1000 not loaded

func TestPickSpot(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	views := []View{
		{Eye: [3]float64{0, 5.6, 0}, FOV: 70 * math.Pi / 180, Aspect: 16.0 / 9},
		{Eye: [3]float64{30, 5.6, 0}, Look: Look{Yaw: math.Pi}, FOV: 70 * math.Pi / 180, Aspect: 16.0 / 9},
	}
	for range 500 {
		p, ok := PickSpot(rng, [3]float64{0, 4, 0}, ZombieMinDist, ZombieMaxDist, ZombieHeight, views, flatGround{})
		if !ok {
			t.Fatal("no spot")
		}
		if d := math.Hypot(p[0], p[2]); d < ZombieMinDist-0.01 || d > ZombieMaxDist+0.01 {
			t.Fatalf("%v is %.1f blocks away", p, d)
		}
		if math.Hypot(p[0]-30, p[2]) < ZombieMinDist {
			t.Fatalf("%v is near the second player", p)
		}
		if Seen(views, p, ZombieHeight) || p[1] != 4 || p[0] >= 1000 {
			t.Fatalf("bad spot %v", p)
		}
	}
	// Looking everywhere (a view per direction): no spot, reported.
	var all []View
	for a := 0.0; a < 2*math.Pi; a += math.Pi / 4 {
		all = append(all, View{Eye: [3]float64{0, 5.6, 0}, Look: Look{Yaw: a}, FOV: math.Pi / 2, Aspect: 2})
	}
	if p, ok := PickSpot(rng, [3]float64{0, 4, 0}, ZombieMinDist, ZombieMaxDist, ZombieHeight, all, flatGround{}); ok {
		t.Fatalf("found %v in plain sight", p)
	}
}

// TestDefense runs zombie defense on the server: one zombie, gun and ammo
// pickup per SpawnInterval, out of sight; a touched pickup reaches its
// player; a zombie shot dead drops a magazine.
func TestDefense(t *testing.T) {
	loot := Loot{Guns: []string{"test:gun"}, Ammo: []string{"test:9mm"}, Rounds: map[string]int{"test:9mm": 30}}
	srv, err := StartServer(ServerConfig{Addr: "127.0.0.1:0", Name: "zd", Seed: 7, Generator: "flat",
		Reg: voxel.NewRegistry(), Mode: ZombieDefense, Loot: loot, Logf: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a := newBot(t, srv.Addr().String(), "A", netcode.Config{})
	var got []Pickup
	a.c.OnPickup = func(p Pickup) { got = append(got, p) }
	run(t, 3*time.Second, nil, func(int) bool { a.step(0, 0); return a.c.Ready() })

	var zombies, pickups int
	srv.call(func() {
		srv.def.zombies, srv.def.pickups = nil, nil
		srv.def.spawn = SpawnInterval
		p := srv.players[a.c.Slot()]
		for range 2*SpawnInterval - 1 {
			p.dead = 0 // the zombies may get close; this test is about spawning
			n := len(srv.def.zombies)
			srv.tickDefense()
			if len(srv.def.zombies) > n { // just spawned: before it walks
				m := srv.def.zombies[n].m
				at := [3]float64{m.X, m.Y, m.Z}
				if d := math.Hypot(at[0]-p.ch.X, at[2]-p.ch.Z); d < ZombieMinDist || d > ZombieMaxDist || Seen(srv.views(), at, ZombieHeight) {
					t.Errorf("zombie spawned %.0f blocks away, seen %v", d, Seen(srv.views(), at, ZombieHeight))
				}
			}
		}
		zombies, pickups = len(srv.def.zombies), len(srv.def.pickups)
	})
	if zombies != 1 || pickups != 2 {
		t.Fatalf("after %d ticks: %d zombies, %d pickups; want 1 and 2", 2*SpawnInterval-1, zombies, pickups)
	}

	srv.call(func() {
		p := srv.players[a.c.Slot()]
		srv.addPickup(loot.Magazine("test:9mm"), [3]float64{p.ch.X, p.ch.Y, p.ch.Z})
	})
	run(t, 3*time.Second, nil, func(int) bool { a.step(0, 0); return len(got) > 0 })
	if got[0] != (Pickup{Kind: PickupAmmo, ID: "test:9mm", Rounds: 30}) {
		t.Fatalf("took %+v", got[0])
	}

	var drops int
	srv.call(func() {
		p := srv.players[a.c.Slot()]
		p.gun = &weapon.Gun{Ammo: "test:9mm"}
		z := srv.def.zombies[0]
		n := len(srv.def.pickups)
		srv.hurtZombie(p, z, Hit{Gun: true}, ZombieHealth, 1, 0, 0)
		drops = len(srv.def.pickups) - n
		p.gun = nil
	})
	if drops != 1 {
		t.Fatalf("a killed zombie dropped %d pickups", drops)
	}
}

func TestGunCheat(t *testing.T) {
	loot := Loot{Guns: []string{"test:gun"}, Ammo: []string{"test:9mm"}, Rounds: map[string]int{"test:9mm": 30}}
	srv, err := StartServer(ServerConfig{Addr: "127.0.0.1:0", Name: "zd", Seed: 7, Generator: "flat",
		Reg: voxel.NewRegistry(), Mode: ZombieDefense, Loot: loot, Logf: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	a := newBot(t, srv.Addr().String(), "A", netcode.Config{})
	var got []Pickup
	a.c.OnPickup = func(p Pickup) { got = append(got, p) }
	run(t, 3*time.Second, nil, func(int) bool { a.step(0, 0); return a.c.Ready() })
	a.c.SendCheatGuns()
	run(t, 3*time.Second, nil, func(int) bool { a.step(0, 0); return len(got) >= 2 })
	want := []Pickup{{Kind: PickupGun, ID: "test:gun"}, {Kind: PickupAmmo, ID: "test:9mm", Rounds: 30 * cheatMagazines}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("cheat gave %+v, want %+v", got, want)
	}
}
