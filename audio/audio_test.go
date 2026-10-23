package audio

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/memmaker/voxellib/resloc"
)

// testdata/test.ogg is jfreymuth/oggvorbis's test file (MIT): 1 s, mono, 44100 Hz.

func TestDecode(t *testing.T) {
	f, err := os.Open("testdata/test.ogg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if s.Channels != 1 || s.Rate != 44100 || len(s.Samples) != 44100 {
		t.Fatalf("got %d ch %d Hz %d samples", s.Channels, s.Rate, len(s.Samples))
	}
	f2, _ := os.Open("testdata/test.ogg")
	st, err := newStream(f2)
	if err != nil {
		t.Fatal(err)
	}
	defer st.close()
	n := 0
	for i := 0; ; i++ {
		l, _, ok := st.frame(i)
		if !ok {
			break
		}
		if l != s.Samples[i] {
			t.Fatalf("stream differs at %d", i)
		}
		n++
	}
	if n != 44100 {
		t.Fatalf("stream frames %d", n)
	}
}

func TestAttenuation(t *testing.T) {
	// 1.7.10: linear over 16 blocks, or 16 × volume for volume > 1.
	for _, c := range []struct{ d, rng, want float64 }{
		{0, 16, 1}, {4, 16, 0.75}, {8, 16, 0.5}, {16, 16, 0}, {20, 16, 0}, {32, 64, 0.5}, {100, 0, 1},
	} {
		if g := Attenuation(c.d, c.rng); math.Abs(g-c.want) > 1e-9 {
			t.Errorf("Attenuation(%v, %v) = %v, want %v", c.d, c.rng, g, c.want)
		}
	}
	m := NewSilent()
	if v := m.newVoice(At(0, 0, 0, 4, 1), "block", 1, 1); v.rng != 64 {
		t.Errorf("range for volume 4: %v", v.rng)
	}
	if v := m.newVoice(At(0, 0, 0, 0.5, 1), "block", 1, 1); v.rng != 16 {
		t.Errorf("range for volume 0.5: %v", v.rng)
	}
	if v := m.newVoice(At(0, 0, 0, 1, 3), "block", 1, 1); v.pitch != 2 {
		t.Errorf("pitch clamp: %v", v.pitch)
	}
	if v := m.newVoice(At(0, 0, 0, 1, 0.8), "block", 1, 0.5); math.Abs(float64(v.pitch)-0.5) > 1e-6 {
		t.Errorf("pitch clamp low: %v", v.pitch)
	}
	// gain = clamp(volume × entry × category, 0, 1) × master × attenuation
	m.SetVolume("master", 0.5)
	m.SetVolume("block", 0.4)
	v := m.newVoice(At(8, 0, 0, 4, 1), "block", 1, 1) // range 64, d 8 → 0.875; clamp(4×0.4)=1
	v.src = &Sound{Channels: 2, Rate: SampleRate}
	if l, r := m.gains(v); math.Abs(float64(l)-0.5*0.875) > 1e-6 || l != r {
		t.Errorf("gains %v %v", l, r)
	}
	// yaw 0: forward is -z, right is +x.
	l, r := Pan(0, 0, 0, 0, 5, 0, -1)
	if !(r > l) {
		t.Fatalf("source on the right: l=%v r=%v", l, r)
	}
	if l, r = Pan(0, 0, 0, 0, 0, 0, -3); math.Abs(l-r) > 1e-9 {
		t.Fatalf("centre: %v %v", l, r)
	}
}

func sample(p []byte, i int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(p[4*i:]))
}

func TestRead(t *testing.T) {
	m := NewSilent()
	buf := make([]byte, 8*100)
	if n, _ := m.Read(buf); n != len(buf) || sample(buf, 0) != 0 {
		t.Fatal("empty mixer should give silence")
	}
	one := &Sound{Samples: []float32{0.5, 0.5, 0.5, 0.5}, Channels: 1, Rate: SampleRate}
	m.Play(one, UI(1, 1))
	m.Read(buf)
	if sample(buf, 0) == 0 || sample(buf, 0) != sample(buf, 1) {
		t.Fatal("centred sound expected on both channels")
	}
	if sample(buf, 2*4) != 0 || len(m.voices) != 0 {
		t.Fatal("one-shot should end and be removed")
	}
	// Pitch 2 plays a sound in half the frames.
	long := &Sound{Samples: make([]float32, 40), Channels: 1, Rate: SampleRate}
	for i := range long.Samples {
		long.Samples[i] = 0.5
	}
	m.Play(long, UI(1, 2))
	m.Read(buf)
	if sample(buf, 2*19) == 0 || sample(buf, 2*21) != 0 {
		t.Fatal("pitch 2 should take 20 frames")
	}
	p := At(5, 0, -1, 1, 1)
	p.Loop = true
	v := m.Play(one, p)
	m.Read(buf)
	if sample(buf, 2*50+1) == 0 || len(m.voices) != 1 {
		t.Fatal("loop should keep playing")
	}
	v.Stop()
	m.Read(buf)
	if len(m.voices) != 0 || v.Playing() {
		t.Fatal("stopped voice should be removed")
	}
	for i := 0; i < 40; i++ {
		m.Play(one, UI(1, 1))
	}
	if len(m.voices) != maxVoices {
		t.Fatalf("voice cap: %d", len(m.voices))
	}
}

