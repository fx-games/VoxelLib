// Package audio plays sounds the way the 1.7.10 reference does (SoundManager on paulscode/OpenAL):
//
//   - Mixer: a positional stereo mixer on oto. Voices have a volume, a pitch (resampled, 0.5..2),
//     linear distance attenuation over a range, equal-power panning for mono sources, and a
//     category whose volume applies live. Mixer.Read is device independent and testable.
//   - Registry: sound events with sounds.json semantics (categories, weighted variants, per-variant
//     volume/pitch/stream, "event" references, "replace").
//   - Sounds: the registry plus the mixer: Play("block.stone.step"-style event names, Params).
//   - MusicTicker: random delays between tracks, a situation (menu/game) and a Pick hook.
//
// Coordinates are world blocks. The listener yaw is in radians, 0 looks to -z, +x is on the right.
package audio

import (
	"encoding/binary"
	"io"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/jfreymuth/oggvorbis"
)

// SampleRate is the output rate; sources of other rates are resampled.
const SampleRate = 44100

const (
	maxVoices  = 28 // paulscode's default normal channels (1.7.10)
	maxStreams = 4  // and streaming channels
	latency    = 50 * time.Millisecond
)

// Categories are the 1.7.10 reference's SoundCategory names. "master" scales everything.
var Categories = []string{"master", "music", "record", "weather", "block", "hostile", "neutral", "player", "ambient"}

// Sound is decoded PCM: interleaved float32 samples (-1..1).
type Sound struct {
	Samples  []float32
	Channels int // 1 or 2
	Rate     int
}

// Decode reads a whole Ogg Vorbis file.
func Decode(r io.Reader) (*Sound, error) {
	s, f, err := oggvorbis.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return &Sound{Samples: s, Channels: f.Channels, Rate: f.SampleRate}, nil
}

// source yields frames by absolute index (non-decreasing apart from i-1).
type source interface {
	frame(i int) (l, r float32, ok bool)
	channels() int
	rate() int
	close()
}

func (s *Sound) channels() int { return s.Channels }
func (s *Sound) rate() int     { return s.Rate }
func (s *Sound) close()        {}
func (s *Sound) frame(i int) (float32, float32, bool) {
	if s.Channels == 2 {
		if 2*i+1 >= len(s.Samples) {
			return 0, 0, false
		}
		return s.Samples[2*i], s.Samples[2*i+1], true
	}
	if i >= len(s.Samples) {
		return 0, 0, false
	}
	return s.Samples[i], s.Samples[i], true
}

// stream decodes an Ogg Vorbis file while it plays (music, "stream": true).
type stream struct {
	c    io.Closer
	r    *oggvorbis.Reader
	base int // frame index of buf[0]
	buf  []float32
	tmp  []float32
	eof  bool
}

func newStream(rc io.ReadCloser) (*stream, error) {
	r, err := oggvorbis.NewReader(rc)
	if err != nil {
		rc.Close()
		return nil, err
	}
	if r.Channels() > 2 {
		rc.Close()
		return nil, errChannels
	}
	return &stream{c: rc, r: r, tmp: make([]float32, 4096*r.Channels())}, nil
}

func (s *stream) channels() int { return s.r.Channels() }
func (s *stream) rate() int     { return s.r.SampleRate() }
func (s *stream) close()        { s.c.Close() }
func (s *stream) frame(i int) (float32, float32, bool) {
	ch := s.r.Channels()
	for i >= s.base+len(s.buf)/ch {
		if s.eof {
			return 0, 0, false
		}
		if drop := i - 1 - s.base; drop > 0 { // keep frame i-1 for interpolation
			drop = min(drop, len(s.buf)/ch)
			s.buf = append(s.buf[:0], s.buf[drop*ch:]...)
			s.base += drop
		}
		// ponytail: decodes on the mixer goroutine; fine for 4 streams, add a read-ahead goroutine if Read stalls
		n, err := s.r.Read(s.tmp)
		s.buf = append(s.buf, s.tmp[:n]...)
		if err != nil {
			s.eof = true
		}
	}
	if i < s.base {
		i = s.base
	}
	j := (i - s.base) * ch
	if ch == 2 {
		return s.buf[j], s.buf[j+1], true
	}
	return s.buf[j], s.buf[j], true
}

type audioError string

func (e audioError) Error() string { return string(e) }

const errChannels = audioError("audio: only mono and stereo are supported")

// Voice is a playing sound. Its methods are safe to call from the game thread.
type Voice struct {
	m          *Mixer
	src        source // nil while loading
	pos        float64
	loop       bool
	streaming  bool
	positional bool
	x, y, z    float64
	rng        float64 // linear attenuation range in blocks; 0 = none
	vol        float32 // ISound volume × entry volume, before the category (unclamped)
	pitch      float32
	cat        string
	curL, curR float32 // smoothed gains, owned by Read
	fresh      bool
	dead       bool
}

