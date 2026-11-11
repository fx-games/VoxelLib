package mp

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/weapon"
)

const (
	// interpDelay is how far behind the server's time other players are
	// drawn (ticks): between the two newest snapshots, with room for one
	// late packet (Q3's cl_timeNudge and snapshot interpolation).
	interpDelay = 1.5
	cmdRing     = 64 // commands kept for replay
	snapRing    = 32
)

// Client is a connection to a Server plus the client half of the game: it
// turns the local player's ticks into commands, predicts the local player
// and keeps the snapshots to draw everyone else from.
//
// Per tick, the game calls Reconcile then Command before its player moves,
// and Predicted after (all on the game's tick goroutine); Poll once per frame
// or tick to take in what arrived.
type Client struct {
	Welcome Welcome
	Edits   *EditLog // every block change from the server
	Info    [4]PlayerInfo
	Scores  [4]Score // by slot; see Ranking

	// Callbacks, run from Poll.
	OnBlock  func(BlockEdit)
	OnHit    func(Hit)
	OnText   func(string)
	OnPlayer func(PlayerInfo)
	OnFired  func(slot int) // another player's gun went off
	OnPickup func(Pickup)   // the local player took a pickup

	// Bullets are the server's flying bullets by ID, flown on by
	// TickBullets (the client-side entity tick) until the server says
	// they are gone.
	Bullets map[uint32]*RemoteBullet

	net  *netcode.Client
	dec  netcode.Decoder
	gone string // disconnect reason once disconnected

	// Snapshots, oldest first.
	snaps []snapshot
	// Server time estimate (ticks), advanced by the local clock.
	serverTime float64
	lastSnap   time.Time
	clockInit  bool

	// Prediction.
	cmds     [cmdRing]predicted
	nextCmd  uint32
	cmdSent  bool        // Predicted sent a command: shots may follow
	own      PlayerState // from the newest snapshot
	ownAck   uint32      // the command it includes
	ownFresh bool        // not reconciled yet
	// The world time in the newest snapshot, its tick, and whether it
	// stands still.
	worldTime  int64
	timeTick   uint32
	timeFrozen bool
	Errors     int // reconciliations that moved the player (diagnostics)
}

type predicted struct {
	cmd   Cmd
	valid bool
}

type snapshot struct {
	tick    uint32
	players map[int]PlayerView
	zombies map[uint32]ZombieView
	pickups []PickupView
	round   Round
}

// Connect joins the server at addr ("10.0.0.5:28960") as name.
func Connect(addr, name string, cfg netcode.Config) (*Client, error) {
	if !strings.Contains(addr, ":") {
		addr = fmt.Sprintf("%s:%d", addr, DefaultPort)
	}
	cfg.Protocol = Protocol
	nc, reply, err := netcode.Dial(addr, cfg, Hello{Name: name}.encode())
	if err != nil {
		return nil, err
	}
	w, err := decodeWelcome(reply)
	if err != nil {
		nc.Close()
		return nil, err
	}
	return &Client{Welcome: w, Edits: NewEditLog(), Bullets: map[uint32]*RemoteBullet{}, net: nc, nextCmd: 1, serverTime: float64(w.Tick),
		own: w.Own, worldTime: w.Time, timeTick: w.Tick, timeFrozen: w.TimeFrozen}, nil
}

// RemoteBullet is a bullet the server fired, as a client flies and draws it.
type RemoteBullet struct {
	Shooter int    // slot
	Gun     string // the shooter's gun ID
	*projectile.Bullet
}

// TickBullets flies the server's bullets one tick through w (motion only:
// the server decides what they hit); one past its life is dropped.
func (c *Client) TickBullets(w projectile.BulletWorld) {
	for id, b := range c.Bullets {
		if b.Tick(w, nil); b.Gone {
			delete(c.Bullets, id)
		}
	}
}

// Slot is the local player's slot.
func (c *Client) Slot() int { return c.Welcome.Slot }

// Close leaves the server.
func (c *Client) Close() { c.net.Close() }

// Gone returns why the connection ended, or "" while it is up.
func (c *Client) Gone() string { return c.gone }

// Stats describes the connection.
func (c *Client) Stats() netcode.Stats { return c.net.Stats() }

// SendSkin sets the local player's skin (png bytes) for everyone.
func (c *Client) SendSkin(png []byte) { c.net.SendReliable(append([]byte{msgSkin}, png...)) }

