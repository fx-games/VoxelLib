package overworld

import (
	"math"

	"github.com/memmaker/voxellib/voxel"
)

// Generator produces deterministic overworld-like terrain for a seed.
type Generator struct {
	Seed     int64
	SeaLevel int

	cont, erosion, peaks, temp, detail *Octaves
	overhang, cheese, spagA, spagB     *Octaves

	air, stone, deepslate, bedrock, dirt, grass, snowyGrass, sand, sandstone,
	gravel, water, lava, snowBlock, clay voxel.StateID
	ores      []ore
	tallGrass voxel.StateID
	fern      voxel.StateID
	flowers   []voxel.StateID
	deadBush  voxel.StateID
	trees     []treeKind
}

type ore struct {
	normal, deep voxel.StateID
	veins, size  int
	minY, maxY   int
}

type treeKind struct {
	log, leaves voxel.StateID
	spruce      bool
	minH, maxH  int
}

// New creates a generator. It registers the states it needs in reg.
func New(reg *voxel.Registry, seed int64) *Generator {
	g := &Generator{Seed: seed, SeaLevel: 62}
	g.cont = NewOctaves(seed+1, 4, 1.0/700)
	g.erosion = NewOctaves(seed+2, 3, 1.0/400)
	g.peaks = NewOctaves(seed+3, 4, 1.0/250)
	g.temp = NewOctaves(seed+4, 2, 1.0/900)
	g.detail = NewOctaves(seed+5, 3, 1.0/40)
	g.overhang = NewOctaves(seed+6, 2, 1.0/48)
	g.cheese = NewOctaves(seed+7, 2, 1.0/70)
	g.spagA = NewOctaves(seed+8, 1, 1.0/55)
	g.spagB = NewOctaves(seed+9, 1, 1.0/55)

	s := reg.Default
	g.air, g.stone, g.deepslate, g.bedrock = voxel.Air, s("stone"), reg.Parse("deepslate[axis=y]"), s("bedrock")
	g.dirt, g.grass, g.snowyGrass = s("dirt"), reg.Parse("grass_block[snowy=false]"), reg.Parse("grass_block[snowy=true]")
	g.sand, g.sandstone, g.gravel, g.clay = s("sand"), s("sandstone"), s("gravel"), s("clay")
	g.water, g.lava, g.snowBlock = reg.Parse("water[level=0]"), reg.Parse("lava[level=0]"), s("snow_block")
	g.ores = []ore{
		{s("coal_ore"), s("deepslate_coal_ore"), 18, 12, 0, 136},
		{s("iron_ore"), s("deepslate_iron_ore"), 10, 8, -24, 64},
		{s("copper_ore"), s("copper_ore"), 6, 9, 0, 96},
		{s("gold_ore"), s("deepslate_gold_ore"), 4, 7, -64, 32},
		{s("redstone_ore"), s("redstone_ore"), 4, 7, -64, 16},
		{s("lapis_ore"), s("lapis_ore"), 2, 6, -64, 32},
		{s("diamond_ore"), s("deepslate_diamond_ore"), 2, 5, -64, 16},
		{s("gravel"), s("gravel"), 6, 20, -64, 128},
		{s("granite"), s("granite"), 3, 30, 0, 100},
		{s("andesite"), s("andesite"), 3, 30, 0, 100},
	}
	g.tallGrass, g.fern, g.deadBush = s("short_grass"), s("fern"), s("dead_bush")
	for _, f := range []string{"dandelion", "poppy", "cornflower", "oxeye_daisy", "red_tulip", "allium", "blue_orchid"} {
		g.flowers = append(g.flowers, s(f))
	}
	g.trees = []treeKind{
		{reg.Parse("oak_log[axis=y]"), reg.Parse("oak_leaves[distance=1,persistent=false,waterlogged=false]"), false, 4, 6},
		{reg.Parse("birch_log[axis=y]"), reg.Parse("birch_leaves[distance=1,persistent=false,waterlogged=false]"), false, 5, 7},
		{reg.Parse("spruce_log[axis=y]"), reg.Parse("spruce_leaves[distance=1,persistent=false,waterlogged=false]"), true, 6, 9},
	}
	return g
}

func smoothstep(a, b, x float64) float64 {
	t := math.Max(0, math.Min(1, (x-a)/(b-a)))
	return t * t * (3 - 2*t)
}

// column describes the 2D climate/height at x,z.
type column struct {
	height   float64 // target surface height
	rough    float64 // 0..1 how much 3D overhang noise applies
	temp     float64
	beachish bool
}

