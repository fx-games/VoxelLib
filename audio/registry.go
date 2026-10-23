package audio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"slices"
	"strings"
)

// Entry is one variant of a sound event.
type Entry struct {
	Name   string  // "domain:path" of a file (domain/sounds/path.ogg), or an event name if Event
	Volume float64 // default 1
	Pitch  float64 // default 1
	Weight int     // default 1
	Stream bool    // decode while playing (music)
	Event  bool    // "type": "event": a reference to another event
}

// Event is a sound event: a category and weighted variants.
type Event struct {
	Category string
	Sounds   []Entry
}

// Registry maps "domain:name" to events.
type Registry map[string]*Event

type jsonEntry struct {
	Name   string   `json:"name"`
	Volume *float64 `json:"volume,omitempty"`
	Pitch  *float64 `json:"pitch,omitempty"`
	Weight *int     `json:"weight,omitempty"`
	Stream bool     `json:"stream,omitempty"`
	Type   string   `json:"type,omitempty"`
}

type jsonEvent struct {
	Category string            `json:"category,omitempty"`
	Replace  bool              `json:"replace,omitempty"`
	Sounds   []json.RawMessage `json:"sounds,omitempty"`
}

func qualify(domain, name string) string {
	if strings.Contains(name, ":") {
		return name
	}
	return domain + ":" + name
}

// Load merges one sounds.json of a domain like 1.7.10's SoundHandler: the whole file is parsed
// first and rejected on any error (unknown category or type, volume/pitch/weight <= 0); then each
// event is appended to an existing one of that name (keeping its category) unless it sets
// "replace", which starts it afresh. Keys may already be qualified ("domain:name").
// exists reports whether a file entry's "domain:path" exists; missing files are skipped.
// Sounds are loaded in pack order, lowest priority first.
func (r Registry) Load(domain string, data []byte, exists func(file string) bool) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var keys []string
	var events []jsonEvent
	// Decode in file order with duplicate detection (Gson rejects duplicate keys).
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return fmt.Errorf("sounds.json: not an object")
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		k := t.(string)
		if slices.Contains(keys, k) {
			return fmt.Errorf("sounds.json: duplicate key %q", k)
		}
		var e jsonEvent
		if err := dec.Decode(&e); err != nil {
			return fmt.Errorf("sounds.json %q: %w", k, err)
		}
		if e.Category == "" {
			e.Category = "master"
		}
		if !slices.Contains(Categories, e.Category) {
			return fmt.Errorf("sounds.json %q: invalid category %q", k, e.Category)
		}
		keys, events = append(keys, k), append(events, e)
	}
	parsed := make([][]Entry, len(events))
	for i, e := range events {
		for _, raw := range e.Sounds {
			var je jsonEntry
			if err := json.Unmarshal(raw, &je.Name); err != nil {
				if err := json.Unmarshal(raw, &je); err != nil {
					return fmt.Errorf("sounds.json %q: %w", keys[i], err)
				}
			}
			en := Entry{Name: je.Name, Volume: 1, Pitch: 1, Weight: 1, Stream: je.Stream}
			switch je.Type {
			case "", "file":
			case "event":
				en.Event = true
			default:
				return fmt.Errorf("sounds.json %q: invalid type %q", keys[i], je.Type)
			}
			if je.Volume != nil {
				en.Volume = *je.Volume
			}
			if je.Pitch != nil {
				en.Pitch = *je.Pitch
			}
			if je.Weight != nil {
				en.Weight = *je.Weight
			}
			if en.Volume <= 0 || en.Pitch <= 0 || en.Weight <= 0 {
				return fmt.Errorf("sounds.json %q: invalid volume, pitch or weight", keys[i])
			}
			parsed[i] = append(parsed[i], en)
		}
	}
	for i, k := range keys {
		name := qualify(domain, k)
		ev := r[name]
		if ev == nil || events[i].Replace {
			ev = &Event{Category: events[i].Category}
			r[name] = ev
		}
		evDomain := name[:strings.Index(name, ":")]
		for _, en := range parsed[i] {
			d := evDomain
			if j := strings.Index(en.Name, ":"); j >= 0 {
				d = en.Name[:j]
			}
			if en.Event {
				// 1.7.10 builds ResourceLocation(domain, name) from the full name, so a
				// qualified reference ends up "d:d:name" and never resolves.
				en.Name = d + ":" + en.Name
			} else {
				en.Name = d + ":" + en.Name[strings.Index(en.Name, ":")+1:]
				if exists != nil && !exists(en.Name) {
					continue
				}
			}
			ev.Sounds = append(ev.Sounds, en)
		}
	}
	return nil
}

func (r Registry) weight(name string, depth int) int {
	ev := r[name]
	if ev == nil || depth > 16 {
		return 0
	}
	w := 0
	for _, e := range ev.Sounds {
		if e.Event {
			w += r.weight(e.Name, depth+1)
		} else {
			w += e.Weight
		}
	}
	return w
}

// Pick chooses a file variant of an event like SoundEventAccessorComposite: weighted by
// Weight, an event reference weighing as much as that event's total. It returns the event's
// category; ok is false for unknown or empty events.
func (r Registry) Pick(name string, rng *rand.Rand) (e Entry, category string, ok bool) {
	ev := r[name]
	if ev == nil {
		return Entry{}, "", false
	}
	e, ok = r.pick(name, rng, 0)
	return e, ev.Category, ok
}

func (r Registry) pick(name string, rng *rand.Rand, depth int) (Entry, bool) {
	total := r.weight(name, depth)
	if total == 0 {
		return Entry{}, false
	}
	j := rng.Intn(total)
	for _, e := range r[name].Sounds {
		w := e.Weight
		if e.Event {
			w = r.weight(e.Name, depth+1)
		}
		if j -= w; j < 0 {
			if e.Event {
				return r.pick(e.Name, rng, depth+1)
			}
			return e, true
		}
	}
	return Entry{}, false
}

// MarshalJSON writes the registry as one sounds.json with qualified keys and names, which Load
// (any domain) reads back unchanged.
func (r Registry) MarshalJSON() ([]byte, error) {
	out := map[string]jsonEvent{}
	for k, ev := range r {
		je := jsonEvent{Category: ev.Category, Sounds: []json.RawMessage{}}
		for _, e := range ev.Sounds {
			name := e.Name
			if e.Event { // undo the domain Load put in front
				name = name[strings.Index(name, ":")+1:]
			}
			var v any = name
			if e.Volume != 1 || e.Pitch != 1 || e.Weight != 1 || e.Stream || e.Event {
				x := jsonEntry{Name: name, Stream: e.Stream}
				if e.Volume != 1 {
					x.Volume = &e.Volume
				}
				if e.Pitch != 1 {
					x.Pitch = &e.Pitch
				}
				if e.Weight != 1 {
					x.Weight = &e.Weight
				}
				if e.Event {
					x.Type = "event"
				}
				v = x
			}
			b, _ := json.Marshal(v)
			je.Sounds = append(je.Sounds, b)
		}
		out[k] = je
	}
	return json.MarshalIndent(out, "", " ")
}
