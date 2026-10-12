// Package audio is a small positional mixer on top of oto. The mixing reader
// (Mixer.Read) is independent of the device so it can be tested standalone.
package audio

import (
	"encoding/binary"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// SampleRate is the only supported sample rate.
const SampleRate = 44100

const (
	maxVoices = 32
	refDist   = 2.0
	maxDist   = 48.0
	rolloff   = 1.0
	behind    = 0.8
	latency   = 50 * time.Millisecond
)

// Sound is mono PCM at SampleRate, -1..1.
type Sound struct{ Samples []float32 }

// Voice is a playing sound. Methods are safe to call from the game thread.
type Voice struct {
	m          *Mixer
	s          *Sound
	pos        int
	loop       bool
	positional bool
	x, y, z    float64
	gain       float32
	curL, curR float32 // smoothed gains, owned by Read
	fresh      bool    // no previous gain to ramp from
	dead       bool
}

// Mixer mixes all voices into stereo float32. It implements io.Reader.
type Mixer struct {
	mu               sync.Mutex // ponytail: held during Read; fine for 32 voices, split if profiling says otherwise
	voices           []*Voice
	lx, ly, lz, lyaw float64
	ctx              *oto.Context
	player           *oto.Player
}

// New opens the audio device and starts the mixer.
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
	<-ready
	m := &Mixer{ctx: ctx}
	m.player = ctx.NewPlayer(m)
	m.player.SetBufferSize(int(SampleRate * latency.Seconds() * 8))
	m.player.Play()
	return m, nil
}

// Close stops playback.
func (m *Mixer) Close() {
	if m.player != nil {
		m.player.Close()
	}
}

// SetListener sets the listener position and yaw (radians).
func (m *Mixer) SetListener(x, y, z, yaw float64) {
	m.mu.Lock()
	m.lx, m.ly, m.lz, m.lyaw = x, y, z, yaw
	m.mu.Unlock()
}

func (m *Mixer) add(v *Voice) *Voice {
	v.m, v.fresh = m, true
	m.mu.Lock()
	if len(m.voices) >= maxVoices {
		m.voices = append(m.voices[:0], m.voices[1:]...)
	}
	m.voices = append(m.voices, v)
	m.mu.Unlock()
	return v
}

// Play plays a non-positional one-shot, centred.
func (m *Mixer) Play(s *Sound, gain float32) {
	m.add(&Voice{s: s, gain: gain})
}

// PlayAt plays a positional one-shot.
func (m *Mixer) PlayAt(s *Sound, x, y, z float64, gain float32) {
	m.add(&Voice{s: s, gain: gain, positional: true, x: x, y: y, z: z})
}

// Loop starts a looping positional sound.
func (m *Mixer) Loop(s *Sound, x, y, z float64, gain float32) *Voice {
	return m.add(&Voice{s: s, gain: gain, loop: true, positional: true, x: x, y: y, z: z})
}

// Set moves the voice and changes its gain.
func (v *Voice) Set(x, y, z float64, gain float32) {
	v.m.mu.Lock()
	v.x, v.y, v.z, v.gain = x, y, z, gain
	v.m.mu.Unlock()
}

// Stop ends the voice.
func (v *Voice) Stop() {
	v.m.mu.Lock()
	v.dead = true
	v.m.mu.Unlock()
}

// Spatial returns left/right gains for a source heard by a listener at yaw:
// inverse-distance-clamped attenuation, equal-power pan, behind dampened.
func Spatial(lx, ly, lz, lyaw, sx, sy, sz float64) (left, right float32) {
	dx, dy, dz := sx-lx, sy-ly, sz-lz
	d := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if d >= maxDist {
		return 0, 0
	}
	att := refDist / (refDist + rolloff*(math.Max(d, refDist)-refDist))
	pan, fwd := 0.0, 1.0
	if d > 1e-6 {
		sin, cos := math.Sincos(lyaw)
		pan = (dx*cos + dz*sin) / d
		fwd = dx*sin - dz*cos
	}
	if fwd < 0 {
		att *= behind
	}
	a := (pan + 1) * math.Pi / 4
	return float32(att * math.Cos(a)), float32(att * math.Sin(a))
}

// Read fills p with stereo float32LE frames. It never returns an error.
func (m *Mixer) Read(p []byte) (int, error) {
	n := len(p) / 8
	mix := make([]float32, n*2)
	m.mu.Lock()
	for _, v := range m.voices {
		if v.dead || len(v.s.Samples) == 0 {
			v.dead = true
			continue
		}
		tl, tr := v.gain, v.gain
		if v.positional {
			l, r := Spatial(m.lx, m.ly, m.lz, m.lyaw, v.x, v.y, v.z)
			tl, tr = l*v.gain, r*v.gain
		}
		if v.fresh {
			v.curL, v.curR, v.fresh = tl, tr, false
		}
		dl, dr := (tl-v.curL)/float32(n), (tr-v.curR)/float32(n)
		for i := 0; i < n; i++ {
			if v.pos >= len(v.s.Samples) {
				if !v.loop {
					v.dead = true
					break
				}
				v.pos = 0
			}
			v.curL += dl
			v.curR += dr
			x := v.s.Samples[v.pos]
			v.pos++
			mix[2*i] += x * v.curL
			mix[2*i+1] += x * v.curR
		}
		v.curL, v.curR = tl, tr
	}
	live := m.voices[:0]
	for _, v := range m.voices {
		if !v.dead {
			live = append(live, v)
		}
	}
	m.voices = live
	m.mu.Unlock()
	for i, x := range mix {
		binary.LittleEndian.PutUint32(p[4*i:], math.Float32bits(float32(math.Tanh(float64(x)))))
	}
	return n * 8, nil
}
