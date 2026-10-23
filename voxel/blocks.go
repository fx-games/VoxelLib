package voxel

// Built-in block definitions using Minecraft ids and Minecraft texture names, so
// a vanilla resource pack (or anything laid out like one) skins them directly.

func all6(t string) [6]string { return [6]string{t, t, t, t, t, t} }

// column: end texture on top/bottom, side texture around (logs, pillars).
func column(end, side string) [6]string { return [6]string{end, end, side, side, side, side} }

func bottomTop(bottom, top, side string) [6]string {
	return [6]string{bottom, top, side, side, side, side}
}

func cube(name string, tex [6]string) *BlockDef {
	return &BlockDef{Name: name, Shape: ShapeCube, Textures: tex, Opaque: true, Collides: true, LightOpacity: 15}
}

func simple(name string) *BlockDef { return cube(name, all6("block/"+name)) }

func cross(name, tex string, tint Tint) *BlockDef {
	return &BlockDef{Name: name, Shape: ShapeCross, Layer: LayerCutout, Textures: all6(tex), Tint: tint, Replaceable: true}
}

// boxes: a ShapeBoxes block (see shapes.go); collision comes from its boxes.
func boxes(name string, tex [6]string) *BlockDef {
	return &BlockDef{Name: name, Shape: ShapeBoxes, Layer: LayerCutout, Textures: tex}
}

func leaves(name string, tint Tint) *BlockDef {
	return &BlockDef{Name: name, Shape: ShapeCube, Layer: LayerCutout, Textures: all6("block/" + name),
		Tint: tint, Collides: true, LightOpacity: 1}
}

