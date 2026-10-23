package actors

import (
	"math"
	"strconv"

	"github.com/memmaker/voxellib/voxel/collide"
)

// SpawnRule is one entry of the game's spawn table (Minecraft's biome spawn
// lists, LOTR's per-biome NPC spawn lists).
type SpawnRule struct {
	Kind     string   `json:"kind"`
	Category string   `json:"category"` // caps are per category ("monster", "creature"...)
	Weight   int      `json:"weight"`
	Min      int      `json:"min"` // group size
	Max      int      `json:"max"`
	Biomes   []string `json:"biomes,omitempty"` // empty: every biome
	MinLight int      `json:"minLight,omitempty"`
	MaxLight int      `json:"maxLight,omitempty"` // 0 means 15
	Time     string   `json:"time,omitempty"`     // "", "day" or "night"
}

// SpawnWorld is what spawning asks the game.
type SpawnWorld interface {
	collide.Solids
	Biome(x, z int) string
	// Light is the light level a mob sees at a block now (sky light dimmed
	// by the time of day, or block light, whichever is brighter).
	Light(x, y, z int) int
	// Surface is the feet height of the top walkable spot of a column;
	// ok is false where the terrain is not loaded.
	Surface(x, z int) (y int, ok bool)
}

// Spawner fills the area around the player from the spawn table and
// despawns what wandered far off, both only where no player can see.
type Spawner struct {
	W     SpawnWorld
	Rules []SpawnRule
	Caps  map[string]int // max actors per category within Radius
	Every map[string]int // ticks between attempts per category (0 = every tick)
	// MinDist and Radius bound the spawn ring around the player (Minecraft:
	// 24 and 128). Beyond Radius spawned actors despawn; beyond 32 they
	// despawn at random after 30 s without a fight (1 in 800 a tick).
	MinDist, Radius float64
	Night           func() bool
	// Visible reports whether a player could see a point; spawning and
	// despawning skip such points. nil: nothing beyond MinDist is seen.
	Visible func(x, y, z float64) bool
	// Make builds an actor of the rule's kind (the game's template).
	Make func(r *SpawnRule, id string, x, y, z float64) *Actor
	// Can is an optional extra test (ground block, water...).
	Can func(r *SpawnRule, x, y, z int) bool

	ticks, next int
}

func (s *Spawner) visible(x, y, z float64) bool { return s.Visible != nil && s.Visible(x, y, z) }

// Tick runs one spawning and despawning step around the player at c.
func (s *Spawner) Tick(m *Manager, c Vec) {
	s.ticks++
	if s.MinDist == 0 {
		s.MinDist = 24
	}
	if s.Radius == 0 {
		s.Radius = 128
	}
	s.despawn(m, c)
	count := map[string]int{}
	for _, a := range m.Actors {
		if a.Category != "" && a.Pos().hdist(c) < s.Radius {
			count[a.Category]++
		}
	}
	for cat, limit := range s.Caps {
		if e := s.Every[cat]; e > 1 && s.ticks%e != 0 {
			continue
		}
		if count[cat] < limit {
			count[cat] += s.attempt(m, c, cat)
		}
	}
}

func (s *Spawner) despawn(m *Manager, c Vec) {
	var gone []*Actor
	for _, a := range m.Actors {
		if a.Persistent || a.Player || a.Category == "" || a.Rider != nil {
			continue
		}
		p := a.Pos()
		d := p.hdist(c)
		if s.visible(p.X, p.Y, p.Z) {
			continue
		}
		if d > s.Radius || (d > 32 && a.age > 600 && m.Rand.Intn(800) == 0) {
			gone = append(gone, a)
		}
	}
	for _, a := range gone {
		m.Remove(a)
	}
}

// attempt tries one group of category cat at a random column of the ring
// and returns how many actors it added.
func (s *Spawner) attempt(m *Manager, c Vec, cat string) int {
	rnd := m.Rand
	ang := rnd.Float64() * 2 * math.Pi
	r := s.MinDist + rnd.Float64()*(s.Radius-s.MinDist)
	x, z := int(math.Floor(c.X+math.Cos(ang)*r)), int(math.Floor(c.Z+math.Sin(ang)*r))
	y, ok := s.W.Surface(x, z)
	if !ok {
		return 0
	}
	rule := s.pick(m, cat, x, y, z)
	if rule == nil {
		return 0
	}
	n := rule.Min
	if rule.Max > rule.Min {
		n += rnd.Intn(rule.Max - rule.Min + 1)
	}
	added := 0
	for i := 0; i < n*4 && added < n; i++ { // a pack spreads up to 5 blocks
		px, pz := x+rnd.Intn(6)-rnd.Intn(6), z+rnd.Intn(6)-rnd.Intn(6)
		py, ok := s.W.Surface(px, pz)
		fx, fy, fz := float64(px)+0.5, float64(py), float64(pz)+0.5
		if !ok || !s.fits(rule, px, py, pz) || math.Hypot(fx-c.X, fz-c.Z) < s.MinDist || s.visible(fx, fy, fz) {
			continue
		}
		s.next++
		a := s.Make(rule, "spawn-"+strconv.Itoa(s.next), fx, fy, fz)
		if a == nil || !collide.Fits(s.W, a.Body.Box()) {
			continue
		}
		a.Category, a.Persistent = cat, false
		a.Yaw = rnd.Float64() * 2 * math.Pi
		m.Add(a)
		added++
	}
	return added
}

// fits checks the light and the extra test at one spot.
func (s *Spawner) fits(r *SpawnRule, x, y, z int) bool {
	l := s.W.Light(x, y, z)
	hi := r.MaxLight
	if hi == 0 {
		hi = 15
	}
	return l >= r.MinLight && l <= hi && (s.Can == nil || s.Can(r, x, y, z))
}

// pick draws a rule of the category by weight among those allowed here.
func (s *Spawner) pick(m *Manager, cat string, x, y, z int) *SpawnRule {
	biome := s.W.Biome(x, z)
	night := s.Night != nil && s.Night()
	var ok []*SpawnRule
	total := 0
	for i := range s.Rules {
		r := &s.Rules[i]
		if r.Category != cat || r.Weight <= 0 || (len(r.Biomes) > 0 && !has(r.Biomes, biome)) ||
			(r.Time == "day" && night) || (r.Time == "night" && !night) {
			continue
		}
		ok = append(ok, r)
		total += r.Weight
	}
	if total == 0 {
		return nil
	}
	n := m.Rand.Intn(total)
	for _, r := range ok {
		if n -= r.Weight; n < 0 {
			return r
		}
	}
	return nil
}
