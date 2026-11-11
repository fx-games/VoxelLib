package mp

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/memmaker/voxellib/importer/tacz"
	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/weapon"
)

func TestShotEncoding(t *testing.T) {
	in := Shot{Gun: "tacz:m870", Pellets: []Look{{0.5, -0.25}, {-3, 1}}}
	msg := in.encode()
	if msg[0] != msgShoot {
		t.Fatalf("type %d", msg[0])
	}
	r := rbuf{p: msg[1:]}
	out := decodeShot(&r)
	if r.err != nil || out.Gun != in.Gun || !slices.Equal(out.Pellets, in.Pellets) {
		t.Fatalf("got %+v (%v), want %+v", out, r.err, in)
	}
	info := PlayerInfo{Slot: 2, Joined: true, Name: "a", Team: TeamRed,
		Held: Hold{Gun: "tacz:ak47", Choice: tacz.Choice{
			Attachments: map[tacz.Slot]string{tacz.SlotMuzzle: "tacz:muzzle_silencer_ursus", tacz.SlotScope: "", tacz.SlotGrip: "tacz:grip_x"},
			Adapters:    map[tacz.Slot]string{tacz.SlotGrip: "tacz:riser"}}}}
	r = rbuf{p: info.encode()[1:]}
	got := decodePlayerInfo(&r)
	if r.err != nil || got.Held.Gun != info.Held.Gun || got.Team != info.Team || got.Held.Choice.Key() != info.Held.Choice.Key() ||
		len(got.Held.Choice.Attachments) != 2 || got.Held.Choice.Adapters[tacz.SlotGrip] != "tacz:riser" {
		t.Fatalf("got %+v (%v)", got, r.err)
	}
}

