package actors

// Saved is an actor in the save blob: everything that changes in play.
// What a Kind looks like and fights with (model, speed, attacks) comes
// back from the game's template when it rebuilds the actor.
type Saved struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	Category string            `json:"category,omitempty"`
	Name     string            `json:"name,omitempty"`
	Title    string            `json:"title,omitempty"`
	ShowName bool              `json:"showName,omitempty"`
	Pos      Vec               `json:"pos"`
	Yaw      float64           `json:"yaw"`
	Health   float64           `json:"health"`
	Faction  string            `json:"faction,omitempty"`
	Brain    Brain             `json:"brain"`
	Riding   string            `json:"riding,omitempty"`
	Tags     map[string]string `json:"tags,omitempty"`
	// Rolled per actor (a horse's stats), so not from the template.
	MaxHealth float64     `json:"maxHealth,omitempty"`
	Speed     float64     `json:"speed,omitempty"`
	Mount     *MountStats `json:"mount,omitempty"`
}

// Snapshot is the Manager's part of the save blob. Players are not saved
// (the game owns them); their ID still links followers and riders.
type Snapshot struct {
	Actors   []Saved        `json:"actors"`
	Standing map[string]int `json:"standing,omitempty"`
}

// Save captures every non-player actor and the player's faction standing.
func (m *Manager) Save() Snapshot {
	var s Snapshot
	for _, a := range m.Actors {
		if a.Player || !a.Alive() {
			continue
		}
		v := Saved{ID: a.ID, Kind: a.Kind, Category: a.Category, Name: a.Name, Title: a.Title,
			ShowName: a.ShowName, Pos: a.Pos(), Yaw: a.Yaw, Health: a.Health, Faction: a.Faction,
			Brain: a.Brain, Tags: a.Tags, MaxHealth: a.MaxHealth, Speed: a.Speed, Mount: a.Mount}
		if a.Riding != nil {
			v.Riding = a.Riding.ID
		}
		s.Actors = append(s.Actors, v)
	}
	if m.Factions != nil && len(m.Factions.Standing) > 0 {
		s.Standing = map[string]int{}
		for k, v := range m.Factions.Standing {
			s.Standing[k] = v
		}
	}
	return s
}

// Load adds the saved actors, each built by make from its kind (the game's
// template: model, speed, attacks...) and then given its saved state.
// Players must already be in the Manager so links to them resolve. make
// may return nil for a kind that no longer exists.
func (m *Manager) Load(s Snapshot, make func(id, kind string) *Actor) {
	var riders []Saved
	for _, v := range s.Actors {
		a := make(v.ID, v.Kind)
		if a == nil {
			continue
		}
		a.ID, a.Kind, a.Category = v.ID, v.Kind, v.Category
		a.Name, a.Title, a.ShowName = v.Name, v.Title, v.ShowName
		b := a.Body
		b.X, b.Y, b.Z, b.PrevX, b.PrevY, b.PrevZ = v.Pos.X, v.Pos.Y, v.Pos.Z, v.Pos.X, v.Pos.Y, v.Pos.Z
		a.Yaw, a.PrevYaw, a.HeadYaw = v.Yaw, v.Yaw, v.Yaw
		if v.MaxHealth > 0 {
			a.MaxHealth = v.MaxHealth
		}
		if v.Speed > 0 {
			a.Speed = v.Speed
		}
		if s, t := v.Mount, a.Mount; s != nil && t != nil { // keep the template's fixed fields
			t.JumpStrength, t.Tame, t.Temper, t.Saddled, t.Owner, t.Eating = s.JumpStrength, s.Tame, s.Temper, s.Saddled, s.Owner, s.Eating
		}
		a.Health, a.Faction, a.Brain = v.Health, v.Faction, v.Brain
		if v.Tags != nil {
			a.Tags = v.Tags
		}
		m.Add(a)
		if v.Riding != "" {
			riders = append(riders, v)
		}
	}
	for _, v := range riders {
		if r, mt := m.byID[v.ID], m.byID[v.Riding]; r != nil && mt != nil {
			m.Mount(r, mt)
		}
	}
	if m.Factions != nil {
		for k, v := range s.Standing {
			m.Factions.Standing[k] = v
		}
	}
}
