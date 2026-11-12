package mp

import (
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/importer/tacz"
	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/weapon"
)

// Game rules.
const (
	MaxHealth    = 20
	RespawnTicks = 3 * TickRate
	// ProtectTicks after (re)spawning a player can't be hurt and bullets
	// fly through them (EntityPlayerMP.respawnInvulnerabilityTicks).
	ProtectTicks = 60
	// historyTicks of player poses are kept for lag compensation.
	historyTicks = 32
	// maxLag caps how far back a shooter's view is rewound (ticks).
	maxLag = 10
	// stallLimit is how long a server frame may run before the watchdog
	// logs the stacks.
	stallLimit = 3 * time.Second
	// spawnSpread scatters spawn points around the world spawn (blocks).
	spawnSpread = 24
)

// ServerConfig sets up a Server.
type ServerConfig struct {
	Addr      string // UDP address to listen on (default ":28960")
	Name      string // shown in discovery
	Seed      int64
	Generator string // "overworld", "flat" or "map:<name>" (see MapsDir)
	Mode      Mode
	// ZoneInterval is how long a battle royale wall stands before it
	// closes in (0: DefaultZoneInterval).
	ZoneInterval time.Duration
	// ShrinkTime is how long the wall takes to close in (0:
	// DefaultShrinkTime).
	ShrinkTime time.Duration
	Reg        *voxel.Registry
	Net        netcode.Config // Protocol is set by the server
	// Spawn, when set, picks where a player (re)spawns: feet position.
	// Without it players spawn around the world spawn.
	Spawn func(slot int, w *World) (x, y, z float64)
	// Gun resolves a held gun with its attachments (tacz.Pack.Gun); a
	// hold it rejects is logged and ignored. Nil: players hold no guns.
	Gun func(id string, c tacz.Choice, mode weapon.FireMode) (weapon.Gun, error)
	// Loot is what zombie defense spawns (NewLoot of the gun pack); empty:
	// no pickups.
	Loot Loot
	// Zombies is a stress test: this many more zombies come with zombie
	// defense's first wave, and the server logs its tick times.
	Zombies int
	Logf    func(format string, args ...any)
}

// Server is the authoritative simulation (Q3's server game): it runs every
// player's commands as they arrive, flies the bullets, runs the zombies, deals damage and sends
// each client a delta-compressed snapshot every tick. It runs on its own
// goroutine; a listen server's host connects to it like everyone else.
type Server struct {
	cfg   ServerConfig
	net   *netcode.Server
	world *World
	edits *EditLog
	body  *Body

	tick   uint32 // atomic: the AcceptFunc reads it
	time   int64  // world time (see DayLength)
	frozen bool   // the time stands still
	spawn  [3]float64
	// spawnTop is the highest ground a respawn takes.
	spawnTop float64
	players  [4]*sPlayer
	// joining are players accept placed whose Connected event has not
	// been handled yet, by slot.
	joining [4]*sPlayer
	bullets []*sBullet
	rng     *rand.Rand // bullet spread, zombie spots and loot
	nextID  uint32     // entity ids (bullets, zombies, pickups), above every slot
	m       match
	def     defense // zombie defense

	calls chan func() // run on the server goroutine (the console)
	stop  chan struct{}
	done  chan struct{}
	once  sync.Once
}

type sPlayer struct {
	slot  int
	name  string
	skin  []byte
	ch    *character.Character
	anim  Anim
	last  Cmd // last command run
	ran   bool
	yaw   float64
	pitch float64

	health  float32 // LivingEntity health: fractional; the wire carries hearts' Health
	dead    int     // ticks until respawn while dead
	protect int     // ticks of spawn protection left
	kills   int
	deaths  int
	wins    int                // battle royale rounds won
	team    int                // TeamRed, TeamBlue or 0
	held    Hold               // the gun in hand
	gun     *weapon.Gun        // held's spec (cfg.Gun), nil for none
	flags   GunFlags           // the held gun's state (msgGun; isAiming, reloadState, ammo)
	aim     weapon.AimProgress // ShooterDataHolder.aimingProgress, synced to everyone
	out     bool               // out of the battle royale round: watching until the next
	lag     float64            // smoothed ticks between the server's time and what the player saw
	view    View               // the player's projection (msgView); its eye and look follow the player

	hist    [historyTicks]pose
	tracers projectile.Shooter // the held gun's bullet count, for its tracers
	enc     netcode.Encoder
	sentTo  int // edit log entries queued for this client
}

