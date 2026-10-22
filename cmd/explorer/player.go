package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/platform/glfw"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/physics"
)

// terrain adapts the engine's world to character.World (and collide.Solids
// for item drops). Unloaded chunks are solid so nothing falls out of the
// world while it streams in.
type terrain struct{ eng *engine.Engine }

func (t *terrain) Solid(x, y, z int) bool {
	w := t.eng.World
	if y < w.MinY {
		return true
	}
	if w.Chunk(voxel.ChunkPos{X: x >> 4, Z: z >> 4}) == nil {
		return true
	}
	return w.Reg.Info(w.Block(x, y, z)).Collides
}

// Boxes adds stairs, slabs, doors, fences, ... (collide.Boxer).
func (t *terrain) Boxes(area collide.AABB, out []collide.AABB) []collide.AABB {
	return t.eng.World.Boxes(area, out)
}

func (t *terrain) Liquid(x, y, z int) character.Liquid {
	name, _, _ := strings.Cut(t.eng.World.Reg.Info(t.eng.World.Block(x, y, z)).Name, "#") // map block keys
	switch name {
	case "minecraft:water", "minecraft:flowing_water":
		return character.LiquidWater
	case "minecraft:lava", "minecraft:flowing_lava":
		return character.LiquidLava
	}
	return character.LiquidNone
}

var _ character.World = (*terrain)(nil)

// tickPlayer runs one 20 Hz step: terrain readiness, debug edits, held
// mouse buttons, then the character controller.
func (g *game) tickPlayer() {
	p := g.player
	g.tick++
	cx, cz := int(math.Floor(p.X))>>4, int(math.Floor(p.Z))>>4
	if !g.eng.ChunkReady(voxel.ChunkPos{X: cx, Z: cz}) {
		g.frozen = true
		p.PrevX, p.PrevY, p.PrevZ = p.X, p.Y, p.Z
		return // wait for terrain under us
	}
	g.frozen = false
	if g.findGround {
		if y, ok := g.eng.SurfaceY(int(math.Floor(p.X)), int(math.Floor(p.Z))); ok {
			if !g.spawnKnown || float64(y) > p.Y {
				p.Teleport(p.X, float64(y), p.Z)
			}
			g.spawnY = p.Y
			g.findGround = false
		}
	}
	if g.edits != "" {
		g.applyEdits()
	}
	if g.swing > 0 {
		g.swing--
	}

	if g.captured {
		if g.win.GetMouseButton(glfw.MouseButtonLeft) == glfw.Press && g.crossbowOn() {
			g.fire() // held down: repeat at the reload rate
		} else if g.win.GetMouseButton(glfw.MouseButtonLeft) == glfw.Press {
			if g.breakCD--; g.breakCD <= 0 {
				g.breakBlock()
			}
		}
		if g.win.GetMouseButton(glfw.MouseButtonRight) == glfw.Press {
			if g.placeCD--; g.placeCD <= 0 {
				g.placeBlock()
			}
		}
	}

	if g.autofly > 0 {
		p.PrevX, p.PrevY, p.PrevZ = p.X, p.Y, p.Z
		p.Flying = true
		p.X += math.Sin(g.cam.Yaw) * g.autofly / 20
		p.Z -= math.Cos(g.cam.Yaw) * g.autofly / 20
		return
	}
	in := character.Input{Yaw: g.cam.Yaw, Pitch: g.cam.Pitch}
	if g.captured {
		in.Forward = axis(g.key(glfw.KeyW), g.key(glfw.KeyS))
		in.Strafe = axis(g.key(glfw.KeyD), g.key(glfw.KeyA))
		in.Jump = g.key(glfw.KeySpace)
		in.Sneak = g.key(glfw.KeyLeftShift)
		in.Sprint = g.key(glfw.KeyLeftControl)
	}
	p.Tick(g.terrain, in)
	g.prevWalk = g.walkDist
	if p.OnGround {
		g.walkDist += math.Hypot(p.X-p.PrevX, p.Z-p.PrevZ)
	}
}

