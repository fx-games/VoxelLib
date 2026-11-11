package tacz

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/memmaker/voxellib/weapon"
)

// TestSoundsResolve fails when a sound the pack references (clip cues, display
// sounds) resolves to no pack file, unless testdata/absent_sounds.txt lists
// it: ids with no .ogg at their TACZ location (TACZ plays nothing for them).
func TestSoundsResolve(t *testing.T) {
	p, err := Load(os.DirFS("../../assets/tacz/tacz_default_gun"), os.DirFS("../../assets/tacz/defaults"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/absent_sounds.txt")
	if err != nil {
		t.Fatal(err)
	}
	// One id per line, verbatim: pack ids may carry spaces.
	absent := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	for _, id := range p.Missing {
		if !slices.Contains(absent, id) {
			t.Errorf("sound %q resolves to no file (%s)", id, SoundFile(id))
		}
	}
	for _, id := range absent {
		if !slices.Contains(p.Missing, id) {
			t.Errorf("sound %q resolves now; drop it from testdata/absent_sounds.txt", id)
		}
	}
	for _, g := range p.Guns {
		if g.Sounds[weapon.SoundShoot] == "" || g.Sounds[weapon.SoundDryFire] == "" {
			t.Errorf("%s: no shoot or dry_fire sound: %v", g.ID, g.Sounds)
		}
	}
}
