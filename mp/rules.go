package mp

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"
)

// Mode is the game a server runs.
type Mode uint8

const (
	// Deathmatch: everyone against everyone, respawning; kills rank.
	Deathmatch Mode = iota
	// TeamDeathmatch: two teams, Red and Blue; no friendly fire; the
	// team's kills add up.
	TeamDeathmatch
	// BattleRoyale: rounds without respawns in a square playing field
	// whose energy wall closes in every ZoneInterval; the last one
	// standing wins the round.
	BattleRoyale
	modeCount
)

var modeNames = [modeCount][2]string{{"dm", "Deathmatch"}, {"tdm", "Team deathmatch"}, {"br", "Battle royale"}}

func (m Mode) String() string {
	if m < modeCount {
		return modeNames[m][1]
	}
	return fmt.Sprintf("Mode(%d)", m)
}

// ParseMode reads a mode by its short (dm, tdm, br) or long name.
func ParseMode(s string) (Mode, error) {
	for m, n := range modeNames {
		if strings.EqualFold(s, n[0]) || strings.EqualFold(s, n[1]) {
			return Mode(m), nil
		}
	}
	return 0, fmt.Errorf("mp: unknown mode %q (dm, tdm or br)", s)
}

// Teams of TeamDeathmatch (PlayerInfo.Team); 0 is no team.
const (
	TeamRed  = 1
	TeamBlue = 2
)

// TeamName is a team's name, TeamColor its formatting code.
func TeamName(t int) string {
	switch t {
	case TeamRed:
		return "Red"
	case TeamBlue:
		return "Blue"
	}
	return ""
}

func TeamColor(t int) string {
	switch t {
	case TeamRed:
		return "§c"
	case TeamBlue:
		return "§9"
	}
	return ""
}

// Battle royale timing.
const (
	// DefaultZoneInterval is how long the wall stands before closing in.
	DefaultZoneInterval = 5 * time.Minute
	// DefaultShrinkTime is how long the wall takes to close in.
	DefaultShrinkTime = 30 * time.Second
	// CountdownTicks before a round starts.
	CountdownTicks = 10 * TickRate
	// RoundOverTicks the winner is shown before the next countdown.
	RoundOverTicks = 8 * TickRate
	// zoneMargin pads the square round the players at the start; the
	// first square is at least zoneMinSide.
	zoneMargin  = 16
	zoneMinSide = 48
	// Each closing leaves zoneShrink of the side; below zoneLastSide
	// the wall closes completely.
	zoneShrink   = 0.6
	zoneLastSide = 12
)

// Phase is where a battle royale round is.
type Phase uint8

const (
	PhaseWaiting   Phase = iota // fewer than two players: play freely
	PhaseCountdown              // the round starts when Ticks run out
	PhasePlaying                // the zone stands (Ticks until it closes) or closes (Shrinking)
	PhaseOver                   // the round is won; the next countdown follows
)

// Zone is the battle royale playing field: inside the square, the wall at
// its edges.
type Zone struct{ MinX, MinZ, MaxX, MaxZ float64 }

// Lerp is the zone t of the way to b.
func (z Zone) Lerp(b Zone, t float64) Zone {
	return Zone{z.MinX + (b.MinX-z.MinX)*t, z.MinZ + (b.MinZ-z.MinZ)*t, z.MaxX + (b.MaxX-z.MaxX)*t, z.MaxZ + (b.MaxZ-z.MaxZ)*t}
}

// Side is the zone's width.
func (z Zone) Side() float64 { return z.MaxX - z.MinX }

// Touches reports whether a body of half width r at x,z touches the wall
// or stands outside.
func (z Zone) Touches(x, zz, r float64) bool {
	return x-r <= z.MinX || x+r >= z.MaxX || zz-r <= z.MinZ || zz+r >= z.MaxZ
}

// Round is the match state every snapshot carries.
type Round struct {
	Mode      Mode
	Phase     Phase
	Ticks     int  // left in the countdown, until the wall closes in, of the closing, or of the round's end
	Shrinking bool // the wall is closing in
	Alive     int  // players still in the round
	Zone      Zone // battle royale while playing
	RedKills  int  // team deathmatch totals
	BlueKills int
}

