package respack

import (
	"math"
	"os"
	"testing"
	"testing/fstest"

	"github.com/memmaker/voxellib/resloc"
	"github.com/memmaker/voxellib/voxel"
)

const ns = "assets/" + resloc.Default

// stateTestPack has a stair (straight and outer_left models, turned by y),
// a slab and a multipart post with arms.
func stateTestPack() *Pack {
	return FromFS(fstest.MapFS{
		ns + "/blockstates/rock_stairs.json": {Data: []byte(`{"variants": {
			"facing=east,shape=straight": {"model": "block/rock_stairs"},
			"facing=south,shape=straight": {"model": "block/rock_stairs", "y": 90, "uvlock": true},
			"facing=east,shape=outer_left": {"model": "block/rock_stairs_outer", "y": 270, "uvlock": true},
			"facing=south,shape=outer_left": {"model": "block/rock_stairs_outer"}}}`)},
		ns + "/models/block/rock_stairs.json": {Data: []byte(`{"textures": {"t": "block/rock", "particle": "block/rock"},
			"elements": [
				{"from": [0,0,0], "to": [16,8,16], "faces": {"up": {"texture": "#t"}, "down": {"texture": "#t", "cullface": "down"}}},
				{"from": [8,8,0], "to": [16,16,16], "faces": {"up": {"texture": "#t"}, "east": {"texture": "#t", "cullface": "east"}}}]}`)},
		ns + "/models/block/rock_stairs_outer.json": {Data: []byte(`{"textures": {"t": "block/rock"},
			"elements": [
				{"from": [0,0,0], "to": [16,8,16], "faces": {"up": {"texture": "#t"}}},
				{"from": [8,8,8], "to": [16,16,16], "faces": {"up": {"texture": "#t"}}}]}`)},
		ns + "/blockstates/rock_slab.json": {Data: []byte(`{"variants": {"type=bottom": {"model": "block/rock_slab"}}}`)},
		ns + "/models/block/rock_slab.json": {Data: []byte(`{"textures": {"t": "block/rock"},
			"elements": [{"from": [0,0,0], "to": [16,8,16], "faces": {"up": {"texture": "#t"}}}]}`)},
		ns + "/blockstates/post.json": {Data: []byte(`{"multipart": [
			{"apply": {"model": "block/post"}},
			{"when": {"north": "true"}, "apply": {"model": "block/arm", "uvlock": true}},
			{"when": {"OR": [{"east": "true"}, {"west": "low|tall"}]}, "apply": {"model": "block/arm", "y": 90, "uvlock": true}}]}`)},
		ns + "/models/block/post.json": {Data: []byte(`{"textures": {"t": "block/rock"},
			"elements": [{"from": [6,0,6], "to": [10,16,10], "faces": {"up": {"texture": "#t"}}}]}`)},
		ns + "/models/block/arm.json": {Data: []byte(`{"textures": {"t": "block/rock"},
			"elements": [{"from": [7,6,0], "to": [9,9,6], "faces": {"up": {"texture": "#t"}}}]}`)},
	})
}

// TestResolveBoxes: a non-cube model resolves to its elements, turned by
// the variant's y; a stair carries every shape for its neighbours.
func TestResolveBoxes(t *testing.T) {
	p := stateTestPack()
	m, ok := p.Resolver()(resloc.Default+":rock_slab", map[string]string{"type": "bottom"})
	if !ok || m.Shape != voxel.ShapeBoxes || len(m.Boxes) != 1 || m.Boxes[0].Max[1] != 8 || m.Tex[voxel.FaceUp] != "block/rock" {
		t.Fatalf("slab: %+v %v", m, ok)
	}
	east, _ := p.Resolver()(resloc.Default+":rock_stairs", map[string]string{"facing": "east", "shape": "straight"})
	south, _ := p.Resolver()(resloc.Default+":rock_stairs", map[string]string{"facing": "south", "shape": "straight"})
	if len(east.Boxes) != 2 || len(south.Boxes) != 2 {
		t.Fatalf("stairs: %d and %d boxes", len(east.Boxes), len(south.Boxes))
	}
	// y=90 with uvlock turns the step from the east half to the south half.
	if lo, hi := turnedBounds(south.Boxes[1]); lo != [3]float32{0, 8, 8} || hi != [3]float32{16, 16, 16} {
		t.Errorf("south step %v..%v, want the south half", lo, hi)
	}

	reg := voxel.NewRegistry()
	reg.Resolver = p.Resolver()
	info := reg.Info(reg.State(resloc.Default+":rock_stairs", map[string]string{"facing": "east", "shape": "outer_left"}))
	if info.Shape != voxel.ShapeBoxes || !info.Step || len(info.Variants[""]) != 2 || len(info.Variants["outer_left"]) != 2 {
		t.Fatalf("stair state: shape %v step %v variants %v", info.Shape, info.Step, info.Variants)
	}
	if got := info.Variants[""][1].Min; got != [3]float32{8, 8, 0} {
		t.Errorf(`stair Variants[""] is not the straight shape: step at %v`, got)
	}
}

