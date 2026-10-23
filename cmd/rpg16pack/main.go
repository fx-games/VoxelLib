// Command rpg16pack converts the RPG16 Minetest texture pack (CC BY-SA 4.0,
// Hugues Alexandre Ross) into a vanilla resource pack layout, so the
// demo loads it through exactly the same code path as a vanilla pack.
//
//	go run ./cmd/rpg16pack -src ~/Downloads/rpg16 -out assets/rpg16pack
package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/memmaker/voxellib/resloc"
)

// vanilla texture name -> RPG16 file(s). Several files are composited in
// order (ores = stone + mineral overlay).
var mapping = map[string][]string{
	"stone": {"default/default_stone"}, "cobblestone": {"default/default_cobble"},
	"mossy_cobblestone": {"default/default_mossycobble"}, "dirt": {"default/default_dirt"},
	"coarse_dirt": {"default/default_dry_dirt"}, "grass_block_top": {"default/default_grass"},
	"grass_block_side": {"default/default_dirt", "default/default_grass_side"}, "grass_block_snow": {"default/default_dirt", "default/default_snow_side"},
	"snow": {"default/default_snow"}, "sand": {"default/default_sand"}, "red_sand": {"default/default_desert_sand"},
	"gravel": {"default/default_gravel"}, "clay": {"default/default_clay"}, "obsidian": {"default/default_obsidian"},
	"bricks": {"default/default_brick"}, "stone_bricks": {"default/default_stone_brick"},
	"sandstone": {"default/default_sandstone"}, "sandstone_top": {"default/default_sandstone"},
	"sandstone_bottom": {"default/default_sandstone"}, "ice": {"default/default_ice"},
	"packed_ice": {"default/default_ice"}, "glass": {"default/default_glass"},
	"water_still": {"default/default_water"}, "lava_still": {"default/default_lava"},
	"bookshelf": {"default/default_bookshelf"}, "snow_block": {"default/default_snow"},
	"cactus_side": {"default/default_cactus_side"}, "cactus_top": {"default/default_cactus_top"},
	"cactus_bottom": {"default/default_cactus_top"},
	"furnace_front": {"default/default_furnace_front"}, "furnace_side": {"default/default_furnace_side"},
	"furnace_top": {"default/default_furnace_top"},
	"coal_block":  {"default/default_coal_block"}, "iron_block": {"default/default_steel_block"},
	"gold_block": {"default/default_gold_block"}, "diamond_block": {"default/default_diamond_block"},
	"oak_log": {"default/default_tree"}, "oak_log_top": {"default/default_tree_top"},
	"oak_planks": {"default/default_wood"}, "oak_leaves": {"default/default_leaves"},
	"birch_log": {"default/default_aspen_tree"}, "birch_log_top": {"default/default_aspen_tree_top"},
	"birch_planks": {"default/default_aspen_wood"}, "birch_leaves": {"default/default_aspen_leaves"},
	"spruce_log": {"default/default_pine_tree"}, "spruce_log_top": {"default/default_pine_tree_top"},
	"spruce_planks": {"default/default_pine_wood"}, "spruce_leaves": {"default/default_pine_needles"},
	"jungle_log": {"default/default_jungletree"}, "jungle_log_top": {"default/default_jungletree_top"},
	"jungle_planks": {"default/default_junglewood"}, "jungle_leaves": {"default/default_jungleleaves"},
	"acacia_log": {"default/default_acacia_tree"}, "acacia_log_top": {"default/default_acacia_tree_top"},
	"acacia_planks": {"default/default_acacia_wood"}, "acacia_leaves": {"default/default_acacia_leaves"},
	"dark_oak_log": {"moretrees/moretrees_oak_trunk"}, "dark_oak_log_top": {"moretrees/moretrees_oak_trunk_top"},
	"dark_oak_planks": {"moretrees/moretrees_oak_wood"}, "dark_oak_leaves": {"moretrees/moretrees_oak_leaves"},
	"deepslate": {"darks/darkblock"}, "deepslate_top": {"darks/darkblock_top"},
	"bedrock": {"default/default_obsidian_block"}, "tuff": {"default/default_desert_stone"},
	"granite": {"default/default_desert_stone"}, "diorite": {"default/default_silver_sandstone"},
	"andesite": {"default/default_stone_block"}, "calcite": {"default/default_silver_sandstone_block"},
	"terracotta":            {"baked_clay/baked_clay_orange"},
	"coal_ore":              {"default/default_stone", "default/default_mineral_coal"},
	"iron_ore":              {"default/default_stone", "default/default_mineral_iron"},
	"gold_ore":              {"default/default_stone", "default/default_mineral_gold"},
	"diamond_ore":           {"default/default_stone", "default/default_mineral_diamond"},
	"copper_ore":            {"default/default_stone", "default/default_mineral_copper"},
	"redstone_ore":          {"default/default_stone", "default/default_mineral_mese"},
	"lapis_ore":             {"default/default_stone", "default/default_mineral_tin"},
	"emerald_ore":           {"default/default_stone", "default/default_mineral_sulfur"},
	"deepslate_coal_ore":    {"darks/darkblock", "default/default_mineral_coal"},
	"deepslate_iron_ore":    {"darks/darkblock", "default/default_mineral_iron"},
	"deepslate_gold_ore":    {"darks/darkblock", "default/default_mineral_gold"},
	"deepslate_diamond_ore": {"darks/darkblock", "default/default_mineral_diamond"},
	"glowstone":             {"caverealms/caverealms_glow_crystal"},
	"sea_lantern":           {"default/default_meselamp"},
	"short_grass":           {"default/default_grass_3"},
	"fern":                  {"default/default_fern_2"},
	"dead_bush":             {"default/default_dry_shrub"},
	"dandelion":             {"flowers/flowers_dandelion_yellow"},
	"poppy":                 {"flowers/flowers_rose"},
	"blue_orchid":           {"flowers/flowers_geranium"},
	"allium":                {"flowers/flowers_viola"},
	"oxeye_daisy":           {"flowers/flowers_dandelion_white"},
	"cornflower":            {"flowers/flowers_geranium"},
	"red_tulip":             {"flowers/flowers_tulip"},
	"brown_mushroom":        {"flowers/flowers_mushroom_brown"},
	"red_mushroom":          {"flowers/flowers_mushroom_red"},
	"oak_sapling":           {"default/default_sapling"},
	"sugar_cane":            {"default/default_papyrus"},
	"crafting_table_top":    {"default/default_wood"},
	"crafting_table_side":   {"default/default_wood"},
	"crafting_table_front":  {"default/default_wood"},
	"tnt_side":              {"tnt/tnt_side"},
	"tnt_top":               {"tnt/tnt_top"},
	"tnt_bottom":            {"tnt/tnt_bottom"},
	"white_wool":            {"wool/wool_white"}, "orange_wool": {"wool/wool_orange"},
	"magenta_wool": {"wool/wool_magenta"}, "light_blue_wool": {"wool/wool_cyan"},
	"yellow_wool": {"wool/wool_yellow"}, "lime_wool": {"wool/wool_green"}, "pink_wool": {"wool/wool_pink"},
	"gray_wool": {"wool/wool_dark_grey"}, "light_gray_wool": {"wool/wool_grey"}, "cyan_wool": {"wool/wool_cyan"},
	"purple_wool": {"wool/wool_violet"}, "blue_wool": {"wool/wool_blue"}, "brown_wool": {"wool/wool_brown"},
	"green_wool": {"wool/wool_dark_green"}, "red_wool": {"wool/wool_red"}, "black_wool": {"wool/wool_black"},
}

