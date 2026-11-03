package actors

import (
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/resloc"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/nav"
)

// Terrain is a World (and nav.World) over a voxel.World with Java block
// states: Solid is the path finder's cell test, BodySolid the cubes bodies
// collide with, doors and fence gates open through their "open" property.
// Unloaded chunks and below the world are solid. Map worlds, whose block
// keys hold no properties, have no doors.
type Terrain struct {
	W   *voxel.World
	buf []collide.AABB
}

func (t *Terrain) loaded(x, y, z int) bool {
	return y >= t.W.MinY && t.W.Chunk(voxel.ChunkPos{X: x >> 4, Z: z >> 4}) != nil
}

// Solid is a cell paths cannot enter (PathFinder's -1/-2/-3 cells): full
// blocks, blocks with a collision box in the cell taller than a snow layer
// (slabs, stairs, fence posts, doors; not carpet), and the cell above a
// block whose box reaches into it (fences, walls, closed gates). Doors
// count whatever their state; nav.Graph.Door lets walkers through.
func (t *Terrain) Solid(x, y, z int) bool {
	if !t.loaded(x, y, z) {
		return true
	}
	reg := t.W.Reg
	id := t.W.Block(x, y, z)
	info := reg.Info(id)
	if info.Collides {
		return true
	}
	if info.Shape != voxel.ShapeBoxes {
		if b := reg.Info(t.W.Block(x, y-1, z)); b.Shape != voxel.ShapeBoxes {
			return false // nothing reaches up into the cell
		}
	}
	fy := float64(y)
	t.buf = t.W.Boxes(collide.AABB{MinX: float64(x), MinY: fy, MinZ: float64(z), MaxX: float64(x) + 1, MaxY: fy + 1, MaxZ: float64(z) + 1}, t.buf[:0])
	for _, b := range t.buf {
		if b.MinY < fy || b.MaxY-b.MinY > 2.0/16 {
			return true
		}
	}
	return false
}

// BodySolid is the whole-cube test bodies collide with (BodySolids):
// slabs, fences and doors collide through Boxes with their real boxes.
func (t *Terrain) BodySolid(x, y, z int) bool {
	return !t.loaded(x, y, z) || t.W.Reg.Info(t.W.Block(x, y, z)).Collides
}

// Boxes is the world's block boxes (collide.Boxer).
func (t *Terrain) Boxes(area collide.AABB, out []collide.AABB) []collide.AABB {
	return t.W.Boxes(area, out)
}

func (t *Terrain) Climbable(x, y, z int) bool { return t.W.Climbable(x, y, z) }

// Stairs is nav.Graph.Stairs.
func (t *Terrain) Stairs(x, y, z int) bool { return t.W.StairOrSlab(x, y, z) }

func (t *Terrain) Liquid(x, y, z int) character.Liquid {
	switch t.W.Reg.Info(t.W.Block(x, y, z)).Name {
	case resloc.Qualify("water"), resloc.Qualify("flowing_water"):
		return character.LiquidWater
	case resloc.Qualify("lava"), resloc.Qualify("flowing_lava"):
		return character.LiquidLava
	}
	return character.LiquidNone
}

// Water is nav.Graph.Water.
func (t *Terrain) Water(x, y, z int) bool { return t.Liquid(x, y, z) == character.LiquidWater }

func doorName(name string) bool {
	return strings.HasSuffix(name, "_door") || strings.HasSuffix(name, "_fence_gate")
}

// Door reports a door or fence gate at x, y, z and whether it is open.
func (t *Terrain) Door(x, y, z int) (door, open bool) {
	id := t.W.Block(x, y, z)
	if !doorName(t.W.Reg.Info(id).Name) {
		return false, false
	}
	return true, t.W.Reg.StateProps(id)["open"] == "true"
}

// IsDoor is nav.Graph.Door.
func (t *Terrain) IsDoor(x, y, z int) bool { d, _ := t.Door(x, y, z); return d }

// SetDoor opens or closes the door or gate at x, y, z, both halves of a door.
func (t *Terrain) SetDoor(x, y, z int, open bool) {
	reg := t.W.Reg
	id := t.W.Block(x, y, z)
	name := reg.Info(id).Name
	if !doorName(name) {
		return
	}
	other := y
	switch reg.StateProps(id)["half"] {
	case "lower":
		other = y + 1
	case "upper":
		other = y - 1
	}
	for _, y := range []int{y, other} {
		if id := t.W.Block(x, y, z); reg.Info(id).Name == name {
			p := reg.StateProps(id)
			p["open"] = strconv.FormatBool(open)
			t.W.SetBlock(x, y, z, reg.State(name, p))
		}
	}
}

// Graph is a path finder over t for a walker of the given size (nav.SizeOf),
// opening doors, swimming, taking stairs and ladders.
func (t *Terrain) Graph(size [2]int) *nav.Graph {
	g := &nav.Graph{W: t, Size: size, Door: t.IsDoor, Water: t.Water, Stairs: t.Stairs}
	g.Links = g.Ladders(t.Climbable)
	return g
}

var (
	_ World      = (*Terrain)(nil)
	_ BodySolids = (*Terrain)(nil)
)
