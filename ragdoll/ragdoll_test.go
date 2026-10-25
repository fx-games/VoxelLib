package ragdoll

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/voxel/collide"
)

// room: a floor at y < 0 and a wall at x >= 5.
type room struct{}

func (room) Solid(x, y, z int) bool              { return y < 0 || x >= 5 }
func (room) Liquid(x, y, z int) character.Liquid { return character.LiquidNone }

var _ collide.Solids = room{}

func TestKickSlamsIntoWall(t *testing.T) {
	c := character.New(1.5, 0, 0.5)
	Kick(c, math.Pi/2, 0.8, 0.3) // towards +X
	var f Flight
	wall := false
	for range 100 {
		imp, rest := f.Tick(c, room{})
		if imp.Wall {
			wall = true
			if imp.Speed < 0.2 || len(imp.Cells) == 0 {
				t.Fatalf("impact %+v", imp)
			}
		}
		if rest {
			break
		}
	}
	if !wall || c.X > 5 {
		t.Fatalf("no wall hit; at x %.2f", c.X)
	}
}

func humanoid() *model.Model {
	m := &model.Model{}
	for _, n := range []string{"body", "head", "rightArm", "leftArm", "rightLeg", "leftLeg", "hat"} {
		p := -1
		if n == "hat" {
			p = 1
		}
		m.Bones = append(m.Bones, model.Bone{Name: n, Parent: p})
	}
	return m
}

func TestDollRestsOnFloorAndSkinsAtBind(t *testing.T) {
	m := humanoid()
	d := New(m, mgl32.Translate3D(2, 0, 2))
	skin := d.Skin(mgl32.Translate3D(2, 0, 2), 1, nil)
	if len(skin) != len(m.Bones) {
		t.Fatalf("%d skin matrices for %d bones", len(skin), len(m.Bones))
	}
	for i, s := range skin {
		if !s.ApproxEqualThreshold(mgl32.Ident4(), 1e-4) {
			t.Fatalf("bone %d not at bind: %v", i, s)
		}
	}
	d.Push(mgl32.Vec3{0.3, 0.2, 0}, 0.5)
	for range 200 {
		d.Step(room{})
	}
	for j := range numJoints {
		p := d.Joint(j)
		if p.Y() < -0.01 || p.X() > 5 {
			t.Fatalf("joint %d through the world at %v", j, p)
		}
	}
	if d.Joint(Crown).Y() > 0.6 {
		t.Fatalf("still standing: crown at %v", d.Joint(Crown))
	}
}