// SendHold tells the server which gun is in hand with its attachments;
// everyone sees it in PlayerInfo.Held.
func (c *Client) SendHold(h Hold) {
	b := wbuf{msgHold}
	h.encode(&b)
	c.net.SendReliable(b)
}

// SendGun tells the server the held gun's state; send it when that changes.
func (c *Client) SendGun(f GunFlags) { c.net.SendReliable([]byte{msgGun, uint8(f)}) }

// SendMelee lands the held gun's melee hit along look; the server deals the damage.
func (c *Client) SendMelee(gun string, look Look) {
	b := wbuf{msgMelee}
	b.str(gun)
	b.f32(look.Yaw)
	b.f32(look.Pitch)
	c.net.SendReliable(b)
}

// ErrNoCommand: a shot before any command, which the server would fire
// with no input (standing, not sneaking).
var ErrNoCommand = errors.New("mp: shot before the first command")

// SendShot fires a round of the held gun; the server deals the damage. It
// rides on the next command packet and runs before its commands, so it is
// fired with the input of the command sent last (Predicted).
func (c *Client) SendShot(s Shot) error {
	if !c.cmdSent {
		return ErrNoCommand
	}
	return c.net.SendReliable(s.encode())
}

// SendEdit reports a block the local player changed. It is logged at once
// so a column that loads later shows it, and the server passes it on.
func (c *Client) SendEdit(e BlockEdit) {
	c.Edits.Set(e)
	c.net.SendReliable(e.encode(msgSetBlock))
}

// Poll takes in what arrived: reliable events (through the callbacks) and
// snapshots.
func (c *Client) Poll() {
	for _, e := range c.net.Poll() {
		switch e.Kind {
		case netcode.Disconnected:
			c.gone = e.Reason
		case netcode.Reliable:
			c.reliable(e.Data)
		case netcode.Message:
			c.snapshot(e.Seq, e.Data)
		}
	}
	// The server's clock runs on: estimate it from the newest snapshot
	// plus the time since it arrived, smoothed so drawing stays even.
	if c.clockInit {
		est := float64(c.newest().tick) + time.Since(c.lastSnap).Seconds()*TickRate
		if d := est - c.serverTime; math.Abs(d) > 3 {
			c.serverTime = est
		} else {
			c.serverTime += d * 0.1
		}
	}
}

// Advance moves the server clock estimate on by dt (call once per frame
// before drawing).
func (c *Client) Advance(dt time.Duration) { c.serverTime += dt.Seconds() * TickRate }

func (c *Client) newest() *snapshot { return &c.snaps[len(c.snaps)-1] }

func (c *Client) reliable(msg []byte) {
	if len(msg) == 0 {
		return
	}
	r := rbuf{p: msg[1:]}
	switch msg[0] {
	case msgBlock:
		e := decodeBlockEdit(&r)
		if r.err == nil {
			c.Edits.Set(e)
			if c.OnBlock != nil {
				c.OnBlock(e)
			}
		}
	case msgHit:
		h := decodeHit(&r)
		if r.err == nil && c.OnHit != nil {
			c.OnHit(h)
		}
	case msgBullet:
		if b := decodeBulletSpawn(&r); r.err == nil {
			c.Bullets[b.ID] = &RemoteBullet{Shooter: b.Shooter, Gun: b.Gun, Bullet: &projectile.Bullet{Pos: b.Pos, Prev: b.Pos, Start: b.Pos, Vel: b.Vel,
				Gravity: float64(b.Gravity), Friction: float64(b.Friction), Life: b.Life, Pierce: 1, Tracer: b.Tracer}}
		}
	case msgBulletGone:
		if id := r.u32(); r.err == nil {
			delete(c.Bullets, id)
		}
	case msgPickup:
		if pk := decodePickup(&r); r.err == nil && c.OnPickup != nil {
			c.OnPickup(pk)
		}
	case msgFired:
		slot := int(r.u8())
		if r.err == nil && slot < len(c.Info) && c.OnFired != nil {
			c.OnFired(slot)
		}
	case msgText:
		if c.OnText != nil {
			c.OnText(string(msg[1:]))
		}
	case msgScore:
		sc := decodeScore(&r)
		if r.err == nil && sc.Slot >= 0 && sc.Slot < len(c.Scores) {
			c.Scores[sc.Slot] = sc
		}
	case msgPlayer:
		p := decodePlayerInfo(&r)
		if r.err == nil && p.Slot >= 0 && p.Slot < len(c.Info) {
			if !p.Joined || !c.Info[p.Slot].Joined {
				c.Scores[p.Slot] = Score{Slot: p.Slot} // a new player in the slot
			}
			c.Info[p.Slot] = p
			if c.OnPlayer != nil {
				c.OnPlayer(p)
			}
		}
	}
}