// Animated textures: vertical frame strips copied whole, with a .png.mcmeta.
// alpha > 0 replaces the alpha of every visible pixel (see-through water).
var animated = map[string]struct {
	file      string
	frametime int
	alpha     uint8
}{
	"water_still": {"default/default_water_source_animated", 2, 150},
	"water_flow":  {"default/default_water_flowing_animated", 2, 150},
	"lava_still":  {"default/default_lava_source_animated", 4, 0},
	"lava_flow":   {"default/default_lava_flowing_animated", 3, 0},
}

func writeAnimated(src, dst string) int {
	n := 0
	for name, a := range animated {
		img, err := load(src, a.file)
		if err != nil {
			log.Printf("skip %s: %v", name, err)
			continue
		}
		b := img.Bounds()
		strip := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(strip, strip.Bounds(), img, b.Min, draw.Src)
		if a.alpha > 0 {
			for i := 3; i < len(strip.Pix); i += 4 {
				if strip.Pix[i] > 0 {
					strip.Pix[i] = a.alpha
				}
			}
		}
		f, err := os.Create(filepath.Join(dst, name+".png"))
		if err != nil {
			log.Fatal(err)
		}
		png.Encode(f, strip)
		f.Close()
		meta := fmt.Sprintf(`{"animation":{"frametime":%d}}`+"\n", a.frametime)
		os.WriteFile(filepath.Join(dst, name+".png.mcmeta"), []byte(meta), 0o644)
		n++
	}
	return n
}

func load(src, rel string) (image.Image, error) {
	f, err := os.Open(filepath.Join(src, rel+".png"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func main() {
	src := flag.String("src", filepath.Join(os.Getenv("HOME"), "Downloads/rpg16"), "RPG16 directory")
	out := flag.String("out", "assets/rpg16pack", "output resource pack directory")
	flag.Parse()

	dst := filepath.Join(*out, "assets/"+resloc.Default+"/textures/block")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		log.Fatal(err)
	}
	n := writeAnimated(*src, dst)
	for name, files := range mapping {
		if _, ok := animated[name]; ok {
			continue
		}
		var canvas *image.NRGBA
		for _, rel := range files {
			img, err := load(*src, rel)
			if err != nil {
				log.Printf("skip %s: %v", name, err)
				canvas = nil
				break
			}
			b := img.Bounds()
			w := b.Dx()
			if canvas == nil {
				canvas = image.NewNRGBA(image.Rect(0, 0, w, w))
				draw.Draw(canvas, canvas.Bounds(), img, b.Min, draw.Src)
			} else {
				draw.Draw(canvas, canvas.Bounds(), img, b.Min, draw.Over)
			}
		}
		if canvas == nil {
			continue
		}
		f, err := os.Create(filepath.Join(dst, name+".png"))
		if err != nil {
			log.Fatal(err)
		}
		png.Encode(f, canvas)
		f.Close()
		n++
	}
	meta := `{"pack":{"pack_format":34,"description":"RPG16 by Hugues Alexandre Ross (CC BY-SA 4.0), remapped for voxellib"}}` + "\n"
	os.WriteFile(filepath.Join(*out, "pack.mcmeta"), []byte(meta), 0o644)
	if lic, err := os.ReadFile(filepath.Join(*src, "LICENSE.txt")); err == nil {
		os.WriteFile(filepath.Join(*out, "LICENSE.txt"), lic, 0o644)
	}
	credit := "Textures: RPG16 by Hugues Alexandre Ross - https://huguesross.net\n" +
		"License: CC BY-SA 4.0 (see LICENSE.txt). Remapped to vanilla texture names;\n" +
		"ore textures are composites of the RPG16 stone and mineral overlays.\n"
	os.WriteFile(filepath.Join(*out, "CREDITS.txt"), []byte(credit), 0o644)
	fmt.Printf("wrote %d textures to %s\n", n, dst)
}
