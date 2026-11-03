package mp

import (
	"fmt"
	"log"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel"
)

// Game rules.
const (
	MaxHealth    = 20
	ArrowDamage  = 6 // a body hit; four kill
	HeadDamage   = MaxHealth
	ReloadTicks  = 8 // the explorer's repeating crossbow: 2.5 shots a second
	RespawnTicks = 3 * TickRate
	// historyTicks of player poses are kept for lag compensation.
	historyTicks = 32
	// maxLag caps how far back a shooter's view is rewound (ticks).
	maxLag = 10
	// spawnSpread scatters spawn points around the world spawn (blocks).
	spawnSpread = 24
	// maxArrows in the world; the oldest stuck one goes first.
	maxArrows = 400
)

// ServerConfig sets up a Server.
type ServerConfig struct {
	Addr      string // UDP address to listen on (default ":28960")
	Name      string // shown in discovery
	Seed      int64
	Generator string // "overworld" or "flat"
	Reg       *voxel.Registry
	Net       netcode.Config // Protocol is set by the server
	// Spawn, when set, picks where a player (re)spawns: feet position.
	// Without it players spawn around the world spawn.
	Spawn func(slot int, w *World) (x, y, z float64)
	Logf  func(format string, args ...any)
}

// Server is the authoritative simulation (Q3's server game): it runs every
// player's commands as they arrive, flies the arrows, deals damage and sends
// each client a delta-compressed snapshot every tick. It runs on its own
// goroutine; a listen server's host connects to it like everyone else.
type Server struct {
	cfg   ServerConfig
	net   *netcode.Server
	world *World
	edits *EditLog
	body  *Body

	tick    uint32 // atomic: the AcceptFunc reads it
	spawn   [3]float64
	players [4]*sPlayer
	arrows  map[*projectile.Arrow]arrowMeta
	nextID  uint32

	stop chan struct{}
	done chan struct{}
	once sync.Once
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

	health int
	dead   int // ticks until respawn while dead
	reload int
	shots  uint16
	lag    float64 // smoothed ticks between the server's time and what the player saw

	hist   [historyTicks]pose
	arrows projectile.World
	enc    netcode.Encoder
	sentTo int // edit log entries queued for this client
}

// pose is a player at one server tick, for lag compensation.
type pose struct {
	tick    uint32
	x, y, z float64
	yaw     float64
	anim    Anim
	alive   bool
}

// arrowMeta is who shot an arrow, and its snapshot id.
type arrowMeta struct {
	id      uint32
	shooter int
	shot    uint16
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
	gen, err := NewGenerator(cfg.Reg, cfg.Generator, cfg.Seed)
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, edits: NewEditLog(), body: NewBody(), arrows: map[*projectile.Arrow]arrowMeta{},
		stop: make(chan struct{}), done: make(chan struct{})}
	s.world = NewWorld(cfg.Reg, s.edits.Wrap(gen))
	x, y, z := s.world.SpawnPoint(0, 0)
	s.spawn = [3]float64{x, y, z}
	s.world.LoadAround(x, z, 32)

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

// accept answers a connect request (on the network goroutine).
func (s *Server) accept(slot int, _ net.Addr, hello []byte) ([]byte, error) {
	if _, err := decodeHello(hello); err != nil {
		return nil, err
	}
	return Welcome{Slot: slot, Seed: s.cfg.Seed, Generator: s.cfg.Generator, Tick: atomic.LoadUint32(&s.tick),
		SpawnX: s.spawn[0], SpawnY: s.spawn[1], SpawnZ: s.spawn[2]}.encode(), nil
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
	return out
}

func (s *Server) run() {
	defer close(s.done)
	t := time.NewTicker(time.Second / TickRate)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.frame()
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
	for _, p := range s.players {
		if p == nil {
			continue
		}
		if p.dead > 0 {
			if p.dead--; p.dead == 0 {
				s.respawn(p)
			}
		}
		p.hist[tick%historyTicks] = pose{tick: tick, x: p.ch.X, y: p.ch.Y, z: p.ch.Z, yaw: p.yaw, anim: p.anim,
			alive: p.dead == 0}
	}
	s.tickArrows(tick)
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
		h, _ := decodeHello(e.Data)
		p := &sPlayer{slot: e.Client, name: h.Name, ch: character.New(0, 0, 0), health: MaxHealth}
		if p.name == "" {
			p.name = fmt.Sprintf("Player %d", e.Client+1)
		}
		s.players[e.Client] = p
		s.respawn(p)
		for _, o := range s.players {
			if o != nil && o != p {
				s.net.SendReliable(p.slot, PlayerInfo{Slot: o.slot, Joined: true, Name: o.name, Skin: o.skin}.encode())
			}
		}
		s.broadcast(PlayerInfo{Slot: p.slot, Joined: true, Name: p.name}.encode())
		s.broadcastText(p.name + " joined the game")
		s.setInfo()
		s.cfg.Logf("mp: %s joined (slot %d)", p.name, p.slot)
	case netcode.Disconnected:
		p := s.players[e.Client]
		if p == nil {
			return
		}
		s.players[e.Client] = nil
		for _, a := range p.arrows.Arrows {
			delete(s.arrows, a)
		}
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
		s.broadcast(PlayerInfo{Slot: p.slot, Joined: true, Name: p.name, Skin: p.skin}.encode())
	case msgSetBlock:
		e := decodeBlockEdit(&r)
		if r.err != nil || s.cfg.Reg.Parse(e.State) == voxel.Air && e.State != s.cfg.Reg.Info(voxel.Air).Name {
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
		a := float64(p.slot)*math.Pi/2 + float64(atomic.LoadUint32(&s.tick)%97)
		x := int(math.Floor(s.spawn[0] + math.Cos(a)*spawnSpread*0.5))
		z := int(math.Floor(s.spawn[2] + math.Sin(a)*spawnSpread*0.5))
		*p.ch = *character.New(float64(x)+0.5, float64(s.world.Surface(x, z)), float64(z)+0.5)
	}
	p.health, p.dead, p.reload = MaxHealth, 0, 0
	p.anim = Anim{}
}

// runCmd runs one command: the player's movement tick and the crossbow.
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
	x0, z0 := p.ch.X, p.ch.Z
	s.world.LoadAround(p.ch.X, p.ch.Z, 8)
	p.ch.Tick(s.world, in)
	p.anim.Walk(p.ch.X-x0, p.ch.Z-z0)
	p.anim.Pitch, p.anim.Sneak = c.Pitch, in.Sneak
	if c.Buttons&BtnAttack != 0 && p.reload == 0 {
		ex, ey, ez := p.ch.Eye()
		a := p.arrows.Shoot(ex, ey-0.1, ez, c.Yaw, c.Pitch, projectile.Speed)
		p.shots++
		s.nextID++
		s.arrows[a] = arrowMeta{id: s.nextID, shooter: p.slot, shot: p.shots}
		p.reload = ReloadTicks
	}
	if p.reload > 0 {
		p.reload--
	}
}