func TestRegistryMerge(t *testing.T) {
	r := Registry{}
	exists := func(f string) bool { return f != "test:step/missing" }
	vanilla := `{"step.stone": {"category": "block", "sounds": ["step/stone1", "step/stone2", "step/missing"]},
		"random.click": {"category": "block", "sounds": ["random/click"]},
		"music.game": {"category": "music", "sounds": [{"name": "music/calm1", "stream": true, "weight": 3}]}}`
	pack := `{"step.stone": {"category": "ambient", "sounds": ["mypack:stone3"]},
		"random.click": {"category": "player", "replace": true, "sounds": [{"name": "click2", "volume": 0.5, "pitch": 1.2}]}}`
	if err := r.Load("test", []byte(vanilla), exists); err != nil {
		t.Fatal(err)
	}
	if err := r.Load("test", []byte(pack), exists); err != nil {
		t.Fatal(err)
	}
	st := r["test:step.stone"]
	if st.Category != "block" || len(st.Sounds) != 3 || st.Sounds[2].Name != "mypack:stone3" || st.Sounds[0].Name != "test:step/stone1" {
		t.Fatalf("append: %+v", st)
	}
	cl := r["test:random.click"]
	if cl.Category != "player" || len(cl.Sounds) != 1 || cl.Sounds[0].Volume != 0.5 || cl.Sounds[0].Pitch != 1.2 {
		t.Fatalf("replace: %+v", cl)
	}
	if e := r["test:music.game"].Sounds[0]; !e.Stream || e.Weight != 3 {
		t.Fatalf("entry fields: %+v", e)
	}
	// Invalid files are rejected whole and change nothing.
	for _, bad := range []string{
		`{"a": {"category": "nope", "sounds": ["x"]}}`,
		`{"a": {"sounds": [{"name": "x", "type": "nope"}]}}`,
		`{"a": {"sounds": [{"name": "x", "volume": 0}]}}`,
		`{"a": {"sounds": ["x"]}, "a": {"sounds": ["y"]}}`,
		``,
	} {
		if err := r.Load("test", []byte(bad), nil); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if r["test:a"] != nil {
		t.Fatal("rejected file left an event")
	}
	// Round trip through MarshalJSON.
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	r2 := Registry{}
	if err := r2.Load("other", b, nil); err != nil {
		t.Fatal(err)
	}
	b2, _ := json.Marshal(r2)
	if string(b) != string(b2) {
		t.Fatalf("round trip differs:\n%s\n%s", b, b2)
	}
}

func TestPickVariants(t *testing.T) {
	r := Registry{}
	err := r.Load("x", []byte(`{
		"a": {"sounds": [{"name": "a1", "weight": 3}, "a2"]},
		"b": {"sounds": ["b1", {"name": "a", "type": "event"}]},
		"empty": {"sounds": []}}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(1))
	count := map[string]int{}
	for i := 0; i < 40000; i++ {
		e, cat, ok := r.Pick("x:b", rng)
		if !ok || cat != "master" {
			t.Fatal("pick failed")
		}
		count[e.Name]++
	}
	// b1 weighs 1, the reference to a weighs a's total 4: b1 1/5, a1 3/5, a2 1/5.
	for name, want := range map[string]float64{"x:b1": 0.2, "x:a1": 0.6, "x:a2": 0.2} {
		if got := float64(count[name]) / 40000; math.Abs(got-want) > 0.015 {
			t.Errorf("%s: %.3f, want %.2f", name, got, want)
		}
	}
	if _, _, ok := r.Pick("x:empty", rng); ok {
		t.Error("empty event picked")
	}
	if _, _, ok := r.Pick("x:none", rng); ok {
		t.Error("unknown event picked")
	}
}

func TestSoundsPlay(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, resloc.Default, "sounds"), 0o755)
	ogg, _ := os.ReadFile("testdata/test.ogg")
	os.WriteFile(filepath.Join(dir, resloc.Default, "sounds", "t.ogg"), ogg, 0o644)
	os.WriteFile(filepath.Join(dir, "sounds.json"), []byte(`{"t": {"category": "block", "sounds": ["t"]},
		"s": {"category": "music", "sounds": [{"name": "t", "stream": true}]}}`), 0o644)
	s, err := Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	v := s.Play("t", At(0, 0, 0, 1, 1))
	if v == nil || !v.Playing() {
		t.Fatal("not playing")
	}
	w := s.Play("s", UI(1, 1))
	if w == nil || !w.streaming {
		t.Fatal("stream not playing")
	}
	buf := make([]byte, 8*4096)
	for i := 0; i < 2000 && (v.Playing() || w.Playing()); i++ {
		s.Mixer.Read(buf) // the buffered one decodes in the background first
		time.Sleep(time.Millisecond)
	}
	if v.Playing() || w.Playing() {
		t.Fatal("1 s sound should end after 20×4096 frames")
	}
	s.Mixer.SetVolume("block", 0)
	if s.Play("t", At(0, 0, 0, 1, 1)) != nil {
		t.Fatal("zero category volume should skip")
	}
	if s.Play("t", Params{Volume: 1, Pitch: 1, Delay: 2}) != nil || len(s.delayed) != 1 {
		t.Fatal("delay")
	}
	s.Tick()
	s.Tick()
	if len(s.delayed) != 0 {
		t.Fatal("delayed sound not released")
	}
	if DistanceDelay(0, 0, 0, 5, 0, 0) != 0 || DistanceDelay(0, 0, 0, 80, 0, 0) != 40 {
		t.Fatal("DistanceDelay")
	}
}

type fakeTrack struct{ left int }

func (f *fakeTrack) Playing() bool { return f.left > 0 }
func (f *fakeTrack) Stop()         { f.left = 0 }

func TestMusicTicker(t *testing.T) {
	var cur *fakeTrack
	plays := []int{}
	tick := 0
	mt := NewMusicTicker(func(string) Track {
		plays = append(plays, tick)
		cur = &fakeTrack{left: 50}
		return cur
	}, rand.New(rand.NewSource(1)))
	game := MusicType{Name: "game", MinDelay: 200, MaxDelay: 300, Pick: func(*rand.Rand) string { return "music.game" }}
	step := func(m MusicType) {
		if cur != nil && cur.left > 0 {
			cur.left--
		}
		mt.Tick(m)
		tick++
	}
	for tick < 2000 {
		step(game)
	}
	// First after 100 ticks (vanilla `timer-- <= 0`: tick 100), then 50 playing + [200, 300] delay.
	if plays[0] != 100 {
		t.Fatalf("first track at %d", plays[0])
	}
	for i := 1; i < len(plays); i++ {
		gap := plays[i] - plays[i-1]
		if gap < 50+200 || gap > 50+300+2 {
			t.Fatalf("gap %d: %v", gap, plays)
		}
	}
	// Vanilla: a type change stops the track and waits [0, MinDelay/2].
	menu := MusicType{Name: "menu", MinDelay: 20, MaxDelay: 600, Pick: func(*rand.Rand) string { return "music.menu" }}
	for cur.left == 0 {
		step(game)
	}
	n := len(plays)
	step(menu)
	if cur.left != 0 {
		t.Fatal("type change should stop the track")
	}
	for i := 0; i < 12 && len(plays) == n; i++ {
		step(menu)
	}
	if len(plays) != n+1 {
		t.Fatal("menu track should start within MinDelay/2 ticks")
	}
	// Fixed change delay and retry when nothing fits.
	mt2 := NewMusicTicker(func(string) Track { return &fakeTrack{left: 1 << 30} }, rand.New(rand.NewSource(2)))
	mt2.ChangeDelay = 100
	empty := MusicType{Name: "game", MinDelay: 1, MaxDelay: 1, Pick: func(*rand.Rand) string { return "" }}
	for i := 0; i <= 100; i++ {
		mt2.Tick(empty)
	}
	if mt2.timer != 400 || mt2.Current() != nil {
		t.Fatalf("retry timer %d", mt2.timer)
	}
	mt2.Tick(MusicType{Name: "menu", Pick: empty.Pick})
	if mt2.timer != 99 {
		t.Fatalf("change delay timer %d", mt2.timer)
	}
}

func TestMusicPool(t *testing.T) {
	p := MusicPool{{Event: "a", Weight: 1, Tags: []string{"day"}}, {Event: "b", Weight: 3, Tags: []string{"day", "night"}}, {Event: "c", Weight: 1, Tags: []string{"cave"}}}
	rng := rand.New(rand.NewSource(3))
	n := map[string]int{}
	for i := 0; i < 20000; i++ {
		n[p.Pick(rng, "day")]++
	}
	if n["c"] != 0 || math.Abs(float64(n["b"])/20000-0.75) > 0.02 {
		t.Fatalf("%v", n)
	}
	if p.Pick(rng, "snow") != "" {
		t.Fatal("no fit should give empty")
	}
}
