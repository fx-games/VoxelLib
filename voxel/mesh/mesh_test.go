package mesh

import (
	"math"
	"testing"
	"time"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/worldgen/overworld"
)

func TestMeshGenerated(t *testing.T) {
	reg := voxel.NewRegistry()
	w := voxel.NewWorld(reg, -64, 384)
	g := overworld.New(reg, 7)
	for cx := -2; cx <= 2; cx++ {
		for cz := -2; cz <= 2; cz++ {
			c := w.NewChunk(voxel.ChunkPos{X: cx, Z: cz})
			g.Generate(w, c)
			w.AddChunk(c)
		}
	}
	start := time.Now()
	for cx := -1; cx <= 1; cx++ {
		for cz := -1; cz <= 1; cz++ {
			w.TryLightChunk(voxel.ChunkPos{X: cx, Z: cz})
		}
	}
	t.Logf("light: %v/chunk", time.Since(start)/9)
	m := NewMesher()
	quads, n := 0, 0
	start = time.Now()
	for sy := -4; sy < 20; sy++ {
		m.Snap.Capture(w, voxel.SectionPos{X: 0, Y: sy, Z: 0})
		res := m.Build(reg.Table(), nil)
		quads += res.Quads()
		n++
	}
	t.Logf("mesh: %v/section, %d quads in column", time.Since(start)/time.Duration(n), quads)
	if quads == 0 {
		t.Fatal("no quads")
	}
}

func TestLiquidCorners(t *testing.T) {
	reg := voxel.NewRegistry()
	w := voxel.NewWorld(reg, 0, 64)
	c := w.NewChunk(voxel.ChunkPos{})
	w.AddChunk(c)
	stone, source, film := reg.Parse("stone"), reg.Parse("water"), reg.Parse("water[level=7]")
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			w.SetBlock(x, 0, z, stone)
		}
	}
	w.SetBlock(2, 1, 2, source)
	for _, p := range [][2]int{{6, 6}, {7, 6}, {6, 7}, {7, 7}} { // a 2×2 puddle
		w.SetBlock(p[0], 1, p[1], film)
	}
	m := NewMesher()
	m.Snap.Capture(w, voxel.SectionPos{})
	m.table = reg.Table()
	def := reg.Info(source).Def
	at := func(x, z, cx, cz int) int { return m.liquidCorner(idx(x, 1, z), def, cx, cz) }
	if h := at(2, 2, 0, 0); h != 22 { // source: 8/9*10 over weight 13 → 0.68
		t.Errorf("lone source corner %d, want 22", h)
	}
	if h := at(6, 6, 1, 1); h != 4 { // puddle centre: 1/9 all round
		t.Errorf("puddle centre %d, want 4", h)
	}
	if h := at(6, 6, 0, 0); h != 1 { // puddle edge: a film
		t.Errorf("puddle edge %d, want 1", h)
	}
	if h := at(7, 6, 0, 1); h != 4 { // the same centre corner seen from a neighbour
		t.Errorf("shared corner %d, want 4", h)
	}
}

// TestNeighbourLight: a useNeighborBrightness block (light blocking stairs)
// takes the brightest light above or beside it, never from below.
func TestNeighbourLight(t *testing.T) {
	reg := voxel.NewRegistry()
	reg.Register(&voxel.BlockDef{Name: "test:slab", Shape: voxel.ShapeBoxes, Layer: voxel.LayerSolid,
		Textures: [6]string{"a", "a", "a", "a", "a", "a"}, NeighbourLight: true, LightOpacity: 15,
		Boxes: []voxel.Box{{Max: [3]float32{16, 8, 16}}}})
	slab := reg.State("test:slab", nil)
	m := NewMesher()
	i := idx(4, 4, 4)
	m.Snap.States[i] = slab
	m.Snap.Light[i+nbOff[voxel.FaceDown]] = 15<<4 | 15
	m.Snap.Light[i+nbOff[voxel.FaceUp]] = 9 << 4
	m.Snap.Light[i+nbOff[voxel.FaceEast]] = 3<<4 | 7
	m.table = reg.Table()
	m.neighbourLight()
	if got := m.Snap.Light[i]; got != 9<<4|7 {
		t.Errorf("mixed light sky %d block %d, want 9 and 7", got>>4, got&15)
	}
}

