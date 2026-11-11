package mp

import (
	"encoding/binary"
	"errors"
	"github.com/memmaker/voxellib/weapon"
	"math"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/importer/tacz"
)

// Protocol is the netcode protocol id: "VLMP" plus a version byte. Bump the
// version on any wire change; older peers are refused and stay invisible to
// discovery.
const Protocol = 0x564C4D00 | 9

// DefaultPort is the server's UDP port.
const DefaultPort = 28960

// TickRate is the simulation and snapshot rate (the engine's 20 Hz).
const TickRate = 20

var errShort = errors.New("mp: short message")

// wbuf appends little-endian values.
type wbuf []byte

func (b *wbuf) u8(v uint8)    { *b = append(*b, v) }
func (b *wbuf) u16(v uint16)  { *b = binary.LittleEndian.AppendUint16(*b, v) }
func (b *wbuf) u32(v uint32)  { *b = binary.LittleEndian.AppendUint32(*b, v) }
func (b *wbuf) i32(v int32)   { b.u32(uint32(v)) }
func (b *wbuf) f32(v float64) { b.u32(math.Float32bits(float32(v))) }
func (b *wbuf) f64(v float64) { *b = binary.LittleEndian.AppendUint64(*b, math.Float64bits(v)) }
func (b *wbuf) str(s string) {
	b.u16(uint16(len(s)))
	*b = append(*b, s[:min(len(s), math.MaxUint16)]...)
}
func (b *wbuf) bytes(p []byte) {
	b.u32(uint32(len(p)))
	*b = append(*b, p...)
}

// rbuf reads what wbuf wrote; a short read sets err and returns zeros.
type rbuf struct {
	p   []byte
	err error
}

func (r *rbuf) take(n int) []byte {
	if r.err != nil || len(r.p) < n {
		r.err = errShort
		return make([]byte, n)
	}
	s := r.p[:n]
	r.p = r.p[n:]
	return s
}
func (r *rbuf) u8() uint8    { return r.take(1)[0] }
func (r *rbuf) u16() uint16  { return binary.LittleEndian.Uint16(r.take(2)) }
func (r *rbuf) u32() uint32  { return binary.LittleEndian.Uint32(r.take(4)) }
func (r *rbuf) i32() int32   { return int32(r.u32()) }
func (r *rbuf) f32() float64 { return float64(math.Float32frombits(r.u32())) }
func (r *rbuf) f64() float64 { return math.Float64frombits(binary.LittleEndian.Uint64(r.take(8))) }
func (r *rbuf) str() string  { return string(r.take(int(r.u16()))) }
func (r *rbuf) bytes() []byte {
	n := r.u32()
	if uint64(n) > uint64(len(r.p)) {
		r.err = errShort
		return nil
	}
	return append([]byte(nil), r.take(int(n))...)
}

// Hello is what a client sends with its connect request.
type Hello struct{ Name string }

func (h Hello) encode() []byte {
	var b wbuf
	b.str(h.Name)
	return b
}

func decodeHello(p []byte) (Hello, error) {
	r := rbuf{p: p}
	h := Hello{Name: r.str()}
	return h, r.err
}

// Welcome is the server's answer to a connect: everything a client needs to
// build the same world.
type Welcome struct {
	Slot      int
	Seed      int64
	Generator string // "overworld", "flat" or "map:<name>"
	Mode      Mode
	Tick      uint32
	SpawnX    float64
	SpawnY    float64
	SpawnZ    float64
}

func (w Welcome) encode() []byte {
	var b wbuf
	b.u8(uint8(w.Slot))
	b.u32(uint32(w.Seed))
	b.u32(uint32(w.Seed >> 32))
	b.str(w.Generator)
	b.u32(w.Tick)
	b.f64(w.SpawnX)
	b.f64(w.SpawnY)
	b.f64(w.SpawnZ)
	b.u8(uint8(w.Mode))
	return b
}

