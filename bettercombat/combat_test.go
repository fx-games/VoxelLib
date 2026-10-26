package bettercombat

// Checks the vanilla side against Minecraft 26.2's own classes, run
// headless (testdata/vanilla.json): HumanoidModel poses,
// WalkAnimationState, LivingEntity's body yaw (with Mth.atan2) and the
// first-person render chain's matrices.

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

type vanillaData struct {
	Poses []struct {
		In    [5]float32
		Parts [6]part
	}
	Walk [][][6]float32
	Yaw  []struct {
		Start float32
		Ticks []json.RawMessage // [yRot, xd, zd, attacking, yBodyRotO, yBodyRot]
	}
	Chain []struct {
		Body, Arm, Item [9]float32
		BodyRot         float32
		Display         [9]float32
		ArmM, ItemM     [16]float32
	}
}

func loadVanilla(t *testing.T) vanillaData {
	b, err := os.ReadFile("testdata/vanilla.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vanillaData
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVanillaPose(t *testing.T) {
	for i, p := range loadVanilla(t).Poses {
		var got [6]part
		got[0], got[1], got[2], got[3], got[4], got[5] = vanillaPose(p.In[0], p.In[1], p.In[2], p.In[3], p.In[4])
		for j := range got {
			for k := range got[j] {
				if !near(got[j][k], p.Parts[j][k], 1e-6) {
					t.Fatalf("pose %d part %d[%d]: %v, want %v (in %v)", i, j, k, got[j][k], p.Parts[j][k], p.In)
				}
			}
		}
	}
}

func TestWalkState(t *testing.T) {
	for i, seq := range loadVanilla(t).Walk {
		var w walkState
		for j, s := range seq {
			w.update(min(s[0]*4, 1), 0.4, 1)
			got := [4]float32{w.positionAt(s[1]), w.speedAt(s[1]), w.position, w.speed}
			for k, g := range got {
				if g != s[2+k] {
					t.Fatalf("walk %d tick %d [%d]: %v, want %v", i, j, k, g, s[2+k])
				}
			}
		}
	}
}

func TestBodyYaw(t *testing.T) {
	for i, seq := range loadVanilla(t).Yaw {
		g := &Humanoid{bodyYaw: seq.Start, prevBodyYaw: seq.Start}
		for j, raw := range seq.Ticks {
			var tk [6]json.Number
			var attacking bool
			var row []any
			json.Unmarshal(raw, &row)
			attacking = row[3].(bool)
			json.Unmarshal(raw, &[]any{&tk[0], &tk[1], &tk[2], &attacking, &tk[4], &tk[5]})
			f := func(n json.Number) float64 { v, _ := n.Float64(); return v }
			g.tickBodyYaw(float32(f(tk[0])), f(tk[1]), f(tk[2]), attacking)
			if g.prevBodyYaw != float32(f(tk[4])) || g.bodyYaw != float32(f(tk[5])) {
				t.Fatalf("yaw %d tick %d: %v %v, want %v %v", i, j, g.prevBodyYaw, g.bodyYaw, f(tk[4]), f(tk[5]))
			}
		}
	}
}

func TestChain(t *testing.T) {
	for i, c := range loadVanilla(t).Chain {
		disp := display{mgl32.Vec3{c.Display[0], c.Display[1], c.Display[2]}, mgl32.Vec3{c.Display[3], c.Display[4], c.Display[5]}, mgl32.Vec3{c.Display[6], c.Display[7], c.Display[8]}}
		var body, arm, item [axes]float32
		copy(body[:], c.Body[:])
		copy(arm[:], c.Arm[:])
		copy(item[:], c.Item[:])
		_, armM, itemM := bcChain(body, arm, item, c.BodyRot, 0, 0, disp)
		itemM = itemM.Mul4(mgl32.Translate3D(-0.5, -0.5, -0.5)) // the item model's centring
		for k := range 16 {
			if !near(armM[k], c.ArmM[k], 2e-4) {
				t.Fatalf("chain %d armM[%d]: %v, want %v", i, k, armM[k], c.ArmM[k])
			}
			if !near(itemM[k], c.ItemM[k], 2e-4) {
				t.Fatalf("chain %d itemM[%d]: %v, want %v", i, k, itemM[k], c.ItemM[k])
			}
		}
	}
}
