package mesh

import (
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/javarand"
)

// the 1.7 reference's RenderBlocks texture mapping. 1.8 block models (uvTab,
// projUV) project textures differently: 1.7 draws the bottom with v along
// +z, mirrors north and east faces within the box's own extent, and turns
// faces by its uvRotate fields, each with its own mirroring.

// rbUV is the u,v (1/16 of the tile) RenderBlocks.renderFace* puts on each
// corner (cornerOff order) of face f of the box lo..hi (1/16) with uvRotate
// rot, transcribed from 1.7.10 (field_152631_f and flipTexture off). d[3]..
// d[10] are the Java locals of the same names.
func rbUV(f voxel.Face, lo, hi [3]float32, rot int) (uv [4][2]float32) {
	const x, y, z = 0, 1, 2
	var d [11]float32
	tail := func(kind byte) {
		switch kind {
		case 'A':
			d[9], d[10], d[7], d[8], d[5] = d[5], d[6], d[3], d[4], d[6]
			d[6] = d[9]
		case 'B':
			d[7], d[8] = d[4], d[3]
			d[3], d[4] = d[4], d[8]
			d[9], d[10] = d[6], d[5]
		default: // 'C', and rot 0
			d[7], d[8], d[9], d[10] = d[4], d[3], d[5], d[6]
		}
	}
	set := func(d3, d4, d5, d6 float32, kind byte) {
		d[3], d[4], d[5], d[6] = d3, d4, d5, d6
		tail(kind)
	}
	switch f {
	case voxel.FaceDown, voxel.FaceUp:
		set(lo[x], hi[x], lo[z], hi[z], 0)
		// renderFaceYNeg turns by 2 what renderFaceYPos turns by 1.
		a, b := 2, 1
		if f == voxel.FaceUp {
			a, b = 1, 2
		}
		switch rot {
		case a:
			set(lo[z], hi[z], 16-hi[x], 16-lo[x], 'A')
		case b:
			set(16-hi[z], 16-lo[z], lo[x], hi[x], 'B')
		case 3:
			set(16-lo[x], 16-hi[x], 16-lo[z], 16-hi[z], 'C')
		}
	case voxel.FaceNorth, voxel.FaceSouth: // renderFaceZNeg (uvRotateEast), ZPos (uvRotateWest)
		set(lo[x], hi[x], 16-hi[y], 16-lo[y], 0)
		n := f == voxel.FaceNorth
		switch {
		case rot == 2 && n:
			set(lo[y], hi[y], 16-lo[x], 16-hi[x], 'A')
		case rot == 1 && !n:
			set(lo[y], hi[y], 16-hi[x], 16-lo[x], 'A')
		case rot == 1 && n:
			set(16-hi[y], 16-lo[y], hi[x], lo[x], 'B')
		case rot == 2 && !n:
			set(16-hi[y], 16-lo[y], lo[x], hi[x], 'B')
		case rot == 3:
			set(16-lo[x], 16-hi[x], hi[y], lo[y], 'C')
		}
	default: // renderFaceXNeg (uvRotateNorth), XPos (uvRotateSouth)
		set(lo[z], hi[z], 16-hi[y], 16-lo[y], 0)
		w := f == voxel.FaceWest
		switch {
		case rot == 1 && w:
			set(lo[y], hi[y], 16-hi[z], 16-lo[z], 'A')
		case rot == 2 && !w:
			set(lo[y], hi[y], 16-lo[z], 16-hi[z], 'A')
		case rot == 2 && w:
			set(16-hi[y], 16-lo[y], lo[z], hi[z], 'B')
		case rot == 1 && !w:
			set(16-hi[y], 16-lo[y], hi[z], lo[z], 'B')
		case rot == 3:
			set(16-lo[z], 16-hi[z], hi[y], lo[y], 'C')
		}
	}
	// The (u, v) locals each Java vertex gets, by its corner: lo or hi
	// along the face's first and second axis (Y faces x,z; Z faces x,y;
	// X faces z,y).
	t := rbVerts[f]
	a0, a1 := 0, 2
	switch f.Axis() {
	case 0:
		a0, a1 = 2, 1
	case 2:
		a0, a1 = 0, 1
	}
	for k, co := range cornerOff[f] {
		e := t[co[a0]][co[a1]]
		uv[k] = [2]float32{d[e[0]], d[e[1]]}
	}
	return
}

var rbVerts = [6][2][2][2]uint8{
	voxel.FaceDown:  {{{3, 5}, {8, 10}}, {{7, 9}, {4, 6}}},
	voxel.FaceUp:    {{{3, 5}, {8, 10}}, {{7, 9}, {4, 6}}},
	voxel.FaceNorth: {{{4, 6}, {7, 9}}, {{8, 10}, {3, 5}}},
	voxel.FaceSouth: {{{8, 10}, {3, 5}}, {{4, 6}, {7, 9}}},
	voxel.FaceWest:  {{{8, 10}, {3, 5}}, {{4, 6}, {7, 9}}},
	voxel.FaceEast:  {{{4, 6}, {7, 9}}, {{8, 10}, {3, 5}}},
}

// rbTab is rbUV for a full cube in tile units, [face][uvRotate][corner].
var rbTab = func() (t [6][4][4][2]uint32) {
	for f := voxel.Face(0); f < voxel.FaceCount; f++ {
		for r := range 4 {
			for k, uv := range rbUV(f, [3]float32{}, [3]float32{16, 16, 16}, r) {
				t[f][r][k] = [2]uint32{uint32(uv[0]) / 16, uint32(uv[1]) / 16}
			}
		}
	}
	return
}()

// RenderBlocksSides is the default StateInfo.UVSeed: the side whose hash
// turns each face when RenderBlocks' uvRotate fields are set per side
// (LOTR's natural blocks): renderFaceZNeg reads uvRotateEast (side 5), and
// so on.
var RenderBlocksSides = [6]uint8{0, 1, 5, 4, 2, 3}

// uvRandom is the uvRotate (0..3) of face f of the block at world x,y,z
// with a UVRandom face: h = (x*234890405 ^ z*37383934 ^ y) + side*285502
// in int arithmetic, then java.util.Random setSeed(h), setSeed(nextLong()),
// nextInt(4) (LOTRReplacedMethods.BlockRendering.renderStandardBlock).
func uvRandom(info *voxel.StateInfo, x, y, z int, f voxel.Face) int {
	side := RenderBlocksSides[f]
	if info.UVSeed != nil {
		side = info.UVSeed[f]
	}
	h := int32(x)*234890405 ^ int32(z)*37383934 ^ int32(y)
	h += int32(side) * 285502
	var r javarand.Random
	r.SetSeed(int64(h))
	r.SetSeed(r.NextLong())
	return int(r.NextInt(4)) // nextInt(4): (4 * next(31)) >> 31
}
