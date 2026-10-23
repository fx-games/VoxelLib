package mesh

import (
	"math"
	"testing"

	"github.com/memmaker/voxellib/voxel"
)

// vert is one decoded vertex: position in 1/32 block, u/v, normal, tint.
type vert struct {
	pos          [3]uint32
	u, v, normal uint32
	tint         uint32
}

// meshBoxes meshes block def at 1,1,1 of section 0 with optional stone
// neighbours, and returns its vertices (stone is a cube: only boxes are
// non-cube faces, but stone faces are filtered out by texture).
func meshBoxes(t *testing.T, def *voxel.BlockDef, stoneAt ...[3]int) []vert {
	t.Helper()
	reg := voxel.NewRegistry()
	reg.Register(def)
	w := voxel.NewWorld(reg, 0, 64)
	w.AddChunk(w.NewChunk(voxel.ChunkPos{}))
	stone := reg.Parse("stone")
	for _, p := range stoneAt {
		w.SetBlock(p[0], p[1], p[2], stone)
	}
	w.SetBlock(1, 1, 1, reg.State(def.Name, nil))
	m := NewMesher()
	m.Snap.Capture(w, voxel.SectionPos{})
	res := m.Build(reg.Table(), nil)
	var out []vert
	for i := 0; i+VertexWords <= len(res.Data); i += VertexWords {
		d := res.Data[i : i+VertexWords]
		if d[1]&projectedUV == 0 {
			continue // a stone cube face
		}
		out = append(out, vert{
			pos: pos32(d),
			u:   d[1] >> 12 & 31, v: d[1] >> 17 & 31, normal: d[1] >> 22 & 7,
			tint: d[2] & 0xFFFFFF,
		})
	}
	return out
}

func cube(name string, b voxel.Box) *voxel.BlockDef {
	b.Min, b.Max = [3]float32{0, 0, 0}, [3]float32{16, 16, 16}
	return &voxel.BlockDef{Name: name, Shape: voxel.ShapeBoxes, Layer: voxel.LayerCutout,
		Textures: [6]string{"a", "a", "a", "a", "a", "a"}, Boxes: []voxel.Box{b}}
}

func faces(vs []vert) map[voxel.Face]int {
	n := map[voxel.Face]int{}
	for _, v := range vs {
		n[voxel.Face(v.normal)]++
	}
	for f := range n {
		n[f] /= 4
	}
	return n
}

// TestFaceUVRotation: "rotation": 90 turns the up face's texture clockwise,
// so each corner shows the texel of the corner before it.
func TestFaceUVRotation(t *testing.T) {
	uv := [6][4]float32{}
	for f := range uv {
		uv[f] = [4]float32{0, 0, 16, 16}
	}
	b := voxel.Box{UV: &uv, Skip: 0x3f &^ (1 << voxel.FaceUp)}
	b.UVRot[voxel.FaceUp] = 1
	vs := meshBoxes(t, cube("test:rot", b))
	if len(vs) != 4 {
		t.Fatalf("%d vertices, want one quad", len(vs))
	}
	// corner (x,z) in block pixels → want u,v (unrotated: u=x, v=z)
	want := map[[2]uint32][2]uint32{{0, 0}: {0, 16}, {16, 0}: {0, 0}, {16, 16}: {16, 0}, {0, 16}: {16, 16}}
	for _, v := range vs {
		c := [2]uint32{v.pos[0]/2 - 16, v.pos[2]/2 - 16}
		if got := [2]uint32{v.u, v.v}; got != want[c] {
			t.Errorf("corner %v: uv %v, want %v", c, got, want[c])
		}
	}
}

// TestCullface: a face is hidden only by an opaque neighbour on its
// cullface side; a face without cullface stays even against stone.
func TestCullface(t *testing.T) {
	cull := [6]voxel.Face{}
	for f := range cull {
		cull[f] = voxel.FaceCount
	}
	cull[voxel.FaceUp] = voxel.FaceUp
	cull[voxel.FaceEast] = voxel.FaceDown // culled by what is below it
	def := cube("test:cull", voxel.Box{Cull: &cull})
	n := faces(meshBoxes(t, def, [3]int{1, 2, 1}, [3]int{1, 1, 0}))
	if n[voxel.FaceUp] != 0 || n[voxel.FaceNorth] != 1 || n[voxel.FaceEast] != 1 || len(n) != 5 {
		t.Fatalf("stone above and north: faces %v; want up culled, north (no cullface) kept", n)
	}
	n = faces(meshBoxes(t, def, [3]int{1, 0, 1}))
	if n[voxel.FaceEast] != 0 || n[voxel.FaceUp] != 1 || n[voxel.FaceDown] != 1 {
		t.Fatalf("stone below: faces %v; want only east culled", n)
	}
	// without Cull (code-built boxes) boundary faces are culled as before
	if n := faces(meshBoxes(t, cube("test:auto", voxel.Box{}), [3]int{1, 1, 0})); n[voxel.FaceNorth] != 0 || len(n) != 5 {
		t.Fatalf("no Cull: faces %v; want north culled", n)
	}
}