// pose is a player at one server tick, for lag compensation.
type pose struct {
	tick    uint32
	x, y, z float64
	yaw     float64
	anim    Anim
	alive   bool
}

// StartServer generates the spawn area, opens the socket and starts the
// simulation.
func StartServer(cfg ServerConfig) (*Server, error) {
	if cfg.Addr == "" {
		cfg.Addr = fmt.Sprintf(":%d", DefaultPort)
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	if cfg.Generator == "" {
		cfg.Generator = "overworld"
	}
	if cfg.ZoneInterval <= 0 {
		cfg.ZoneInterval = DefaultZoneInterval
	}
	if cfg.ShrinkTime <= 0 {
		cfg.ShrinkTime = DefaultShrinkTime
	}
	gen, err := NewGenerator(cfg.Reg, cfg.Generator, cfg.Seed)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, edits: NewEditLog(), body: NewBody(), rng: rand.New(rand.NewSource(cfg.Seed)), nextID: firstEntityID,
		time: TimeDay, calls: make(chan func()), stop: make(chan struct{}), done: make(chan struct{})}
	s.m.round.Mode = cfg.Mode // before the first tick: a Welcome carries it
	s.m.interval = max(1, int(cfg.ZoneInterval*TickRate/time.Second))
	s.m.shrinkTicks = max(1, int(cfg.ShrinkTime*TickRate/time.Second))
	minY, height := Heights(cfg.Generator)
	s.world = NewWorld(cfg.Reg, s.edits.Wrap(gen), minY, height)
	x, y, z := s.world.SpawnPoint(0, 0)
	s.spawn, s.spawnTop = [3]float64{x, y, z}, math.Inf(1)
	if MapName(cfg.Generator) != "" {
		s.spawnTop = y + 3 // a map's streets, not its roofs
	}
	s.world.LoadAround(x, z, 32)
	s.def.loot = cfg.Loot
	s.def.spawn = SpawnInterval
	s.def.extra = cfg.Zombies

	nc := cfg.Net
	nc.Protocol = Protocol
	nc.MaxClients = len(s.players)
	s.net, err = netcode.Listen(cfg.Addr, nc, s.accept)
	if err != nil {
		return nil, err
	}
	s.setInfo()
	go s.run()
	return s, nil
}

// Addr is the address the server listens on.
func (s *Server) Addr() *net.UDPAddr { return s.net.Addr() }

// Close stops the simulation and disconnects everyone.
func (s *Server) Close() {
	s.once.Do(func() {
		close(s.stop)
		<-s.done
		s.net.Close()
	})
}

func (p *sPlayer) score() Score {
	return Score{Slot: p.slot, Kills: p.kills, Deaths: p.deaths, Wins: p.wins}
}

func (p *sPlayer) info() PlayerInfo {
	return PlayerInfo{Slot: p.slot, Joined: true, Name: p.name, Skin: p.skin, Team: p.team, Held: p.held}
}

// Ranking is the players by kills, most first, then by fewest deaths.
func (s *Server) Ranking() []PlayerStatus {
	ps := s.Status()
	slices.SortStableFunc(ps, func(a, b PlayerStatus) int {
		if a.Wins != b.Wins {
			return b.Wins - a.Wins
		}
		if a.Kills != b.Kills {
			return b.Kills - a.Kills
		}
		return a.Deaths - b.Deaths
	})
	return ps
}

