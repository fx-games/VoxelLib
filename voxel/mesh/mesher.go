package mesh

import (
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
//	w0: x:10 y:10 z:10 (1/32 block, section local) ao:2
//	w1: texture layer:12 u:5 v:5 normal:3
//	w2: tint r:8 g:8 b:8, section index in region:8
//	w3: sky light:8 block light:8 (level*16, smoothly averaged)
const VertexWords = 4

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
	mask  [256]uint64
	ridx  uint32
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

func (m *Mesher) tint(info *voxel.StateInfo, f voxel.Face) uint32 {
	if info.Tint == voxel.TintNone || info.TintMask&(1<<f) == 0 {
		return 0xFFFFFF
	}
	tex := info.Tex[f]
	if m.cfg != nil && int(tex) < len(m.cfg.Tintable) && !m.cfg.Tintable[tex] {
		return 0xFFFFFF
	}
	c := TintColors[info.Tint]
	return (c>>16)&0xff | (c>>8&0xff)<<8 | (c&0xff)<<16 // to r,g,b byte order
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
					if info.Shape != voxel.ShapeCube || !faceVisible(info, &t[st[i+off]]) {
						continue
					}
					cl := m.light(p[0], p[1], p[2], f)
					tint := m.tint(info, f)
					if info.Layer != voxel.LayerTranslucent && cl[0] == cl[1] && cl[1] == cl[2] && cl[2] == cl[3] {
						// key: tex 12 | layer 2 | ao 2 | sky 8 | blk 8 | tint 24 ; +1 so 0 = empty
						m.mask[b<<4|a] = (uint64(info.Tex[f]) | uint64(info.Layer)<<12 | uint64(cl[0].ao)<<14 |
							uint64(cl[0].sky)<<16 | uint64(cl[0].blk)<<24 | uint64(tint)<<32) + 1
						any = true
						continue
					}
					m.quad(f, p, 1, 1, info.Tex[f], tint, info.Layer, cl, 32)
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
	m.quadTops(f, p, w, h, tex, tint, layer, cl, [4]int{top, top, top, top})
}

// quadTops is quad with the top height per corner, indexed x|z<<1 (liquid
// surfaces slope).
func (m *Mesher) quadTops(f voxel.Face, p [3]int, w, h int, tex uint16, tint uint32, layer voxel.Layer, cl [4]cornerLight, top [4]int) {
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
		u := uvTab[f][k][0] * uint32(size[ua])
		v := uvTab[f][k][1] * uint32(size[va])
		c := cl[k]
		out = append(out,
			uint32(x)|uint32(y)<<10|uint32(z)<<20|uint32(c.ao)<<30,
			uint32(tex)|u<<12|v<<17|uint32(f)<<22,
			tint|m.ridx,
			uint32(c.sky)|uint32(c.blk)<<8)
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
						tint := m.tint(info, f)
						p := [3]int{x, y, z}
						m.quadTops(f, p, 1, 1, info.Tex[f], tint, info.Layer, cl, top)
						if f == voxel.FaceUp && info.Layer == voxel.LayerTranslucent {
							m.quadReversed(p, info.Tex[f], tint, cl, top)
						}
					}
				case voxel.ShapeCross:
					l := li[i]
					c := cornerLight{3, (l >> 4) * 16, (l & 15) * 16}
					m.cross(x, y, z, info.Tex[0], m.tint(info, voxel.FaceUp), info.Layer, c)
				}
			}
		}
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
		out = append(out, uint32(x)|uint32(y)<<10|uint32(z)<<20|3<<30,
			uint32(tex)|u<<12|v<<17|uint32(voxel.FaceDown)<<22, tint|m.ridx, uint32(cl[0].sky)|uint32(cl[0].blk)<<8)
	}
	m.out[voxel.LayerTranslucent][GroupUnassigned] = out
	m.trans = append(m.trans, [3]float32{cx / 128, float32(y) / 32, cz / 128})
}

// cross emits the two diagonal planes of a plant, double sided.
func (m *Mesher) cross(x, y, z int, tex uint16, tint uint32, layer voxel.Layer, c cornerLight) {
	const lo, hi = 2, 30 // 1/32 inset like vanilla's 0.8/16
	bx, by, bz := x*32, y*32, z*32
	planes := [2][4][3]int{
		{{lo, 0, lo}, {hi, 0, hi}, {hi, 32, hi}, {lo, 32, lo}},
		{{hi, 0, lo}, {lo, 0, hi}, {lo, 32, hi}, {hi, 32, lo}},
	}
	uvs := [4][2]uint32{{0, 1}, {1, 1}, {1, 0}, {0, 0}}
	group := GroupUnassigned
	out := m.out[layer][group]
	for _, pl := range planes {
		for _, order := range [2][4]int{{0, 1, 2, 3}, {1, 0, 3, 2}} {
			for _, k := range order {
				v := pl[k]
				out = append(out,
					uint32(bx+v[0])|uint32(by+v[1])<<10|uint32(bz+v[2])<<20|3<<30,
					uint32(tex)|uvs[k][0]<<12|uvs[k][1]<<17|6<<22,
					tint|m.ridx,
					uint32(c.sky)|uint32(c.blk)<<8)
			}
			if layer == voxel.LayerTranslucent {
				m.trans = append(m.trans, [3]float32{float32(x) + .5, float32(y) + .5, float32(z) + .5})
			}
		}
	}
	m.out[layer][group] = out
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
