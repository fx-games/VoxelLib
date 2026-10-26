package bettercombat

// Replays testdata/golden.json.gz: the loaded keyframes of every Better
// Combat animation and 69 scenarios of attacks driven through the real
// Player Animation Lib core as the mod drives it, sampled per tick and
// partial tick.

import (
	"compress/gzip"
	"encoding/json"
	"math"
	"os"
	"testing"
)

type goldenKey struct {
	Len, Start, End float32
	Ease            string
}

type goldenAnim struct {
	Length, BeginTick, EndTick float32
	Loop                       bool
	Bones                      map[string]map[string]json.RawMessage
}

type goldenBone struct{ Pos, Rot, Scale [3]float32 }

type goldenSample struct {
	Tick      int
	Pt        float32
	Active    bool
	AnimTicks float32
	Speed     float32
	Bones     map[string]goldenBone
}

type goldenScenario struct {
	Name        string
	FirstPerson bool
	Length      float32
	Upswing     float32
	PitchDeg    float32
	OffHand     bool
	Vanilla     map[string][9]float32
	Attacks     []struct {
		Tick int
		Anim string
	}
	Samples []goldenSample
}

func loadGolden(t *testing.T) (map[string]goldenAnim, []goldenScenario) {
	f, err := os.Open("testdata/golden.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Animations map[string]goldenAnim
		Scenarios  []goldenScenario
	}
	if err := json.NewDecoder(z).Decode(&g); err != nil {
		t.Fatal(err)
	}
	return g.Animations, g.Scenarios
}

func loadAnims(*testing.T) map[string]*animation { return Builtin() }

func near(a, b, tol float32) bool { return math.Abs(float64(a-b)) <= float64(tol) }

var axisKeys = [axes]string{"px", "py", "pz", "rx", "ry", "rz", "sx", "sy", "sz", "bend"}

func TestLoader(t *testing.T) {
	want, _ := loadGolden(t)
	anims := loadAnims(t)
	if len(anims) != len(want) {
		t.Fatalf("%d animations, want %d", len(anims), len(want))
	}
	for name, w := range want {
		a := anims[name]
		if a == nil {
			t.Fatalf("%s: not loaded", name)
		}
		if a.length != w.Length || a.loop != w.Loop || a.begin == nil || *a.begin != w.BeginTick || a.end == nil || *a.end != w.EndTick {
			t.Errorf("%s: length %v begin %v end %v loop %v, want %v %v %v %v", name, a.length, a.begin, a.end, a.loop, w.Length, w.BeginTick, w.EndTick, w.Loop)
		}
		if len(a.bones) != len(w.Bones) {
			t.Errorf("%s: bones %v, want %v", name, len(a.bones), len(w.Bones))
		}
		for bone, wb := range w.Bones {
			b := a.bones[bone]
			if b == nil {
				t.Errorf("%s: no bone %s", name, bone)
				continue
			}
			for ax, key := range axisKeys {
				if key == "bend" {
					var n int
					json.Unmarshal(wb[key], &n)
					if len(b.keys[ax]) != n {
						t.Errorf("%s %s bend: %d keyframes, want %d", name, bone, len(b.keys[ax]), n)
					}
					continue
				}
				var wk []goldenKey
				json.Unmarshal(wb[key], &wk)
				if len(b.keys[ax]) != len(wk) {
					t.Errorf("%s %s %s: %d keyframes, want %d", name, bone, key, len(b.keys[ax]), len(wk))
					continue
				}
				for i, k := range b.keys[ax] {
					if k.length != wk[i].Len || k.start != wk[i].Start || k.end != wk[i].End || k.ease != wk[i].Ease {
						t.Errorf("%s %s %s[%d]: %+v, want %+v", name, bone, key, i, k, wk[i])
					}
				}
			}
		}
	}
}

// TestController replays the scenarios: every sampled bone value must
// match the library's to 1e-5 (float32 rounding differs by operation
// order in places).
func TestController(t *testing.T) {
	_, scenarios := loadGolden(t)
	anims := loadAnims(t)
	for _, sc := range scenarios {
		s := newAttackStack()
		s.pitch = sc.PitchDeg
		i := 0
		for tick := 0; i < len(sc.Samples); tick++ {
			for _, at := range sc.Attacks {
				if at.Tick == tick {
					s.play(anims[at.Anim], sc.OffHand, sc.Length, sc.Upswing)
				}
			}
			s.handle(0, true, sc.FirstPerson)
			for ; i < len(sc.Samples) && sc.Samples[i].Tick == tick; i++ {
				sm := sc.Samples[i]
				if sm.Pt != 0 {
					s.handle(sm.Pt, false, sc.FirstPerson)
				}
				if s.c.isActive() != sm.Active || s.speed.speed != sm.Speed || !near(s.c.animTicks(), sm.AnimTicks, 1e-4) {
					t.Fatalf("%s tick %d pt %v: active %v speed %v animTicks %v, want %v %v %v", sc.Name, tick, sm.Pt, s.c.isActive(), s.speed.speed, s.c.animTicks(), sm.Active, sm.Speed, sm.AnimTicks)
				}
				for name, wb := range sm.Bones {
					v := sc.Vanilla[name]
					b := &animBone{name: name}
					copy(b.v[:], v[:])
					if s.c.isActive() {
						s.transform(b)
					}
					want := [axes]float32{wb.Pos[0], wb.Pos[1], wb.Pos[2], wb.Rot[0], wb.Rot[1], wb.Rot[2], wb.Scale[0], wb.Scale[1], wb.Scale[2]}
					for ax := range axBend {
						if !near(b.v[ax], want[ax], 1e-5) {
							t.Fatalf("%s tick %d pt %v %s %s: %v, want %v", sc.Name, tick, sm.Pt, name, axisKeys[ax], b.v[ax], want[ax])
						}
					}
				}
			}
		}
	}
}