func (c *Client) snapshot(seq uint32, data []byte) {
	r := rbuf{p: data}
	tick, ack := r.u32(), r.u32()
	wt, wf := int64(r.f64()), r.u8()&1 != 0
	round := decodeRound(&r)
	own := decodePlayerState(&r)
	if r.err != nil {
		return
	}
	snap, _, err := c.dec.Decode(seq, r.p)
	if err != nil {
		return // a delta against a frame we lost: the next one will do
	}
	s := snapshot{tick: tick, players: map[int]PlayerView{}, zombies: map[uint32]ZombieView{}, round: round}
	for id, st := range snap {
		if len(st) == 0 {
			continue
		}
		switch entityKind(st[0]) {
		case kindPlayer:
			if p, ok := decodePlayerView(st); ok {
				s.players[p.Slot] = p
			}
		case kindZombie:
			if z, ok := decodeZombieView(id, st); ok {
				s.zombies[id] = z
			}
		case kindPickup:
			if pk, ok := decodePickupView(id, st); ok {
				s.pickups = append(s.pickups, pk)
			}
		}
	}
	if len(c.snaps) > 0 && tick <= c.newest().tick {
		return
	}
	c.snaps = append(c.snaps, s)
	if len(c.snaps) > snapRing {
		c.snaps = append(c.snaps[:0], c.snaps[len(c.snaps)-snapRing:]...)
	}
	c.own, c.ownAck, c.ownFresh = own, ack, true
	c.worldTime, c.timeTick, c.timeFrozen = wt, tick, wf
	c.lastSnap = time.Now()
	if !c.clockInit {
		c.serverTime, c.clockInit = float64(tick), true
	}
}

// Own is the local player's state from the newest snapshot (health,
// death).
func (c *Client) Own() PlayerState { return c.own }

// Ready reports whether a snapshot has arrived.
func (c *Client) Ready() bool { return len(c.snaps) > 0 }

// Reconcile resets the predicted character to the server's newest state
// for it and replays the commands the server had not run yet (Q3's
// CG_PredictPlayerState). It returns how far the prediction was off.
func (c *Client) Reconcile(ch *character.Character, w character.World) float64 {
	if !c.ownFresh {
		return 0
	}
	c.ownFresh = false
	x, y, z := ch.X, ch.Y, ch.Z
	c.own.Apply(ch)
	if !c.own.Dead {
		for n := c.ownAck + 1; n < c.nextCmd; n++ {
			p := &c.cmds[n%cmdRing]
			if !p.valid || p.cmd.Num != n {
				continue
			}
			ch.Tick(w, p.cmd.Input())
		}
	}
	d := math.Sqrt(sq(ch.X-x) + sq(ch.Y-y) + sq(ch.Z-z))
	if d > 1e-6 {
		c.Errors++
	}
	return d
}

// Command makes this tick's command from the player's input and look.
func (c *Client) Command(in character.Input) Cmd {
	cmd := Cmd{Num: c.nextCmd, ViewTime: c.RenderTime(), Buttons: Buttons(in), Yaw: in.Yaw, Pitch: in.Pitch}
	c.nextCmd++
	c.cmds[cmd.Num%cmdRing] = predicted{cmd: cmd, valid: true}
	return cmd
}

// RenderTime is the server time (ticks) other players are drawn at.
func (c *Client) RenderTime() float64 { return c.serverTime - interpDelay }

// Rank is a line of the scoreboard.
type Rank struct {
	Score
	Name string
	Team int
}

// Ranking lists the players in the game (this one too): by rounds won
// (battle royale), kills, most first, then by fewest deaths.
func (c *Client) Ranking() []Rank {
	var out []Rank
	for i, p := range c.Info {
		if p.Joined {
			out = append(out, Rank{Score: c.Scores[i], Name: p.Name, Team: p.Team})
		}
	}
	slices.SortStableFunc(out, func(a, b Rank) int {
		if a.Wins != b.Wins {
			return b.Wins - a.Wins
		}
		if a.Kills != b.Kills {
			return b.Kills - a.Kills
		}
		return a.Deaths - b.Deaths
	})
	return out
}