func axis(pos, neg bool) float64 {
	v := 0.0
	if pos {
		v++
	}
	if neg {
		v--
	}
	return v
}

func (g *game) key(k glfw.Key) bool { return g.win.GetKey(k) == glfw.Press }

func (g *game) target() (physics.Hit, bool) {
	ex, ey, ez := g.player.Eye()
	f := g.cam.Forward()
	return g.eng.Raycast(ex, ey, ez, float64(f[0]), float64(f[1]), float64(f[2]), 5)
}

func (g *game) breakBlock() {
	g.breakCD = 5
	h, ok := g.target()
	if !ok || g.eng.World.Reg.Info(h.State).Name == "minecraft:bedrock" || g.toggleDoor(h.Block) {
		return
	}
	g.swing = 6
	g.eng.SetBlock(h.Block.X, h.Block.Y, h.Block.Z, voxel.Air)
	// plants can't float: pop the one standing on the removed block
	above := g.eng.World.Reg.Info(g.eng.Block(h.Block.X, h.Block.Y+1, h.Block.Z))
	if above.Shape == voxel.ShapeCross {
		g.eng.SetBlock(h.Block.X, h.Block.Y+1, h.Block.Z, voxel.Air)
	}
}

func (g *game) placeBlock() {
	g.placeCD = 4
	h, ok := g.target()
	if !ok {
		return
	}
	p := h.Block
	if !g.eng.World.Reg.Info(h.State).Def.Replaceable {
		p = p.Offset(h.Face)
	}
	cur := g.eng.World.Reg.Info(g.eng.Block(p.X, p.Y, p.Z))
	if !cur.Def.Replaceable {
		return
	}
	id := g.hotbar[g.selected]
	if g.eng.World.Reg.Info(id).Collides && !g.player.NoClip && g.player.Box().Intersects(collide.Block(p.X, p.Y, p.Z)) {
		return
	}
	g.swing = 6
	g.eng.SetBlock(p.X, p.Y, p.Z, id)
}

// toggleDoor opens or closes the door at p, both halves (left click or E);
// false if p is not a door.
func (g *game) toggleDoor(p voxel.BlockPos) bool {
	reg := g.eng.World.Reg
	info := reg.Info(g.eng.Block(p.X, p.Y, p.Z))
	if !strings.HasSuffix(info.Name, "_door") {
		return false
	}
	props := reg.StateProps(g.eng.Block(p.X, p.Y, p.Z))
	open := strconv.FormatBool(props["open"] != "true")
	other := p.Y + 1
	if props["half"] == "upper" {
		other = p.Y - 1
	}
	for _, y := range []int{p.Y, other} {
		id := g.eng.Block(p.X, y, p.Z)
		if reg.Info(id).Name != info.Name {
			continue
		}
		pr := reg.StateProps(id)
		pr["open"] = open
		g.eng.SetBlock(p.X, y, p.Z, reg.State(info.Name, pr))
	}
	g.breakCD, g.swing = math.MaxInt32, 6 // holding the button doesn't flap it; the next click resets this
	return true
}

// applyEdits applies the -edits debug flag relative to the player.
func (g *game) applyEdits() {
	p := g.player
	bx, by, bz := int(math.Floor(p.X)), int(math.Floor(p.Y)), int(math.Floor(p.Z))
	for _, e := range strings.Split(g.edits, ";") {
		parts := strings.SplitN(e, ",", 4)
		if len(parts) != 4 {
			continue
		}
		var dx, dy, dz int
		fmt.Sscan(parts[0], &dx)
		fmt.Sscan(parts[1], &dy)
		fmt.Sscan(parts[2], &dz)
		id := voxel.Air
		if parts[3] != "air" {
			id = g.eng.World.Reg.Parse(parts[3])
		}
		g.eng.SetBlock(bx+dx, by+dy, bz+dz, id)
	}
	g.edits = ""
}
