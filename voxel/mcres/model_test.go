package mcres

import (
	"testing"
	"testing/fstest"

	"github.com/memmaker/voxellib/voxel"
)

// TestModelBoxesFaceDetails: face rotation, cullface, tintindex and the
// element's rescale reach the boxes.
func TestModelBoxesFaceDetails(t *testing.T) {
	p := FromFS(fstest.MapFS{"assets/minecraft/models/block/t.json": {Data: []byte(`{
		"textures": {"all": "block/x"},
		"elements": [{"from": [0,0,0], "to": [16,16,16],
			"rotation": {"origin": [8,8,8], "axis": "y", "angle": 45, "rescale": true},
			"faces": {
				"up":    {"texture": "#all", "cullface": "up", "tintindex": 0, "rotation": 270},
				"north": {"texture": "#all"},
				"down":  {"texture": "#all", "cullface": "bottom", "tintindex": -1}
			}}]}`)}})
	bs, err := p.ModelBoxes("block/t", nil)
	if err != nil || len(bs) != 1 {
		t.Fatal(bs, err)
	}
	b := bs[0]
	if b.UV == nil || b.UVRot[voxel.FaceUp] != 3 || b.UVRot[voxel.FaceNorth] != 0 {
		t.Errorf("uv rotation %v (uv %v)", b.UVRot, b.UV)
	}
	if b.Cull == nil || b.Cull[voxel.FaceUp] != voxel.FaceUp || b.Cull[voxel.FaceDown] != voxel.FaceDown || b.Cull[voxel.FaceNorth] != voxel.FaceCount {
		t.Errorf("cull %v", b.Cull)
	}
	if b.NoTint&(1<<voxel.FaceUp) != 0 || b.NoTint&(1<<voxel.FaceNorth) == 0 || b.NoTint&(1<<voxel.FaceDown) == 0 {
		t.Errorf("untinted faces %06b", b.NoTint)
	}
	if len(b.Rot) != 1 || !b.Rot[0].Rescale || b.Rot[0].Deg != 45 {
		t.Errorf("rotation %+v", b.Rot)
	}
	// a blockstate y=90 turns the cullfaces with the model
	r := voxel.RotateBoxes(bs, 1, 90)[0]
	if r.Cull[voxel.FaceUp] != voxel.FaceUp || bs[0].Cull[voxel.FaceUp] != voxel.FaceUp {
		t.Errorf("y-turned cull %v", r.Cull)
	}
	r = voxel.RotateBoxes(bs, 0, 90)[0]
	if r.Cull[voxel.FaceUp] == voxel.FaceUp || r.Cull[voxel.FaceUp] >= voxel.FaceCount || bs[0].Cull[voxel.FaceUp] != voxel.FaceUp {
		t.Errorf("x-turned cull %v, original %v", r.Cull, bs[0].Cull)
	}
}