func decodeWelcome(p []byte) (Welcome, error) {
	r := rbuf{p: p}
	w := Welcome{Slot: int(r.u8())}
	lo, hi := r.u32(), r.u32()
	w.Seed = int64(uint64(hi)<<32 | uint64(lo))
	w.Generator, w.Tick = r.str(), r.u32()
	w.SpawnX, w.SpawnY, w.SpawnZ = r.f64(), r.f64(), r.f64()
	w.Mode = Mode(r.u8())
	return w, r.err
}

// Cmd is one tick of a player's input (Q3's usercmd_t). Clients send their
// newest cmdsPerPacket in every packet, so a lost packet loses no input;
// the server runs each new one once, in order, as it arrives.
type Cmd struct {
	Num        uint32  // increases by one per client tick
	ViewTime   float64 // server time (ticks) of the world the player saw: lag compensation
	Buttons    uint8
	Yaw, Pitch float64
}

// Cmd buttons.
const (
	BtnForward = 1 << iota
	BtnBack
	BtnLeft
	BtnRight
	BtnJump
	BtnSneak
	BtnSprint
	BtnAttack
)

const cmdsPerPacket = 8

// Input is the character controller input of a command.
func (c Cmd) Input() character.Input {
	axis := func(pos, neg uint8) float64 {
		v := 0.0
		if c.Buttons&pos != 0 {
			v++
		}
		if c.Buttons&neg != 0 {
			v--
		}
		return v
	}
	return character.Input{Forward: axis(BtnForward, BtnBack), Strafe: axis(BtnRight, BtnLeft),
		Jump: c.Buttons&BtnJump != 0, Sneak: c.Buttons&BtnSneak != 0, Sprint: c.Buttons&BtnSprint != 0,
		Yaw: c.Yaw, Pitch: c.Pitch}
}

// Buttons packs a controller input (and the attack button) into cmd buttons.
func Buttons(in character.Input, attack bool) uint8 {
	var b uint8
	set := func(on bool, bit uint8) {
		if on {
			b |= bit
		}
	}
	set(in.Forward > 0, BtnForward)
	set(in.Forward < 0, BtnBack)
	set(in.Strafe > 0, BtnRight)
	set(in.Strafe < 0, BtnLeft)
	set(in.Jump, BtnJump)
	set(in.Sneak, BtnSneak)
	set(in.Sprint, BtnSprint)
	set(attack, BtnAttack)
	return b
}

func encodeCmds(cmds []Cmd) []byte {
	var b wbuf
	b.u8(uint8(len(cmds)))
	for _, c := range cmds {
		b.u32(c.Num)
		b.f32(c.ViewTime)
		b.u8(c.Buttons)
		b.f32(c.Yaw)
		b.f32(c.Pitch)
	}
	return b
}

func decodeCmds(p []byte) ([]Cmd, error) {
	r := rbuf{p: p}
	n := int(r.u8())
	cmds := make([]Cmd, 0, n)
	for range n {
		cmds = append(cmds, Cmd{Num: r.u32(), ViewTime: r.f32(), Buttons: r.u8(), Yaw: r.f32(), Pitch: r.f32()})
	}
	return cmds, r.err
}

// PlayerState is a player's own state at full precision (Q3's
// playerState_t), sent to that player only: the client resets its predicted
// character to it and replays the commands the server has not run yet.
type PlayerState struct {
	X, Y, Z, VX, VY, VZ float64
	OnGround, Sprinting bool
	HorizontalCollision bool
	FallDistance        float64
	Health              int
	Dead                bool
	Protected           bool // spawn protection: can't be hurt
	Reload              int  // ticks until the crossbow can fire
	Shots               uint16
}

// Apply copies the state into a character.
func (s PlayerState) Apply(c *character.Character) {
	c.X, c.Y, c.Z, c.VX, c.VY, c.VZ = s.X, s.Y, s.Z, s.VX, s.VY, s.VZ
	c.OnGround, c.Sprinting, c.HorizontalCollision, c.FallDistance = s.OnGround, s.Sprinting, s.HorizontalCollision, s.FallDistance
}

