package mp

import (
	"math"
	"testing"
	"time"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel"
)

// bot is a headless client: its own copy of the world and a character it
// moves the way the game's player tick does.
type bot struct {
	t     *testing.T
	c     *Client
	ch    *character.Character
	w     *World
	hits  []Hit
	edits []BlockEdit
	texts []string
}

func newBot(t *testing.T, addr, name string, cfg netcode.Config) *bot {
	t.Helper()
	c, err := Connect(addr, name, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	reg := voxel.NewRegistry()
	gen, err := NewGenerator(reg, c.Welcome.Generator, c.Welcome.Seed)
	if err != nil {
		t.Fatal(err)
	}
	b := &bot{t: t, c: c, ch: character.New(0, 0, 0)}
	c.Welcome.Own.Apply(b.ch)
	b.w = NewWorld(reg, c.Edits.Wrap(gen), -64, 384)
	c.OnHit = func(h Hit) { b.hits = append(b.hits, h) }
	c.OnBlock = func(e BlockEdit) {
		b.edits = append(b.edits, e)
		b.w.SetBlock(e.X, e.Y, e.Z, reg.Parse(e.State))
	}
	c.OnText = func(s string) { b.texts = append(b.texts, s) }
	return b
}

// step runs one client tick with the given look and buttons.
func (b *bot) step(yaw, pitch float64, attack bool) bool {
	b.c.Poll()
	b.c.Reconcile(b.ch, b.w)
	in := character.Input{Yaw: yaw, Pitch: pitch}
	cmd := b.c.Command(in, attack)
	b.ch.Tick(b.w, in)
	return b.c.Predicted(cmd, b.ch, b.w)
}

// run ticks every bot at the tick rate until done reports true.
func run(t *testing.T, limit time.Duration, bots []*bot, tick func(i int) bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for i := 0; ; i++ {
		if tick(i) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(time.Second / TickRate)
	}
}

// aim returns the look that sends an arrow from eye to target, found by
// flying test arrows with the real physics (a sniper's holdover).
func aim(eye, target [3]float64) (yaw, pitch float64) {
	dx, dy, dz := target[0]-eye[0], target[1]-eye[1], target[2]-eye[2]
	yaw = math.Atan2(dx, -dz)
	dist := math.Hypot(dx, dz)
	heightAt := func(p float64) float64 {
		var w projectile.World
		a := w.Shoot(0, 0, 0, 0, p, projectile.Speed)
		for range 400 {
			px, py := -a.Z, a.Y
			w.Tick(noBlocks{}, nil)
			if -a.Z >= dist {
				f := (dist - px) / (-a.Z - px)
				return py + (a.Y-py)*f
			}
		}
		return math.Inf(-1)
	}
	lo, hi := -0.5, 0.8
	for range 60 {
		pitch = (lo + hi) / 2
		if heightAt(pitch) < dy {
			lo = pitch
		} else {
			hi = pitch
		}
	}
	return yaw, pitch
}

type noBlocks struct{}

func (noBlocks) Solid(x, y, z int) bool { return false }

func startServer(t *testing.T, spawn func(slot int, w *World) (x, y, z float64)) *Server {
	t.Helper()
	s, err := StartServer(ServerConfig{Addr: "127.0.0.1:0", Name: "test", Seed: 7, Generator: "flat",
		Reg: voxel.NewRegistry(), Spawn: spawn, Logf: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// TestWelcomePlaces: a joining player is placed while connecting, and the
// Welcome carries their own state: the first snapshot agrees with it, so a
// client starting from the Welcome never snaps.
func TestWelcomePlaces(t *testing.T) {
	srv := startServer(t, nil) // spread around the world spawn
	a := newBot(t, srv.Addr().String(), "A", netcode.Config{})
	b := newBot(t, srv.Addr().String(), "B", netcode.Config{})
	for _, x := range []*bot{a, b} {
		w := x.c.Welcome.Own
		if w.Health != MaxHealth || !w.Protected || w.Dead {
			t.Fatalf("slot %d welcome state %+v", x.c.Slot(), w)
		}
		if w.X == srv.spawn[0] && w.Z == srv.spawn[2] {
			t.Fatalf("slot %d welcomed at the world spawn", x.c.Slot())
		}
	}
	if a.c.Welcome.Own.X == b.c.Welcome.Own.X && a.c.Welcome.Own.Z == b.c.Welcome.Own.Z {
		t.Fatal("both players welcomed at the same spot")
	}
	run(t, 3*time.Second, nil, func(int) bool {
		a.c.Poll()
		b.c.Poll()
		return a.c.Ready() && b.c.Ready()
	})
	for _, x := range []*bot{a, b} {
		w, s := x.c.Welcome.Own, x.c.Own()
		if w.X != s.X || w.Y != s.Y || w.Z != s.Z {
			t.Fatalf("slot %d welcomed at %.2f,%.2f,%.2f, first snapshot at %.2f,%.2f,%.2f", x.c.Slot(), w.X, w.Y, w.Z, s.X, s.Y, s.Z)
		}
	}
}

func TestSnipeAndEdits(t *testing.T) {
	const range_ = 120.0
	srv := startServer(t, func(slot int, w *World) (float64, float64, float64) {
		return 0.5, float64(w.Surface(0, 0)), 0.5 - float64(slot)*range_ // slot 1 stands 120 blocks north
	})
	cfg := netcode.Config{Latency: 3 * time.Millisecond, Jitter: 2 * time.Millisecond, Drop: 0.05}
	a := newBot(t, srv.Addr().String(), "Archer", cfg)
	b := newBot(t, srv.Addr().String(), "Target", cfg)
	bots := []*bot{a, b}
	ground := a.w.Surface(0, 0) // the same generator as the server's

	// Archer places a block; the target sees it and so does a late joiner.
	edit := BlockEdit{X: 3, Y: ground, Z: 3, State: a.w.W.Reg.StateString(a.w.W.Reg.Parse("stone"))}
	a.c.SendEdit(edit)
	run(t, 3*time.Second, bots, func(int) bool {
		for _, bt := range bots {
			bt.step(0, 0, false)
		}
		return len(b.edits) > 0 && a.c.Ready() && b.c.Ready()
	})
	if b.edits[0] != edit || b.w.Block(3, ground, 3) == voxel.Air {
		t.Fatalf("target got edit %+v", b.edits)
	}

	// The archer aims at the target's head 120 blocks off and fires once.
	// Health 20, a head shot kills.
	var shotAt int
	run(t, 12*time.Second, bots, func(i int) bool {
		var views []PlayerView
		views = a.c.Players()
		fire := false
		yaw, pitch := 0.0, 0.0
		for _, v := range views {
			if v.Slot == b.c.Slot() {
				ex, ey, ez := a.ch.Eye()
				yaw, pitch = aim([3]float64{ex, ey - 0.1, ez}, [3]float64{v.X, v.Y + 1.75, v.Z})
				fire = shotAt == 0 && i > 10
			}
		}
		if a.step(yaw, pitch, fire) {
			shotAt = i
		}
		b.step(math.Pi, 0, false) // facing the archer
		return len(b.hits) > 0
	})
	h := b.hits[0]
	if !h.Head || !h.Killed || h.Attacker != a.c.Slot() || h.Victim != b.c.Slot() {
		t.Fatalf("hit %+v", h)
	}
	if !b.c.Own().Dead && b.c.Own().Health != 0 {
		// The death may already be in the next snapshot; give it a tick.
		run(t, time.Second, bots, func(int) bool {
			for _, bt := range bots {
				bt.step(0, 0, false)
			}
			return b.c.Own().Dead
		})
	}
	t.Logf("hit after %d ticks; kill feed %q; prediction corrections: archer %d, target %d",
		len(b.hits), b.texts, a.c.Errors, b.c.Errors)

	late := newBot(t, srv.Addr().String(), "Late", netcode.Config{})
	run(t, 3*time.Second, nil, func(int) bool {
		late.step(0, 0, false)
		return len(late.edits) > 0
	})
	if late.edits[0] != edit {
		t.Fatalf("late joiner got %+v", late.edits)
	}
}

// TestNoHitDeadOrProtected: a dead player can't be killed again and a
// (re)spawned one is protected for ProtectTicks.
func TestNoHitDeadOrProtected(t *testing.T) {
	srv := startServer(t, nil)
	a := newBot(t, srv.Addr().String(), "A", netcode.Config{})
	b := newBot(t, srv.Addr().String(), "B", netcode.Config{})
	run(t, 3*time.Second, nil, func(int) bool {
		a.step(0, 0, false)
		b.step(0, 0, false)
		return a.c.Ready() && b.c.Ready()
	})
	shoot := func() (health int, dead, protected bool) {
		srv.call(func() {
			shooter, victim := srv.players[a.c.Slot()], srv.players[b.c.Slot()]
			var w projectile.World
			srv.hit(shooter, victim, w.Shoot(0, 0, 0, 0, 0, projectile.Speed))
			health, dead, protected = victim.health, victim.dead > 0, victim.protect > 0
		})
		return
	}
	if h, _, p := shoot(); h != MaxHealth || !p {
		t.Fatalf("just spawned: health %d, protected %v", h, p)
	}
	var deaths int
	srv.call(func() {
		v := srv.players[b.c.Slot()]
		v.protect = 0
		deaths = v.deaths
	})
	for range MaxHealth/ArrowDamage + 1 {
		shoot()
	}
	h, dead, _ := shoot()
	srv.call(func() { deaths = srv.players[b.c.Slot()].deaths - deaths })
	if h != 0 || !dead || deaths != 1 {
		t.Fatalf("after shooting the dead: health %d, dead %v, died %d times", h, dead, deaths)
	}
	srv.call(func() { srv.respawn(srv.players[b.c.Slot()]) })
	if h, _, p := shoot(); h != MaxHealth || !p {
		t.Fatalf("respawned: health %d, protected %v", h, p)
	}
}

// TestBattleRoyale plays a round with a fast wall: it starts round
// everyone, kills whoever it reaches, the last one standing wins, and
// everyone is back for the next round.
func TestBattleRoyale(t *testing.T) {
	srv, err := StartServer(ServerConfig{Addr: "127.0.0.1:0", Name: "br", Seed: 7, Generator: "flat",
		Reg: voxel.NewRegistry(), Mode: BattleRoyale, ZoneInterval: time.Second, ShrinkTime: time.Second, Logf: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	var bots []*bot
	for _, n := range []string{"A", "B", "C"} {
		bots = append(bots, newBot(t, srv.Addr().String(), n, netcode.Config{}))
	}
	step := func() {
		for _, b := range bots {
			b.step(0, 0, false)
		}
	}
	phase := func(p Phase) func(int) bool {
		return func(int) bool { step(); return bots[0].c.Round().Phase == p }
	}
	run(t, 3*time.Second, bots, phase(PhaseCountdown))
	srv.call(func() { srv.m.round.Ticks = 1 }) // skip the countdown
	run(t, 2*time.Second, bots, phase(PhasePlaying))
	r := srv.Round()
	for _, p := range srv.Status() {
		if p.Dead || p.Out {
			t.Fatalf("round started with %+v", p)
		}
	}
	for _, b := range bots {
		if r.Zone.Touches(b.ch.X, b.ch.Z, PlayerHalfWidth) {
			t.Fatalf("%s at %.1f,%.1f outside the zone %+v", b.c.Info[b.c.Slot()].Name, b.ch.X, b.ch.Z, r.Zone)
		}
	}
	run(t, 20*time.Second, bots, phase(PhaseOver))
	wins, outs := 0, 0
	for _, p := range srv.Status() {
		wins += p.Wins
		if p.Out {
			outs++
		}
	}
	if wins > 1 || outs < 2 || wins+outs != 3 {
		t.Fatalf("round over with %d wins, %d out: %+v", wins, outs, srv.Status())
	}
	t.Logf("texts: %q", bots[0].texts)
	srv.call(func() { srv.m.round.Ticks = 1 }) // skip the winner's moment
	run(t, 3*time.Second, bots, phase(PhaseCountdown))
	for _, p := range srv.Status() {
		if p.Out || p.Dead {
			t.Fatalf("next round: %+v", p)
		}
	}
}

// TestTeams: joiners fill the smaller team; teammates can't hurt each
// other.
func TestTeams(t *testing.T) {
	srv, err := StartServer(ServerConfig{Addr: "127.0.0.1:0", Name: "tdm", Seed: 7, Generator: "flat",
		Reg: voxel.NewRegistry(), Mode: TeamDeathmatch, Logf: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	var bots []*bot
	for _, n := range []string{"A", "B", "C"} {
		bots = append(bots, newBot(t, srv.Addr().String(), n, netcode.Config{}))
	}
	run(t, 3*time.Second, bots, func(int) bool {
		for _, b := range bots {
			b.step(0, 0, false)
		}
		return bots[0].c.Info[bots[2].c.Slot()].Team != 0
	})
	want := []int{TeamRed, TeamBlue, TeamRed}
	for i, b := range bots {
		if got := bots[0].c.Info[b.c.Slot()].Team; got != want[i] {
			t.Errorf("%d joined on team %d, want %d", i, got, want[i])
		}
	}
	srv.call(func() {
		a, b, c := srv.players[bots[0].c.Slot()], srv.players[bots[1].c.Slot()], srv.players[bots[2].c.Slot()]
		if enemies(a, c) || !enemies(a, b) || !enemies(b, c) {
			t.Error("teammates are enemies or enemies aren't")
		}
	})
}