func TestPlantOffset(t *testing.T) {
	// Expected values from an independent transcription of 1.7's
	// renderCrossedSquares / renderBlockDoublePlant arithmetic.
	for _, c := range []struct {
		o       voxel.PlantOffset
		x, y, z int
		want    [3]int
	}{
		{voxel.OffsetGrass, 0, 64, 0, [3]int{-23, -12, 15}},
		{voxel.OffsetGrass, -123, 70, 4567, [3]int{19, -9, -19}},
		{voxel.OffsetFlower, 1000, 3, -77, [3]int{-12, 0, 17}},
		{voxel.OffsetClover, 0, 64, 0, [3]int{-23, 0, 15}}, // the grass hash, y kept
		{voxel.OffsetDouble, 5, 9, -6, [3]int{-1, 0, -12}},
		{voxel.OffsetDouble, 5, 10, -6, [3]int{-1, 0, -12}},
		{voxel.OffsetNone, 5, 10, -6, [3]int{}},
	} {
		if got := plantOffset(c.o, c.x, c.y, c.z); got != c.want {
			t.Errorf("%d at %d,%d,%d: %v, want %v", c.o, c.x, c.y, c.z, got, c.want)
		}
	}
}

// TestPlantTurn: the sunflower head turn (renderBlockDoublePlant), values
// from a Python transcription of the Java long/double arithmetic.
func TestPlantTurn(t *testing.T) {
	for _, c := range []struct {
		x, z int
		want float64
	}{{0, 0, -18}, {5, -6, 13.179454553040834}} {
		if got := plantTurn(c.x, c.z); math.Abs(float64(got)-c.want) > 1e-3 {
			t.Errorf("%d,%d: %v, want %v", c.x, c.z, got, c.want)
		}
	}
}

// Each plane's back quad shows the texture unmirrored: at the same corner
// its u is the front's 1-u (drawCrossedSquares).
func TestCrossBackUV(t *testing.T) {
	m := NewMesher()
	m.cross(0, 0, 0, [3]int{}, 1, 0, voxel.LayerCutout, cornerLight{})
	d := m.out[voxel.LayerCutout][GroupUnassigned]
	if len(d) != 16*VertexWords {
		t.Fatalf("%d words", len(d))
	}
	for q := 0; q < 4; q += 2 {
		u := map[[3]uint32]uint32{}
		for k := range 4 {
			v := d[(q*4+k)*VertexWords:]
			u[pos32(v)] = v[1] >> 12 & 31
		}
		for k := range 4 {
			v := d[((q+1)*4+k)*VertexWords:]
			if p := pos32(v); v[1]>>12&31 != 1-u[p] {
				t.Errorf("plane %d corner %v: back u %d, front %d", q/2, p, v[1]>>12&31, u[p])
			}
		}
	}
}

