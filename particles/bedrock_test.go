package particles

import (
	"math"
	"math/rand"
	"os"
	"testing"

	"github.com/memmaker/voxellib/weapon"
)

// TestTACZMuzzle loads TACZ's muzzle flash and smoke and checks one shot
// of the AK-47's muzzle_flash block with a seeded RNG.
func TestTACZMuzzle(t *testing.T) {
	pack := os.DirFS("../assets/tacz/tacz_default_gun")
	ak := weapon.MuzzleFlash{Scale: 0.75, SmokeScale: 1.65, SpeedScale: 1.7, SmokeCount: 16}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-5 }

	flash, err := LoadEffect(pack, "assets/tacz/particle_definitions/muzzle_flash.particle.json")
	if err != nil {
		t.Fatal(err)
	}
	em := flash.Start(ak.FlashVars(), rand.New(rand.NewSource(1)), 1)
	em.Tick(0, nil)
	ps := em.Sprites(nil)
	if len(ps) != 1 || !near(ps[0].Life, 0.02) || !near(float64(ps[0].Size0), 2*0.75*0.45) || !flash.Additive || flash.Lit {
		t.Fatalf("flash %+v", ps)
	}
	if u := ps[0].UV[0] * 1152; u < 256 || math.Mod(float64(u)+1e-3, 64) > 2e-3 || !near(float64(ps[0].UV[2]-ps[0].UV[0]), 64.0/1152) {
		t.Fatalf("flash uv %v", ps[0].UV)
	}
	em.Tick(0.05, nil)
	if len(em.Sprites(nil)) != 0 || em.Done() {
		t.Fatal("flash should be gone, emitter still active")
	}

	smoke, err := LoadEffect(pack, "assets/tacz/particle_definitions/muzzle_flash_smoke.particle.json")
	if err != nil {
		t.Fatal(err)
	}
	shot := func() []Particle {
		var world System
		smoke.Start(ak.SmokeVars(), rand.New(rand.NewSource(7)), 2).Tick(0, &world)
		return world.P
	}
	a, b := shot(), shot()
	if len(a) < 4 || len(a) > 16 || len(a) != len(b) || a[0].X != b[0].X || a[0].Life != b[0].Life {
		t.Fatalf("smoke count %d vs %d", len(a), len(b))
	}
	for _, p := range a {
		speed := math.Sqrt(p.VX*p.VX + p.VY*p.VY + p.VZ*p.VZ)
		if p.Life < 0.12 || p.Life > 0.25 || !near(speed, 6*1.7) || !p.Lit || !p.Ghost || p.Frames != 6 ||
			!near(float64(p.Size1/p.Size0), 3.2) || p.Size0 < 2*1.65*0.1*0.8-1e-5 || p.Size0 > 2*1.65*0.1*1.2+1e-5 {
			t.Fatalf("smoke %+v", p)
		}
	}
	if c := a[0].Grad.At(0); !near(float64(c[3]), 128.0/255) || !near(float64(c[0]), 140.0/255) {
		t.Fatalf("smoke colour %v", c)
	}
}
