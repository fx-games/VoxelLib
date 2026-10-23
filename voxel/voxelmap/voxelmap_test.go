package voxelmap_test

import (
	"errors"
	"testing"

	"github.com/memmaker/voxellib/resloc"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/anvil"
	"github.com/memmaker/voxellib/voxel/voxelmap"
	"github.com/memmaker/voxellib/voxel/voxelmap/maptest"
)

func TestReadFixture(t *testing.T) {
	dir := t.TempDir()
	if err := maptest.Write(dir); err != nil {
		t.Fatal(err)
	}
	reg := voxel.NewRegistry()
	m, err := voxelmap.Open(dir, reg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.Seed != maptest.Seed || m.Dimension(0).Generator != "test:flat" || m.Spawn.Y != 5 {
		t.Fatalf("map.json: %+v", m)
	}
	w := voxel.NewWorld(reg, voxelmap.MinY, voxelmap.Height)
	c, level, err := m.ReadChunk(w, 0, voxel.ChunkPos{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		x, y, z int
		key     string
	}{{0, 0, 0, resloc.Default + ":stone#0"}, {1, 4, 1, "test:lamp#2"}, {2, 4, 2, "test:plant#0"}, {3, 4, 3, "test:post#0"}, {0, 4, 0, ""}} {
		id := c.Get(tc.x, tc.y, tc.z)
		if tc.key == "" {
			if id != voxel.Air {
				t.Errorf("%d,%d,%d: want air, got %s", tc.x, tc.y, tc.z, m.Key(id))
			}
			continue
		}
		if got := m.Key(id); got != tc.key || id != m.State(tc.key) {
			t.Errorf("%d,%d,%d: got %s, want %s", tc.x, tc.y, tc.z, got, tc.key)
		}
	}
	lamp := reg.Info(m.State("test:lamp#2"))
	if lamp.Emission != 15 || lamp.Shape != voxel.ShapeCube || !lamp.Opaque {
		t.Errorf("lamp: %+v", lamp)
	}
	if reg.Info(m.State("test:lamp#2")).Tex[voxel.FaceUp] == lamp.Tex[voxel.FaceDown] {
		t.Error("lamp: top texture not applied")
	}
	plant := reg.Info(m.State("test:plant#0"))
	if plant.Shape != voxel.ShapeCross || plant.Tint != voxel.TintGrass || plant.Collides {
		t.Errorf("plant: %+v", plant)
	}
	post := reg.Info(m.State("test:post#0"))
	if post.Shape != voxel.ShapeBoxes || len(post.Boxes) != 1 || len(post.Boxes[0].Rot) != 1 || post.Boxes[0].Tex[voxel.FaceUp] != "blocks/stone" {
		t.Errorf("post: %+v", post)
	}
	sign := reg.Info(m.State(resloc.Default + ":standing_sign#0"))
	if sign.Collision == nil || len(sign.Collision) != 0 || sign.Tint != voxel.TintFixed || sign.TintColor != 0xff8000 {
		t.Errorf("sign: %+v", sign)
	}
	if m.Blocks["test:post#0"] == nil || len(m.Blocks["test:post#0"].Raw) == 0 {
		t.Error("raw blocks.json entry missing")
	}
	if m.State(voxelmap.Air) != voxel.Air {
		t.Error("air key")
	}
	if c.Signs[voxel.BlockPos{X: 4, Y: 4, Z: 4}][0] != maptest.SignText {
		t.Errorf("signs: %v", c.Signs)
	}
	if len(c.Biomes) != 256 || c.Biomes[17] != maptest.Biome {
		t.Errorf("biomes: %v", c.Biomes)
	}
	if ents := level.List("Entities").Items; len(ents) != 1 || ents[0].(anvil.Compound).String("id") != "Pig" {
		t.Errorf("entities: %v", ents)
	}
	if _, _, err := m.ReadChunk(w, 0, voxel.ChunkPos{X: 1}); !errors.Is(err, anvil.ErrNoChunk) {
		t.Errorf("missing chunk: %v", err)
	}
	if img, err := m.Texture("test:blocks/plant"); err != nil || img.Bounds().Dx() != 16 {
		t.Errorf("texture: %v", err)
	}
}