// TestRescale: a 45° turned full-width plane with "rescale" spans the block
// diagonal corner to corner; without it, it stops short.
func TestRescale(t *testing.T) {
	for _, rescale := range []bool{false, true} {
		b := voxel.Box{Skip: 0x3f &^ (1 << voxel.FaceNorth)}
		b.Rot = []voxel.Rot{{Axis: 1, Deg: 45, Origin: [3]float32{8, 8, 8}, Rescale: rescale}}
		def := cube("test:x", b)
		def.Boxes[0].Min[2], def.Boxes[0].Max[2] = 8, 8
		vs := meshBoxes(t, def)
		if len(vs) != 4 {
			t.Fatalf("%d vertices", len(vs))
		}
		lo, hi := uint32(1023), uint32(0)
		for _, v := range vs {
			lo, hi = min(lo, v.pos[0]), max(hi, v.pos[0])
		}
		// block at x=1: 32..64 in 1/32; unscaled 8±8·cos45 = 2.34..13.66 px
		wantLo, wantHi := uint32(32), uint32(64)
		if !rescale {
			d := 8 * math.Sqrt2 / 2
			wantLo, wantHi = uint32(32+(8-d)*2+0.5), uint32(32+(8+d)*2+0.5)
		}
		if lo != wantLo || hi != wantHi {
			t.Errorf("rescale %v: x %d..%d, want %d..%d", rescale, lo, hi, wantLo, wantHi)
		}
	}
}

// TestFaceTint: only faces with a tintindex take the block's tint colour.
func TestFaceTint(t *testing.T) {
	def := cube("test:tint", voxel.Box{NoTint: 0x3f &^ (1 << voxel.FaceUp)})
	def.Tint = voxel.TintGrass
	vs := meshBoxes(t, def)
	g := TintColors[voxel.TintGrass]
	grass := (g>>16)&0xff | (g>>8&0xff)<<8 | (g&0xff)<<16
	for _, v := range vs {
		want := uint32(0xFFFFFF)
		if voxel.Face(v.normal) == voxel.FaceUp {
			want = grass
		}
		if v.tint != want {
			t.Fatalf("face %v tint %06x, want %06x", voxel.Face(v.normal), v.tint, want)
		}
	}
	if len(vs) != 24 {
		t.Fatalf("%d vertices, want 6 faces", len(vs))
	}
}

// TestCubeFaceRules: a resolved cube culls and tints per face like its
// model: the face without cullface stays against stone, only the face with
// a tintindex is tinted.
func TestCubeFaceRules(t *testing.T) {
	reg := voxel.NewRegistry()
	rules := voxel.FaceRules{Cull: [6]voxel.Face{0, 1, voxel.FaceCount, 3, 4, 5}, Tinted: 1 << voxel.FaceSouth}
	reg.Resolver = func(name string, _ map[string]string) (voxel.Shape, [6]string, bool) {
		return voxel.ShapeCube, [6]string{"c", "c", "c", "c", "c", "c"}, name == "test:c"
	}
	reg.Faces = func(name string, _ map[string]string) (voxel.FaceRules, bool) { return rules, name == "test:c" }
	reg.Register(&voxel.BlockDef{Name: "test:c", Shape: voxel.ShapeCube, Tint: voxel.TintGrass, Opaque: true, Collides: true})
	w := voxel.NewWorld(reg, 0, 64)
	w.AddChunk(w.NewChunk(voxel.ChunkPos{}))
	stone := reg.Parse("stone")
	w.SetBlock(1, 1, 1, reg.State("test:c", nil))
	w.SetBlock(1, 2, 1, stone) // above: up culled
	w.SetBlock(1, 1, 0, stone) // north: kept, no cullface
	m := NewMesher()
	m.Snap.Capture(w, voxel.SectionPos{})
	res := m.Build(reg.Table(), nil)
	cTex := reg.Info(reg.State("test:c", nil)).Tex[0]
	n := map[voxel.Face]int{}
	for i := 0; i+VertexWords <= len(res.Data); i += VertexWords {
		d := res.Data[i : i+VertexWords]
		if uint16(d[1]&0xfff) != cTex {
			continue
		}
		f := voxel.Face(d[1] >> 22 & 7)
		n[f]++
		if tinted := d[2]&0xFFFFFF != 0xFFFFFF; tinted != (f == voxel.FaceSouth) {
			t.Errorf("face %v tint %06x", f, d[2]&0xFFFFFF)
		}
	}
	if n[voxel.FaceUp] != 0 || n[voxel.FaceNorth] != 4 || len(n) != 5 {
		t.Fatalf("vertices per face %v; want up culled, north kept", n)
	}
}