// TestHoldChoice: a held gun's attachments reach the server's resolver and
// everyone's PlayerInfo; the server deals the resolved spec's damage, and a
// hold the resolver rejects is ignored.
func TestHoldChoice(t *testing.T) {
	const id, silencer, riser = "test:rifle", "test:silencer", "test:riser"
	resolve := func(gid string, c tacz.Choice, mode weapon.FireMode) (weapon.Gun, error) {
		m := c.Attachments[tacz.SlotMuzzle]
		if gid != id || (m != "" && m != silencer) || (c.Adapters[tacz.SlotMuzzle] != "" && c.Adapters[tacz.SlotMuzzle] != riser) {
			return weapon.Gun{}, fmt.Errorf("not allowed: %v", c)
		}
		g := weapon.Gun{ID: id}
		g.Bullet.Damage, g.Bullet.Pellets = 3, 1
		if m == silencer {
			g.Bullet.Damage = 5
		}
		if mode == weapon.Auto { // a fire_mode_adjust
			g.Bullet.Damage += 2
		}
		return g, nil
	}
	s, err := StartServer(ServerConfig{Addr: "127.0.0.1:0", Name: "test", Seed: 7, Generator: "flat",
		Reg: voxel.NewRegistry(), Logf: t.Logf, Gun: resolve,
		Spawn: func(slot int, w *World) (float64, float64, float64) {
			return 0.5, float64(w.Surface(0, 0)), 0.5 - float64(slot)*10
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	a := newBot(t, s.Addr().String(), "A", netcode.Config{})
	b := newBot(t, s.Addr().String(), "B", netcode.Config{})
	step := func() { a.step(0, 0, false); b.step(0, 0, false) }
	held := tacz.Choice{}
	held.Adapt(tacz.SlotMuzzle, riser)
	held.Attach(tacz.SlotMuzzle, silencer)
	a.c.SendHold(Hold{Gun: id, Choice: held, Mode: weapon.Auto})
	run(t, 3*time.Second, nil, func(int) bool {
		step()
		h := b.c.Info[a.c.Slot()].Held
		return a.c.Ready() && b.c.Ready() && h.Choice.Key() == held.Key() && h.Mode == weapon.Auto
	})
	a.c.SendHold(Hold{Gun: id, Choice: tacz.Choice{Attachments: map[tacz.Slot]string{tacz.SlotMuzzle: silencer}, Adapters: map[tacz.Slot]string{tacz.SlotMuzzle: "test:bogus"}}}) // rejected: stays silenced
	s.call(func() { s.players[b.c.Slot()].protect = 0 })
	ex, ey, ez := a.ch.Eye()
	pitch := math.Atan2(b.ch.Y+1-ey, math.Hypot(b.ch.X-ex, b.ch.Z-ez))
	a.c.SendShot(Shot{Gun: id, Pellets: []Look{{0, pitch}}})
	run(t, 3*time.Second, nil, func(int) bool {
		step()
		return len(b.hits) >= 1
	})
	if h := b.hits[0]; h.Damage != 7 {
		t.Fatalf("hit %+v, want the silenced auto spec's 7", h)
	}
	if c := b.c.Info[a.c.Slot()].Held.Choice; c.Key() != held.Key() {
		t.Fatalf("rejected hold reached the others: %v", c)
	}
}

// TestGunShot: the shooter's held gun shows in everyone's PlayerInfo, a
// shot deals the gun's damage once per pellet on the server, and the
// others hear it go off.
func TestGunShot(t *testing.T) {
	gun := weapon.Gun{ID: "test:shotgun"}
	gun.Bullet.Damage, gun.Bullet.Pellets = 4, 2
	s, err := StartServer(ServerConfig{Addr: "127.0.0.1:0", Name: "test", Seed: 7, Generator: "flat",
		Reg: voxel.NewRegistry(), Logf: t.Logf, Gun: func(string, tacz.Choice, weapon.FireMode) (weapon.Gun, error) { return gun, nil },
		Spawn: func(slot int, w *World) (float64, float64, float64) {
			return 0.5, float64(w.Surface(0, 0)), 0.5 - float64(slot)*10 // slot 1 stands 10 blocks north
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	a := newBot(t, s.Addr().String(), "A", netcode.Config{})
	b := newBot(t, s.Addr().String(), "B", netcode.Config{})
	fired := 0
	b.c.OnFired = func(slot int) {
		if slot == a.c.Slot() {
			fired++
		}
	}
	a.c.SendHold(Hold{Gun: gun.ID})
	run(t, 3*time.Second, nil, func(int) bool {
		a.step(0, 0, false)
		b.step(0, 0, false)
		return a.c.Ready() && b.c.Ready() && b.c.Info[a.c.Slot()].Held.Gun == gun.ID
	})
	s.call(func() { s.players[b.c.Slot()].protect = 0 })
	ex, ey, ez := a.ch.Eye()
	pitch := math.Atan2(b.ch.Y+1-ey, math.Hypot(b.ch.X-ex, b.ch.Z-ez)) // the chest
	a.c.SendShot(Shot{Gun: gun.ID, Pellets: []Look{{0, pitch}, {0, pitch}}})
	run(t, 3*time.Second, nil, func(int) bool {
		a.step(0, 0, false)
		b.step(0, 0, false)
		return len(b.hits) >= 2 && len(a.hits) >= 2 && fired > 0
	})
	for _, h := range b.hits {
		if h.Damage != 4 || h.Attacker != a.c.Slot() || h.Victim != b.c.Slot() || h.Head || !h.Gun {
			t.Fatalf("hit %+v", h)
		}
	}
	var health int
	s.call(func() { health = s.players[b.c.Slot()].health })
	if health != MaxHealth-8 || len(b.hits) != 2 {
		t.Fatalf("health %d after %d hits", health, len(b.hits))
	}
	if a.hits[0] != b.hits[0] {
		t.Fatalf("shooter got %+v, victim %+v", a.hits[0], b.hits[0])
	}
	// a killing pellet: the shooter's hit mark says kill
	s.call(func() { s.players[b.c.Slot()].health = 4 })
	a.c.SendShot(Shot{Gun: gun.ID, Pellets: []Look{{0, pitch}}})
	run(t, 3*time.Second, nil, func(int) bool {
		a.step(0, 0, false)
		b.step(0, 0, false)
		return len(a.hits) >= 3
	})
	if h := a.hits[2]; !h.Killed || !h.Gun || h.Attacker != a.c.Slot() {
		t.Fatalf("kill hit %+v", h)
	}
}

// TestAimSync: the client sends only the aim state; the server raises the
// aiming progress over the held gun's aim time, poses the body's hold with
// it and syncs it to everyone (AIMING_PROGRESS_KEY), and lowers it again.
func TestAimSync(t *testing.T) {
	gun := weapon.Gun{ID: "test:minigun", Aim: 0.2, ThirdPersonHold: weapon.HoldMinigun}
	s, err := StartServer(ServerConfig{Addr: "127.0.0.1:0", Name: "test", Seed: 7, Generator: "flat",
		Reg: voxel.NewRegistry(), Logf: t.Logf, Gun: func(string, tacz.Choice, weapon.FireMode) (weapon.Gun, error) { return gun, nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	a := newBot(t, s.Addr().String(), "A", netcode.Config{})
	b := newBot(t, s.Addr().String(), "B", netcode.Config{})
	seen := func() weapon.AimProgress {
		for _, v := range b.c.Players() {
			if v.Slot == a.c.Slot() {
				return v.Aim
			}
		}
		return -1
	}
	a.c.SendHold(Hold{Gun: gun.ID})
	a.c.SendAim(true)
	run(t, 3*time.Second, nil, func(int) bool {
		a.step(0, 0, false)
		b.step(0, 0, false)
		return a.c.Own().Aim == weapon.Aimed && seen() == weapon.Aimed
	})
	var anim Anim
	s.call(func() { anim = s.players[a.c.Slot()].anim })
	if !anim.Gun || anim.Hold != weapon.HoldMinigun || anim.Aim != weapon.Aimed {
		t.Fatalf("server pose %+v", anim)
	}
	a.c.SendAim(false)
	run(t, 3*time.Second, nil, func(int) bool {
		a.step(0, 0, false)
		b.step(0, 0, false)
		return a.c.Own().Aim == 0 && seen() == 0
	})
}
