package mesh

import "github.com/memmaker/voxellib/voxel"

// VisGraph records which pairs of section faces are connected through
// non-opaque blocks - the reference's VisibilitySet. The renderer's occlusion BFS
// only walks from a section's entry face to exit faces connected to it, which
// culls whole cave systems behind solid ground ("advanced cave culling").
type VisGraph uint64

// AllVisible connects every face pair.
const AllVisible VisGraph = 1<<36 - 1

func (v VisGraph) Connected(a, b voxel.Face) bool { return v&(1<<(uint(a)*6+uint(b))) != 0 }

func (v *VisGraph) connect(mask uint8) {
	for a := 0; a < 6; a++ {
		if mask&(1<<a) == 0 {
			continue
		}
		for b := 0; b < 6; b++ {
			if mask&(1<<b) != 0 {
				*v |= 1 << (uint(a)*6 + uint(b))
			}
		}
	}
}

func (m *Mesher) visGraph() VisGraph {
	t, st := m.table, &m.Snap.States
	var open [voxel.SectionVolume]bool
	n := 0
	for y := 0; y < 16; y++ {
		for z := 0; z < 16; z++ {
			for x := 0; x < 16; x++ {
				if !t[st[idx(x, y, z)]].Opaque {
					open[y<<8|z<<4|x] = true
					n++
				}
			}
		}
	}
	// Like vanilla: with fewer than 256 opaque blocks, don't bother.
	if voxel.SectionVolume-n < 256 {
		return AllVisible
	}
	if n == 0 {
		return 0
	}
	var vis VisGraph
	var seen [voxel.SectionVolume]bool
	stack := make([]int, 0, 512)
	for start := 0; start < voxel.SectionVolume; start++ {
		if !open[start] || seen[start] {
			continue
		}
		var faces uint8
		seen[start] = true
		stack = append(stack[:0], start)
		for len(stack) > 0 {
			i := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y, z := i&15, i>>8, (i>>4)&15
			if x == 0 {
				faces |= 1 << voxel.FaceWest
			} else if j := i - 1; open[j] && !seen[j] {
				seen[j] = true
				stack = append(stack, j)
			}
			if x == 15 {
				faces |= 1 << voxel.FaceEast
			} else if j := i + 1; open[j] && !seen[j] {
				seen[j] = true
				stack = append(stack, j)
			}
			if z == 0 {
				faces |= 1 << voxel.FaceNorth
			} else if j := i - 16; open[j] && !seen[j] {
				seen[j] = true
				stack = append(stack, j)
			}
			if z == 15 {
				faces |= 1 << voxel.FaceSouth
			} else if j := i + 16; open[j] && !seen[j] {
				seen[j] = true
				stack = append(stack, j)
			}
			if y == 0 {
				faces |= 1 << voxel.FaceDown
			} else if j := i - 256; open[j] && !seen[j] {
				seen[j] = true
				stack = append(stack, j)
			}
			if y == 15 {
				faces |= 1 << voxel.FaceUp
			} else if j := i + 256; open[j] && !seen[j] {
				seen[j] = true
				stack = append(stack, j)
			}
		}
		vis.connect(faces)
	}
	return vis
}
