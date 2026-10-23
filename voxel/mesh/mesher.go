package mesh

import (
	"math"
	"sort"

	"github.com/memmaker/voxellib/voxel"
)

// Region layout shared with the renderer: sections are grouped into regions of
// 8x4x8 that share one vertex buffer (Sodium-style), and each vertex stores
// its section's index inside the region so a whole region renders with one
// multi-draw call and no per-section uniforms.
const (
	RegionX, RegionY, RegionZ = 8, 4, 8
	RegionSections            = RegionX * RegionY * RegionZ
)

// Draw groups: one per face direction so the renderer can skip faces that
// point away from the camera, plus one for faces with no single direction.
const (
	GroupUnassigned = 6
	GroupCount      = 7
)

// VertexWords is the vertex size in uint32s (16 bytes).
//
//	w0: x:10 y:10 z:10 (high bits of the position) ao:2
//	w1: texture layer:12 u:5 v:5 normal:3 projected uv:1,
//	    x:2 y:2 z:2 (low bits of the position)
//
// A position is section local in 1/128 block, biased by 8 blocks, so
// geometry may reach from -8 to 24 blocks (plants offset past the section,
// parts poking out of it); see vpos.
//
//	w2: tint r:8 g:8 b:8, section index in region:8
//	w3: sky light:8 block light:8 (level*16, smoothly averaged),
//	    dynamic tint weight:8 (see Config.BlockTint)
const VertexWords = 4

// vpos packs a section-local position in 1/128 block into w0 and w1 bits.
func vpos(x, y, z int) (w0, w1 uint32) {
	for a, v := range [3]int{x, y, z} {
		q := uint32(min(max(v+8*128, 0), 4095))
		w0 |= q >> 2 << (10 * a)
		w1 |= q & 3 << (26 + 2*a)
	}
	return
}

// RegionOf returns the region coordinate containing a section.
func RegionOf(p voxel.SectionPos) [3]int {
	return [3]int{p.X >> 3, p.Y >> 2, p.Z >> 3}
}

// RegionIndex returns a section's index inside its region.
func RegionIndex(p voxel.SectionPos) uint32 {
	return uint32(p.X&7) | uint32(p.Y&3)<<3 | uint32(p.Z&7)<<5
}

// Range is a run of vertices.
type Range struct{ First, Count int }

// Mesh is the result of meshing one section.
type Mesh struct {
	Pos    voxel.SectionPos
	Data   []uint32 // solid and cutout vertices, grouped [layer][group]
	Ranges [voxel.LayerCount][GroupCount]Range
	// Translucent geometry is kept apart so it can be re-sorted back to front
	// when the camera moves, as Minecraft does.
	Trans        []uint32
	TransCenters [][3]float32
	Vis          VisGraph
	Empty        bool // nothing to draw
}

// Quads returns the number of quads in the mesh.
func (m *Mesh) Quads() int { return (len(m.Data) + len(m.Trans)) / (4 * VertexWords) }

// Config holds per-texture data the mesher needs.
type Config struct {
	// Tintable[layer] is true when a texture is greyscale and should be
	// coloured by the block's tint (vanilla grass), false when the pack has
	// already painted it (most third party packs).
	Tintable []bool
	// BlockTint, when set, decides the colour of every tinted block (any
	// Tint but TintNone and TintFixed) at world position x,y,z by the
	// game's rule instead of the Tintable heuristic: biome returns the
	// biome id of any column within one block of the section (world x,z).
	// The result is 0xWWRRGGBB: RGB is baked into the vertices, and WW
	// (0..255) is the weight of the renderer's per-frame dynamic tint
	// (render.Options.DynamicTint) added on top, so a colour that changes
	// at run time (water following the player) needs no remeshing:
	// colour = RGB + WW/255 * DynamicTint. Without it, or without biome
	// data, TintColors apply. It runs on mesh workers, concurrently, and
	// is called at most once per block and mesh build.
	BlockTint func(info *voxel.StateInfo, x, y, z int, biome func(x, z int) uint8) uint32
	// RenderBlocksUV maps cube faces and boxes without model UVs like
	// Minecraft 1.7's RenderBlocks (bottom v along +z, north and east
	// mirrored within the box) instead of 1.8 block models. Faces in a
	// state's UVRandom always follow RenderBlocks.
	RenderBlocksUV bool
}

// Tint colours (plains biome defaults).
var TintColors = [...]uint32{
	voxel.TintNone:    0xFFFFFF,
	voxel.TintGrass:   0x91BD59,
	voxel.TintFoliage: 0x77AB2F,
	voxel.TintWater:   0x3F76E4,
	voxel.TintBirch:   0x80A755,
	voxel.TintSpruce:  0x619961,
}