func stateOf(c *character.Character) PlayerState {
	return PlayerState{X: c.X, Y: c.Y, Z: c.Z, VX: c.VX, VY: c.VY, VZ: c.VZ,
		OnGround: c.OnGround, Sprinting: c.Sprinting, HorizontalCollision: c.HorizontalCollision, FallDistance: c.FallDistance}
}

func flag(on bool, bit uint8) uint8 {
	if on {
		return bit
	}
	return 0
}

func (s PlayerState) encode(b *wbuf) {
	b.f64(s.X)
	b.f64(s.Y)
	b.f64(s.Z)
	b.f64(s.VX)
	b.f64(s.VY)
	b.f64(s.VZ)
	b.u8(flag(s.OnGround, 1) | flag(s.Sprinting, 2) | flag(s.HorizontalCollision, 4) | flag(s.Dead, 8) | flag(s.Protected, 16))
	b.f32(s.FallDistance)
	b.u8(uint8(s.Health))
	b.u8(uint8(s.Reload))
	b.u16(s.Shots)
}

func decodePlayerState(r *rbuf) PlayerState {
	s := PlayerState{X: r.f64(), Y: r.f64(), Z: r.f64(), VX: r.f64(), VY: r.f64(), VZ: r.f64()}
	f := r.u8()
	s.OnGround, s.Sprinting, s.HorizontalCollision, s.Dead = f&1 != 0, f&2 != 0, f&4 != 0, f&8 != 0
	s.Protected = f&16 != 0
	s.FallDistance = r.f32()
	s.Health, s.Reload, s.Shots = int(r.u8()), int(r.u8()), r.u16()
	return s
}

// Snapshot entity ids: players are their slot+1, arrows arrowBase and up.
const arrowBase = 1 << 16

// Entity kinds, the first byte of an entity state.
const (
	kindPlayer = 1
	kindArrow  = 2
)

// PlayerView is another player as the snapshot shows them.
type PlayerView struct {
	Slot       int
	X, Y, Z    float64
	Yaw, Pitch float64
	Sneak      bool
	Dead       bool
	Protected  bool // spawn protection: can't be hurt
	Health     int
	Shots      uint16 // counts shots fired: a change means one went off
}

func (p PlayerView) encode() []byte {
	var b wbuf
	b.u8(kindPlayer)
	b.u8(uint8(p.Slot))
	b.f32(p.X)
	b.f32(p.Y)
	b.f32(p.Z)
	b.u16(angle16(p.Yaw))
	b.u16(angle16(p.Pitch))
	b.u8(flag(p.Sneak, 1) | flag(p.Dead, 2) | flag(p.Protected, 4))
	b.u8(uint8(p.Health))
	b.u16(p.Shots)
	return b
}

func decodePlayerView(p []byte) (PlayerView, bool) {
	r := rbuf{p: p}
	if r.u8() != kindPlayer {
		return PlayerView{}, false
	}
	v := PlayerView{Slot: int(r.u8()), X: r.f32(), Y: r.f32(), Z: r.f32(), Yaw: unangle16(r.u16()), Pitch: unangle16(r.u16())}
	f := r.u8()
	v.Sneak, v.Dead, v.Protected = f&1 != 0, f&2 != 0, f&4 != 0
	v.Health, v.Shots = int(r.u8()), r.u16()
	return v, r.err == nil
}

// angle16 maps an angle to 65536 steps (Q3's ANGLE2SHORT).
func angle16(a float64) uint16 {
	return uint16(int64(math.Round(a/(2*math.Pi)*65536)) & 0xFFFF)
}

func unangle16(v uint16) float64 {
	a := float64(int16(v)) / 65536 * 2 * math.Pi
	return a
}

// ArrowView is an arrow as the snapshot shows it: its full flight state, so
// a client can run it on to the present with the same physics.
type ArrowView struct {
	ID         uint32
	X, Y, Z    float64
	VX, VY, VZ float64
	Yaw, Pitch float64
	Stuck      bool
	Shooter    int
	Shot       uint16 // the shooter's Shots count when it went off
}

