package audio

import (
	"encoding/json"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Sounds plays sound events from a sound directory:
//
//	dir/sounds.json            events (Registry format, keys "domain:name")
//	dir/<domain>/sounds/<path>.ogg  the file of entry "domain:path"
//	dir/blocksounds.json       optional {"<type>": BlockSound}
type Sounds struct {
	Events Registry
	Blocks map[string]BlockSound
	Mixer  *Mixer
	Rand   *rand.Rand
	Dir    string

	mu      sync.Mutex
	cache   map[string]*cached
	delayed []delayedSound
}

type cached struct {
	once sync.Once
	snd  *Sound
}

type delayedSound struct {
	name  string
	p     Params
	ticks int
}

// Open reads dir's sounds.json (and blocksounds.json) and plays through m (nil: NewSilent()).
func Open(dir string, m *Mixer) (*Sounds, error) {
	if m == nil {
		m = NewSilent()
	}
	s := &Sounds{Events: Registry{}, Mixer: m, Dir: dir, Rand: rand.New(rand.NewSource(rand.Int63())), cache: map[string]*cached{}}
	data, err := os.ReadFile(filepath.Join(dir, "sounds.json"))
	if err != nil {
		return nil, err
	}
	if err := s.Events.Load("minecraft", data, nil); err != nil {
		return nil, err
	}
	if data, err := os.ReadFile(filepath.Join(dir, "blocksounds.json")); err == nil {
		if err := json.Unmarshal(data, &s.Blocks); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// File returns the path of an entry's file ("domain:path").
func (s *Sounds) File(name string) string {
	d, p, _ := strings.Cut(name, ":")
	return filepath.Join(s.Dir, d, "sounds", filepath.FromSlash(p)+".ogg")
}

// Play plays a sound event like 1.7.10's SoundManager.playSound: a random variant, volume
// clamp(p.Volume × variant × category, 0, 1) × master, pitch clamp(p.Pitch × variant, 0.5, 2),
// linear attenuation over 16 × max(1, p.Volume) blocks. Names without a domain are
// "minecraft:". It returns nil when nothing plays (unknown or empty event, zero volume, delayed).
// Files are decoded in the background; the voice starts when ready.
func (s *Sounds) Play(name string, p Params) *Voice {
	name = qualify("minecraft", name)
	if p.Delay > 0 {
		s.mu.Lock()
		s.delayed = append(s.delayed, delayedSound{name, p, p.Delay})
		s.mu.Unlock()
		return nil
	}
	s.mu.Lock()
	e, cat, ok := s.Events.Pick(name, s.Rand)
	s.mu.Unlock()
	if !ok {
		return nil
	}
	m := s.Mixer
	if m.Volume("master") <= 0 {
		return nil
	}
	v := m.newVoice(p, cat, e.Volume, e.Pitch)
	cv := float32(1)
	if cat != "master" {
		cv = m.Volume(cat)
	}
	if min(1, v.vol*cv) <= 0 {
		return nil
	}
	file := s.File(e.Name)
	if e.Stream {
		f, err := os.Open(file)
		if err != nil {
			log.Printf("audio: %v", err)
			return nil
		}
		st, err := newStream(f)
		if err != nil {
			log.Printf("audio: %s: %v", file, err)
			return nil
		}
		v.src, v.streaming = st, true
		return m.add(v)
	}
	s.mu.Lock()
	c := s.cache[file]
	if c == nil {
		c = &cached{}
		s.cache[file] = c
	}
	s.mu.Unlock()
	m.add(v)
	go func() {
		c.once.Do(func() {
			f, err := os.Open(file)
			if err == nil {
				c.snd, err = Decode(f)
				f.Close()
			}
			if err == nil && c.snd.Channels > 2 {
				c.snd, err = nil, errChannels
			}
			if err != nil {
				log.Printf("audio: %s: %v", file, err)
			}
		})
		m.mu.Lock()
		if c.snd == nil {
			v.dead = true
		} else if !v.dead {
			v.src = c.snd
		}
		m.mu.Unlock()
	}()
	return v
}

// Tick advances delayed sounds; call it once per game tick (20 Hz) while not paused.
func (s *Sounds) Tick() {
	s.mu.Lock()
	var due []delayedSound
	keep := s.delayed[:0]
	for _, d := range s.delayed {
		if d.ticks--; d.ticks <= 0 {
			due = append(due, d)
		} else {
			keep = append(keep, d)
		}
	}
	s.delayed = keep
	s.mu.Unlock()
	for _, d := range due {
		d.p.Delay = 0
		s.Play(d.name, d.p)
	}
}

// DistanceDelay is WorldClient.playSound's distanceDelay (thunder): sounds farther than
// 10 blocks from the listener wait distance/40 seconds, in ticks.
func DistanceDelay(lx, ly, lz, x, y, z float64) int {
	d2 := (x-lx)*(x-lx) + (y-ly)*(y-ly) + (z-lz)*(z-lz)
	if d2 <= 100 {
		return 0
	}
	return int(math.Sqrt(d2) / 40 * 20)
}

// BlockSound is a Minecraft Block.SoundType: event names and the type's volume and pitch.
// Its methods return the event and Params vanilla uses at a block centre, for example
// s.Play(b.Broken(x, y, z)).
type BlockSound struct {
	Break  string  `json:"break"`
	Step   string  `json:"step"`
	Place  string  `json:"place"`
	Volume float32 `json:"volume"`
	Pitch  float32 `json:"pitch"`
}

func blockAt(x, y, z int) (float64, float64, float64) {
	return float64(x) + 0.5, float64(y) + 0.5, float64(z) + 0.5
}

// Broken is the break sound (RenderGlobal 2001): (volume+1)/2, pitch×0.8.
func (b BlockSound) Broken(x, y, z int) (string, Params) {
	px, py, pz := blockAt(x, y, z)
	return b.Break, At(px, py, pz, (b.Volume+1)/2, b.Pitch*0.8)
}

// Placed is the place sound (ItemBlock): (volume+1)/2, pitch×0.8.
func (b BlockSound) Placed(x, y, z int) (string, Params) {
	px, py, pz := blockAt(x, y, z)
	return b.Place, At(px, py, pz, (b.Volume+1)/2, b.Pitch*0.8)
}

// Hit is the mining sound, played every 4th tick while digging: (volume+1)/8, pitch×0.5.
func (b BlockSound) Hit(x, y, z int) (string, Params) {
	px, py, pz := blockAt(x, y, z)
	return b.Step, At(px, py, pz, (b.Volume+1)/8, b.Pitch*0.5)
}

// Stepped is a footstep of an entity at its feet (Entity.func_145780_a): volume×0.15, pitch.
// Vanilla uses the snow layer's sound when a snow layer lies on the block, and none for liquids.
func (b BlockSound) Stepped(x, y, z float64) (string, Params) {
	return b.Step, At(x, y, z, b.Volume*0.15, b.Pitch)
}

// Landed is the block part of a fall (EntityLivingBase.fall): volume×0.5, pitch×0.75.
func (b BlockSound) Landed(x, y, z float64) (string, Params) {
	return b.Step, At(x, y, z, b.Volume*0.5, b.Pitch*0.75)
}
