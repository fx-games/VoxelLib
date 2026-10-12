package mcmodel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/memmaker/voxellib/model"
)

func TestImportRepo(t *testing.T) {
	src := filepath.Join("..", "..", "assets", "cleannrooster")
	if _, err := os.Stat(src); err != nil {
		t.Skip("assets not available")
	}
	p, err := Import(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range p.Skipped {
		t.Log("skipped:", s)
	}
	if len(p.Models) < 20 || len(p.Items) < 100 || len(p.Animations) < 80 {
		t.Fatalf("%d models, %d items, %d animations", len(p.Models), len(p.Items), len(p.Animations))
	}
	humanoids := 0
	for _, m := range p.Models {
		if m.Tags["humanoid"] {
			humanoids++
			if len(m.Clips) < 50 {
				t.Errorf("%s: humanoid with only %d clips", m.Name, len(m.Clips))
			}
		}
		lo, hi := m.Bounds()
		a := model.NewAnimator(m)
		for _, n := range m.ClipNames() {
			a.Play(n, true)
			a.Update(0.7)
			for _, s := range a.Skin(nil) {
				for _, f := range s {
					if f != f {
						t.Fatalf("%s/%s: NaN in skin", m.Name, n)
					}
				}
			}
		}
		t.Logf("%s: %d bones, %d clips, bounds %.2f..%.2f", m.Name, len(m.Bones), len(m.Clips), lo, hi)
	}
	if humanoids < 15 {
		t.Errorf("only %d humanoids", humanoids)
	}
}
