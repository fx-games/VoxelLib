package actors

// Stance is how one side sees another.
type Stance int8

const (
	Hostile  Stance = -1
	Neutral  Stance = 0
	Friendly Stance = 1
)

func (s Stance) String() string {
	switch s {
	case Hostile:
		return "hostile"
	case Friendly:
		return "friendly"
	}
	return "neutral"
}

// ParseStance reads "friendly", "neutral" or "hostile".
func ParseStance(s string) Stance {
	switch s {
	case "friendly":
		return Friendly
	case "hostile":
		return Hostile
	}
	return Neutral
}

// Faction is one side. Relations between factions come from Enemies and
// Allies (either side listing the other is enough). The player's standing
// is a number of points (CustomNPC faction points, LOTR alignment): at or
// above FriendlyAt the faction is friendly, at or above NeutralAt neutral,
// below it hostile.
type Faction struct {
	Name       string   `json:"name"`
	Enemies    []string `json:"enemies,omitempty"`
	Allies     []string `json:"allies,omitempty"`
	FriendlyAt int      `json:"friendlyAt"`
	NeutralAt  int      `json:"neutralAt"`
	Default    int      `json:"default"` // the player's starting points
	Hidden     bool     `json:"hidden,omitempty"`
}

// Factions is the game's faction table plus the player's standing.
type Factions struct {
	ByName   map[string]*Faction
	Standing map[string]int // player points per faction; missing = Default
}

// NewFactions builds the table.
func NewFactions(fs ...*Faction) *Factions {
	t := &Factions{ByName: map[string]*Faction{}, Standing: map[string]int{}}
	for _, f := range fs {
		t.ByName[f.Name] = f
	}
	return t
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Relation is how faction a sees faction b.
func (t *Factions) Relation(a, b string) Stance {
	if a == b {
		return Friendly
	}
	fa, fb := t.ByName[a], t.ByName[b]
	if fa == nil || fb == nil {
		return Neutral
	}
	switch {
	case has(fa.Enemies, b) || has(fb.Enemies, a):
		return Hostile
	case has(fa.Allies, b) || has(fb.Allies, a):
		return Friendly
	}
	return Neutral
}

// Points returns the player's standing with a faction.
func (t *Factions) Points(f string) int {
	if p, ok := t.Standing[f]; ok {
		return p
	}
	if fa := t.ByName[f]; fa != nil {
		return fa.Default
	}
	return 0
}

// Add changes the player's standing.
func (t *Factions) Add(f string, n int) { t.Standing[f] = t.Points(f) + n }

// PlayerStance is how faction f sees the player.
func (t *Factions) PlayerStance(f string) Stance {
	fa := t.ByName[f]
	if fa == nil {
		return Neutral
	}
	p := t.Points(f)
	switch {
	case p >= fa.FriendlyAt:
		return Friendly
	case p >= fa.NeutralAt:
		return Neutral
	}
	return Hostile
}

// Sees is how actor a regards actor b.
func (t *Factions) Sees(a, b *Actor) Stance {
	switch {
	case t == nil:
		return Neutral
	case a.Player && b.Player:
		return Friendly
	case b.Player:
		return t.PlayerStance(a.Faction)
	case a.Player:
		return t.PlayerStance(b.Faction)
	}
	return t.Relation(a.Faction, b.Faction)
}