func (g *Generator) column(x, z int) column {
	fx, fz := float64(x), float64(z)
	c := g.cont.At2(fx, fz) * 1.6
	e := g.erosion.At2(fx, fz)
	p := g.peaks.At2(fx, fz)
	ridge := 1 - math.Abs(p)*2 // ridged noise for mountain chains
	// continent spline: deep ocean -> coast -> lowland -> inland plateau
	var h float64
	switch {
	case c < -0.35:
		h = 30 + (c+1)*30
	case c < -0.1:
		h = 49 + (c+0.35)/0.25*15
	default:
		h = 64 + (c+0.1)*40
	}
	inland := smoothstep(-0.05, 0.25, c)
	mount := smoothstep(0.0, -0.45, e) * inland // low erosion => mountains
	h += mount * math.Max(0, ridge) * 95
	hill := smoothstep(0.4, -0.2, e) * inland
	h += g.detail.At2(fx, fz) * (3 + 10*hill)
	col := column{height: h, rough: mount, temp: g.temp.At2(fx, fz) - (h-64)/250}
	col.beachish = h < float64(g.SeaLevel)+3 && h > float64(g.SeaLevel)-4
	return col
}

const (
	cellXZ = 4
	cellY  = 4
)

// Generate fills a fresh chunk and computes its heightmap.
func (g *Generator) Generate(w *voxel.World, ch *voxel.Chunk) {
	bx, bz := ch.Pos.X<<4, ch.Pos.Z<<4
	minY, maxY := w.MinY, w.MaxY()
	ny := (maxY-minY)/cellY + 1

	var cols [16][16]column
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			cols[z][x] = g.column(bx+x, bz+z)
		}
	}
	// Coarse 3D lattice of overhang + cave fields, interpolated below.
	type cell struct{ over, cheese, spa, spb float64 }
	lat := make([]cell, 5*5*ny)
	maxSurf := 0.0
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			maxSurf = math.Max(maxSurf, cols[z][x].height)
		}
	}
	topEval := int(maxSurf) + 40
	for lz := 0; lz < 5; lz++ {
		for lx := 0; lx < 5; lx++ {
			wx, wz := float64(bx+lx*cellXZ), float64(bz+lz*cellXZ)
			for ly := 0; ly < ny; ly++ {
				wy := float64(minY + ly*cellY)
				c := &lat[(lz*5+lx)*ny+ly]
				if int(wy) > topEval {
					continue
				}
				c.over = g.overhang.At3(wx, wy*1.5, wz)
				c.cheese = g.cheese.At3(wx, wy*1.8, wz)
				c.spa = g.spagA.At3(wx, wy*1.2, wz)
				c.spb = g.spagB.At3(wx, wy*1.2, wz)
			}
		}
	}
	sample := func(x, y, z int) cell {
		lx, lz := x/cellXZ, z/cellXZ
		fy := y - minY
		ly := fy / cellY
		tx, tz, ty := float64(x%cellXZ)/cellXZ, float64(z%cellXZ)/cellXZ, float64(fy%cellY)/cellY
		if ly >= ny-1 {
			ly, ty = ny-2, 1
		}
		at := func(ax, ay, az int) cell { return lat[(az*5+ax)*ny+ay] }
		mix := func(a, b cell, t float64) cell {
			return cell{lerp(t, a.over, b.over), lerp(t, a.cheese, b.cheese), lerp(t, a.spa, b.spa), lerp(t, a.spb, b.spb)}
		}
		c00 := mix(at(lx, ly, lz), at(lx+1, ly, lz), tx)
		c10 := mix(at(lx, ly+1, lz), at(lx+1, ly+1, lz), tx)
		c01 := mix(at(lx, ly, lz+1), at(lx+1, ly, lz+1), tx)
		c11 := mix(at(lx, ly+1, lz+1), at(lx+1, ly+1, lz+1), tx)
		return mix(mix(c00, c10, ty), mix(c01, c11, ty), tz)
	}

	rnd := newRand(int64(hash2(g.Seed, ch.Pos.X, ch.Pos.Z)))
	sea := g.SeaLevel
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			col := cols[z][x]
			surf := int(math.Floor(col.height))
			top := surf
			if col.rough > 0.01 {
				top = surf + int(col.rough*24)
			}
			bedrockTop := minY + rnd.intn(4)
			for y := minY; y <= min(max(top+1, sea), maxY-1); y++ {
				var id voxel.StateID
				switch {
				case y <= bedrockTop:
					id = g.bedrock
				default:
					solid := y <= surf
					if col.rough > 0.01 && y > surf-24 {
						d := (col.height - float64(y)) / 24
						s := sample(x, y, z)
						solid = d+s.over*col.rough*1.4 > 0
					}
					if solid {
						if y < 0 || (y < 8 && rnd.intn(8) < 8-y) {
							id = g.deepslate
						} else {
							id = g.stone
						}
						// caves: cheese chambers and spaghetti tunnels
						if y < surf-5 || (!col.beachish && y < surf+1 && surf > sea+2) {
							s := sample(x, y, z)
							depth := float64(surf - y)
							cheese := s.cheese > 0.32+0.25*smoothstep(30, 0, depth)
							spag := math.Abs(s.spa) < 0.035 && math.Abs(s.spb) < 0.035
							if cheese || spag {
								if y < minY+10 {
									id = g.lava
								} else {
									id = g.air
								}
							}
						}
					} else if y <= sea {
						id = g.water
					}
				}
				if id != g.air {
					ch.SetRaw(x, y, z, id)
				}
			}
			g.surface(w, ch, x, z, col, rnd)
		}
	}
	g.placeOres(w, ch, rnd)
	g.placeTrees(w, ch)
	g.placePlants(w, ch, rnd)
	ch.RecomputeHeights(w.Reg.Table())
}

