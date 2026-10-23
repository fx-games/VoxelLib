// Package maptest writes a tiny VoxelMap for tests: one dimension (id 0,
// generator "test:flat") holding the single legacy chunk 0,0.
package maptest

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"github.com/memmaker/voxellib/resloc"
	"github.com/memmaker/voxellib/voxel/anvil"
)

// What the fixture's chunk 0,0 holds (dimension 0):
//
//	y 0..3   stone (id 1)
//	1,4,1    "test:lamp#2" (id 300: needs the Add nibble), emits light
//	2,4,2    "test:plant#0" (id 6), a cross
//	3,4,3    "test:post#0" (id 301), model block/post turned y=90
//	4,4,4    a sign (id 63) whose first line is "Hello"
//
// Its biome bytes are all 7 and its Entities hold one {"id": "Pig"}.
const (
	Seed     = 1234
	SignText = "Hello"
	Biome    = 7
)

// Write creates the fixture in dir.
func Write(dir string) error {
	files := map[string]string{
		"map.json": `{"name": "test", "format": 1, "seed": 1234, "spawn": {"dim": 0, "x": 8, "y": 5, "z": 8},
			"dimensions": [{"id": 0, "name": "Overworld", "regions": "regions/0", "generator": "test:flat"}]}`,
		"ids.json": `{"0": "` + resloc.Default + `:air", "1": "` + resloc.Default + `:stone", "6": "test:plant", "63": "` + resloc.Default + `:standing_sign",
			"300": "test:lamp", "301": "test:post"}`,
		"blocks.json": `[
			{"key": "` + resloc.Default + `:stone#*", "textures": "blocks/stone"},
			{"key": "test:plant#0", "shape": "cross", "textures": "test:blocks/plant", "tint": "grass"},
			{"key": "test:lamp#*", "textures": {"all": "blocks/stone", "top": "test:blocks/plant"}, "emission": 15},
			{"key": "test:post#0", "shape": "model:test:block/post", "y": 90, "textures": {"wood": "blocks/stone"}, "sounds": "wood"},
			{"key": "` + resloc.Default + `:standing_sign#*", "shape": "none", "collision": "none", "tint": "#ff8000"}
		]`,
		"models/test/block/post.json": `{"textures": {"particle": "#wood"},
			"elements": [{"from": [6, 0, 6], "to": [10, 16, 10],
				"faces": {"up": {"texture": "#wood", "uv": [6, 6, 10, 10]}, "north": {"texture": "#wood"}}}]}`,
	}
	for name, body := range files {
		if err := writeFile(filepath.Join(dir, name), []byte(body)); err != nil {
			return err
		}
	}
	for _, t := range []string{"textures/blocks/stone.png", "textures/test/blocks/plant.png"} {
		if err := writePNG(filepath.Join(dir, t)); err != nil {
			return err
		}
	}
	return writeRegion(filepath.Join(dir, "regions", "0"))
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writePNG(path string) error {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for i := 0; i < 256; i++ {
		img.Set(i%16, i/16, color.NRGBA{uint8(i), 128, 64, 255})
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func writeRegion(dir string) error {
	blocks, add, data := make([]byte, 4096), make([]byte, 2048), make([]byte, 2048)
	set := func(x, y, z, id, meta int) {
		i := y<<8 | z<<4 | x
		blocks[i] = byte(id)
		add[i/2] |= byte(id>>8) << (4 * (i % 2))
		data[i/2] |= byte(meta) << (4 * (i % 2))
	}
	for y := 0; y < 4; y++ {
		for z := 0; z < 16; z++ {
			for x := 0; x < 16; x++ {
				set(x, y, z, 1, 0)
			}
		}
	}
	set(1, 4, 1, 300, 2)
	set(2, 4, 2, 6, 0)
	set(3, 4, 3, 301, 0)
	set(4, 4, 4, 63, 0)
	biomes := make([]byte, 256)
	for i := range biomes {
		biomes[i] = Biome
	}
	level := anvil.Compound{
		"xPos": int32(0), "zPos": int32(0),
		"Biomes": biomes,
		"Sections": anvil.List{Type: anvil.TagCompound, Items: []any{anvil.Compound{
			"Y": int8(0), "Blocks": blocks, "Add": add, "Data": data,
			"SkyLight": make([]byte, 2048), "BlockLight": make([]byte, 2048),
		}}},
		"Entities": anvil.List{Type: anvil.TagCompound, Items: []any{anvil.Compound{"id": "Pig"}}},
		"TileEntities": anvil.List{Type: anvil.TagCompound, Items: []any{anvil.Compound{
			"id": "Sign", "x": int32(4), "y": int32(4), "z": int32(4),
			"Text1": SignText, "Text2": "", "Text3": "", "Text4": "",
		}}},
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	r, err := anvil.OpenRegion(filepath.Join(dir, "r.0.0.mca"), true)
	if err != nil {
		return err
	}
	defer r.Close()
	return r.WriteChunk(0, 0, anvil.Compound{"Level": level})
}
