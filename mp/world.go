package mp

import (
	"math"
	"strings"

	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/resloc"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/worldgen"
)

// Generator fills a new chunk column (engine.Generator's shape; the
// worldgen generators implement it).
type Generator interface {
	Generate(w *voxel.World, c *voxel.Chunk)
}

// World is the server's copy of the world: the same generator as the
// clients', filled in on demand wherever a player walks or an arrow flies,
// plus every block edit. It is a character.World and a collide.Solids, so
// players move and arrows fly in it exactly as on the clients.
type World struct {
	W   *voxel.World
	Gen Generator
}

// NewWorld makes an empty world with the build limits of a generator
// (see Heights).
func NewWorld(reg *voxel.Registry, gen Generator, minY, height int) *World {
	return &World{W: voxel.NewWorld(reg, minY, height), Gen: gen}
}

// Load makes sure the column holding block x,z exists.
func (w *World) Load(x, z int) *voxel.Chunk {
	p := voxel.ChunkPos{X: x >> 4, Z: z >> 4}
	if c := w.W.Chunk(p); c != nil {
		return c
	}
	c := w.W.NewChunk(p)
	if w.Gen != nil {
		w.Gen.Generate(w.W, c)
	}
	w.W.AddChunk(c)
	return c
}

// LoadAround loads the columns within r blocks of x,z.
func (w *World) LoadAround(x, z float64, r int) {
	bx, bz := int(math.Floor(x)), int(math.Floor(z))
	for cx := (bx - r) >> 4; cx <= (bx+r)>>4; cx++ {
		for cz := (bz - r) >> 4; cz <= (bz+r)>>4; cz++ {
			w.Load(cx<<4, cz<<4)
		}
	}
}

// Block returns the block at x,y,z, generating its column if needed.
func (w *World) Block(x, y, z int) voxel.StateID {
	if y < w.W.MinY || y >= w.W.MaxY() {
		return voxel.Air
	}
	w.Load(x, z)
	return w.W.Block(x, y, z)
}

// SetBlock edits the world; it reports false for a block outside it.
func (w *World) SetBlock(x, y, z int, id voxel.StateID) bool {
	if y < w.W.MinY || y >= w.W.MaxY() {
		return false
	}
	w.Load(x, z)
	ok := w.W.SetBlock(x, y, z, id)
	w.W.TakeDirty() // no renderer to tell
	return ok
}

func (w *World) Solid(x, y, z int) bool {
	if y < w.W.MinY {
		return true
	}
	return w.W.Reg.Info(w.Block(x, y, z)).Collides
}

// Boxes adds stairs, slabs, doors, fences, ... (collide.Boxer).
func (w *World) Boxes(area collide.AABB, out []collide.AABB) []collide.AABB {
	for cx := (int(math.Floor(area.MinX)) - 1) >> 4; cx <= (int(math.Floor(area.MaxX))+1)>>4; cx++ {
		for cz := (int(math.Floor(area.MinZ)) - 1) >> 4; cz <= (int(math.Floor(area.MaxZ))+1)>>4; cz++ {
			w.Load(cx<<4, cz<<4)
		}
	}
	return w.W.Boxes(area, out)
}

func (w *World) Climbable(x, y, z int) bool { return w.W.Climbable(x, y, z) }

func (w *World) Liquid(x, y, z int) character.Liquid { return LiquidOf(w.W.Reg, w.Block(x, y, z)) }

// LiquidOf is the liquid a block is (app.Terrain's rule).
func LiquidOf(reg *voxel.Registry, s voxel.StateID) character.Liquid {
	name, _, _ := strings.Cut(reg.Info(s).Name, "#")
	switch name {
	case resloc.Qualify("water"), resloc.Qualify("flowing_water"):
		return character.LiquidWater
	case resloc.Qualify("lava"), resloc.Qualify("flowing_lava"):
		return character.LiquidLava
	}
	return character.LiquidNone
}

// Surface is the y above the highest solid block of column x,z.
func (w *World) Surface(x, z int) int {
	w.Load(x, z)
	t := w.W.Reg.Table()
	for y := w.W.MaxY() - 1; y >= w.W.MinY; y-- {
		if t[w.W.Block(x, y, z)].Collides {
			return y + 1
		}
	}
	return w.W.MinY
}

// Ground is Surface for a place to stand: false when the top block is
// leaves (a tree top) or there is liquid above it.
func (w *World) Ground(x, z int) (int, bool) {
	y := w.Surface(x, z)
	top, _, _ := strings.Cut(w.W.Reg.Info(w.Block(x, y-1, z)).Name, "#") // a map block's key
	return y, !strings.HasSuffix(top, "_leaves") && !strings.HasSuffix(top, ":leaves") && w.Liquid(x, y, z) == character.LiquidNone
}

// SpawnPoint picks a dry place to stand near the generator's land
// around x,z, at least on solid ground.
func (w *World) SpawnPoint(x, z int) (float64, float64, float64) {
	if sp, ok := w.Gen.(worldgen.Spawner); ok {
		x, z = sp.FindLand(x, z)
	}
	y := w.Surface(x, z)
	return float64(x) + 0.5, float64(y), float64(z) + 0.5
}

var (
	_ character.World   = (*World)(nil)
	_ character.Climber = (*World)(nil)
	_ collide.Solids    = (*World)(nil)
)