// surface replaces the top of a column with grass/dirt/sand per climate.
func (g *Generator) surface(w *voxel.World, ch *voxel.Chunk, x, z int, col column, rnd *rng) {
	maxY := w.MaxY()
	depth := -1
	for y := maxY - 1; y >= w.MinY; y-- {
		id := ch.Get(x, y, z)
		if id != g.stone && id != g.deepslate {
			if id == g.air || id == g.water {
				depth = -1
			}
			continue
		}
		depth++
		if depth > 4 {
			continue
		}
		above := ch.Get(x, y+1, z)
		under := above == g.water
		if depth == 0 && !under && above != g.air {
			continue
		}
		var nid voxel.StateID
		switch {
		case y > 135+rnd.intn(6) && col.rough > 0.3:
			if depth == 0 && col.temp < 0.1 {
				nid = g.snowBlock
			} else {
				continue
			}
		case col.beachish && y >= g.SeaLevel-4 && y <= g.SeaLevel+2:
			if depth < 3 {
				nid = g.sand
			} else {
				nid = g.sandstone
			}
		case under || y < g.SeaLevel-1:
			if depth < 2 {
				if col.temp > 0.15 {
					nid = g.sand
				} else if (x+z+y)%7 == 0 {
					nid = g.clay
				} else {
					nid = g.gravel
				}
			} else {
				continue
			}
		default:
			if depth == 0 {
				if col.temp < -0.25 || y > 120 {
					nid = g.snowyGrass
				} else {
					nid = g.grass
				}
			} else if depth < 4 {
				nid = g.dirt
			} else {
				continue
			}
		}
		ch.SetRaw(x, y, z, nid)
	}
}

func (g *Generator) placeOres(w *voxel.World, ch *voxel.Chunk, rnd *rng) {
	for _, o := range g.ores {
		for v := 0; v < o.veins; v++ {
			cx, cy, cz := rnd.intn(16), o.minY+rnd.intn(o.maxY-o.minY), rnd.intn(16)
			for k := 0; k < o.size; k++ {
				x, y, z := cx+rnd.intn(3)-1, cy+rnd.intn(3)-1, cz+rnd.intn(3)-1
				cx, cy, cz = x, y, z
				if x < 0 || x > 15 || z < 0 || z > 15 || y < w.MinY || y >= w.MaxY() {
					continue
				}
				switch ch.Get(x, y, z) {
				case g.stone:
					ch.SetRaw(x, y, z, o.normal)
				case g.deepslate:
					ch.SetRaw(x, y, z, o.deep)
				}
			}
		}
	}
}

