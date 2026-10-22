// Package mesh turns world sections into compact GPU-ready vertex data. It is
// pure CPU code with no OpenGL dependency, safe to run on worker goroutines.
package mesh

import "github.com/memmaker/voxellib/voxel"

// Snapshot is a padded 18^3 copy of a section plus a one-block border of its
// 26 neighbours (states and packed light). Meshing reads only the snapshot, so
// the world lock is held just long enough to copy - the same idea as
// Minecraft's RenderChunkRegion.
type Snapshot struct {
	Pos    voxel.SectionPos
	States [18 * 18 * 18]voxel.StateID
	Light  [18 * 18 * 18]uint8 // sky<<4 | block
	Empty  bool                // centre section has only air
	// Biomes holds the legacy biome id per column of the section and its
	// border, [(z+1)*18+(x+1)] for x,z in -1..16 (a missing chunk repeats
	// the nearest captured column); HasBiomes is false when the centre
	// chunk has none.
	Biomes    [18 * 18]uint8
	HasBiomes bool
}

// idx addresses the padded grid; x,y,z in -1..16.
func idx(x, y, z int) int { return ((y+1)*18+(z+1))*18 + (x + 1) }

// Capture copies a section and its border from the world. It takes the world
// read lock itself.
func (s *Snapshot) Capture(w *voxel.World, p voxel.SectionPos) {
	s.Pos = p
	w.RLock()
	defer w.RUnlock()

	var secs [3][3][3]*voxel.Section
	var above [3][3][3]bool // section is above the world: open sky
	maxSec := w.MinSection() + w.SectionCount()
	for dz := -1; dz <= 1; dz++ {
		for dx := -1; dx <= 1; dx++ {
			c := w.ChunkUnlocked(voxel.ChunkPos{X: p.X + dx, Z: p.Z + dz})
			for dy := -1; dy <= 1; dy++ {
				sy := p.Y + dy
				if sy >= maxSec {
					above[dx+1][dy+1][dz+1] = true
				}
				if c != nil {
					secs[dx+1][dy+1][dz+1] = c.Section(sy * 16)
				}
			}
		}
	}
	center := secs[1][1][1]
	s.Empty = center.IsEmpty()
	s.captureBiomes(w, p)

	// centre: bulk unpack
	var buf [voxel.SectionVolume]voxel.StateID
	if center != nil {
		center.Blocks.Unpack(&buf)
		for y := 0; y < 16; y++ {
			for z := 0; z < 16; z++ {
				base := idx(0, y, z)
				src := y<<8 | z<<4
				copy(s.States[base:base+16], buf[src:src+16])
				for x := 0; x < 16; x++ {
					i := src | x
					sh := uint(i&1) << 2
					sky := center.Sky[i>>1] >> sh & 15
					bl := center.Block[i>>1] >> sh & 15
					s.Light[base+x] = sky<<4 | bl
				}
			}
		}
	}
	// border: per cell
	for y := -1; y <= 16; y++ {
		for z := -1; z <= 16; z++ {
			for x := -1; x <= 16; x++ {
				if x >= 0 && x < 16 && y >= 0 && y < 16 && z >= 0 && z < 16 {
					continue
				}
				sx, sy, sz := sidx(x), sidx(y), sidx(z)
				i := idx(x, y, z)
				sec := secs[sx][sy][sz]
				if sec == nil {
					s.States[i] = voxel.Air
					if above[sx][sy][sz] {
						s.Light[i] = 15 << 4
					} else {
						s.Light[i] = 0
					}
					continue
				}
				li := voxel.SectionIndex(x, y, z)
				sh := uint(li&1) << 2
				s.States[i] = sec.Blocks.Get(li)
				s.Light[i] = (sec.Sky[li>>1]>>sh&15)<<4 | sec.Block[li>>1]>>sh&15
			}
		}
	}
}

func sidx(v int) int {
	if v < 0 {
		return 0
	}
	if v > 15 {
		return 2
	}
	return 1
}

// captureBiomes fills Biomes from the centre chunk and its neighbours.
// Caller holds the world read lock.
func (s *Snapshot) captureBiomes(w *voxel.World, p voxel.SectionPos) {
	var cols [3][3][]byte
	for dz := -1; dz <= 1; dz++ {
		for dx := -1; dx <= 1; dx++ {
			if c := w.ChunkUnlocked(voxel.ChunkPos{X: p.X + dx, Z: p.Z + dz}); c != nil && len(c.Biomes) == 256 {
				cols[dx+1][dz+1] = c.Biomes
			}
		}
	}
	s.HasBiomes = cols[1][1] != nil
	if !s.HasBiomes {
		return
	}
	for z := -1; z <= 16; z++ {
		for x := -1; x <= 16; x++ {
			b := cols[sidx(x)][sidx(z)]
			lx, lz := x&15, z&15
			if b == nil { // no neighbour: the nearest centre column
				b, lx, lz = cols[1][1], min(max(x, 0), 15), min(max(z, 0), 15)
			}
			s.Biomes[(z+1)*18+x+1] = b[lz<<4|lx]
		}
	}
}