// Mixer mixes voices into stereo float32 frames. It implements io.Reader.
type Mixer struct {
	mu               sync.Mutex // ponytail: held during Read; fine for 32 voices, split if profiling says otherwise
	voices           []*Voice
	lx, ly, lz, lyaw float64
	vols             map[string]float32
	ctx              *oto.Context
	player           *oto.Player
}

// New opens the default audio device and starts mixing.
func New() (*Mixer, error) {
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   SampleRate,
		ChannelCount: 2,
		Format:       oto.FormatFloat32LE,
		BufferSize:   latency,
	})
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "js" {
		<-ready // in the browser the context only resumes after a user gesture; oto waits for it
	}
	m := NewSilent()
	m.ctx = ctx
	m.player = ctx.NewPlayer(m)
	m.player.SetBufferSize(int(SampleRate * latency.Seconds() * 8))
	m.player.Play()
	return m, nil
}

// NewSilent returns a mixer without a device (headless runs, tests). Read still works.
func NewSilent() *Mixer { return &Mixer{vols: map[string]float32{}} }

// Close stops playback.
func (m *Mixer) Close() {
	if m.player != nil {
		m.player.Close()
	}
}

// SetListener sets the listener (the player's eyes) and its yaw in radians.
func (m *Mixer) SetListener(x, y, z, yaw float64) {
	m.mu.Lock()
	m.lx, m.ly, m.lz, m.lyaw = x, y, z, yaw
	m.mu.Unlock()
}

// Listener returns the listener position.
func (m *Mixer) Listener() (x, y, z float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lx, m.ly, m.lz
}

// SetVolume sets a category volume (0..1, default 1). It applies to playing voices too.
func (m *Mixer) SetVolume(category string, v float32) {
	m.mu.Lock()
	m.vols[category] = v
	m.mu.Unlock()
}

// Volume returns a category volume.
func (m *Mixer) Volume(category string) float32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.volume(category)
}

func (m *Mixer) volume(c string) float32 {
	if v, ok := m.vols[c]; ok {
		return v
	}
	return 1
}

// Params describe one playback like the reference's ISound.
type Params struct {
	X, Y, Z       float64
	Volume, Pitch float32 // Volume > 1 extends the range (16 blocks × Volume) but plays no louder
	Global        bool    // not positioned: centred, no attenuation (UI, music)
	NoAttenuation bool    // positioned (panned) but as loud at any distance (AttenuationType.NONE)
	Loop          bool    // repeat until stopped (buffered sounds only)
	Delay         int     // ticks to wait (Sounds.Play only; see DistanceDelay)
}

// At is Params for a positioned sound, as World.playSound(x, y, z, name, volume, pitch).
func At(x, y, z float64, volume, pitch float32) Params {
	return Params{X: x, Y: y, Z: z, Volume: volume, Pitch: pitch}
}

// UI is Params for a non-positioned sound (button clicks: UI(0.25, 1)).
func UI(volume, pitch float32) Params { return Params{Volume: volume, Pitch: pitch, Global: true} }

// Play plays a decoded sound in category "master" with the entry volume/pitch 1.
func (m *Mixer) Play(s *Sound, p Params) *Voice {
	v := m.newVoice(p, "master", 1, 1)
	v.src = s
	return m.add(v)
}

// newVoice applies 1.7.10's SoundManager.playSound: pitch = clamp(p×entry, 0.5, 2),
// range = 16 × max(1, p.Volume).
func (m *Mixer) newVoice(p Params, cat string, entryVol, entryPitch float64) *Voice {
	v := &Voice{m: m, fresh: true, cat: cat, loop: p.Loop, positional: !p.Global,
		x: p.X, y: p.Y, z: p.Z,
		vol:   float32(float64(p.Volume) * entryVol),
		pitch: float32(math.Min(2, math.Max(0.5, float64(p.Pitch)*entryPitch)))}
	if !p.Global && !p.NoAttenuation {
		v.rng = 16 * math.Max(1, float64(p.Volume))
	}
	return v
}

func (m *Mixer) add(v *Voice) *Voice {
	m.mu.Lock()
	n, oldest := 0, -1
	for i, o := range m.voices {
		if o.streaming == v.streaming {
			if oldest < 0 {
				oldest = i
			}
			n++
		}
	}
	if limit := map[bool]int{false: maxVoices, true: maxStreams}[v.streaming]; n >= limit {
		m.voices[oldest].kill()
		m.voices = append(m.voices[:oldest], m.voices[oldest+1:]...)
	}
	m.voices = append(m.voices, v)
	m.mu.Unlock()
	return v
}