func builtinBlocks() []*BlockDef {
	defs := []*BlockDef{
		simple("stone"), simple("cobblestone"), simple("dirt"), simple("sand"), simple("gravel"),
		simple("bedrock"), simple("clay"), simple("obsidian"), simple("bricks"), simple("stone_bricks"),
		simple("mossy_cobblestone"), simple("coarse_dirt"), simple("red_sand"), simple("granite"),
		simple("diorite"), simple("andesite"), simple("tuff"), simple("calcite"), simple("snow_block"),
		simple("coal_ore"), simple("iron_ore"), simple("gold_ore"), simple("diamond_ore"),
		simple("copper_ore"), simple("redstone_ore"), simple("lapis_ore"), simple("emerald_ore"),
		simple("deepslate_coal_ore"), simple("deepslate_iron_ore"), simple("deepslate_gold_ore"),
		simple("deepslate_diamond_ore"), simple("coal_block"), simple("iron_block"), simple("gold_block"),
		simple("diamond_block"), simple("terracotta"), simple("packed_ice"),
		simple("oak_planks"), simple("spruce_planks"), simple("birch_planks"), simple("jungle_planks"),
		simple("acacia_planks"), simple("dark_oak_planks"),
		cube("deepslate", column("block/deepslate_top", "block/deepslate")),
		cube("sandstone", bottomTop("block/sandstone_bottom", "block/sandstone_top", "block/sandstone")),
		cube("bookshelf", column("block/oak_planks", "block/bookshelf")),
		cube("crafting_table", [6]string{"block/oak_planks", "block/crafting_table_top", "block/crafting_table_front",
			"block/crafting_table_side", "block/crafting_table_side", "block/crafting_table_front"}),
		cube("furnace", [6]string{"block/furnace_top", "block/furnace_top", "block/furnace_front",
			"block/furnace_side", "block/furnace_side", "block/furnace_side"}),
		cube("podzol", bottomTop("block/dirt", "block/podzol_top", "block/podzol_side")),
		cube("mycelium", bottomTop("block/dirt", "block/mycelium_top", "block/mycelium_side")),
		cube("cactus", bottomTop("block/cactus_bottom", "block/cactus_top", "block/cactus_side")),
		cube("pumpkin", column("block/pumpkin_top", "block/pumpkin_side")),
		cube("melon", column("block/melon_top", "block/melon_side")),
		cube("tnt", bottomTop("block/tnt_bottom", "block/tnt_top", "block/tnt_side")),
		simple("netherrack"),
		cube("smooth_stone", column("block/smooth_stone", "block/smooth_stone_slab_side")),
		cube("jukebox", column("block/jukebox_top", "block/jukebox_side")),
		boxes("smooth_stone_slab", column("block/smooth_stone", "block/smooth_stone_slab_side")),
		boxes("oak_stairs", all6("block/oak_planks")), boxes("cobblestone_stairs", all6("block/cobblestone")),
		boxes("oak_door", all6("block/oak_door_bottom")), boxes("ladder", all6("block/ladder")),
		boxes("snow", all6("block/snow")), boxes("lever", all6("block/cobblestone")),
		boxes("farmland", bottomTop("block/dirt", "block/farmland", "block/dirt")),
		boxes("stone_pressure_plate", all6("block/stone")), boxes("redstone_wire", all6("block/redstone_dust_dot")),
		// ponytail: chests and signs are entity-rendered in vanilla; planks until we cut their atlases.
		boxes("chest", all6("block/oak_planks")), boxes("oak_sign", all6("block/oak_planks")), boxes("oak_wall_sign", all6("block/oak_planks")),
		{Name: "oak_fence", Shape: ShapeBoxes, Layer: LayerCutout, Textures: all6("block/oak_planks"), Connects: true},
		{Name: "torch", Shape: ShapeBoxes, Layer: LayerCutout, Textures: all6("block/torch"), LightEmission: 14},
		{Name: "wall_torch", Shape: ShapeBoxes, Layer: LayerCutout, Textures: all6("block/torch"), LightEmission: 14},
		{Name: "fire", Shape: ShapeCross, Layer: LayerCutout, Textures: all6("block/fire_0"), LightEmission: 15},
		cross("wheat", "block/wheat_stage7", TintNone),
		cube("hay_block", column("block/hay_block_top", "block/hay_block_side")),

		leaves("oak_leaves", TintFoliage), leaves("birch_leaves", TintBirch),
		leaves("spruce_leaves", TintSpruce), leaves("jungle_leaves", TintFoliage),
		leaves("acacia_leaves", TintFoliage), leaves("dark_oak_leaves", TintFoliage),

		cross("short_grass", "block/short_grass", TintGrass), cross("fern", "block/fern", TintGrass),
		cross("dead_bush", "block/dead_bush", TintNone), cross("dandelion", "block/dandelion", TintNone),
		cross("poppy", "block/poppy", TintNone), cross("blue_orchid", "block/blue_orchid", TintNone),
		cross("allium", "block/allium", TintNone), cross("oxeye_daisy", "block/oxeye_daisy", TintNone),
		cross("cornflower", "block/cornflower", TintNone), cross("red_tulip", "block/red_tulip", TintNone),
		cross("brown_mushroom", "block/brown_mushroom", TintNone),
		cross("red_mushroom", "block/red_mushroom", TintNone),
		cross("oak_sapling", "block/oak_sapling", TintNone), cross("sugar_cane", "block/sugar_cane", TintGrass),

		{Name: "glass", Shape: ShapeCube, Layer: LayerCutout, Textures: all6("block/glass"), Collides: true, CullSelf: true},
		{Name: "ice", Shape: ShapeCube, Layer: LayerTranslucent, Textures: all6("block/ice"), Collides: true, CullSelf: true, LightOpacity: 1},
		{Name: "water", Shape: ShapeLiquid, Layer: LayerTranslucent, Textures: all6("block/water_still"),
			Tint: TintWater, CullSelf: true, Replaceable: true, LightOpacity: 1},
		{Name: "lava", Shape: ShapeLiquid, Layer: LayerSolid, Textures: all6("block/lava_still"),
			CullSelf: true, Replaceable: true, LightOpacity: 15, LightEmission: 15},
		// light: the invisible light source (level ignored: always 15).
		{Name: "light", Shape: ShapeNone, Replaceable: true, LightEmission: 15},
		{Name: "glowstone", Shape: ShapeCube, Textures: all6("block/glowstone"), Opaque: true, Collides: true, LightOpacity: 15, LightEmission: 15},
		{Name: "sea_lantern", Shape: ShapeCube, Textures: all6("block/sea_lantern"), Opaque: true, Collides: true, LightOpacity: 15, LightEmission: 15},
		{Name: "jack_o_lantern", Shape: ShapeCube, Textures: [6]string{"block/pumpkin_top", "block/pumpkin_top",
			"block/jack_o_lantern", "block/pumpkin_side", "block/pumpkin_side", "block/pumpkin_side"},
			Opaque: true, Collides: true, LightOpacity: 15, LightEmission: 15},
		{Name: "grass_block", Shape: ShapeCube, Textures: bottomTop("block/dirt", "block/grass_block_top", "block/grass_block_side"),
			Tint: TintGrass, TintMask: 1 << FaceUp, Opaque: true, Collides: true, LightOpacity: 15},
	}
	for _, w := range []string{"oak", "spruce", "birch", "jungle", "acacia", "dark_oak"} {
		defs = append(defs, cube(w+"_log", column("block/"+w+"_log_top", "block/"+w+"_log")))
		defs = append(defs, cube(w+"_wood", all6("block/"+w+"_log")))
	}
	for _, c := range []string{"white", "orange", "magenta", "light_blue", "yellow", "lime", "pink", "gray",
		"light_gray", "cyan", "purple", "blue", "brown", "green", "red", "black"} {
		defs = append(defs, simple(c+"_wool"))
	}
	return defs
}