func (r Round) encode(b *wbuf) {
	b.u8(uint8(r.Mode))
	b.u8(uint8(r.Phase) | flag(r.Shrinking, 0x10))
	b.u32(uint32(r.Ticks))
	b.u8(uint8(r.Alive))
	switch r.Mode {
	case BattleRoyale:
		b.f64(r.Zone.MinX)
		b.f64(r.Zone.MinZ)
		b.f64(r.Zone.MaxX)
		b.f64(r.Zone.MaxZ)
	case TeamDeathmatch:
		b.u16(uint16(r.RedKills))
		b.u16(uint16(r.BlueKills))
	}
}

func decodeRound(r *rbuf) Round {
	rd := Round{Mode: Mode(r.u8())}
	f := r.u8()
	rd.Phase, rd.Shrinking = Phase(f&0xF), f&0x10 != 0
	rd.Ticks, rd.Alive = int(r.u32()), int(r.u8())
	switch rd.Mode {
	case BattleRoyale:
		rd.Zone = Zone{r.f64(), r.f64(), r.f64(), r.f64()}
	case TeamDeathmatch:
		rd.RedKills, rd.BlueKills = int(r.u16()), int(r.u16())
	}
	return rd
}

// match is the server's side of the rules.
type match struct {
	round       Round
	from, to    Zone // a closing wall's start and end
	shrinkLeft  int
	interval    int // ticks the wall stands
	shrinkTicks int // ticks it takes to close in
}

// colored is a player's name in their team's colour.
func (p *sPlayer) colored() string {
	if c := TeamColor(p.team); c != "" {
		return c + p.name + "§r"
	}
	return p.name
}

// pickTeam is the smaller team (Red on a tie).
func (s *Server) pickTeam() int {
	var n [3]int
	for _, p := range s.players {
		if p != nil {
			n[p.team]++
		}
	}
	if n[TeamBlue] < n[TeamRed] {
		return TeamBlue
	}
	return TeamRed
}

// enemies reports whether a can hurt b.
func enemies(a, b *sPlayer) bool { return a != b && (a.team == 0 || a.team != b.team) }

// kill ends victim's life: by killer (nil: the wall), counted and told as
// "killer how victim detail".
func (s *Server) kill(victim, killer *sPlayer, how, detail string) {
	victim.health = 0
	victim.dead = RespawnTicks
	victim.deaths++
	var feed string
	if killer != nil {
		feed = fmt.Sprintf("%s %s %s%s", killer.colored(), how, victim.colored(), detail)
		if killer != victim {
			killer.kills++
			s.broadcast(killer.score().encode())
		}
	} else {
		feed = victim.colored() + " " + how + detail
	}
	if s.cfg.Mode == BattleRoyale && s.m.round.Phase == PhasePlaying {
		victim.out = true
		feed += fmt.Sprintf(" (%d left)", s.alive())
	}
	s.broadcast(victim.score().encode())
	s.broadcastText(feed)
	s.cfg.Logf("mp: %s", feed)
}

// alive counts the players still in a battle royale round.
func (s *Server) alive() int {
	n := 0
	for _, p := range s.players {
		if p != nil && !p.out && p.dead == 0 {
			n++
		}
	}
	return n
}

func (s *Server) count() int {
	n := 0
	for _, p := range s.players {
		if p != nil {
			n++
		}
	}
	return n
}

// tickMatch runs the mode's rules for one tick.
func (s *Server) tickMatch() {
	m := &s.m
	m.round.Mode = s.cfg.Mode
	switch s.cfg.Mode {
	case TeamDeathmatch:
		m.round.RedKills, m.round.BlueKills = 0, 0
		for _, p := range s.players {
			if p != nil && p.team == TeamRed {
				m.round.RedKills += p.kills
			} else if p != nil && p.team == TeamBlue {
				m.round.BlueKills += p.kills
			}
		}
	case BattleRoyale:
		s.tickRoyale()
	}
}

