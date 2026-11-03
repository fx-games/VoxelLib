package mp

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/netcode"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/collide"
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
// and their arrows, and keeps the snapshots to draw everyone else from.
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
	own      PlayerState // from the newest snapshot
	ownAck   uint32      // the command it includes
	ownFresh bool        // not reconciled yet
	// The world time in the newest snapshot, its tick, and whether it
	// stands still.
	worldTime  int64
	timeTick   uint32
	timeFrozen bool
	reload     int
	shots      uint16
	mine       []*mineArrow
	Errors     int // reconciliations that moved the player (diagnostics)
}

type predicted struct {
	cmd   Cmd
	valid bool
	fired bool
}

type snapshot struct {
	tick    uint32
	players map[int]PlayerView
	arrows  []ArrowView
}

// mineArrow is an arrow the local player fired that the server has not
// confirmed yet: drawn at once, flown with the same physics.
type mineArrow struct {
	shot uint16
	a    *projectile.Arrow
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
	return &Client{Welcome: w, Edits: NewEditLog(), net: nc, nextCmd: 1, serverTime: float64(w.Tick)}, nil
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
	own := decodePlayerState(&r)
	if r.err != nil {
		return
	}
	snap, _, err := c.dec.Decode(seq, r.p)
	if err != nil {
		return // a delta against a frame we lost: the next one will do
	}
	s := snapshot{tick: tick, players: map[int]PlayerView{}}
	for id, st := range snap {
		if id >= arrowBase {
			if a, ok := decodeArrowView(id, st); ok {
				s.arrows = append(s.arrows, a)
			}
		} else if p, ok := decodePlayerView(st); ok {
			s.players[p.Slot] = p
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
	// Arrows the server has fired are drawn from its snapshots now.
	keep := c.mine[:0]
	for _, m := range c.mine {
		if m.shot > own.Shots && m.a.Flown < 2*TickRate {
			keep = append(keep, m)
		}
	}
	c.mine = keep
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
	c.reload, c.shots = c.own.Reload, c.own.Shots
	if !c.own.Dead {
		for n := c.ownAck + 1; n < c.nextCmd; n++ {
			p := &c.cmds[n%cmdRing]
			if !p.valid || p.cmd.Num != n {
				continue
			}
			ch.Tick(w, p.cmd.Input())
			c.weapon(p)
		}
	}
	d := math.Sqrt(sq(ch.X-x) + sq(ch.Y-y) + sq(ch.Z-z))
	if d > 1e-6 {
		c.Errors++
	}
	return d
}

// Command makes this tick's command from the player's input and look.
func (c *Client) Command(in character.Input, attack bool) Cmd {
	cmd := Cmd{Num: c.nextCmd, ViewTime: c.RenderTime(), Buttons: Buttons(in, attack), Yaw: in.Yaw, Pitch: in.Pitch}
	c.nextCmd++
	c.cmds[cmd.Num%cmdRing] = predicted{cmd: cmd, valid: true}
	return cmd
}

// Predicted finishes the tick of cmd after the character moved with its
// input: it predicts the crossbow (firing an arrow from the eye) and sends
// the newest commands. It returns whether a shot went off.
func (c *Client) Predicted(cmd Cmd, ch *character.Character, w collide.Solids) bool {
	p := &c.cmds[cmd.Num%cmdRing]
	fired := false
	if !c.own.Dead {
		fired = c.weapon(p)
	}
	if fired {
		var aw projectile.World
		ex, ey, ez := ch.Eye()
		c.mine = append(c.mine, &mineArrow{shot: c.shots, a: aw.Shoot(ex, ey-0.1, ez, cmd.Yaw, cmd.Pitch, projectile.Speed)})
	}
	// Our own arrows fly on locally until the server's take over.
	var aw projectile.World
	for _, m := range c.mine {
		aw.Arrows = append(aw.Arrows[:0], m.a)
		aw.Tick(w, nil)
	}
	var send []Cmd
	for n := cmd.Num - min(cmd.Num-1, cmdsPerPacket-1); n <= cmd.Num; n++ {
		if q := c.cmds[n%cmdRing]; q.valid && q.cmd.Num == n {
			send = append(send, q.cmd)
		}
	}
	c.net.Send(encodeCmds(send))
	return fired
}

// weapon runs the crossbow for one command, as the server does.
func (c *Client) weapon(p *predicted) bool {
	p.fired = false
	if p.cmd.Buttons&BtnAttack != 0 && c.reload == 0 {
		c.reload = ReloadTicks
		c.shots++
		p.fired = true
	}
	if c.reload > 0 {
		c.reload--
	}
	return p.fired
}

// Reload is the predicted ticks until the crossbow can fire.
func (c *Client) Reload() int { return c.reload }

// RenderTime is the server time (ticks) other players are drawn at.
func (c *Client) RenderTime() float64 { return c.serverTime - interpDelay }

// Rank is a line of the scoreboard.
type Rank struct {
	Score
	Name string
}

// Ranking lists the players in the game (this one too) by kills, most
// first, then by fewest deaths.
func (c *Client) Ranking() []Rank {
	var out []Rank
	for i, p := range c.Info {
		if p.Joined {
			out = append(out, Rank{Score: c.Scores[i], Name: p.Name})
		}
	}
	slices.SortStableFunc(out, func(a, b Rank) int {
		if a.Kills != b.Kills {
			return b.Kills - a.Kills
		}
		return a.Deaths - b.Deaths
	})
	return out
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
	t := c.RenderTime()
	i := len(c.snaps) - 1
	for i > 0 && float64(c.snaps[i].tick) > t {
		i--
	}
	a := &c.snaps[i]
	b := a
	if i+1 < len(c.snaps) {
		b = &c.snaps[i+1]
	}
	f := 0.0
	if b.tick > a.tick {
		f = math.Max(0, math.Min(1, (t-float64(a.tick))/float64(b.tick-a.tick)))
	}
	var out []PlayerView
	for slot, pb := range b.players {
		pa, ok := a.players[slot]
		if !ok || pa.Dead != pb.Dead {
			pa = pb
		}
		v := pb
		v.X, v.Y, v.Z = pa.X+(pb.X-pa.X)*f, pa.Y+(pb.Y-pa.Y)*f, pa.Z+(pb.Z-pa.Z)*f
		v.Yaw, v.Pitch = lerpAngle(pa.Yaw, pb.Yaw, f), lerpAngle(pa.Pitch, pb.Pitch, f)
		out = append(out, v)
	}
	return out
}

// ArrowPose is where to draw an arrow.
type ArrowPose struct {
	X, Y, Z, Yaw, Pitch float64
	Mine                bool // fired by the local player and not confirmed yet
	Fresh               bool // in its first two ticks (hidden near the eye)
}

// Arrows returns every arrow at the present: the server's run on from
// their newest snapshot with the arrow physics (Q3 evaluates missile
// trajectories the same way, so they need no interpolation delay), plus
// the local player's unconfirmed shots. alpha is the local tick fraction
// for those.
func (c *Client) Arrows(w collide.Solids, alpha float64, out []ArrowPose) []ArrowPose {
	out = out[:0]
	if len(c.snaps) > 0 {
		s := c.newest()
		dt := math.Max(0, math.Min(3, c.serverTime-float64(s.tick)))
		for _, a := range s.arrows {
			p := ArrowPose{X: a.X, Y: a.Y, Z: a.Z, Yaw: a.Yaw, Pitch: a.Pitch}
			if !a.Stuck {
				p.X, p.Y, p.Z = extrapolate(w, a, dt)
			}
			out = append(out, p)
		}
	}
	for _, m := range c.mine {
		x, y, z, yaw, pitch := m.a.Lerp(alpha)
		out = append(out, ArrowPose{X: x, Y: y, Z: z, Yaw: yaw, Pitch: pitch, Mine: true, Fresh: m.a.Flown < 2})
	}
	return out
}

// extrapolate runs a flying arrow dt ticks on, stopping short of solid
// blocks (the server's next snapshot says where it really stuck).
func extrapolate(w collide.Solids, a ArrowView, dt float64) (x, y, z float64) {
	x, y, z = a.X, a.Y, a.Z
	vx, vy, vz := a.VX, a.VY, a.VZ
	for dt > 0 {
		f := math.Min(1, dt)
		nx, ny, nz := x+vx*f, y+vy*f, z+vz*f
		if w != nil && w.Solid(int(math.Floor(nx)), int(math.Floor(ny)), int(math.Floor(nz))) {
			return x, y, z
		}
		x, y, z = nx, ny, nz
		vx, vy, vz = vx*projectile.Drag, vy*projectile.Drag-projectile.Gravity, vz*projectile.Drag
		dt -= f
	}
	return x, y, z
}