// A cube's Retex swaps its side textures while snow lies on it.
func TestRetex(t *testing.T) {
	reg := voxel.NewRegistry()
	reg.Register(&voxel.BlockDef{Name: "test:stone", Shape: voxel.ShapeCube, Layer: voxel.LayerSolid, Opaque: true,
		Textures: [6]string{"s", "s", "s", "s", "s", "s"}, LightOpacity: 15,
		Retex: []voxel.Retex{{When: []voxel.Cond{{Face: voxel.FaceUp, Any: []voxel.TagTest{{Tag: "snow"}}}},
			Tex: [6]string{2: "snowy", 3: "snowy", 4: "snowy", 5: "snowy"}}}})
	reg.Register(&voxel.BlockDef{Name: "test:snow", Shape: voxel.ShapeNone, Tags: []string{"snow"}})
	snowy := reg.Texture("snowy")
	for _, snow := range []bool{false, true} {
		w := voxel.NewWorld(reg, 0, 64)
		w.AddChunk(w.NewChunk(voxel.ChunkPos{}))
		w.SetBlock(1, 1, 1, reg.State("test:stone", nil))
		if snow {
			w.SetBlock(1, 2, 1, reg.State("test:snow", nil))
		}
		m := NewMesher()
		m.Snap.Capture(w, voxel.SectionPos{})
		res := m.Build(reg.Table(), nil)
		n := 0
		for i := 0; i+VertexWords <= len(res.Data); i += VertexWords {
			if uint16(res.Data[i+1]&4095) == snowy {
				n++
			}
		}
		if want := 16 * b2i(snow); n != want {
			t.Errorf("snow %v: %d snowy vertices, want %d", snow, n, want)
		}
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// A bottom slab's top face is lit from the slab's own layer: a stone block
// beside it darkens the corners on that side; a Flat box stays unshaded.
func TestBoxSmoothLight(t *testing.T) {
	for _, flat := range []bool{false, true} {
		reg := voxel.NewRegistry()
		reg.Register(&voxel.BlockDef{Name: "test:slab", Shape: voxel.ShapeBoxes, Layer: voxel.LayerSolid,
			Textures: [6]string{"a", "a", "a", "a", "a", "a"}, Boxes: []voxel.Box{{Max: [3]float32{16, 8, 16}, Flat: flat}}})
		m := NewMesher()
		i := idx(4, 4, 4)
		m.Snap.States[i] = reg.State("test:slab", nil)
		m.Snap.States[i+nbOff[voxel.FaceEast]] = reg.Parse("stone")
		m.table = reg.Table()
		m.boxes(4, 4, 4, i, &m.table[m.Snap.States[i]])
		d := m.out[voxel.LayerSolid][voxel.FaceUp]
		if len(d) != 4*VertexWords {
			t.Fatalf("%d words on top", len(d))
		}
		for k := 0; k < 4; k++ {
			v := d[k*VertexWords:]
			east := pos32(v)[0] == 5*32
			if ao := v[0] >> 30; (ao < 3) != (east && !flat) {
				t.Errorf("flat %v: corner %v ao %d", flat, pos32(v), ao)
			}
		}
	}
}

// A NoShade box ("shade": false) is drawn with the unshaded normal entry.
func TestBoxNoShade(t *testing.T) {
	for _, no := range []bool{false, true} {
		reg := voxel.NewRegistry()
		reg.Register(&voxel.BlockDef{Name: "test:pane", Shape: voxel.ShapeBoxes, Layer: voxel.LayerSolid,
			Textures: [6]string{"a", "a", "a", "a", "a", "a"}, Boxes: []voxel.Box{{Min: [3]float32{0, 0, 8}, Max: [3]float32{16, 16, 8}, NoShade: no}}})
		m := NewMesher()
		i := idx(4, 4, 4)
		m.Snap.States[i] = reg.State("test:pane", nil)
		m.table = reg.Table()
		m.boxes(4, 4, 4, i, &m.table[m.Snap.States[i]])
		d := m.out[voxel.LayerSolid][voxel.FaceNorth]
		if len(d) == 0 {
			t.Fatal("no north face")
		}
		want := uint32(voxel.FaceNorth)
		if no {
			want = 6
		}
		if n := d[1] >> 22 & 7; n != want {
			t.Errorf("NoShade %v: normal %d, want %d", no, n, want)
		}
	}
}

// TestScatterBoxes: the first fallen leaf at 10,64,-3 (LOTR's
// renderFallenLeaves), from a Python transcription of java.util.Random.
func TestScatterBoxes(t *testing.T) {
	bs := scatterBoxes(&voxel.Scatter{Leaves: [2]int32{6, 10}, XSize: [2]int32{2, 6}, ZSize: [2]int32{2, 6}}, 10, 64, -3)
	if len(bs) != 7 {
		t.Fatalf("%d leaves, want 7", len(bs))
	}
	b := bs[0]
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }
	if !near(b.Min[0], 4.224-2) || !near(b.Max[2], 3.6934+2) || !near(b.Min[1], 0.16) ||
		b.UV[voxel.FaceUp] != [4]float32{10, 12, 14, 16} || !near(b.Rot[0].Deg, 266.4302) {
		t.Errorf("first leaf: %+v %v %+v", b, b.UV[voxel.FaceUp], b.Rot)
	}
}