func (v *Voice) kill() {
	if !v.dead && v.src != nil {
		v.src.close()
	}
	v.dead = true
}

// Move sets the voice position.
func (v *Voice) Move(x, y, z float64) {
	v.m.mu.Lock()
	v.x, v.y, v.z = x, y, z
	v.m.mu.Unlock()
}

// SetVolume changes the voice volume; its range stays as started.
func (v *Voice) SetVolume(vol float32) {
	v.m.mu.Lock()
	v.vol = vol
	v.m.mu.Unlock()
}

// Stop ends the voice.
func (v *Voice) Stop() {
	v.m.mu.Lock()
	v.kill()
	v.m.mu.Unlock()
}

// StopAll stops every voice (SoundHandler.stopSounds).
func (m *Mixer) StopAll() {
	m.mu.Lock()
	for _, v := range m.voices {
		v.kill()
	}
	m.mu.Unlock()
}

// Playing reports whether the voice is loading or still playing.
func (v *Voice) Playing() bool {
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	return !v.dead
}

// Attenuation is paulscode's linear model as the reference uses it: 1 at the source, 0 at rng blocks.
func Attenuation(d, rng float64) float64 {
	if rng <= 0 {
		return 1
	}
	return math.Max(0, math.Min(1, 1-d/rng))
}

// Pan returns equal-power left/right factors for a source seen by a listener at yaw.
func Pan(lx, ly, lz, lyaw, sx, sy, sz float64) (left, right float64) {
	dx, dy, dz := sx-lx, sy-ly, sz-lz
	d := math.Sqrt(dx*dx + dy*dy + dz*dz)
	pan := 0.0
	if d > 1e-6 {
		sin, cos := math.Sincos(lyaw)
		pan = (dx*cos + dz*sin) / d
	}
	a := (pan + 1) * math.Pi / 4
	return math.Cos(a), math.Sin(a) // centre: 0.707 each, like OpenAL Soft's stereo panning
}

// gains computes the voice's target left/right gain. Called with m.mu held.
func (m *Mixer) gains(v *Voice) (float32, float32) {
	cv := float32(1)
	if v.cat != "master" {
		cv = m.volume(v.cat)
	}
	g := float64(min(1, max(0, v.vol*cv))) * float64(m.volume("master"))
	if !v.positional {
		if v.src.channels() == 1 {
			g *= math.Sqrt2 / 2 // a mono source still goes through the panner, centred
		}
		return float32(g), float32(g)
	}
	dx, dy, dz := v.x-m.lx, v.y-m.ly, v.z-m.lz
	g *= Attenuation(math.Sqrt(dx*dx+dy*dy+dz*dz), v.rng)
	if v.src.channels() == 2 { // OpenAL does not spatialize stereo sources
		return float32(g), float32(g)
	}
	l, r := Pan(m.lx, m.ly, m.lz, m.lyaw, v.x, v.y, v.z)
	return float32(g * l), float32(g * r)
}

// Read fills p with stereo float32LE frames. It never returns an error.
func (m *Mixer) Read(p []byte) (int, error) {
	n := len(p) / 8
	mix := make([]float32, n*2)
	m.mu.Lock()
	for _, v := range m.voices {
		if v.dead || v.src == nil {
			continue
		}
		tl, tr := m.gains(v)
		if v.fresh {
			v.curL, v.curR, v.fresh = tl, tr, false
		}
		dl, dr := (tl-v.curL)/float32(n), (tr-v.curR)/float32(n)
		step := float64(v.src.rate()) / SampleRate * float64(v.pitch)
		for i := 0; i < n; i++ {
			k := int(v.pos)
			l0, r0, ok := v.src.frame(k)
			if !ok && v.loop && k > 0 {
				v.pos -= float64(k)
				k = 0
				l0, r0, ok = v.src.frame(0)
			}
			if !ok {
				v.kill()
				break
			}
			l1, r1, ok1 := v.src.frame(k + 1)
			if !ok1 {
				l1, r1 = l0, r0
			}
			f := float32(v.pos - float64(k))
			v.curL += dl
			v.curR += dr
			mix[2*i] += (l0 + (l1-l0)*f) * v.curL
			mix[2*i+1] += (r0 + (r1-r0)*f) * v.curR
			v.pos += step
		}
		v.curL, v.curR = tl, tr
	}
	live := m.voices[:0]
	for _, v := range m.voices {
		if !v.dead {
			live = append(live, v)
		}
	}
	clear(m.voices[len(live):])
	m.voices = live
	m.mu.Unlock()
	for i, x := range mix {
		binary.LittleEndian.PutUint32(p[4*i:], math.Float32bits(max(-1, min(1, x))))
	}
	return n * 8, nil
}