// call runs f on the server goroutine and waits for it; false if the
// server has stopped.
func (s *Server) call(f func()) bool {
	ran := make(chan struct{})
	select {
	case s.calls <- func() { f(); close(ran) }:
		<-ran
		return true
	case <-s.done:
		return false
	}
}

// PlayerStatus is one connected player, for a server console.
type PlayerStatus struct {
	Slot          int
	Name          string
	Health        int
	Dead          bool
	Kills, Deaths int
	Wins          int  // battle royale
	Team          int  // team deathmatch
	Out           bool // out of the battle royale round
	Ping          time.Duration
}

// Status lists the connected players.
func (s *Server) Status() []PlayerStatus {
	var out []PlayerStatus
	s.call(func() {
		for _, p := range s.players {
			if p != nil {
				out = append(out, PlayerStatus{Slot: p.slot, Name: p.name, Health: p.hearts(), Dead: p.dead > 0,
					Kills: p.kills, Deaths: p.deaths, Wins: p.wins, Team: p.team, Out: p.out, Ping: s.net.Stats(p.slot).RTT})
			}
		}
	})
	return out
}

// Say sends every player a chat line.
func (s *Server) Say(text string) { s.call(func() { s.broadcastText(text) }) }

// Kick disconnects a player; false if the slot is empty.
func (s *Server) Kick(slot int, reason string) bool {
	ok := false
	s.call(func() {
		if slot >= 0 && slot < len(s.players) && s.players[slot] != nil {
			s.net.Kick(slot, reason)
			ok = true
		}
	})
	return ok
}

// Time is the world time (see DayLength) and whether it stands still.
func (s *Server) Time() (t int64, frozen bool) {
	s.call(func() { t, frozen = s.time, s.frozen })
	return
}

// SetTime sets the world time and whether it runs; every player sees it
// with their next snapshot.
func (s *Server) SetTime(t int64, frozen bool) {
	s.call(func() { s.time, s.frozen = max(t, 0), frozen })
}

// Round is the match state (mode, battle royale phase and zone, team
// scores).
func (s *Server) Round() (r Round) {
	s.call(func() { r = s.m.round })
	return
}

// Config is what the server was started with.
func (s *Server) Config() ServerConfig { return s.cfg }

// accept answers a connect request (on the network goroutine): the player
// is placed at their spawn point on the server goroutine, and the Welcome
// carries where, so the client starts there.
func (s *Server) accept(slot int, _ net.Addr, hello []byte) ([]byte, error) {
	h, err := decodeHello(hello)
	if err != nil {
		return nil, err
	}
	var w Welcome
	if !s.call(func() { w = s.join(slot, h) }) {
		return nil, errors.New("server is stopping")
	}
	return w.encode(), nil
}

// join makes the player connecting to slot, placed and waiting for its
// Connected event (see joining), and their Welcome.
func (s *Server) join(slot int, h Hello) Welcome {
	p := &sPlayer{slot: slot, name: h.Name, ch: character.New(0, 0, 0), health: MaxHealth, view: defaultView}
	if p.name == "" {
		p.name = fmt.Sprintf("Player %d", slot+1)
	}
	if s.cfg.Mode == TeamDeathmatch {
		p.team = s.pickTeam()
	}
	s.joining[slot] = p
	s.respawn(p)
	s.joinedLate(p)
	return Welcome{Slot: slot, Seed: s.cfg.Seed, Generator: s.cfg.Generator, Tick: atomic.LoadUint32(&s.tick),
		Time: s.time, TimeFrozen: s.frozen, Round: s.m.round, Own: p.state()}
}

func (s *Server) setInfo() {
	n := 0
	for _, p := range s.players {
		if p != nil {
			n++
		}
	}
	var b wbuf
	b.str(s.cfg.Name)
	b.u8(uint8(n))
	b.u8(uint8(len(s.players)))
	s.net.SetInfo(b)
}

// ServerInfo is what discovery shows about a server.
type ServerInfo struct {
	Name         string
	Players, Max int
	Addr         string
	Ping         time.Duration
}