// Round is the match state in the newest snapshot, its zone where it
// stands at the render time (a closing wall moves smoothly).
func (c *Client) Round() Round {
	if len(c.snaps) == 0 {
		return c.Welcome.Round
	}
	r := c.newest().round
	t := c.RenderTime()
	for i := len(c.snaps) - 1; i > 0; i-- {
		a, b := &c.snaps[i-1], &c.snaps[i]
		if float64(a.tick) <= t && b.tick > a.tick && a.round.Phase == PhasePlaying && b.round.Phase == PhasePlaying {
			f := math.Min(1, (t-float64(a.tick))/float64(b.tick-a.tick))
			r.Zone = a.round.Zone.Lerp(b.round.Zone, f)
			break
		}
	}
	return r
}

// WorldTime is the server's world time now (see DayLength), whole ticks
// and the fraction into the next: run on from the last snapshot at the
// clock's pace.
func (c *Client) WorldTime() (t int64, partial float64) {
	if c.timeFrozen {
		return c.worldTime, 0
	}
	d := math.Max(0, c.serverTime-float64(c.timeTick))
	return c.worldTime + int64(d), d - math.Floor(d)
}

// ServerTime is the estimated present on the server (ticks).
func (c *Client) ServerTime() float64 { return c.serverTime }

// Players returns the other players at the render time, interpolated
// between the snapshots around it, and each one's walk animation.
func (c *Client) Players() []PlayerView {
	if len(c.snaps) == 0 {
		return nil
	}
	a, b, f := c.around()
	var out []PlayerView
	for slot, pb := range b.players {
		pa, ok := a.players[slot]
		if !ok || pa.Dead != pb.Dead {
			pa = pb
		}
		v := pb
		v.X, v.Y, v.Z = pa.X+(pb.X-pa.X)*f, pa.Y+(pb.Y-pa.Y)*f, pa.Z+(pb.Z-pa.Z)*f
		v.Yaw, v.Pitch = lerpAngle(pa.Yaw, pb.Yaw, f), lerpAngle(pa.Pitch, pb.Pitch, f)
		v.Aim = pa.Aim + (pb.Aim-pa.Aim)*weapon.AimProgress(f)
		out = append(out, v)
	}
	return out
}

// Send sends the newest commands, cmd last, after the character moved
// with cmd's input (call once per tick).
func (c *Client) Send(cmd Cmd) {
	var send []Cmd
	for n := cmd.Num - min(cmd.Num-1, cmdsPerPacket-1); n <= cmd.Num; n++ {
		if q := c.cmds[n%cmdRing]; q.valid && q.cmd.Num == n {
			send = append(send, q.cmd)
		}
	}
	c.net.Send(encodeCmds(send))
	c.cmdSent = true
}

// SendView tells the server the player's unzoomed projection (vertical
// field of view in radians, aspect ratio): zombie defense spawns outside it.
func (c *Client) SendView(fov, aspect float64) {
	b := wbuf{msgView}
	b.f32(fov)
	b.f32(aspect)
	c.net.SendReliable(b)
}

// around finds the snapshots around the render time and how far between
// them it is.
func (c *Client) around() (a, b *snapshot, f float64) {
	t := c.RenderTime()
	i := len(c.snaps) - 1
	for i > 0 && float64(c.snaps[i].tick) > t {
		i--
	}
	a = &c.snaps[i]
	b = a
	if i+1 < len(c.snaps) {
		b = &c.snaps[i+1]
	}
	if b.tick > a.tick {
		f = math.Max(0, math.Min(1, (t-float64(a.tick))/float64(b.tick-a.tick)))
	}
	return a, b, f
}

// Zombies returns the zombies at the render time, interpolated between the
// snapshots around it.
func (c *Client) Zombies(out []ZombieView) []ZombieView {
	out = out[:0]
	if len(c.snaps) == 0 {
		return out
	}
	a, b, f := c.around()
	for id, zb := range b.zombies {
		za, ok := a.zombies[id]
		if !ok {
			za = zb
		}
		v := zb
		v.X, v.Y, v.Z = za.X+(zb.X-za.X)*f, za.Y+(zb.Y-za.Y)*f, za.Z+(zb.Z-za.Z)*f
		v.Yaw = lerpAngle(za.Yaw, zb.Yaw, f)
		out = append(out, v)
	}
	return out
}

// Pickups are the pickups lying in the world in the newest snapshot.
func (c *Client) Pickups() []PickupView {
	if len(c.snaps) == 0 {
		return nil
	}
	return c.newest().pickups
}
