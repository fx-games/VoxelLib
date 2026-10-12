package audio

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestSpatial(t *testing.T) {
	// yaw 0: forward is -z, right is +x.
	l, r := Spatial(0, 0, 0, 0, 5, 0, -1)
	if !(r > l) {
		t.Fatalf("source on the right: l=%v r=%v", l, r)
	}
	l, r = Spatial(0, 0, 0, 0, -5, 0, -1)
	if !(l > r) {
		t.Fatalf("source on the left: l=%v r=%v", l, r)
	}
	if l, r = Spatial(0, 0, 0, 0, 0, 0, 100); l != 0 || r != 0 {
		t.Fatalf("beyond max should be silent: %v %v", l, r)
	}
	fl, fr := Spatial(0, 0, 0, 0, 0, 0, -10)
	bl, br := Spatial(0, 0, 0, 0, 0, 0, 10)
	if !(bl < fl && br < fr) {
		t.Fatal("behind should be dampened")
	}
	prev := float32(math.MaxFloat32)
	for d := 0.0; d < 48; d += 1 {
		l, r := Spatial(0, 0, 0, 0, 0, 0, -d)
		if l > prev || l < 0 || r < 0 {
			t.Fatalf("not monotonic at %v", d)
		}
		prev = l
	}
}

func sample(p []byte, i int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(p[4*i:]))
}

func TestRead(t *testing.T) {
	m := &Mixer{}
	buf := make([]byte, 8*100)
	if n, _ := m.Read(buf); n != len(buf) || sample(buf, 0) != 0 {
		t.Fatal("empty mixer should give silence")
	}
	one := &Sound{Samples: []float32{0.5, 0.5, 0.5, 0.5}}
	m.Play(one, 1)
	m.Read(buf)
	if sample(buf, 0) == 0 || sample(buf, 0) != sample(buf, 1) {
		t.Fatal("centred sound expected on both channels")
	}
	if sample(buf, 2*4) != 0 || len(m.voices) != 0 {
		t.Fatal("one-shot should end and be removed")
	}
	v := m.Loop(one, 5, 0, -1, 1)
	m.Read(buf)
	if sample(buf, 2*50+1) == 0 || len(m.voices) != 1 {
		t.Fatal("loop should keep playing")
	}
	v.Stop()
	m.Read(buf)
	if len(m.voices) != 0 {
		t.Fatal("stopped voice should be removed")
	}
	for i := 0; i < 40; i++ {
		m.Play(one, 1)
	}
	if len(m.voices) != maxVoices {
		t.Fatalf("voice cap: %d", len(m.voices))
	}
}