// Mesher builds meshes. Keep one per worker goroutine: it owns scratch buffers.
type Mesher struct {
	Snap  Snapshot
	table []voxel.StateInfo
	cfg   *Config
	out   [voxel.LayerCount][GroupCount][]uint32
	trans [][3]float32
	nl    [][2]int32 // neighbourLight scratch: cell, mixed light
	mask  [256]uint64
	ridx  uint32
	// BlockTint results per block of the section, valid where tintStamp
	// equals builds (counts Build calls).
	tintCache [voxel.SectionVolume]uint32
	tintStamp [voxel.SectionVolume]uint32
	builds    uint32
}

func NewMesher() *Mesher { return &Mesher{} }

type cornerLight struct{ ao, sky, blk uint8 }

var (
	cornerOff = [6][4][3]int{
		{{0, 0, 0}, {1, 0, 0}, {1, 0, 1}, {0, 0, 1}}, // down
		{{0, 1, 0}, {0, 1, 1}, {1, 1, 1}, {1, 1, 0}}, // up
		{{1, 0, 0}, {0, 0, 0}, {0, 1, 0}, {1, 1, 0}}, // north
		{{0, 0, 1}, {1, 0, 1}, {1, 1, 1}, {0, 1, 1}}, // south
		{{0, 0, 0}, {0, 0, 1}, {0, 1, 1}, {0, 1, 0}}, // west
		{{1, 0, 1}, {1, 0, 0}, {1, 1, 0}, {1, 1, 1}}, // east
	}
	uvTab = [6][4][2]uint32{
		{{0, 1}, {1, 1}, {1, 0}, {0, 0}},
		{{0, 0}, {0, 1}, {1, 1}, {1, 0}},
		{{0, 1}, {1, 1}, {1, 0}, {0, 0}},
		{{0, 1}, {1, 1}, {1, 0}, {0, 0}},
		{{0, 1}, {1, 1}, {1, 0}, {0, 0}},
		{{0, 1}, {1, 1}, {1, 0}, {0, 0}},
	}
	// texture u / v axes per face (0=x,1=y,2=z)
	uAxis = [6]int{0, 0, 0, 0, 2, 2}
	vAxis = [6]int{2, 2, 1, 1, 1, 1}
)

var nbOff = func() (o [6]int) {
	for f := voxel.Face(0); f < voxel.FaceCount; f++ {
		n := f.Normal()
		o[f] = idx(n.X, n.Y, n.Z) - idx(0, 0, 0)
	}
	return
}()

// Build meshes the captured snapshot.
func (m *Mesher) Build(table []voxel.StateInfo, cfg *Config) *Mesh {
	m.table, m.cfg = table, cfg
	m.builds++
	res := &Mesh{Pos: m.Snap.Pos}
	if m.Snap.Empty {
		res.Empty = true
		res.Vis = AllVisible
		return res
	}
	m.ridx = RegionIndex(m.Snap.Pos) << 24
	for l := range m.out {
		for g := range m.out[l] {
			m.out[l][g] = m.out[l][g][:0]
		}
	}
	m.trans = m.trans[:0]

	m.neighbourLight()
	m.cubes()
	m.specials()

	n := 0
	for l := voxel.Layer(0); l < voxel.LayerTranslucent; l++ {
		for g := 0; g < GroupCount; g++ {
			n += len(m.out[l][g])
		}
	}
	if n > 0 {
		res.Data = make([]uint32, 0, n)
		for l := voxel.Layer(0); l < voxel.LayerTranslucent; l++ {
			for g := 0; g < GroupCount; g++ {
				res.Ranges[l][g] = Range{len(res.Data) / VertexWords, len(m.out[l][g]) / VertexWords}
				res.Data = append(res.Data, m.out[l][g]...)
			}
		}
	}
	if t := m.out[voxel.LayerTranslucent][GroupUnassigned]; len(t) > 0 {
		res.Trans = append([]uint32(nil), t...)
		res.TransCenters = append([][3]float32(nil), m.trans...)
	}
	res.Vis = m.visGraph()
	res.Empty = len(res.Data) == 0 && len(res.Trans) == 0
	return res
}