func (a ArrowView) encode() []byte {
	var b wbuf
	b.u8(kindArrow)
	b.f32(a.X)
	b.f32(a.Y)
	b.f32(a.Z)
	b.f32(a.VX)
	b.f32(a.VY)
	b.f32(a.VZ)
	b.u16(angle16(a.Yaw))
	b.u16(angle16(a.Pitch))
	b.u8(flag(a.Stuck, 1))
	b.u8(uint8(a.Shooter))
	b.u16(a.Shot)
	return b
}

func decodeArrowView(id uint32, p []byte) (ArrowView, bool) {
	r := rbuf{p: p}
	if r.u8() != kindArrow {
		return ArrowView{}, false
	}
	a := ArrowView{ID: id, X: r.f32(), Y: r.f32(), Z: r.f32(), VX: r.f32(), VY: r.f32(), VZ: r.f32(),
		Yaw: unangle16(r.u16()), Pitch: unangle16(r.u16())}
	a.Stuck = r.u8()&1 != 0
	a.Shooter, a.Shot = int(r.u8()), r.u16()
	return a, r.err == nil
}

// Reliable message types.
const (
	// client → server
	msgSkin      = 1 // png bytes: the player's skin
	msgSetBlock  = 2 // x, y, z i32, state name: a block the player broke or placed
	msgRespawnMe = 3 // (unused: respawn is automatic)
	msgHold      = 4 // Hold: the gun in the player's hand
	msgShoot     = 5 // Shot: a round of the held gun
	msgMelee     = 6 // gun ID, yaw, pitch f32: the held gun's melee hit lands
	// server → client
	msgPlayer = 10 // slot u8, joined u8, name, skin png: a player joined, changed skin or left
	msgBlock  = 11 // x, y, z i32, state name: a block changed
	msgHit    = 12 // attacker u8, victim u8, damage u8, flags (head, killed, gun)
	msgText   = 13 // a line for the chat/kill feed
	msgScore  = 14 // slot u8, kills u16, deaths u16: a player's score changed
	msgFired  = 15 // slot u8: that player's gun went off (not sent to the shooter)
)

// BlockEdit is one block change.
type BlockEdit struct {
	X, Y, Z int
	State   string // voxel.Registry.StateString of the new state
}

func (e BlockEdit) encode(typ uint8) []byte {
	b := wbuf{typ}
	b.i32(int32(e.X))
	b.i32(int32(e.Y))
	b.i32(int32(e.Z))
	b.str(e.State)
	return b
}

func decodeBlockEdit(r *rbuf) BlockEdit {
	return BlockEdit{X: int(r.i32()), Y: int(r.i32()), Z: int(r.i32()), State: r.str()}
}

// Hit reports damage dealt by an arrow.
type Hit struct {
	Attacker, Victim int
	Damage           int
	Head, Killed     bool
	Gun              bool // a bullet: the shooter plays TACZ's hit marks
	Melee            bool // a gun's melee attack
}

func (h Hit) encode() []byte {
	b := wbuf{msgHit}
	b.u8(uint8(h.Attacker))
	b.u8(uint8(h.Victim))
	b.u8(uint8(h.Damage))
	b.u8(flag(h.Head, 1) | flag(h.Killed, 2) | flag(h.Gun, 4) | flag(h.Melee, 8))
	return b
}

func decodeHit(r *rbuf) Hit {
	h := Hit{Attacker: int(r.u8()), Victim: int(r.u8()), Damage: int(r.u8())}
	f := r.u8()
	h.Head, h.Killed, h.Gun, h.Melee = f&1 != 0, f&2 != 0, f&4 != 0, f&8 != 0
	return h
}

// Score is a player's kills and deaths.
type Score struct {
	Slot          int
	Kills, Deaths int
	Wins          int // battle royale rounds won
}