// TestBoxTextureTint: a model face is tinted by whether its own texture is
// greyscale, not the state's (grass: painted side, grey overlay box).
func TestBoxTextureTint(t *testing.T) {
	def := cube("test:overlay", voxel.Box{Tex: [6]string{"", "", "grey", "grey", "grey", "grey"}, Skip: 3})
	def.Tint = voxel.TintGrass
	reg := voxel.NewRegistry()
	reg.Register(def)
	w := voxel.NewWorld(reg, 0, 64)
	w.AddChunk(w.NewChunk(voxel.ChunkPos{}))
	id := reg.State(def.Name, nil)
	w.SetBlock(1, 1, 1, id)
	info := reg.Table()[id]
	grey := info.Boxes[0].TexID[voxel.FaceNorth]
	tintable := make([]bool, int(max(grey, info.Tex[voxel.FaceNorth]))+1)
	tintable[grey] = true // "a" stays painted
	m := NewMesher()
	m.Snap.Capture(w, voxel.SectionPos{})
	res := m.Build(reg.Table(), &Config{Tintable: tintable})
	n := 0
	for i := 0; i+VertexWords <= len(res.Data); i += VertexWords {
		if res.Data[i+2]&0xFFFFFF != 0xFFFFFF {
			n++
		}
	}
	if n != 16 {
		t.Fatalf("%d tinted vertices, want the 4 grey side faces", n)
	}
}

// TestBlockTint: Config.BlockTint colours a tinted block from the biome of
// its column, read across the chunk border.
func TestBlockTint(t *testing.T) {
	def := cube("test:biome", voxel.Box{})
	def.Tint = voxel.TintGrass
	reg := voxel.NewRegistry()
	reg.Register(def)
	w := voxel.NewWorld(reg, 0, 64)
	for _, p := range []voxel.ChunkPos{{}, {X: -1}} {
		c := w.NewChunk(p)
		c.Biomes = make([]byte, 256)
		for i := range c.Biomes {
			c.Biomes[i] = byte(10 + p.X) // 10 here, 9 in the west chunk
		}
		w.AddChunk(c)
	}
	w.SetBlock(0, 1, 0, reg.State(def.Name, nil))
	m := NewMesher()
	m.Snap.Capture(w, voxel.SectionPos{})
	res := m.Build(reg.Table(), &Config{BlockTint: func(_ *voxel.StateInfo, x, y, z int, biome func(x, z int) uint8) uint32 {
		return uint32(biome(x, z))<<16 | uint32(biome(x-1, z))
	}})
	if len(res.Data) == 0 {
		t.Fatal("no faces")
	}
	for i := 0; i+VertexWords <= len(res.Data); i += VertexWords {
		if got := res.Data[i+2] & 0xFFFFFF; got != 9<<16|10 { // r=10, b=9 in r,g,b byte order
			t.Fatalf("tint %06x", got)
		}
	}
}

// TestDynamicTintWeight: BlockTint's top byte lands in w3 bits 16-23 and
// never leaks into the baked colour.
func TestDynamicTintWeight(t *testing.T) {
	def := cube("test:water", voxel.Box{})
	def.Tint = voxel.TintWater
	reg := voxel.NewRegistry()
	reg.Register(def)
	w := voxel.NewWorld(reg, 0, 64)
	c := w.NewChunk(voxel.ChunkPos{})
	c.Biomes = make([]byte, 256)
	w.AddChunk(c)
	w.SetBlock(0, 1, 0, reg.State(def.Name, nil))
	m := NewMesher()
	m.Snap.Capture(w, voxel.SectionPos{})
	res := m.Build(reg.Table(), &Config{BlockTint: func(*voxel.StateInfo, int, int, int, func(x, z int) uint8) uint32 {
		return 0x55102030
	}})
	for i := 0; i+VertexWords <= len(res.Data); i += VertexWords {
		if got := res.Data[i+2] & 0xFFFFFF; got != 0x302010 {
			t.Fatalf("tint %06x", got)
		}
		if got := res.Data[i+3] >> 16 & 255; got != 0x55 {
			t.Fatalf("weight %x", got)
		}
	}
}

