package tacz

import (
	"os"
	"slices"
	"testing"

	"github.com/memmaker/voxellib/weapon"
)

// TestRefit installs attachments on the AK-47 and checks the spec and
// model follow the choice, and that every gun builds its spec with every
// attachment it allows.
func TestRefit(t *testing.T) {
	p, err := Load(os.DirFS("../../assets/tacz/tacz_default_gun"), os.DirFS("../../assets/tacz/defaults"))
	if err != nil {
		t.Fatal(err)
	}
	const gun, scope, silencer, mag = "ak47", "tacz:scope_acog_ta31", "tacz:muzzle_silencer_ursus", "tacz:extended_mag_2"
	if got := p.AttachmentSlots(gun); !slices.Equal(got, []Slot{SlotScope, SlotMuzzle, SlotStock, SlotExtendedMag}) {
		t.Errorf("%s slots %v", gun, got)
	}
	for s, id := range map[Slot]string{SlotScope: scope, SlotMuzzle: silencer, SlotExtendedMag: mag} {
		if !slices.Contains(p.Allowed(gun, s), id) {
			t.Fatalf("%s %s: %v lacks %s", gun, s, p.Allowed(gun, s), id)
		}
	}
	c := p.DefaultChoice(gun)
	c[SlotScope], c[SlotMuzzle], c[SlotExtendedMag] = scope, silencer, mag
	g, err := p.Gun(gun, c)
	if err != nil {
		t.Fatal(err)
	}
	if g.Sight != graftName(SlotScope, "scope_view") || g.Zoom <= 1 || g.Mag != 37 || g.Silence == nil || !g.Silence.UseSilenceSound || g.Silence.DistanceAddend >= 0 {
		t.Errorf("refitted %s: sight %q zoom %v mag %d silence %+v", gun, g.Sight, g.Zoom, g.Mag, g.Silence)
	}
	if m, lens, err := p.Model(gun, c); err != nil || lens == nil || m.Bone(g.Sight) < 0 {
		t.Errorf("refitted %s model: lens %v, %v", gun, lens, err)
	}
	if p.AttachmentSound(scope, SoundInstall) == "" || p.AttachmentSound(scope, SoundUninstall) == "" {
		t.Errorf("%s: no refit sounds", scope)
	}
	if b, err := p.Gun(gun, Choice{SlotMuzzle: "tacz:bayonet_6h3"}); err != nil || b.Melee == nil || b.Melee.Kind != weapon.MeleeBayonet {
		t.Errorf("%s with a bayonet: melee %+v, %v", gun, b.Melee, err)
	}
	if _, err := p.Gun(gun, Choice{SlotScope: silencer}); err == nil {
		t.Error("a muzzle in the scope slot was accepted")
	}
	for _, spec := range p.Guns {
		for _, s := range p.AttachmentSlots(spec.ID) {
			for _, id := range p.Allowed(spec.ID, s) {
				c := p.DefaultChoice(spec.ID)
				c[s] = id
				if _, err := p.Gun(spec.ID, c); err != nil {
					t.Error(err)
				}
			}
		}
	}
}