func (s *Server) tickRoyale() {
	m := &s.m
	r := &m.round
	if r.Ticks > 0 {
		r.Ticks--
	}
	switch r.Phase {
	case PhaseWaiting:
		if s.count() >= 2 {
			r.Phase, r.Ticks = PhaseCountdown, CountdownTicks
			s.broadcastText(fmt.Sprintf("§eThe round starts in %d seconds", CountdownTicks/TickRate))
		}
	case PhaseCountdown:
		if s.count() < 2 {
			r.Phase = PhaseWaiting
			s.broadcastText("§eWaiting for players")
		} else if r.Ticks == 0 {
			s.startRound()
		}
	case PhasePlaying:
		if r.Shrinking {
			m.shrinkLeft--
			r.Zone = m.from.Lerp(m.to, 1-float64(m.shrinkLeft)/float64(m.shrinkTicks))
			r.Ticks = m.shrinkLeft
			if m.shrinkLeft <= 0 {
				r.Zone, r.Shrinking, r.Ticks = m.to, false, m.interval
			}
		} else if r.Ticks == 0 {
			s.closeIn()
		}
		for _, p := range s.players {
			if p != nil && !p.out && p.dead == 0 && r.Zone.Touches(p.ch.X, p.ch.Z, PlayerHalfWidth) {
				s.kill(p, nil, "touched the energy wall", "")
			}
		}
		r.Alive = s.alive()
		if r.Alive <= 1 {
			s.endRound()
		}
	case PhaseOver:
		if r.Ticks == 0 {
			s.reviveAll()
			r.Phase = PhaseWaiting // the next countdown starts with two players
		}
	}
}

// PlayerHalfWidth is half the player's width (character.PlayerWidth).
const PlayerHalfWidth = 0.3

// startRound brings everyone back and puts the wall round them.
func (s *Server) startRound() {
	m := &s.m
	minX, minZ, maxX, maxZ := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range s.players {
		if p == nil {
			continue
		}
		p.out = false
		s.respawn(p)
		minX, maxX = math.Min(minX, p.ch.X), math.Max(maxX, p.ch.X)
		minZ, maxZ = math.Min(minZ, p.ch.Z), math.Max(maxZ, p.ch.Z)
	}
	side := math.Max(math.Max(maxX-minX, maxZ-minZ)+2*zoneMargin, zoneMinSide)
	cx, cz := (minX+maxX)/2, (minZ+maxZ)/2
	m.round = Round{Mode: BattleRoyale, Phase: PhasePlaying, Ticks: m.interval, Alive: s.alive(),
		Zone: Zone{cx - side/2, cz - side/2, cx + side/2, cz + side/2}}
	s.broadcastText(fmt.Sprintf("§eFight! The energy wall closes in every %s", fmtTicks(m.interval)))
	s.cfg.Logf("mp: round started, %d players, zone %.0f blocks", m.round.Alive, side)
}

// closeIn starts the wall's move to a smaller square inside the current.
func (s *Server) closeIn() {
	m := &s.m
	z := m.round.Zone
	side := z.Side() * zoneShrink
	if side < zoneLastSide {
		side = 0
	}
	cx := z.MinX + side/2 + rand.Float64()*(z.Side()-side)
	cz := z.MinZ + side/2 + rand.Float64()*(z.Side()-side)
	m.from, m.to = z, Zone{cx - side/2, cz - side/2, cx + side/2, cz + side/2}
	m.shrinkLeft = m.shrinkTicks
	m.round.Shrinking, m.round.Ticks = true, m.shrinkTicks
	s.broadcastText("§eThe energy wall is closing in!")
}

func (s *Server) endRound() {
	m := &s.m
	var winner *sPlayer
	for _, p := range s.players {
		if p != nil && !p.out && p.dead == 0 {
			winner = p
		}
	}
	if winner != nil {
		winner.wins++
		s.broadcast(winner.score().encode())
		s.broadcastText("§e" + winner.name + " wins the round!")
		s.cfg.Logf("mp: %s wins the round", winner.name)
	} else {
		s.broadcastText("§eNobody survives the round")
	}
	m.round.Phase, m.round.Ticks, m.round.Shrinking = PhaseOver, RoundOverTicks, false
}

// reviveAll brings back everyone out of the last round.
func (s *Server) reviveAll() {
	for _, p := range s.players {
		if p != nil && p.out {
			p.out = false
			s.respawn(p)
		}
	}
}

// joinedLate puts a player who joins a running round out of it: they
// watch until the next.
func (s *Server) joinedLate(p *sPlayer) {
	if s.cfg.Mode == BattleRoyale && (s.m.round.Phase == PhasePlaying || s.m.round.Phase == PhaseOver) {
		p.out, p.dead, p.health = true, RespawnTicks, 0
	}
}

func fmtTicks(t int) string {
	d := time.Duration(t) * time.Second / TickRate
	if d%time.Minute == 0 {
		if d == time.Minute {
			return "minute"
		}
		return fmt.Sprintf("%d minutes", d/time.Minute)
	}
	return d.String()
}