// neighbourLight gives every NeighbourLight cell of the snapshot the
// brightest sky and block light of the cells above it and beside it, as
// Minecraft's ChunkCache.getSkyBlockTypeBrightness does for
// useNeighborBrightness blocks (the cell below does not count). Faces and
// AO samples then read the mixed value like any other light.
func (m *Mesher) neighbourLight() {
	t, st, li := m.table, &m.Snap.States, &m.Snap.Light
	m.nl = m.nl[:0]
	for i, s := range st {
		if !t[s].NeighbourLight {
			continue
		}
		x, z, y := i%18, i/18%18, i/(18*18)
		var sky, blk uint8
		for _, d := range [5][3]int{{0, 1, 0}, {1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
			nx, ny, nz := x+d[0], y+d[1], z+d[2]
			if nx < 0 || nx >= 18 || ny < 0 || ny >= 18 || nz < 0 || nz >= 18 {
				continue
			}
			l := li[(ny*18+nz)*18+nx]
			sky, blk = max(sky, l>>4), max(blk, l&15)
		}
		m.nl = append(m.nl, [2]int32{int32(i), int32(sky<<4 | blk)})
	}
	for _, e := range m.nl { // after the scan: neighbours read raw light
		li[e[0]] = uint8(e[1])
	}
}

func faceVisible(a, b *voxel.StateInfo) bool {
	if b.Opaque {
		return false
	}
	if a.CullSelf && b.Def == a.Def {
		return false
	}
	return true
}

// light computes smooth light + AO for the 4 corners of face f of the block at
// x,y,z (Minecraft's AmbientOcclusionFace, 0fps-style AO levels).
func (m *Mesher) light(x, y, z int, f voxel.Face) (c [4]cornerLight) {
	n := f.Normal()
	qx, qy, qz := x+n.X, y+n.Y, z+n.Z
	q := idx(qx, qy, qz)
	ua, va := uAxis[f], vAxis[f]
	var ustep, vstep int
	switch ua {
	case 0:
		ustep = 1
	case 2:
		ustep = 18
	}
	switch va {
	case 1:
		vstep = 18 * 18
	case 2:
		vstep = 18
	}
	t, st, li := m.table, &m.Snap.States, &m.Snap.Light
	for k := 0; k < 4; k++ {
		co := cornerOff[f][k]
		du, dv := -ustep, -vstep
		if co[ua] == 1 {
			du = ustep
		}
		if co[va] == 1 {
			dv = vstep
		}
		s1, s2, cr := q+du, q+dv, q+du+dv
		o1, o2 := t[st[s1]].Opaque, t[st[s2]].Opaque
		oc := t[st[cr]].Opaque || (o1 && o2)
		sky, blk, cnt := uint32(li[q]>>4), uint32(li[q]&15), uint32(1)
		ao := uint8(3)
		if o1 {
			ao--
		} else {
			sky += uint32(li[s1] >> 4)
			blk += uint32(li[s1] & 15)
			cnt++
		}
		if o2 {
			ao--
		} else {
			sky += uint32(li[s2] >> 4)
			blk += uint32(li[s2] & 15)
			cnt++
		}
		if oc {
			if !(o1 && o2) {
				ao--
			}
		} else {
			sky += uint32(li[cr] >> 4)
			blk += uint32(li[cr] & 15)
			cnt++
		}
		if o1 && o2 {
			ao = 0
		}
		c[k] = cornerLight{ao, uint8((sky*16 + cnt/2) / cnt), uint8((blk*16 + cnt/2) / cnt)}
	}
	return
}

// tint is the vertex colour of face f with texture tex of the block at
// x,y,z in the section.
func (m *Mesher) tint(info *voxel.StateInfo, f voxel.Face, tex uint16, x, y, z int) uint32 {
	if info.Tint == voxel.TintNone || info.TintMask&(1<<f) == 0 {
		return 0xFFFFFF
	}
	rule := m.cfg != nil && m.cfg.BlockTint != nil
	if !rule && m.cfg != nil && int(tex) < len(m.cfg.Tintable) && !m.cfg.Tintable[tex] {
		return 0xFFFFFF
	}
	c := TintColors[min(int(info.Tint), len(TintColors)-1)]
	switch {
	case info.Tint == voxel.TintFixed:
		c = info.TintColor
	case rule && m.Snap.HasBiomes:
		i := y<<8 | z<<4 | x
		if m.tintStamp[i] != m.builds {
			p := m.Snap.Pos
			ox, oz := p.X*16, p.Z*16
			m.tintStamp[i] = m.builds
			m.tintCache[i] = m.cfg.BlockTint(info, ox+x, p.Y*16+y, oz+z, func(bx, bz int) uint8 {
				return m.Snap.Biomes[(min(max(bz-oz, -1), 16)+1)*18+min(max(bx-ox, -1), 16)+1]
			})
		}
		c = m.tintCache[i]
	}
	return (c>>16)&0xff | (c>>8&0xff)<<8 | (c&0xff)<<16 | c&0xff000000 // to r,g,b byte order, weight on top
}

// cubes meshes full-cube faces with culling and greedy merging of faces whose
// texture, tint and (uniform) lighting match.
func (m *Mesher) cubes() {
	t, st := m.table, &m.Snap.States
	for f := voxel.Face(0); f < voxel.FaceCount; f++ {
		ax, ua, va := f.Axis(), uAxis[f], vAxis[f]
		off := nbOff[f]
		var p [3]int
		for d := 0; d < 16; d++ {
			p[ax] = d
			any := false
			for b := 0; b < 16; b++ {
				p[va] = b
				for a := 0; a < 16; a++ {
					p[ua] = a
					m.mask[b<<4|a] = 0
					i := idx(p[0], p[1], p[2])
					info := &t[st[i]]
					if info.Shape != voxel.ShapeCube {
						continue
					}
					if info.Cull == nil {
						if !faceVisible(info, &t[st[i+off]]) {
							continue
						}
					} else if side := info.Cull[f]; side < voxel.FaceCount && !faceVisible(info, &t[st[i+nbOff[side]]]) {
						continue // a model face without cullface (FaceCount) is never hidden
					}
					cl := m.light(p[0], p[1], p[2], f)
					tex := info.Tex[f]
					if info.Retex != nil {
						tex = info.FaceTex(f, func(g voxel.Face) *voxel.StateInfo { return &t[st[i+nbOff[g]]] })
					}
					if info.Connect != nil { // a differing icon is a differing key: no merge across it
						tex = m.connectTex(info, i, f)
					}
					tint := m.tint(info, f, tex, p[0], p[1], p[2])
					uvs, rot := m.cubeUV(info, p, f)
					if rot == 0 && info.Layer != voxel.LayerTranslucent && cl[0] == cl[1] && cl[1] == cl[2] && cl[2] == cl[3] {
						// key: tex 12 | layer 2 | ao 2 | sky 8 | blk 8 | tint 24 ; +1 so 0 = empty
						m.mask[b<<4|a] = (uint64(tex) | uint64(info.Layer)<<12 | uint64(cl[0].ao)<<14 |
							uint64(cl[0].sky)<<16 | uint64(cl[0].blk)<<24 | uint64(tint)<<32) + 1
						any = true
						continue
					}
					m.quadTops(f, p, 1, 1, tex, tint, info.Layer, cl, full, uvs)
				}
			}
			if any {
				m.greedy(f, p, ax)
			}
		}
	}
}

func (m *Mesher) greedy(f voxel.Face, p [3]int, ax int) {
	ua, va := uAxis[f], vAxis[f]
	mask := &m.mask
	for b := 0; b < 16; b++ {
		for a := 0; a < 16; {
			k := mask[b<<4|a]
			if k == 0 {
				a++
				continue
			}
			w := 1
			for a+w < 16 && mask[b<<4|(a+w)] == k {
				w++
			}
			h := 1
		grow:
			for b+h < 16 {
				for i := 0; i < w; i++ {
					if mask[(b+h)<<4|(a+i)] != k {
						break grow
					}
				}
				h++
			}
			for hh := 0; hh < h; hh++ {
				for i := 0; i < w; i++ {
					mask[(b+hh)<<4|(a+i)] = 0
				}
			}
			k--
			cl := cornerLight{uint8(k >> 14 & 3), uint8(k >> 16), uint8(k >> 24)}
			q := p
			q[ua], q[va] = a, b
			m.quad(f, q, w, h, uint16(k&0xfff), uint32(k>>32), voxel.Layer(k>>12&3), [4]cornerLight{cl, cl, cl, cl}, 32)
			a += w
		}
	}
	_ = ax
}

// quad emits one face quad. p is the block position (section local), w/h the
// size along the face's u/v axes, top the height of y=1 corners in 1/32.
func (m *Mesher) quad(f voxel.Face, p [3]int, w, h int, tex uint16, tint uint32, layer voxel.Layer, cl [4]cornerLight, top int) {
	uvs := &uvTab[f]
	if m.cfg != nil && m.cfg.RenderBlocksUV {
		uvs = &rbTab[f][0]
	}
	m.quadTops(f, p, w, h, tex, tint, layer, cl, [4]int{top, top, top, top}, uvs)
}

// uvRotate is the RenderBlocks uvRotate of face f of a block at
// section-local p: random for UVRandom faces, else StateInfo.UVRotate.
func (m *Mesher) uvRotate(info *voxel.StateInfo, p [3]int, f voxel.Face) int {
	if info.UVRandom&(1<<f) == 0 {
		return int(info.UVRotate[f] & 3)
	}
	sp := m.Snap.Pos
	return uvRandom(info, sp.X*16+p[0], sp.Y*16+p[1], sp.Z*16+p[2], f)
}

// rbFace reports whether face f is drawn by RenderBlocks' mapping.
func (m *Mesher) rbFace(info *voxel.StateInfo, f voxel.Face) bool {
	return info.UVRandom&(1<<f) != 0 || info.UVRotate[f] != 0 || m.cfg != nil && m.cfg.RenderBlocksUV
}

// cubeUV is the corner UV table of cube face f, and 0 when the face may
// merge with neighbours drawn by the default table.
func (m *Mesher) cubeUV(info *voxel.StateInfo, p [3]int, f voxel.Face) (*[4][2]uint32, int) {
	if !m.rbFace(info, f) {
		return &uvTab[f], 0
	}
	r := m.uvRotate(info, p, f)
	if r == 0 && !(m.cfg != nil && m.cfg.RenderBlocksUV) {
		r = 4 // RenderBlocks orientation, unlike the 1.8 default
	}
	return &rbTab[f][r&3], r
}

// quadTops is quad with the top height per corner, indexed x|z<<1 (liquid
// surfaces slope), and the tile corners' UVs uvs (repeated over w×h).
func (m *Mesher) quadTops(f voxel.Face, p [3]int, w, h int, tex uint16, tint uint32, layer voxel.Layer, cl [4]cornerLight, top [4]int, uvs *[4][2]uint32) {
	ua, va := uAxis[f], vAxis[f]
	var size [3]int
	size[f.Axis()] = 1
	size[ua], size[va] = w, h
	group := int(f)
	if layer == voxel.LayerTranslucent {
		group = GroupUnassigned
	}
	order := [4]int{0, 1, 2, 3}
	// Flip the diagonal when it would interpolate AO/light anisotropically.
	if int(cl[0].ao)+int(cl[2].ao) < int(cl[1].ao)+int(cl[3].ao) ||
		(cl[0].ao+cl[2].ao == cl[1].ao+cl[3].ao &&
			int(cl[0].sky)+int(cl[2].sky)+int(cl[0].blk)+int(cl[2].blk) < int(cl[1].sky)+int(cl[3].sky)+int(cl[1].blk)+int(cl[3].blk)) {
		order = [4]int{1, 2, 3, 0}
	}
	out := m.out[layer][group]
	var cx, cy, cz float32
	for _, k := range order {
		co := cornerOff[f][k]
		x := (p[0] + co[0]*size[0]) * 32
		y := (p[1] + co[1]*size[1]) * 32
		z := (p[2] + co[2]*size[2]) * 32
		if co[1] == 1 && top != full {
			y = p[1]*32 + top[co[0]|co[2]<<1]
		}
		cx += float32(x)
		cy += float32(y)
		cz += float32(z)
		u, v := uvs[k][0], uvs[k][1]
		u *= uint32(size[ua])
		v *= uint32(size[va])
		c := cl[k]
		p0, p1 := vpos(x*4, y*4, z*4)
		out = append(out,
			p0|uint32(c.ao)<<30,
			p1|uint32(tex)|u<<12|v<<17|uint32(f)<<22,
			tint&0xffffff|m.ridx,
			uint32(c.sky)|uint32(c.blk)<<8|tint>>24<<16)
	}
	m.out[layer][group] = out
	if layer == voxel.LayerTranslucent {
		m.trans = append(m.trans, [3]float32{cx / 128, cy / 128, cz / 128})
	}
}

// specials meshes liquids and cross plants.
func (m *Mesher) specials() {
	t, st, li := m.table, &m.Snap.States, &m.Snap.Light
	for y := 0; y < 16; y++ {
		for z := 0; z < 16; z++ {
			for x := 0; x < 16; x++ {
				i := idx(x, y, z)
				info := &t[st[i]]
				switch info.Shape {
				case voxel.ShapeLiquid:
					above := &t[st[i+nbOff[voxel.FaceUp]]]
					top := full
					if !(above.Def == info.Def || above.Opaque) {
						for k := range top {
							top[k] = m.liquidCorner(i, info.Def, k&1, k>>1)
						}
					}
					for f := voxel.Face(0); f < voxel.FaceCount; f++ {
						nb := &t[st[i+nbOff[f]]]
						if nb.Opaque || nb.Def == info.Def {
							if !(f == voxel.FaceUp && top != full && !nb.Opaque) {
								continue
							}
						}
						l := li[i+nbOff[f]]
						if f == voxel.FaceUp && top != full {
							l = li[i]
						}
						c := cornerLight{3, (l >> 4) * 16, (l & 15) * 16}
						cl := [4]cornerLight{c, c, c, c}
						tint := m.tint(info, f, info.Tex[f], x, y, z)
						p := [3]int{x, y, z}
						m.quadTops(f, p, 1, 1, info.Tex[f], tint, info.Layer, cl, top, &uvTab[f])
						if f == voxel.FaceUp && info.Layer == voxel.LayerTranslucent {
							m.quadReversed(p, info.Tex[f], tint, cl, top)
						}
					}
				case voxel.ShapeBoxes:
					m.boxes(x, y, z, i, info)
				case voxel.ShapeCross:
					l := li[i]
					c := cornerLight{3, (l >> 4) * 16, (l & 15) * 16}
					m.cross(x, y, z, plantOffset(info.PlantOffset, m.Snap.Pos.X*16+x, m.Snap.Pos.Y*16+y, m.Snap.Pos.Z*16+z), info.Tex[0], m.tint(info, voxel.FaceUp, info.Tex[0], x, y, z), info.Layer, c)
				}
			}
		}
	}
}

// projectedUV marks a vertex whose u/v are in 1/16 of a tile (box faces
// show the part of the texture their extent covers, like Minecraft models).
const projectedUV = 1 << 25

// boxes meshes a ShapeBoxes block's elements, flat lit. A face is culled by
// an opaque neighbour on its Box.Cull side (model cullface), or without
// Cull, when it lies on the block boundary facing that neighbour.
func (m *Mesher) boxes(x, y, z, i int, info *voxel.StateInfo) {
	t, st, li := m.table, &m.Snap.States, &m.Snap.Light
	bs := voxel.Connected(info, func(f voxel.Face) *voxel.StateInfo { return &t[st[i+nbOff[f]]] })
	base := [3]float32{float32(x * 16), float32(y * 16), float32(z * 16)}
	if info.PlantOffset != voxel.OffsetNone { // a plant model (cross with overlay, a double plant's top)
		sp := m.Snap.Pos
		off := plantOffset(info.PlantOffset, sp.X*16+x, sp.Y*16+y, sp.Z*16+z)
		for a := range 3 {
			base[a] += float32(off[a]) / 8
		}
	}
	smooth := info.Emission == 0 // RenderBlocks: AO only for blocks giving no light
	for _, b := range bs {
		for f := voxel.Face(0); f < voxel.FaceCount; f++ {
			if b.Skip&(1<<f) != 0 {
				continue
			}
			ax := f.Axis()
			onEdge := len(b.Rot) == 0 && ((f&1 == 0 && b.Min[ax] <= 0) || (f&1 == 1 && b.Max[ax] >= 16))
			side, cull := f, onEdge
			if b.Cull != nil {
				side = b.Cull[f]
				cull = side < voxel.FaceCount
			}
			if cull && t[st[i+nbOff[side]]].Opaque {
				continue
			}
			var cl [4]cornerLight
			if smooth && !b.Flat && len(b.Rot) == 0 {
				// renderStandardBlockWithAmbientOcclusion samples the layer
				// beyond a face on the block boundary, else the block's own.
				lx, ly, lz := x, y, z
				if !onEdge {
					n := f.Normal()
					lx, ly, lz = x-n.X, y-n.Y, z-n.Z
				}
				cl = m.light(lx, ly, lz, f)
			} else {
				l := li[i]
				if cull { // lit from the side that can hide it, as Minecraft's flat lighting
					l = li[i+nbOff[side]]
				}
				c := cornerLight{3, (l >> 4) * 16, (l & 15) * 16}
				cl = [4]cornerLight{c, c, c, c}
			}
			tex := info.Tex[f]
			if b.Tex[f] != "" {
				tex = b.TexID[f]
			}
			if info.Connect != nil {
				tex = m.connectTex(info, i, f)
			}
			tint := uint32(0xFFFFFF)
			if b.NoTint&(1<<f) == 0 {
				tint = m.tint(info, f, tex, x, y, z) // the face's own texture decides whether it is greyscale
			}
			m.boxFace(f, base, &b, tex, tint, info.Layer, &cl, m.uvRotate(info, [3]int{x, y, z}, f), info.UVRandom&(1<<f) != 0 || info.UVRotate[f] != 0)
		}
	}
}

// connectTex is face f's connected texture (voxel.Connect) of the block
// at snapshot index i.
func (m *Mesher) connectTex(info *voxel.StateInfo, i int, f voxel.Face) uint16 {
	t, st := m.table, &m.Snap.States
	return info.Connect.Tex(f, func(d [3]int) *voxel.StateInfo { return &t[st[i+d[0]+d[2]*18+d[1]*18*18]] })
}

// aoShade is the brightness of each cornerLight.ao level (shaders_gpu.go).
var aoShade = [4]float32{0.45, 0.65, 0.82, 1.0}

// cornerMix blends the four block-corner lights of face f bilinearly at
// point q (1/16) of the face, as 1.7 mixes its AO and brightness for
// partial render bounds; AO lands on the nearest level.
func cornerMix(f voxel.Face, cl *[4]cornerLight, q [3]float32) cornerLight {
	if cl[0] == cl[1] && cl[1] == cl[2] && cl[2] == cl[3] {
		return cl[0]
	}
	ua, va := uAxis[f], vAxis[f]
	wu, wv := min(max(q[ua]/16, 0), 1), min(max(q[va]/16, 0), 1)
	var ao, sky, blk float32
	for k, co := range cornerOff[f] {
		w := wu
		if co[ua] == 0 {
			w = 1 - wu
		}
		if co[va] == 0 {
			w *= 1 - wv
		} else {
			w *= wv
		}
		ao += w * aoShade[cl[k].ao]
		sky += w * float32(cl[k].sky)
		blk += w * float32(cl[k].blk)
	}
	c := cornerLight{sky: uint8(sky + 0.5), blk: uint8(blk + 0.5)}
	for l := uint8(1); l < 4; l++ {
		if math.Abs(float64(ao-aoShade[l])) < math.Abs(float64(ao-aoShade[c.ao])) {
			c.ao = l
		}
	}
	return c
}

// projUV is the texture position (1/16) of point q on face f when a texture
// is laid over the block's face (same orientation as uvTab).
func projUV(f voxel.Face, q [3]float32) (float32, float32) {
	switch f {
	case voxel.FaceDown:
		return q[0], 16 - q[2]
	case voxel.FaceUp:
		return q[0], q[2]
	case voxel.FaceNorth:
		return 16 - q[0], 16 - q[1]
	case voxel.FaceSouth:
		return q[0], 16 - q[1]
	case voxel.FaceWest:
		return q[2], 16 - q[1]
	}
	return 16 - q[2], 16 - q[1]
}

// boxFace emits face f of element b of the block at base (all 1/16), lit
// by the face's block corner lights cl interpolated to each vertex
// (renderStandardBlockWithAmbientOcclusionPartial, smooth lighting
// "Maximum"). rot is its uvRotate; rb (or Config.RenderBlocksUV for a face without
// model UVs) maps it like RenderBlocks, replacing the model's UVs.
func (m *Mesher) boxFace(f voxel.Face, base [3]float32, b *voxel.Box, tex uint16, tint uint32, layer voxel.Layer, cl *[4]cornerLight, rot int, rb bool) {
	var q [4][3]float32
	var uv [4][2]float32
	for k := 0; k < 4; k++ {
		co := cornerOff[f][k]
		for a := 0; a < 3; a++ {
			q[k][a] = b.Min[a]
			if co[a] == 1 {
				q[k][a] = b.Max[a]
			}
		}
		uv[k][0], uv[k][1] = projUV(f, q[k])
	}
	if b.UV != nil {
		// stretch the face's own extent over the element's u0,v0..u1,v1
		r := b.UV[f]
		lo, hi := uv[0], uv[0]
		for k := 1; k < 4; k++ {
			lo = [2]float32{min(lo[0], uv[k][0]), min(lo[1], uv[k][1])}
			hi = [2]float32{max(hi[0], uv[k][0]), max(hi[1], uv[k][1])}
		}
		for k := range uv {
			var st [2]float32 // corner position in the face, 0..1 along u, v
			for a := 0; a < 2; a++ {
				if hi[a] > lo[a] {
					st[a] = (uv[k][a] - lo[a]) / (hi[a] - lo[a])
				}
			}
			// Turning the texture clockwise a quarter shows at each corner
			// the texel of the corner before it (Minecraft's BlockFaceUV).
			for n := b.UVRot[f] & 3; n > 0; n-- {
				st = [2]float32{st[1], 1 - st[0]}
			}
			for a := 0; a < 2; a++ {
				uv[k][a] = r[a] + st[a]*(r[a+2]-r[a])
			}
		}
	}
	if len(b.Rot) == 0 && (rb || b.UV == nil && m.cfg != nil && m.cfg.RenderBlocksUV) {
		uv = rbUV(f, b.Min, b.Max, rot)
	}
	group, normal := int(f), uint32(f)
	if len(b.Rot) > 0 {
		for k := range q {
			q[k] = voxel.RotatePoint(q[k], b.Rot)
		}
		group, normal = GroupUnassigned, 6
		if n := voxel.RotateFace(f, b.Rot); n < voxel.FaceCount {
			normal = uint32(n)
		}
	}
	if layer == voxel.LayerTranslucent {
		group = GroupUnassigned
	}
	out := m.out[layer][group]
	var sum [3]float32
	for k := 0; k < 4; k++ {
		var p [3]int
		for a := 0; a < 3; a++ {
			v := base[a] + q[k][a]
			sum[a] += v
			p[a] = int(math.Floor(float64(v*8) + 0.5))
		}
		p0, p1 := vpos(p[0], p[1], p[2])
		c := cornerMix(f, cl, q[k])
		u := uint32(min(max(uv[k][0]+0.5, 0), 31))
		v := uint32(min(max(uv[k][1]+0.5, 0), 31))
		out = append(out,
			p0|uint32(c.ao)<<30,
			p1|uint32(tex)|u<<12|v<<17|normal<<22|projectedUV,
			tint&0xffffff|m.ridx,
			uint32(c.sky)|uint32(c.blk)<<8|tint>>24<<16)
	}
	m.out[layer][group] = out
	if layer == voxel.LayerTranslucent {
		m.trans = append(m.trans, [3]float32{sum[0] / 64, sum[1] / 64, sum[2] / 64})
	}
}

// full is a flat block top: every corner at 32/32.
var full = [4]int{32, 32, 32, 32}

// liquidHeight is a liquid's own surface height for a level (0 source).
func liquidHeight(level uint8) float32 { return float32(8-level) / 9 }

// liquidCorner is the surface height (1/32 block) at the top corner
// (cx,cz in 0..1) of liquid cell i, averaged like Minecraft's fluid
// renderer: the four cells around the corner count their liquid's height
// (ten times over when nearly full), non-solid cells count as empty, and
// liquid above any of them fills the corner.
func (m *Mesher) liquidCorner(i int, def *voxel.BlockDef, cx, cz int) int {
	t, st := m.table, &m.Snap.States
	var total, weight float32
	for dz := cz - 1; dz <= cz; dz++ {
		for dx := cx - 1; dx <= cx; dx++ {
			j := i + dx + dz*18
			if t[st[j+nbOff[voxel.FaceUp]]].Def == def {
				return 32
			}
			nb := &t[st[j]]
			switch {
			case nb.Def == def:
				if h := liquidHeight(nb.Level); h >= 0.8 {
					total, weight = total+h*10, weight+10
				} else {
					total, weight = total+h, weight+1
				}
			case !nb.Collides:
				weight++
			}
		}
	}
	return max(1, int(total/weight*32+0.5))
}

// quadReversed emits the underside of a liquid surface (seen from below).
func (m *Mesher) quadReversed(p [3]int, tex uint16, tint uint32, cl [4]cornerLight, top [4]int) {
	out := m.out[voxel.LayerTranslucent][GroupUnassigned]
	var cx, cz float32
	var y int
	for _, k := range [4]int{3, 2, 1, 0} {
		co := cornerOff[voxel.FaceUp][k]
		x, z := (p[0]+co[0])*32, (p[2]+co[2])*32
		y = p[1]*32 + top[co[0]|co[2]<<1]
		cx += float32(x)
		cz += float32(z)
		u, v := uvTab[voxel.FaceUp][k][0], uvTab[voxel.FaceUp][k][1]
		p0, p1 := vpos(x*4, y*4, z*4)
		out = append(out, p0|3<<30,
			p1|uint32(tex)|u<<12|v<<17|uint32(voxel.FaceDown)<<22, tint&0xffffff|m.ridx, uint32(cl[0].sky)|uint32(cl[0].blk)<<8|tint>>24<<16)
	}
	m.out[voxel.LayerTranslucent][GroupUnassigned] = out
	m.trans = append(m.trans, [3]float32{cx / 128, float32(y) / 32, cz / 128})
}

// cross emits the two diagonal planes of a plant moved by off (1/128),
// double sided (RenderBlocks.drawCrossedSquares: planes 0.05..0.95, the
// back of each plane with u mirrored so the texture reads the same from
// both sides).
func (m *Mesher) cross(x, y, z int, off [3]int, tex uint16, tint uint32, layer voxel.Layer, c cornerLight) {
	const lo, hi = 6, 122 // 0.05, 0.95 to 1/128
	bx, by, bz := x*128+off[0], y*128+off[1], z*128+off[2]
	planes := [2][4][3]int{
		{{lo, 0, lo}, {hi, 0, hi}, {hi, 128, hi}, {lo, 128, lo}},
		{{hi, 0, lo}, {lo, 0, hi}, {lo, 128, hi}, {hi, 128, lo}},
	}
	uvs := [4][2]uint32{{0, 1}, {1, 1}, {1, 0}, {0, 0}}
	group := GroupUnassigned
	out := m.out[layer][group]
	for _, pl := range planes {
		for side, order := range [2][4]int{{0, 1, 2, 3}, {1, 0, 3, 2}} {
			for _, k := range order {
				v := pl[k]
				u := uvs[k][0] ^ uint32(side)
				p0, p1 := vpos(bx+v[0], by+v[1], bz+v[2])
				out = append(out,
					p0|3<<30,
					p1|uint32(tex)|u<<12|uvs[k][1]<<17|6<<22,
					tint&0xffffff|m.ridx,
					uint32(c.sky)|uint32(c.blk)<<8|tint>>24<<16)
			}
			if layer == voxel.LayerTranslucent {
				m.trans = append(m.trans, [3]float32{float32(bx+64) / 128, float32(by+64) / 128, float32(bz+64) / 128})
			}
		}
	}
	m.out[layer][group] = out
}

// plantOffset is the shift (1/128 block) 1.7 gives the plant at world
// x,y,z: s = (long)(x*3129871) ^ z*116129781L ^ y (no y for double
// plants), s = s*s*42317861 + s*11, then 4-bit fields of s pick each axis.
func plantOffset(o voxel.PlantOffset, x, y, z int) (off [3]int) {
	if o == voxel.OffsetNone {
		return
	}
	s := int64(int32(x)*3129871) ^ int64(z)*116129781
	if o != voxel.OffsetDouble {
		s ^= int64(y)
	}
	s = s*s*42317861 + s*11
	n := func(sh uint) float64 { return float64(float32(s>>sh&15) / 15) }
	q := func(d float64) int { return int(math.Floor(d*128 + 0.5)) }
	k := 0.3
	if o == voxel.OffsetGrass {
		k = 0.5
		off[1] = q((n(20) - 1) * 0.2)
	}
	off[0], off[2] = q((n(16)-0.5)*k), q((n(24)-0.5)*k)
	return
}

// SortTranslucent returns the translucent vertices ordered back to front for
// a camera at cam (section-local block units).
func (m *Mesh) SortTranslucent(cam [3]float32) []uint32 {
	n := len(m.TransCenters)
	if n == 0 {
		return nil
	}
	type kv struct {
		d float32
		i int
	}
	ks := make([]kv, n)
	for i, c := range m.TransCenters {
		dx, dy, dz := c[0]-cam[0], c[1]-cam[1], c[2]-cam[2]
		ks[i] = kv{dx*dx + dy*dy + dz*dz, i}
	}
	sort.Slice(ks, func(a, b int) bool { return ks[a].d > ks[b].d })
	out := make([]uint32, 0, len(m.Trans))
	q := 4 * VertexWords
	for _, k := range ks {
		out = append(out, m.Trans[k.i*q:(k.i+1)*q]...)
	}
	return out
}
