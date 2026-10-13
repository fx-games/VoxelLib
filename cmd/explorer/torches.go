package main

// Torch flames: particles (package particles) over every torch near the
// player, from vanilla's TorchBlock/WallTorchBlock.animateTick positions.

import (
	"strings"

	"github.com/memmaker/voxellib/particles"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/render"
)

var (
	torchFlame = particles.Props{Count: 1, Spread: 0.03, Vel: [3]float64{0, 0.5, 0}, VelVar: 0.1, Gravity: -1, Drag: 1, Life: 0.35, LifeVar: 0.1,
		Size0: 0.11, Size1: 0.02, Col0: [4]float32{1, 0.7, 0.25, 0.9}, Col1: [4]float32{0.9, 0.2, 0, 0}, Additive: true}
	torchSmoke = particles.Props{Count: 1, Spread: 0.03, Vel: [3]float64{0, 0.6, 0}, VelVar: 0.15, Gravity: -0.4, Drag: 0.5, Life: 1.6, LifeVar: 0.4,
		Size0: 0.06, Size1: 0.25, Col0: [4]float32{0.2, 0.2, 0.2, 0.45}, Col1: [4]float32{0.3, 0.3, 0.3, 0}}
)

type torchFX struct {
	parts particles.System
	rend  *render.ParticleRenderer
	// tips caches each chunk's flame positions, rescanned every scanEvery
	// ticks so placed and broken torches show up.
	tips map[*voxel.Chunk]torchScan
}

type torchScan struct {
	at   int
	tips [][3]float64
}

const scanEvery = 40

// towardWall is the x,z step from a wall torch to its wall, by facing.
var towardWall = map[string][2]float64{"north": {0, 1}, "south": {0, -1}, "west": {1, 0}, "east": {-1, 0}}

// scanTorches lists the flame positions in a chunk.
func scanTorches(w *voxel.World, ch *voxel.Chunk) [][3]float64 {
	reg := w.Reg
	isTorch := func(s voxel.StateID) bool { return strings.HasSuffix(reg.Info(s).Name, "torch") }
	var out [][3]float64
	for si, s := range ch.Sections {
		if s.IsEmpty() || !s.Blocks.MayContain(isTorch) {
			continue
		}
		for i := 0; i < voxel.SectionVolume; i++ {
			id := s.Blocks.Get(i)
			if !isTorch(id) {
				continue
			}
			x := float64(ch.Pos.X*16+i&15) + 0.5
			y := float64(w.MinY+si*16+i>>8) + 0.7
			z := float64(ch.Pos.Z*16+(i>>4)&15) + 0.5
			if strings.HasSuffix(reg.Info(id).Name, "wall_torch") {
				// 0.27 toward the wall it hangs on, 0.22 up.
				d := towardWall[reg.StateProps(id)["facing"]]
				x, y, z = x+0.27*d[0], y+0.22, z+0.27*d[1]
			}
			out = append(out, [3]float64{x, y, z})
		}
	}
	return out
}

// tickTorches emits flames over torches within 24 blocks.
func (g *game) tickTorches() {
	t := &g.torches
	if t.tips == nil {
		t.tips = map[*voxel.Chunk]torchScan{}
	}
	w := g.eng.World
	px, py, pz := g.player.X, g.player.Y, g.player.Z
	seen := map[*voxel.Chunk]bool{}
	for dz := -2; dz <= 2; dz++ {
		for dx := -2; dx <= 2; dx++ {
			ch := w.Chunk(voxel.ChunkPos{X: int(px)>>4 + dx, Z: int(pz)>>4 + dz})
			if ch == nil {
				continue
			}
			seen[ch] = true
			sc, ok := t.tips[ch]
			if !ok || g.tick-sc.at >= scanEvery {
				sc = torchScan{g.tick, scanTorches(w, ch)}
				t.tips[ch] = sc
			}
			for i, p := range sc.tips {
				if ddx, ddy, ddz := p[0]-px, p[1]-py, p[2]-pz; ddx*ddx+ddy*ddy+ddz*ddz > 24*24 {
					continue
				}
				t.parts.Emit(p[0], p[1], p[2], torchFlame)
				if (g.tick+i)%8 == 0 {
					t.parts.Emit(p[0], p[1]+0.1, p[2], torchSmoke)
				}
			}
		}
	}
	for ch := range t.tips {
		if !seen[ch] {
			delete(t.tips, ch)
		}
	}
	t.parts.Tick(tickDur.Seconds(), nil)
}

// drawTorches draws the flame particles.
func (g *game) drawTorches(alpha float64) {
	t := &g.torches
	if t.rend == nil {
		var err error
		if t.rend, err = render.NewParticleRenderer(g.eng.Renderer); err != nil {
			return
		}
	}
	t.rend.Draw(&g.cam, t.parts.P, alpha, g.light)
}
