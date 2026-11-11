package tacz

import (
	"os"
	"testing"

	"github.com/memmaker/voxellib/weapon"
)

// TestLoadPack loads the committed default gun pack and parses every gun model.
func TestLoadPack(t *testing.T) {
	p, err := Load(os.DirFS("../../assets/tacz/tacz_default_gun"), os.DirFS("../../assets/tacz/defaults"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Guns) < 70 || len(p.Ammo) < 25 {
		t.Fatalf("%d guns, %d ammo", len(p.Guns), len(p.Ammo))
	}
	for _, g := range p.Guns {
		m, err := p.Model(g.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range []string{weapon.ClipDraw, weapon.ClipShoot, weapon.ClipIdle} {
			if m.Clips[c] == nil {
				t.Errorf("%s: no clip %s", g.ID, c)
			}
		}
		if want := (weapon.MuzzleFlash{Flash: "assets/tacz/particle_definitions/muzzle_flash.particle.json",
			Smoke: "assets/tacz/particle_definitions/muzzle_flash_smoke.particle.json", Scale: 0.75, FlashLight: 15,
			SmokeScale: 1.65, SpeedScale: 1.7, SmokeCount: 16}); g.ID == "ak47" && (g.Muzzle == nil || *g.Muzzle != want) {
			t.Errorf("ak47 muzzle %+v", g.Muzzle)
		}
		if g.Mag == 0 && g.Reload.Kind != weapon.Inventory || g.RPM == 0 || g.Name == "" {
			t.Errorf("%s: mag %d rpm %v name %q", g.ID, g.Mag, g.RPM, g.Name)
		}
	}
	t.Logf("%d sound cues name no file", len(p.Missing))
}
