package tacz

import (
	"os"
	"slices"
	"testing"

	"github.com/memmaker/voxellib/model"
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
	shells := 0
	for _, g := range p.Guns {
		m, _, err := p.Model(g.ID)
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
		if g.Shell != nil {
			shells++
			if _, err := p.ShellModel(&g); err != nil || g.Shell.Life <= 0 {
				t.Errorf("%s: shell %+v: %v", g.ID, g.Shell, err)
			}
		}
		if g.Mag == 0 && g.Reload.Kind != weapon.Inventory || g.RPM == 0 || g.Name == "" {
			t.Errorf("%s: mag %d rpm %v name %q", g.ID, g.Mag, g.RPM, g.Name)
		}
	}
	if shells < 30 {
		t.Errorf("%d guns eject shells", shells)
	}
	if g := p.Guns[slices.IndexFunc(p.Guns, func(g weapon.Gun) bool { return g.ID == "svd" })]; g.Sight != "scope/scope_view" || g.Zoom != 8.25 {
		t.Errorf("svd aims through %q at %vx, want the PSO-1's scope_view at 8.25x", g.Sight, g.Zoom)
	} else if m, lens, err := p.Model("svd"); err != nil || m.Bone(g.Sight) < 0 || lens == nil || !lens.Scope || len(lens.Oculars) == 0 || len(lens.Divisions) == 0 || lens.Body == nil {
		t.Errorf("svd: no grafted %s (%v)", g.Sight, err)
	}
	for id, bone := range map[string]string{"ak47": "oem_stock_tactical", "rpk": "oem_stock_heavy"} {
		m, _, err := p.Model(id)
		if err != nil {
			t.Fatal(err)
		}
		if !hasGeometry(m, m.Bone(bone)) {
			t.Errorf("%s: built-in %s not drawn", id, bone)
		}
	}
	t.Logf("%d sound cues name no file", len(p.Missing))
}

// hasGeometry reports whether any vertex is bound to bone b or below it.
func hasGeometry(m *model.Model, b int) bool {
	under := func(j int) bool {
		for ; j >= 0; j = m.Bones[j].Parent {
			if j == b {
				return true
			}
		}
		return false
	}
	for _, me := range m.Meshes {
		for _, v := range me.Verts {
			if b >= 0 && under(int(v.Joints[0])) {
				return true
			}
		}
	}
	return false
}
