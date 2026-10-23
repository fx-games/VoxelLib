package character

import (
	"math"

	"github.com/memmaker/voxellib/voxel/collide"
)

// LiquidMeta is an optional World method: the liquid's metadata at a cell
// (0 source, 1..7 lower flowing levels, 8+ falling). Without it every
// liquid is a still source: no current.
type LiquidMeta interface {
	LiquidMeta(x, y, z int) int
}

// FlowMaterial is an optional World method: a cell's Material
// blocksMovement and isSolid, and whether it is ice (the one solid
// material falling water does not turn down beside). Without it Solid
// stands in for both, and there is no ice.
type FlowMaterial interface {
	FlowMaterial(x, y, z int) (blocksMovement, solid, ice bool)
}

// WaterPush is Entity.handleWaterMovement's World.handleMaterialAcceleration
// for an entity box: whether water is in the box shrunk 0.4 from top and
// bottom and 0.001 all round (its height test always holds, so any water
// cell counts), and the current's push: the sum of those cells' flow
// vectors, normalized, times 0.014.
func WaterPush(w World, box collide.AABB) (in bool, vx, vy, vz float64) {
	b := box.Grow(-0.001, -0.4000000059604645-0.001, -0.001)
	var fx, fy, fz float64
	for x := floorInt(b.MinX); x < floorInt(b.MaxX+1); x++ {
		for y := floorInt(b.MinY); y < floorInt(b.MaxY+1); y++ {
			for z := floorInt(b.MinZ); z < floorInt(b.MaxZ+1); z++ {
				if w.Liquid(x, y, z) == LiquidWater {
					in = true
					x1, y1, z1 := flow(w, x, y, z)
					fx, fy, fz = fx+x1, fy+y1, fz+z1
				}
			}
		}
	}
	if l := sqrtF(fx*fx + fy*fy + fz*fz); l > 0 {
		fx, fy, fz = normalize(fx, fy, fz)
		vx, vy, vz = fx*0.014, fy*0.014, fz*0.014
	}
	return in, vx, vy, vz
}

// decay is BlockLiquid.getEffectiveFlowDecay for water: -1 off water,
// a falling level (8+) counts as a source.
func decay(w World, lm LiquidMeta, x, y, z int) int {
	if w.Liquid(x, y, z) != LiquidWater {
		return -1
	}
	if lm == nil {
		return 0
	}
	if m := lm.LiquidMeta(x, y, z); m < 8 {
		return m
	}
	return 0
}

// flow is BlockLiquid.getFlowVector: toward lower neighbours (or over an
// open edge to water below), straight down beside a solid face when the
// water is falling, normalized.
func flow(w World, x, y, z int) (vx, vy, vz float64) {
	lm, _ := w.(LiquidMeta)
	fm, _ := w.(FlowMaterial)
	mat := func(x, y, z int) (blocks, solid, ice bool) {
		if fm != nil {
			return fm.FlowMaterial(x, y, z)
		}
		s := w.Solid(x, y, z)
		return s, s, false
	}
	l := decay(w, lm, x, y, z)
	for _, d := range [4][2]int{{-1, 0}, {0, -1}, {1, 0}, {0, 1}} {
		nx, nz := x+d[0], z+d[1]
		n := decay(w, lm, nx, y, nz)
		j := n - l
		if n < 0 {
			if blocks, _, _ := mat(nx, y, nz); blocks {
				continue
			}
			if n = decay(w, lm, nx, y-1, nz); n < 0 {
				continue
			}
			j = n - (l - 8)
		}
		vx, vz = vx+float64(d[0]*j), vz+float64(d[1]*j)
	}
	if lm != nil && lm.LiquidMeta(x, y, z) >= 8 {
		// func_149747_d for sides 2..5: not water, not ice, Material.isSolid
		side := func(x, y, z int) bool {
			_, solid, ice := mat(x, y, z)
			return w.Liquid(x, y, z) != LiquidWater && solid && !ice
		}
		for dy := 0; dy <= 1; dy++ {
			if side(x, y+dy, z-1) || side(x, y+dy, z+1) || side(x-1, y+dy, z) || side(x+1, y+dy, z) {
				vx, vy, vz = normalize(vx, vy, vz)
				vy -= 6
				break
			}
		}
	}
	return normalize(vx, vy, vz)
}

// normalize is Vec3.normalize: by the float square root, zero below 1e-4.
func normalize(x, y, z float64) (float64, float64, float64) {
	l := sqrtF(x*x + y*y + z*z)
	if l < 1e-4 {
		return 0, 0, 0
	}
	return x / l, y / l, z / l
}

func sqrtF(v float64) float64 { return float64(float32(math.Sqrt(v))) }

func floorInt(v float64) int { return int(math.Floor(v)) }