// TestResolveMultipart: every part whose "when" holds (plain, "a|b"
// alternatives, OR) adds its elements.
func TestResolveMultipart(t *testing.T) {
	r := stateTestPack().Resolver()
	for _, c := range []struct {
		props map[string]string
		boxes int
	}{
		{map[string]string{}, 1},
		{map[string]string{"north": "true"}, 2},
		{map[string]string{"north": "true", "east": "true"}, 3},
		{map[string]string{"west": "tall"}, 2},
		{map[string]string{"west": "none"}, 1},
	} {
		m, ok := r(resloc.Default+":post", c.props)
		if !ok || m.Shape != voxel.ShapeBoxes || len(m.Boxes) != c.boxes {
			t.Errorf("%v: ok %v, %d boxes, want %d", c.props, ok, len(m.Boxes), c.boxes)
		}
	}
}

// TestResolveJar: blocks VoxelLib has no definition of draw as their jar
// models (needs $VOXELLIB_JAR).
func TestResolveJar(t *testing.T) {
	jar := os.Getenv("VOXELLIB_JAR")
	if jar == "" {
		t.Skip("set VOXELLIB_JAR to a Java Edition client jar")
	}
	p, err := Open(jar)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	reg := voxel.NewRegistry()
	reg.Resolver = p.Resolver()
	for _, s := range []string{
		"stone_stairs[facing=south,half=top,shape=outer_left]", "andesite_slab[type=bottom]",
		"pitcher_plant[half=lower]", "potted_red_tulip",
		"oak_fence[north=true,east=false,south=false,west=true]", "cobblestone_wall[north=low,up=true]",
	} {
		info := reg.Info(reg.Parse(s))
		if info.Def.Unknown && info.Shape == voxel.ShapeCube {
			t.Errorf("%s: still the unknown placeholder cube", s)
			continue
		}
		entries, _ := reg.Textures()
		for _, b := range info.Boxes {
			for f, id := range b.TexID {
				if b.Skip&(1<<f) == 0 && b.Tex[f] != "" {
					if _, err := p.Texture(entries[id].Name); err != nil {
						t.Errorf("%s: face texture %s: %v", s, entries[id].Name, err)
					}
				}
			}
		}
		if info.Shape == voxel.ShapeBoxes && len(info.Boxes) == 0 {
			t.Errorf("%s: no boxes", s)
		}
	}
}

// turnedBounds is a box's bounds after its rotations, rounded to 1/1000.
func turnedBounds(b voxel.Box) (lo, hi [3]float32) {
	lo, hi = [3]float32{1e9, 1e9, 1e9}, [3]float32{-1e9, -1e9, -1e9}
	for c := 0; c < 8; c++ {
		p := b.Min
		for a := 0; a < 3; a++ {
			if c>>a&1 == 1 {
				p[a] = b.Max[a]
			}
		}
		p = voxel.RotatePoint(p, b.Rot)
		for a := 0; a < 3; a++ {
			v := float32(math.Round(float64(p[a])*1000) / 1000)
			lo[a], hi[a] = min(lo[a], v), max(hi[a], v)
		}
	}
	return lo, hi
}