// Discover lists the LAN servers answering within wait.
func Discover(wait time.Duration) []ServerInfo {
	found, err := netcode.Discover(DefaultPort, netcode.Config{Protocol: Protocol}, wait)
	if err != nil {
		return nil
	}
	var out []ServerInfo
	for _, f := range found {
		r := rbuf{p: f.Info}
		si := ServerInfo{Name: r.str(), Players: int(r.u8()), Max: int(r.u8()), Addr: f.Addr.String(), Ping: f.Ping}
		if r.err == nil {
			out = append(out, si)
		}
	}
	// A server on this machine answers on loopback and on the LAN: list
	// it once, at its LAN address.
	return slices.DeleteFunc(out, func(si ServerInfo) bool {
		h, port, _ := net.SplitHostPort(si.Addr)
		if ip := net.ParseIP(h); ip == nil || !ip.IsLoopback() {
			return false
		}
		return slices.ContainsFunc(out, func(o ServerInfo) bool {
			oh, oport, _ := net.SplitHostPort(o.Addr)
			ip := net.ParseIP(oh)
			return ip != nil && !ip.IsLoopback() && oport == port && o.Name == si.Name
		})
	})
}

func (s *Server) run() {
	defer close(s.done)
	t := time.NewTicker(time.Second / TickRate)
	defer t.Stop()
	var frameStart atomic.Int64 // unix ns; 0 between frames
	go s.watchdog(&frameStart)
	var stats tickStats
	for {
		select {
		case <-s.stop:
			return
		case f := <-s.calls:
			f()
		case <-t.C:
			start := time.Now()
			frameStart.Store(start.UnixNano())
			s.frame()
			frameStart.Store(0)
			if s.cfg.Zombies > 0 {
				stats.add(s, time.Since(start))
			}
		}
	}
}

// watchdog logs every goroutine's stack once when a frame runs longer than
// stallLimit: clients time out on a stalled server, and the stacks say why.
func (s *Server) watchdog(frameStart *atomic.Int64) {
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	var reported int64
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			st := frameStart.Load()
			if st == 0 || st == reported || time.Since(time.Unix(0, st)) < stallLimit {
				continue
			}
			reported = st
			buf := make([]byte, 1<<20)
			buf = buf[:runtime.Stack(buf, true)]
			s.cfg.Logf("mp: server frame stalled for %v:\n%s", time.Since(time.Unix(0, st)).Round(time.Millisecond), buf)
		}
	}
}

// frame is one server tick: take in what arrived (running commands as they
// come), advance the world, send snapshots.
func (s *Server) frame() {
	for _, e := range s.net.Poll() {
		s.handle(e)
	}
	tick := atomic.AddUint32(&s.tick, 1)
	if !s.frozen {
		s.time++
	}
	for _, p := range s.players {
		if p == nil {
			continue
		}
		if p.dead > 0 && !p.out {
			if p.dead--; p.dead == 0 {
				s.respawn(p)
			}
		} else if p.protect > 0 {
			p.protect--
		}
		s.tickAim(p)
		p.anim.Age = float64(tick)
		p.hist[tick%historyTicks] = pose{tick: tick, x: p.ch.X, y: p.ch.Y, z: p.ch.Z, yaw: p.yaw, anim: p.anim,
			alive: p.dead == 0}
	}
	s.tickMatch()
	s.tickDefense()
	s.tickBullets(tick)
	s.streamEdits()
	for _, p := range s.players {
		if p != nil {
			s.sendSnapshot(p, tick)
		}
	}
}