// placeTrees decides trees per column from a hash, including trees rooted in
// neighbouring chunks whose canopy reaches into this one, so chunks can be
// generated independently and in any order but still agree at borders.
func (g *Generator) placeTrees(w *voxel.World, ch *voxel.Chunk) {
	bx, bz := ch.Pos.X<<4, ch.Pos.Z<<4
	for wz := bz - 3; wz < bz+19; wz++ {
		for wx := bx - 3; wx < bx+19; wx++ {
			h := hash2(g.Seed^0x7EE5, wx, wz)
			if h%1000 >= 9 {
				continue
			}
			col := g.column(wx, wz)
			if col.rough > 0.01 || col.beachish {
				continue
			}
			base := int(math.Floor(col.height)) + 1
			if base <= g.SeaLevel+1 || base > 200 {
				continue
			}
			// sparse in plains, dense in forests
			forest := g.detail.At2(float64(wx)*0.2, float64(wz)*0.2)
			if forest < 0.05 && h%1000 >= 3 {
				continue
			}
			kind := g.trees[0]
			switch {
			case col.temp < -0.15:
				kind = g.trees[2]
			case (h>>20)%3 == 0:
				kind = g.trees[1]
			}
			th := kind.minH + int((h>>8)%uint64(kind.maxH-kind.minH+1))
			// Decided from 2D noise only (rough==0 makes the height exact), so
			// every chunk agrees about trees that straddle borders.
			g.tree(ch, wx, base, wz, th, kind)
		}
	}
}

func (g *Generator) setIfAir(ch *voxel.Chunk, wx, y, wz int, id voxel.StateID, force bool) {
	if wx>>4 != ch.Pos.X || wz>>4 != ch.Pos.Z {
		return
	}
	cur := ch.Get(wx&15, y, wz&15)
	if cur == g.air || force || cur == g.tallGrass {
		ch.SetRaw(wx&15, y, wz&15, id)
	}
}

func (g *Generator) tree(ch *voxel.Chunk, x, base, z, h int, k treeKind) {
	if k.spruce {
		for dy := 2; dy <= h; dy++ {
			r := (h - dy) / 2
			if dy == h {
				r = 0
			}
			if r > 2 {
				r = 2
			}
			if (h-dy)%2 == 1 && r > 0 {
				r--
			}
			for dz := -r; dz <= r; dz++ {
				for dx := -r; dx <= r; dx++ {
					if r > 0 && abs(dx) == r && abs(dz) == r {
						continue
					}
					g.setIfAir(ch, x+dx, base+dy, z+dz, k.leaves, false)
				}
			}
		}
		g.setIfAir(ch, x, base+h+1, z, k.leaves, false)
	} else {
		for dy := h - 3; dy <= h+1; dy++ {
			r := 2
			if dy >= h {
				r = 1
			}
			for dz := -r; dz <= r; dz++ {
				for dx := -r; dx <= r; dx++ {
					corner := abs(dx) == r && abs(dz) == r
					if corner && (dy >= h || hash2(int64(x*7+dy), x+dx, z+dz)%2 == 0) {
						continue
					}
					g.setIfAir(ch, x+dx, base+dy, z+dz, k.leaves, false)
				}
			}
		}
	}
	for dy := 0; dy < h; dy++ {
		g.setIfAir(ch, x, base+dy, z, k.log, true)
	}
	if x>>4 == ch.Pos.X && z>>4 == ch.Pos.Z {
		ch.SetRaw(x&15, base-1, z&15, g.dirt)
	}
}

func (g *Generator) placePlants(w *voxel.World, ch *voxel.Chunk, rnd *rng) {
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			for y := w.MaxY() - 2; y > g.SeaLevel; y-- {
				id := ch.Get(x, y, z)
				if id == g.air {
					continue
				}
				if ch.Get(x, y+1, z) != g.air {
					break
				}
				switch id {
				case g.grass:
					r := rnd.intn(100)
					switch {
					case r < 12:
						ch.SetRaw(x, y+1, z, g.tallGrass)
					case r < 14:
						ch.SetRaw(x, y+1, z, g.fern)
					case r < 16:
						ch.SetRaw(x, y+1, z, g.flowers[rnd.intn(len(g.flowers))])
					}
				case g.sand:
					if rnd.intn(200) == 0 {
						ch.SetRaw(x, y+1, z, g.deadBush)
					}
				}
				break
			}
		}
	}
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// FindLand searches outward from x,z for a dry, flat-ish spot with trees
// around and returns its coordinates (useful for spawning).
func (g *Generator) FindLand(x, z int) (int, int) {
	for r := 0; r < 4000; r += 16 {
		for a := 0; a < 16; a++ {
			ang := float64(a) / 16 * 2 * math.Pi
			px, pz := x+int(float64(r)*math.Cos(ang)), z+int(float64(r)*math.Sin(ang))
			c := g.column(px, pz)
			if c.height > float64(g.SeaLevel)+6 && c.rough < 0.01 && c.temp > -0.1 {
				return px, pz
			}
		}
	}
	return x, z
}