func (s Score) encode() []byte {
	b := wbuf{msgScore}
	b.u8(uint8(s.Slot))
	b.u16(uint16(s.Kills))
	b.u16(uint16(s.Deaths))
	b.u16(uint16(s.Wins))
	return b
}

func decodeScore(r *rbuf) Score {
	return Score{Slot: int(r.u8()), Kills: int(r.u16()), Deaths: int(r.u16()), Wins: int(r.u16())}
}

// PlayerInfo is who is in a slot.
type PlayerInfo struct {
	Slot   int
	Joined bool // false: the slot was freed
	Name   string
	Skin   []byte // png, nil for the default
	Team   int    // TeamRed, TeamBlue or 0
	Held   Hold   // the gun in hand
}

func (p PlayerInfo) encode() []byte {
	b := wbuf{msgPlayer}
	b.u8(uint8(p.Slot))
	b.u8(flag(p.Joined, 1))
	b.str(p.Name)
	b.bytes(p.Skin)
	b.u8(uint8(p.Team))
	p.Held.encode(&b)
	return b
}

func decodePlayerInfo(r *rbuf) PlayerInfo {
	p := PlayerInfo{Slot: int(r.u8()), Joined: r.u8()&1 != 0, Name: r.str(), Skin: r.bytes(), Team: int(r.u8())}
	p.Held = decodeHold(r)
	return p
}

// Hold is the gun in a player's hand with its attachments and fire mode.
type Hold struct {
	Gun    string          // a weapon.Gun ID, "" for none
	Choice tacz.Choice     // the installed attachments (tacz.Pack.Gun)
	Mode   weapon.FireMode // the selected fire mode (tacz.Pack.Gun: fire_mode_adjust)
}

// encode writes the gun, its fire mode and its filled slots (adapter,
// attachment) in tacz.Slots order.
func (h Hold) encode(b *wbuf) {
	var set []tacz.Slot
	for _, s := range tacz.Slots {
		if h.Choice.Attachments[s] != "" || h.Choice.Adapters[s] != "" {
			set = append(set, s)
		}
	}
	b.str(h.Gun)
	b.u8(uint8(h.Mode))
	b.u8(uint8(len(set)))
	for _, s := range set {
		b.str(string(s))
		b.str(h.Choice.Adapters[s])
		b.str(h.Choice.Attachments[s])
	}
}

func decodeHold(r *rbuf) Hold {
	h := Hold{Gun: r.str(), Mode: weapon.FireMode(r.u8())}
	for range int(r.u8()) {
		s := tacz.Slot(r.str())
		if ad := r.str(); ad != "" {
			h.Choice.Adapt(s, ad)
		}
		if att := r.str(); att != "" {
			h.Choice.Attach(s, att)
		}
	}
	return h
}

// Look is a direction as yaw and pitch (radians, the camera's).
type Look struct{ Yaw, Pitch float64 }

// Dir is the look's unit vector (render.Camera.Forward).
func (l Look) Dir() [3]float64 {
	return [3]float64{math.Sin(l.Yaw) * math.Cos(l.Pitch), math.Sin(l.Pitch), -math.Cos(l.Yaw) * math.Cos(l.Pitch)}
}

// Shot is one round of the shooter's held gun: each pellet's direction
// from their eye (the client rolls the spread). The server casts the rays
// against the players as the shooter saw them and deals the damage.
type Shot struct {
	Gun     string // the held gun's ID
	Pellets []Look
}

func (s Shot) encode() []byte {
	b := wbuf{msgShoot}
	b.str(s.Gun)
	b.u8(uint8(min(len(s.Pellets), math.MaxUint8)))
	for _, l := range s.Pellets[:min(len(s.Pellets), math.MaxUint8)] {
		b.f32(l.Yaw)
		b.f32(l.Pitch)
	}
	return b
}

func decodeShot(r *rbuf) Shot {
	s := Shot{Gun: r.str()}
	n := int(r.u8())
	for range n {
		s.Pellets = append(s.Pellets, Look{Yaw: r.f32(), Pitch: r.f32()})
	}
	return s
}