// TestFaceUVRotHook: turned cube faces are not merged, and turn n shows at
// corner k the uv of corner k+n.
// TestRenderBlocksUV checks rbUV on full cubes against the uvRotate
// tables derived independently by executing RenderBlocks.renderFace* (look
// wave 2), written as turns | transpose<<2 applied to the 1.8 uvTab.
func TestRenderBlocksUV(t *testing.T) {
	ref := [6][4]uint8{{5, 4, 6, 7}, {0, 1, 3, 2}, {0, 6, 4, 2}, {0, 1, 3, 2}, {0, 1, 3, 2}, {0, 6, 4, 2}}
	for f := voxel.Face(0); f < voxel.FaceCount; f++ {
		for r := range 4 {
			for k, uv := range uvTab[f] {
				u, v := uv[0], uv[1]
				if ref[f][r]&4 != 0 {
					u, v = v, u
				}
				for n := ref[f][r] & 3; n > 0; n-- {
					u, v = v, 1-u
				}
				if got := rbTab[f][r][k]; got != [2]uint32{u, v} {
					t.Errorf("%v rot %d corner %d: %v, want %v", f, r, k, got, [2]uint32{u, v})
				}
			}
		}
	}
	// a half slab's north face: u mirrored within x 0..8, v from y 0..8
	uv := rbUV(voxel.FaceNorth, [3]float32{0, 0, 0}, [3]float32{8, 8, 16}, 0)
	if uv[0] != [2]float32{0, 16} || uv[1] != [2]float32{8, 16} || uv[2] != [2]float32{8, 8} {
		t.Errorf("north of a half box: %v", uv)
	}
}

// TestUVRandom: the hash matches java.util.Random, and a turned top face
// is not merged and shows RenderBlocks' uvRotate.
func TestUVRandom(t *testing.T) {
	ident := &[6]uint8{0, 1, 2, 3, 4, 5}
	for _, c := range []struct {
		x, y, z int
		want    string
	}{{0, 0, 0, "120211"}, {251, 64, -320, "200023"}, {-1234, 70, 98765, "112332"}, {7, 3, -9, "020211"}} {
		got := ""
		for f := voxel.Face(0); f < voxel.FaceCount; f++ {
			got += string(rune('0' + uvRandom(&voxel.StateInfo{UVSeed: ident}, c.x, c.y, c.z, f)))
		}
		if got != c.want {
			t.Errorf("rot(%d,%d,%d) = %s, want %s", c.x, c.y, c.z, got, c.want)
		}
	}
	reg := voxel.NewRegistry()
	reg.Register(&voxel.BlockDef{Name: "test:sand", Shape: voxel.ShapeCube, Opaque: true, Collides: true,
		Textures: [6]string{"a", "a", "a", "a", "a", "a"}, UVRandom: 1 << voxel.FaceUp})
	w := voxel.NewWorld(reg, 0, 64)
	w.AddChunk(w.NewChunk(voxel.ChunkPos{}))
	sand := reg.State("test:sand", nil)
	for x := 0; x < 16; x++ {
		w.SetBlock(x, 1, 0, sand)
	}
	m := NewMesher()
	m.Snap.Capture(w, voxel.SectionPos{})
	res := m.Build(reg.Table(), &Config{RenderBlocksUV: true})
	rg := res.Ranges[voxel.LayerSolid][voxel.FaceUp]
	turned := map[int]bool{}
	for x := 0; x < 16; x++ {
		if r := uvRandom(reg.Info(sand), x, 1, 0, voxel.FaceUp); r != 0 {
			turned[x] = true
		}
	}
	for q := rg.First; q < rg.First+rg.Count; q += 4 {
		d := res.Data[q*VertexWords:]
		x0 := int(pos32(d)[0]) / 32
		for k := range 4 {
			x1 := int(pos32(d[k*VertexWords:])[0]) / 32
			x0 = min(x0, x1)
		}
		r := uvRandom(reg.Info(sand), x0, 1, 0, voxel.FaceUp)
		if !turned[x0] {
			continue
		}
		for k := range 4 {
			dv := d[k*VertexWords:]
			cx, cz := pos32(dv)[0]/32-uint32(x0), pos32(dv)[2]/32
			for j, co := range cornerOff[voxel.FaceUp] {
				if uint32(co[0]) == cx && uint32(co[2]) == cz {
					if uv := [2]uint32{dv[1] >> 12 & 31, dv[1] >> 17 & 31}; uv != rbTab[voxel.FaceUp][r][j] {
						t.Errorf("x %d rot %d corner %d: uv %v, want %v", x0, r, j, uv, rbTab[voxel.FaceUp][r][j])
					}
				}
			}
		}
		delete(turned, x0)
	}
	if len(turned) != 0 {
		t.Errorf("turned blocks without their own quad: %v", turned)
	}
}

// pos32 decodes a vertex's section-local position to 1/32 block, rounded.
func pos32(d []uint32) (p [3]uint32) {
	for a := range 3 {
		q := int(d[0]>>(10*a)&1023)<<2 | int(d[1]>>(26+2*a)&3) - 8*128
		p[a] = uint32((q + 2) >> 2)
	}
	return
}
