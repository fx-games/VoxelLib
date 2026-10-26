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

// A doll knocked flat gets up into its standing pose without its limbs
// changing length or going through the floor, and its skin is the bind pose.
func TestDollRises(t *testing.T) {
	m := humanoid()
	d := New(m, mgl32.Translate3D(2, 0, 2))
	d.Push(mgl32.Vec3{0.3, 0.2, 0}, 0.5)
	for range 200 {
		d.Step(room{})
	}
	stand := mgl32.Translate3D(3, 0, 2).Mul4(mgl32.HomogRotate3DY(1))
	d.Rise(stand, 20)
	for d.Rising() {
		d.RiseStep()
		for _, l := range [][2]int{{HipR, FootR}, {ShoulderL, HandL}, {Pelvis, Neck}} {
			got, want := d.Joint(l[1]).Sub(d.Joint(l[0])).Len(), d.bind[l[1]].Sub(d.bind[l[0]]).Len()
			if math.Abs(float64(got-want)) > 1e-3 {
				t.Fatalf("limb %v is %.3f long, want %.3f", l, got, want)
			}
		}
		for j := range numJoints {
			if d.Joint(j).Y() < -0.01 {
				t.Fatalf("joint %d under the floor at %v", j, d.Joint(j))
			}
		}
	}
	for i, s := range d.Skin(stand, 1, nil) {
		if !s.ApproxEqualThreshold(mgl32.Ident4(), 1e-3) {
			t.Fatalf("bone %d not standing: %v", i, s)
		}
	}
}

// A doll turned by Turn before rising swings up the shortest way, at most
// a right angle, however it lies.
func TestDollTurnsToGetUp(t *testing.T) {
	m := humanoid()
	for _, lay := range []mgl32.Mat4{
		mgl32.HomogRotate3DX(math.Pi / 2), mgl32.HomogRotate3DX(-math.Pi / 2), // on its back, on its front
		mgl32.HomogRotate3DY(2).Mul4(mgl32.HomogRotate3DZ(math.Pi / 2)), // on its side, turned
		mgl32.HomogRotate3DY(-1).Mul4(mgl32.HomogRotate3DX(-1.4)),
	} {
		place := mgl32.Translate3D(2, 0, 2).Mul4(mgl32.HomogRotate3DY(0.5))
		d := New(m, place)
		for j := range d.pos {
			d.pos[j] = mgl32.TransformCoordinate(mgl32.TransformCoordinate(d.pos[j], place.Inv()), place.Mul4(lay))
		}
		d.Rise(place.Mul4(mgl32.HomogRotate3DY(float32(d.Turn()))), 20)
		w := d.rise.turn.W
		if a := 2 * math.Acos(math.Min(1, math.Abs(float64(w)))); a > math.Pi/2+1e-3 {
			t.Fatalf("lying %v: swings up through %.2f rad", lay, a)
		}
	}
}

// corner: a floor at y < 0, walls at x >= 5 and z >= 5, and a step at
// x >= 3 (y < 1) for an edge.
type corner struct{}

func (corner) Solid(x, y, z int) bool              { return y < 0 || x >= 5 || z >= 5 || x >= 3 && y < 1 }
func (corner) Liquid(x, y, z int) character.Liquid { return character.LiquidNone }

func TestDollComesToRestInCornerAndOnEdge(t *testing.T) {
	for _, at := range []mgl32.Vec3{{4.6, 1, 4.6}, {3.1, 1, 2}} {
		d := New(humanoid(), mgl32.Translate3D(at.X(), at.Y(), at.Z()))
		d.Push(mgl32.Vec3{0.2, 0, 0.2}, 0.6)
		for range 300 {
			d.Follow(float64(at.X()), float64(at.Y()), float64(at.Z()), 0.3)
			d.Step(corner{})
		}
		if !d.Asleep() {
			t.Fatalf("doll at %v still moving after 300 ticks", at)
		}
		was := d.pos
		d.Follow(float64(at.X()), float64(at.Y()), float64(at.Z()), 0.3)
		d.Step(corner{})
		if d.pos != was {
			t.Fatalf("doll at %v moved while asleep", at)
		}
	}
}