func (s *Server) handle(e netcode.Event) {
	switch e.Kind {
	case netcode.Connected:
		p := s.joining[e.Client]
		if p == nil {
			s.cfg.Logf("mp: slot %d connected without being accepted", e.Client)
			return
		}
		s.joining[e.Client] = nil
		s.players[e.Client] = p
		for _, o := range s.players {
			if o != nil && o != p {
				s.net.SendReliable(p.slot, o.info().encode())
				s.net.SendReliable(p.slot, o.score().encode())
			}
		}
		s.broadcast(p.info().encode())
		joined := p.colored() + " joined the game"
		if p.team != 0 {
			joined += " on team " + TeamColor(p.team) + TeamName(p.team)
		}
		s.broadcastText(joined)
		s.setInfo()
		s.cfg.Logf("mp: %s joined (slot %d)", p.name, p.slot)
	case netcode.Disconnected:
		p := s.players[e.Client]
		if p == nil {
			return
		}
		s.players[e.Client] = nil
		s.broadcast(PlayerInfo{Slot: p.slot}.encode())
		s.broadcastText(p.name + " left the game")
		s.setInfo()
		s.cfg.Logf("mp: %s left (%s)", p.name, e.Reason)
	case netcode.Reliable:
		if p := s.players[e.Client]; p != nil {
			s.reliable(p, e.Data)
		}
	case netcode.Message:
		if p := s.players[e.Client]; p != nil {
			if cmds, err := decodeCmds(e.Data); err == nil {
				for _, c := range cmds {
					if !p.ran || c.Num > p.last.Num {
						s.runCmd(p, c)
					}
				}
			}
		}
	}
}

func (s *Server) reliable(p *sPlayer, msg []byte) {
	if len(msg) == 0 {
		return
	}
	r := rbuf{p: msg[1:]}
	switch msg[0] {
	case msgSkin:
		p.skin = append([]byte(nil), msg[1:]...)
		s.broadcast(p.info().encode())
	case msgHold:
		h := decodeHold(&r)
		if r.err != nil {
			s.cfg.Logf("mp: %s: bad hold: %v", p.name, r.err)
			return
		}
		var spec *weapon.Gun
		if h.Gun != "" {
			if s.cfg.Gun == nil {
				s.cfg.Logf("mp: %s holds gun %q, but this server has no guns", p.name, h.Gun)
				return
			}
			g, err := s.cfg.Gun(h.Gun, h.Choice, h.Mode)
			if err != nil {
				s.cfg.Logf("mp: %s holds invalid gun %q %v %v: %v", p.name, h.Gun, h.Choice, h.Mode, err)
				return
			}
			spec = &g
		}
		if h.Gun != p.held.Gun { // a draw: ShooterDataHolder.initialData
			p.flags, p.aim, p.tracers = 0, 0, projectile.Shooter{}
		}
		p.held, p.gun = h, spec
		s.broadcast(p.info().encode())
	case msgGun:
		if f := GunFlags(r.u8()); r.err == nil {
			p.flags = f
		}
	case msgView:
		v := View{FOV: r.f32(), Aspect: r.f32()}
		if r.err != nil || !v.valid() {
			s.cfg.Logf("mp: %s: bad view %+v: %v", p.name, v, r.err)
			return
		}
		p.view.FOV, p.view.Aspect = v.FOV, v.Aspect
	case msgCheatGuns:
		s.giveAllGuns(p)
	case msgMelee:
		gun, l := r.str(), Look{Yaw: r.f32(), Pitch: r.f32()}
		if r.err == nil {
			s.melee(p, gun, l)
		}
	case msgShoot:
		if sh := decodeShot(&r); r.err == nil {
			s.shoot(p, sh)
		}
	case msgSetBlock:
		e := decodeBlockEdit(&r)
		if r.err != nil || s.cfg.Reg.Parse(e.State) == voxel.Air && e.State != s.cfg.Reg.StateString(voxel.Air) {
			return // unknown state
		}
		if !s.world.SetBlock(e.X, e.Y, e.Z, s.cfg.Reg.Parse(e.State)) {
			return
		}
		// Every client gets the change (the editor already shows it; the
		// echo is harmless): those whose edit log stream is past the
		// position now, the others when the stream reaches it.
		i := s.edits.Set(e)
		for _, o := range s.players {
			if o != nil && i < o.sentTo {
				s.net.SendReliable(o.slot, e.encode(msgBlock))
			}
		}
	}
}