// TestFaceResolver: a cube's per-face cullfaces and tintindexes reach the
// registry, turned with the blockstate; a vanilla-style cube keeps the
// defaults (culled by the facing neighbour).
func TestFaceResolver(t *testing.T) {
	p := FromFS(fstest.MapFS{
		"assets/minecraft/blockstates/grass.json": {Data: []byte(`{"variants": {"": {"model": "block/grass"}}}`)},
		"assets/minecraft/blockstates/odd.json": {Data: []byte(`{"variants": {
			"facing=north": {"model": "block/odd"}, "facing=east": {"model": "block/odd", "y": 90}}}`)},
		"assets/minecraft/models/block/grass.json": {Data: []byte(`{"textures": {"t": "block/g"},
			"elements": [{"from": [0,0,0], "to": [16,16,16], "faces": {
				"down": {"texture": "#t", "cullface": "down"}, "up": {"texture": "#t", "cullface": "up", "tintindex": 0},
				"north": {"texture": "#t", "cullface": "north"}, "south": {"texture": "#t", "cullface": "south"},
				"west": {"texture": "#t", "cullface": "west"}, "east": {"texture": "#t", "cullface": "east"}}}]}`)},
		"assets/minecraft/models/block/odd.json": {Data: []byte(`{"textures": {"t": "block/o"},
			"elements": [{"from": [0,0,0], "to": [16,16,16], "faces": {
				"down": {"texture": "#t", "cullface": "down", "tintindex": 1}, "up": {"texture": "#t", "cullface": "up"},
				"north": {"texture": "#t"}, "south": {"texture": "#t", "cullface": "south"},
				"west": {"texture": "#t", "cullface": "west"}, "east": {"texture": "#t", "cullface": "east"}}}]}`)},
	})
	reg := voxel.NewRegistry()
	reg.Resolver, reg.Faces = p.Resolver(), p.FaceResolver()
	reg.Register(&voxel.BlockDef{Name: "minecraft:grass", Shape: voxel.ShapeCube, Tint: voxel.TintGrass, Opaque: true})
	reg.Register(&voxel.BlockDef{Name: "minecraft:odd", Shape: voxel.ShapeCube, Tint: voxel.TintGrass, Opaque: true})

	g := reg.Info(reg.State("minecraft:grass", nil))
	if g.Cull != nil || g.TintMask != 1<<voxel.FaceUp {
		t.Errorf("grass: cull %v, tint mask %06b; want default culling, only up tinted", g.Cull, g.TintMask)
	}
	o := reg.Info(reg.State("minecraft:odd", map[string]string{"facing": "north"}))
	if o.Cull == nil || o.Cull[voxel.FaceNorth] != voxel.FaceCount || o.Cull[voxel.FaceUp] != voxel.FaceUp || o.TintMask != 1<<voxel.FaceDown {
		t.Errorf("odd: cull %v, tint mask %06b", o.Cull, o.TintMask)
	}
	e := reg.Info(reg.State("minecraft:odd", map[string]string{"facing": "east"}))
	if e.Cull == nil || e.Cull[voxel.FaceEast] != voxel.FaceCount || e.Cull[voxel.FaceSouth] != voxel.FaceSouth ||
		e.Cull[voxel.FaceNorth] != voxel.FaceNorth || e.TintMask != 1<<voxel.FaceDown {
		t.Errorf("odd y=90: cull %v, tint mask %06b; want the cullface-less face on east", e.Cull, e.TintMask)
	}
	// rotateRules keeps a cube's own cullfaces pointing outward for any turn
	id := voxel.FaceRules{Cull: [6]voxel.Face{0, 1, 2, 3, 4, 5}}
	for _, xy := range [][2]int{{90, 0}, {0, 270}, {180, 90}} {
		if r := rotateRules(id, xy[0], xy[1]); r.Cull != id.Cull {
			t.Errorf("x=%d y=%d: %v", xy[0], xy[1], r.Cull)
		}
	}
}

// TestModelMoreRotations: more_rotations follow rotation, in order.
func TestModelMoreRotations(t *testing.T) {
	p := FromFS(fstest.MapFS{"assets/minecraft/models/block/t.json": {Data: []byte(`{
		"elements": [{"from": [0,0,0], "to": [1,1,1],
			"rotation": {"origin": [1,2,3], "axis": "x", "angle": 10},
			"more_rotations": [{"origin": [1,2,3], "axis": "z", "angle": 30}, {"origin": [1,2,3], "axis": "y", "angle": -20}],
			"faces": {"up": {"texture": "#all"}}}]}`)}})
	bs, err := p.ModelBoxes("block/t", nil)
	if err != nil || len(bs) != 1 {
		t.Fatal(bs, err)
	}
	r := bs[0].Rot
	if len(r) != 3 || r[0].Axis != 0 || r[0].Deg != 10 || r[1].Axis != 2 || r[1].Deg != 30 || r[2].Axis != 1 || r[2].Deg != -20 || r[2].Origin != [3]float32{1, 2, 3} {
		t.Errorf("rotations %+v", r)
	}
}
