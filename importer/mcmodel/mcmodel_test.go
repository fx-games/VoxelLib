package mcmodel

import (
	"os"
	"path/filepath"
	"strings"
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

// Boots left at the model root must follow the legs, as in Minecraft.
func TestArmorFollowsLimb(t *testing.T) {
	p, err := Import("../../assets/cleannrooster")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, m := range p.Models {
		for i, b := range m.Bones {
			if p := b.Parent; p >= i {
				t.Errorf("%s: bone %s comes before its parent", m.Name, b.Name)
			}
			want := map[string]string{"armorleftboot": "leftleg", "armorrightboot": "rightleg"}[strings.ToLower(b.Name)]
			if want == "" || !m.Tags["humanoid"] {
				continue
			}
			checked++
			if b.Parent < 0 || canonBone(m.Bones[b.Parent].Name) != want {
				t.Errorf("%s: %s not attached to %s", m.Name, b.Name, want)
			}
		}
	}
	if checked == 0 {
		t.Error("no boots found")
	}
}

// The crossbow artmob carries can be cut out as its own model.
func TestSubtreeCrossbow(t *testing.T) {
	p, err := Import("../../assets/cleannrooster")
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range p.Models {
		if m.Name != "artmob" {
			continue
		}
		cb := m.Subtree("repeater_crossbow_arrow")
		if cb == nil {
			t.Fatal("no crossbow in artmob")
		}
		lo, hi := cb.Bounds()
		size := hi.Sub(lo)
		if size.Len() < 0.5 || size.Len() > 2 {
			t.Errorf("crossbow size %v, want roughly a block", size)
		}
		t.Logf("crossbow bounds %v .. %v", lo, hi)
		return
	}
	t.Fatal("artmob not found")
}