// streamEdits sends each client the edit log it has not had yet, a batch at
// a time while few reliable messages are in flight (a joiner's world
// download; Minecraft sends chunk data, we only send the changes).
func (s *Server) streamEdits() {
	n := s.edits.Len()
	for _, p := range s.players {
		if p == nil || p.sentTo >= n || s.net.Pending(p.slot) > 2 {
			continue
		}
		batch := s.edits.Range(p.sentTo, p.sentTo+256)
		for _, e := range batch {
			s.net.SendReliable(p.slot, e.encode(msgBlock))
		}
		p.sentTo += len(batch)
	}
}

func (s *Server) broadcast(msg []byte) {
	for _, p := range s.players {
		if p != nil {
			s.net.SendReliable(p.slot, msg)
		}
	}
}

func (s *Server) broadcastText(t string) { s.broadcast(append([]byte{msgText}, t...)) }

// respawn puts a player at a spawn point with full health.
func (s *Server) respawn(p *sPlayer) {
	if s.cfg.Spawn != nil {
		*p.ch = *character.New(s.cfg.Spawn(p.slot, s.world))
	} else {
		// Round the world spawn, on the ground (not on a tree or in water)
		// where there is some; a team on its own side (Red west, Blue
		// east).
		a, step := float64(p.slot)*math.Pi/2+float64(atomic.LoadUint32(&s.tick)%97), 0.9
		if p.team != 0 {
			a, step = math.Pi*float64(p.team-1)+float64(atomic.LoadUint32(&s.tick)%7)*0.1-0.3, 0.15
		}
		var x, y, z int
		for i := range 16 {
			r := spawnSpread * (0.5 + float64(i%4)/8)
			x = int(math.Floor(s.spawn[0] - math.Cos(a+float64(i)*step)*r))
			z = int(math.Floor(s.spawn[2] + math.Sin(a+float64(i)*step)*r))
			var ok bool
			if y, ok = s.world.Ground(x, z); ok && float64(y) <= s.spawnTop {
				break
			}
		}
		*p.ch = *character.New(float64(x)+0.5, float64(y), float64(z)+0.5)
	}
	p.health, p.dead, p.protect = MaxHealth, 0, ProtectTicks
	p.anim = Anim{}
}

// runCmd runs one command: the player's movement tick.
func (s *Server) runCmd(p *sPlayer, c Cmd) {
	p.last, p.ran = c, true
	p.yaw, p.pitch = c.Yaw, c.Pitch
	if lag := float64(atomic.LoadUint32(&s.tick)) - c.ViewTime; lag >= 0 {
		p.lag += (math.Min(lag, maxLag) - p.lag) * 0.25
	}
	if p.dead > 0 {
		return
	}
	in := c.Input()
	p.ch.MoveSpeed = character.Speed{}
	if p.gun != nil { // LivingEntitySpeedModifier
		p.ch.MoveSpeed = character.SpeedOf(character.PlayerMoveSpeed * float64(p.gun.SpeedFactor(p.flags&GunAiming != 0, p.flags&GunReloading != 0)))
	}
	x0, z0 := p.ch.X, p.ch.Z
	s.world.LoadAround(p.ch.X, p.ch.Z, 8)
	p.ch.Tick(s.world, in)
	p.anim.Walk(p.ch.X-x0, p.ch.Z-z0)
	p.anim.Pitch, p.anim.Sneak = c.Pitch, in.Sneak
	p.ch.EyeHeight = character.PlayerEye(in.Sneak)
}

// tickAim runs p's aiming progress for a server tick
// (LivingEntityAim.tickAimingProgress, with the held spec's aim time) and
// poses the body's gun hold with it.
func (s *Server) tickAim(p *sPlayer) {
	if p.gun == nil {
		p.aim = 0
	} else {
		p.aim = p.aim.Tick(p.flags&GunAiming != 0, p.gun.Aim, time.Second/TickRate)
		p.anim.Hold = p.gun.ThirdPersonHold
	}
	p.anim.Gun, p.anim.Aim = p.gun != nil, p.aim
}
