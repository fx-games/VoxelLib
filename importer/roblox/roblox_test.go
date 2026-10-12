package roblox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/memmaker/voxellib/model"
)

func TestCubePets(t *testing.T) {
	p := filepath.Join(os.Getenv("HOME"), "Downloads/CubePetsVol2_Free_v1.0.zip")
	if _, err := os.Stat(p); err != nil {
		t.Skip("pack not available")
	}
	ms, err := Import(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 3 {
		t.Fatalf("got %d models", len(ms))
	}
	for _, m := range ms {
		if len(m.Clips) != 5 || len(m.Textures) == 0 || len(m.Meshes) == 0 {
			t.Errorf("%s: %d clips, %d textures, %d meshes", m.Name, len(m.Clips), len(m.Textures), len(m.Meshes))
		}
		if fly := m.Tags["flying"]; fly != (m.Name == "Butterfly") {
			t.Errorf("%s flying=%v", m.Name, fly)
		}
		// Posing must move vertices but keep them finite and near the model.
		a := model.NewAnimator(m)
		a.Play("Walk", false)
		a.Update(0.5)
		sk := a.Skin(nil)
		lo, hi := m.Bounds()
		for _, me := range m.Meshes {
			for _, v := range me.Verts {
				p := sk[v.Joints[0]].Mul4x1(v.Pos.Vec4(1)).Vec3()
				for k := 0; k < 3; k++ {
					if p[k] < lo[k]-1 || p[k] > hi[k]+1 {
						t.Fatalf("%s: posed vertex %v far outside %v..%v", m.Name, p, lo, hi)
					}
				}
			}
		}
		t.Logf("%s: bounds %v..%v clips %v", m.Name, lo, hi, m.ClipNames())
	}
}
