package bedrock

import (
	"fmt"
	"strings"
)

// JavaState is a Java Edition block state: a name without namespace
// ("oak_stairs") and its properties.
type JavaState struct {
	Name  string
	Props map[string]string
}

// String is "name[k=v,...]" with the properties sorted.
func (s JavaState) String() string {
	return BlockState{Name: s.Name, States: anyProps(s.Props)}.Key()
}

func anyProps(p map[string]string) map[string]any {
	if len(p) == 0 {
		return nil
	}
	m := make(map[string]any, len(p))
	for k, v := range p {
		m[k] = v
	}
	return m
}

// Translate turns a Bedrock palette entry into the Java state that looks
// the same. Properties Bedrock doesn't store (stair corners, fence and pane
// arms, the halves of doors) are left for Fix, which sees the neighbours.
// ok is false for a block it doesn't know (the caller keeps it as air).
func Translate(b BlockState) (js JavaState, ok bool) {
	name := strings.TrimPrefix(b.Name, "minecraft:")
	st := b.States
	s := func(k string) string { v, _ := st[k].(string); return v }
	n := func(k string) int { v, _ := st[k].(int64); return int(v) }
	bit := func(k string) bool { return n(k) != 0 }
	tf := func(v bool) string { return fmt.Sprint(v) }
	p := map[string]string{}
	out := func(name string) (JavaState, bool) { return JavaState{name, p}, true }
	half := func(top bool) string {
		if top {
			return "top"
		}
		return "bottom"
	}
	slab := func(java string, double bool) (JavaState, bool) {
		p["type"] = half(bit("top_slot_bit"))
		if double {
			p["type"] = "double"
		}
		p["waterlogged"] = "false"
		return out(java)
	}

	// Colours (silver is light_gray).
	color := func() string {
		c := s("color")
		if c == "silver" {
			c = "light_gray"
		}
		if c == "" {
			c = "white"
		}
		return c
	}
	switch name {
	case "concrete", "concrete_powder", "concretePowder", "wool", "carpet", "stained_glass":
		suffix := map[string]string{"concretePowder": "concrete_powder"}[name]
		if suffix == "" {
			suffix = name
		}
		return out(color() + "_" + suffix)
	case "stained_glass_pane":
		return out(color() + "_stained_glass_pane")
	case "stained_hardened_clay":
		return out(color() + "_terracotta")
	case "shulker_box":
		p["facing"] = "up"
		return out(color() + "_shulker_box")
	case "undyed_shulker_box":
		p["facing"] = "up"
		return out("shulker_box")
	}
	if strings.HasSuffix(name, "_glazed_terracotta") {
		p["facing"] = horizontalFacing(n("facing_direction"))
		return out(strings.Replace(name, "silver_", "light_gray_", 1))
	}

	// Stairs: weirdo_direction 0 east, 1 west, 2 south, 3 north.
	if java, isStairs := stairNames[name]; isStairs || strings.HasSuffix(name, "_stairs") {
		if !isStairs {
			java = name
		}
		p["facing"] = [4]string{"east", "west", "south", "north"}[n("weirdo_direction")&3]
		p["half"] = half(bit("upside_down_bit"))
		p["shape"] = "straight"
		p["waterlogged"] = "false"
		return out(java)
	}
	// Doors: direction 0 east, 1 south, 2 west, 3 north.
	if java, isDoor := doorNames[name]; isDoor {
		p["facing"] = [4]string{"east", "south", "west", "north"}[n("direction")&3]
		p["half"] = "lower"
		if bit("upper_block_bit") {
			p["half"] = "upper"
		}
		p["hinge"] = "left"
		if bit("door_hinge_bit") {
			p["hinge"] = "right"
		}
		p["open"] = tf(bit("open_bit"))
		p["powered"] = "false"
		return out(java)
	}
	// Trapdoors: direction 0 east, 1 west, 2 south, 3 north.
	if java, isTrap := trapdoorNames[name]; isTrap {
		p["facing"] = [4]string{"east", "west", "south", "north"}[n("direction")&3]
		p["half"] = half(bit("upside_down_bit"))
		p["open"] = tf(bit("open_bit"))
		p["powered"] = "false"
		p["waterlogged"] = "false"
		return out(java)
	}
	if java, isGate := gateNames[name]; isGate {
		p["facing"] = cardinal(n("direction"))
		p["in_wall"] = tf(bit("in_wall_bit"))
		p["open"] = tf(bit("open_bit"))
		p["powered"] = "false"
		return out(java)
	}
	if java, isButton := buttonNames[name]; isButton {
		face(p, n("facing_direction"))
		p["powered"] = "false"
		return out(java)
	}
	if java, isPlate := plateNames[name]; isPlate {
		if strings.Contains(java, "weighted") {
			p["power"] = "0"
		} else {
			p["powered"] = "false"
		}
		return out(java)
	}
	if java, isSign := wallSignNames[name]; isSign {
		p["facing"] = horizontalFacing(n("facing_direction"))
		p["waterlogged"] = "false"
		return out(java)
	}

	switch name {
	case "air", "bedrock", "glass", "cobblestone", "glowstone", "clay", "bookshelf",
		"podzol", "smooth_stone", "iron_block", "gold_block", "lapis_block", "coal_block",
		"redstone_block", "netherite_block", "emerald_block", "diamond_block", "iron_bars",
		"glass_pane", "beacon", "lodestone", "blue_ice", "ice", "packed_ice", "netherrack",
		"blackstone", "polished_blackstone", "mossy_cobblestone", "quartz_bricks", "barrier",
		"sea_lantern", "obsidian", "gravel", "snow", "sponge", "tnt", "end_stone", "magma",
		"soul_sand", "mycelium", "fletching_table", "cartography_table", "smithing_table",
		"crafting_table", "honeycomb_block", "melon_block", "pumpkin", "hay_block", "target",
		"nether_brick", "red_nether_brick", "prismarine", "purpur_block":
		switch name {
		case "nether_brick":
			return out("nether_bricks")
		case "red_nether_brick":
			return out("red_nether_bricks")
		case "melon_block":
			return out("melon")
		case "magma":
			return out("magma_block")
		case "prismarine":
			return out(pick(map[string]string{"dark": "dark_prismarine", "bricks": "prismarine_bricks"}, s("prismarine_block_type"), "prismarine"))
		case "purpur_block":
			if s("chisel_type") == "lines" {
				p["axis"] = axis(s("pillar_axis"))
				return out("purpur_pillar")
			}
		case "hay_block":
			p["axis"] = axis(s("pillar_axis"))
		}
		return out(name)
	case "seaLantern":
		return out("sea_lantern")
	case "grass":
		p["snowy"] = "false"
		return out("grass_block")
	case "grass_path":
		return out("dirt_path")
	case "dirt":
		if s("dirt_type") == "coarse" {
			return out("coarse_dirt")
		}
		return out("dirt")
	case "sand":
		if s("sand_type") == "red" {
			return out("red_sand")
		}
		return out("sand")
	case "stone":
		return out(pick(map[string]string{"granite": "granite", "granite_smooth": "polished_granite",
			"diorite": "diorite", "diorite_smooth": "polished_diorite", "andesite": "andesite",
			"andesite_smooth": "polished_andesite"}, s("stone_type"), "stone"))
	case "stonebrick":
		return out(pick(map[string]string{"mossy": "mossy_stone_bricks", "cracked": "cracked_stone_bricks",
			"chiseled": "chiseled_stone_bricks"}, s("stone_brick_type"), "stone_bricks"))
	case "brick_block":
		return out("bricks")
	case "hardened_clay":
		return out("terracotta")
	case "sandstone", "red_sandstone":
		pre := strings.TrimSuffix(name, "sandstone")
		switch s("sand_stone_type") {
		case "heiroglyphs":
			return out("chiseled_" + pre + "sandstone")
		case "cut":
			return out("cut_" + pre + "sandstone")
		case "smooth":
			return out("smooth_" + pre + "sandstone")
		}
		return out(name)
	case "quartz_block":
		switch s("chisel_type") {
		case "chiseled":
			return out("chiseled_quartz_block")
		case "lines":
			p["axis"] = axis(s("pillar_axis"))
			return out("quartz_pillar")
		case "smooth":
			return out("smooth_quartz")
		}
		return out("quartz_block")
	case "planks":
		return out(wood(s("wood_type")) + "_planks")
	case "log", "log2":
		t := s("old_log_type")
		if name == "log2" {
			t = s("new_log_type")
		}
		p["axis"] = axis(s("pillar_axis"))
		return out(wood(t) + "_log")
	case "wood":
		p["axis"] = axis(s("pillar_axis"))
		if bit("stripped_bit") {
			return out("stripped_" + wood(s("wood_type")) + "_wood")
		}
		return out(wood(s("wood_type")) + "_wood")
	case "leaves", "leaves2":
		t := s("old_leaf_type")
		if name == "leaves2" {
			t = s("new_leaf_type")
		}
		p["distance"], p["persistent"], p["waterlogged"] = "7", "true", "false"
		return out(wood(t) + "_leaves")
	case "azalea_leaves_flowered", "azalea_leaves":
		p["distance"], p["persistent"], p["waterlogged"] = "7", "true", "false"
		return out(pick(map[string]string{"azalea_leaves_flowered": "flowering_azalea_leaves"}, name, name))
	case "pearlescent_froglight", "ochre_froglight", "verdant_froglight", "basalt", "bone_block":
		p["axis"] = axis(s("pillar_axis"))
		return out(name)
	case "chain", "iron_chain":
		p["axis"] = axis(s("pillar_axis"))
		p["waterlogged"] = "false"
		return out("iron_chain")
	case "coral":
		kind := pick(map[string]string{"blue": "tube", "pink": "brain", "purple": "bubble", "red": "fire", "yellow": "horn"}, s("coral_color"), "tube")
		p["waterlogged"] = tf(!bit("dead_bit"))
		if bit("dead_bit") {
			return out("dead_" + kind + "_coral")
		}
		return out(kind + "_coral")
	case "redstone_wire":
		p["power"] = fmt.Sprint(n("redstone_signal"))
		p["north"], p["south"], p["east"], p["west"] = "none", "none", "none", "none"
		return out("redstone_wire")
	case "web":
		return out("cobweb")
	case "slime":
		return out("slime_block")
	case "noteblock":
		p["instrument"], p["note"], p["powered"] = "harp", "0", "false"
		return out("note_block")

	// Slabs.
	case "stone_slab", "stone_block_slab", "double_stone_slab", "double_stone_block_slab":
		return slab(slabType1[s("stone_slab_type")]+"_slab", strings.HasPrefix(name, "double_"))
	case "stone_slab2", "stone_block_slab2", "double_stone_slab2", "double_stone_block_slab2":
		return slab(slabType2[s("stone_slab_type_2")]+"_slab", strings.HasPrefix(name, "double_"))
	case "stone_slab3", "stone_block_slab3", "double_stone_slab3", "double_stone_block_slab3":
		return slab(s("stone_slab_type_3")+"_slab", strings.HasPrefix(name, "double_"))
	case "stone_slab4", "stone_block_slab4", "double_stone_slab4", "double_stone_block_slab4":
		return slab(s("stone_slab_type_4")+"_slab", strings.HasPrefix(name, "double_"))
	case "wooden_slab", "double_wooden_slab":
		return slab(wood(s("wood_type"))+"_slab", name == "double_wooden_slab")
	}
	if strings.HasSuffix(name, "_double_slab") {
		return slab(strings.TrimSuffix(name, "_double_slab")+"_slab", true)
	}
	if strings.HasSuffix(name, "_slab") {
		return slab(name, false)
	}

	switch name {
	// Plants.
	case "tallgrass":
		if s("tall_grass_type") == "fern" {
			return out("fern")
		}
		return out("short_grass")
	case "double_plant":
		p["half"] = "lower"
		if bit("upper_block_bit") {
			p["half"] = "upper"
		}
		return out(map[string]string{"sunflower": "sunflower", "syringa": "lilac", "grass": "tall_grass",
			"fern": "large_fern", "rose": "rose_bush", "paeonia": "peony"}[s("double_plant_type")])
	case "red_flower":
		return out(map[string]string{"poppy": "poppy", "orchid": "blue_orchid", "allium": "allium",
			"houstonia": "azure_bluet", "tulip_red": "red_tulip", "tulip_orange": "orange_tulip",
			"tulip_white": "white_tulip", "tulip_pink": "pink_tulip", "oxeye": "oxeye_daisy",
			"cornflower": "cornflower", "lily_of_the_valley": "lily_of_the_valley"}[s("flower_type")])
	case "yellow_flower":
		return out("dandelion")
	case "sapling":
		p["stage"] = "0"
		return out(wood(s("sapling_type")) + "_sapling")
	case "waterlily":
		return out("lily_pad")
	case "vine":
		bits := n("vine_direction_bits")
		p["south"], p["west"], p["north"], p["east"] = tf(bits&1 != 0), tf(bits&2 != 0), tf(bits&4 != 0), tf(bits&8 != 0)
		p["up"] = tf(bits == 0)
		return out("vine")
	case "kelp":
		return out("kelp_plant")
	case "seagrass":
		return out("seagrass")
	case "sea_pickle":
		p["pickles"] = fmt.Sprint(n("cluster_count") + 1)
		p["waterlogged"] = tf(!bit("dead_bit"))
		return out("sea_pickle")
	case "brown_mushroom_block", "red_mushroom_block":
		on := func(v bool) {
			p["up"], p["down"], p["north"], p["south"], p["east"], p["west"] = tf(v), tf(v), tf(v), tf(v), tf(v), tf(v)
		}
		switch n("huge_mushroom_bits") {
		case 10, 15:
			on(true)
			return out("mushroom_stem")
		case 0:
			on(false)
		default:
			on(true)
		}
		return out(name)
	case "turtle_egg":
		p["eggs"] = fmt.Sprint(map[string]int{"one_egg": 1, "two_egg": 2, "three_egg": 3, "four_egg": 4}[s("turtle_egg_count")])
		p["hatch"] = "0"
		return out("turtle_egg")
	case "fire":
		p["age"], p["north"], p["south"], p["east"], p["west"], p["up"] = "0", "false", "false", "false", "false", "false"
		return out("fire")
	case "cake":
		p["bites"] = fmt.Sprint(n("bite_counter"))
		return out("cake")

	// Liquids: level is the Java one (0 source, 1-7 flowing, 8+ falling).
	case "water", "flowing_water", "lava", "flowing_lava":
		p["level"] = fmt.Sprint(n("liquid_depth") & 15)
		return out(strings.TrimPrefix(name, "flowing_"))

	// Light sources and redstone.
	case "torch", "redstone_torch", "unlit_redstone_torch", "soul_torch":
		base := pick(map[string]string{"unlit_redstone_torch": "redstone_torch"}, name, name)
		if base == "redstone_torch" {
			p["lit"] = tf(name == "redstone_torch")
		}
		// torch_facing_direction names the side the torch leans to the
		// opposite of: "west" hangs on the west face of the block east of it.
		f, wall := map[string]string{"west": "east", "east": "west", "north": "south", "south": "north"}[s("torch_facing_direction")]
		if !wall {
			return out(base)
		}
		p["facing"] = f
		return out(strings.TrimSuffix(base, "torch") + "wall_torch")
	case "lantern", "soul_lantern":
		p["hanging"] = tf(bit("hanging"))
		p["waterlogged"] = "false"
		return out(name)
	case "end_rod", "lightning_rod":
		p["facing"] = facing6(n("facing_direction"))
		if name == "lightning_rod" {
			p["powered"], p["waterlogged"] = "false", "false"
		}
		return out(name)
	case "redstone_lamp", "lit_redstone_lamp":
		p["lit"] = tf(name == "lit_redstone_lamp")
		return out("redstone_lamp")
	case "daylight_detector", "daylight_detector_inverted":
		p["inverted"] = tf(name == "daylight_detector_inverted")
		p["power"] = fmt.Sprint(n("redstone_signal"))
		return out("daylight_detector")
	case "lever":
		f, ok := leverDirs[s("lever_direction")]
		if !ok {
			f = [2]string{"floor", "north"}
		}
		p["face"], p["facing"] = f[0], f[1]
		p["powered"] = tf(bit("open_bit"))
		return out("lever")
	case "rail":
		p["shape"] = railShapes[n("rail_direction")%10]
		p["waterlogged"] = "false"
		return out("rail")
	case "golden_rail", "detector_rail", "activator_rail":
		p["shape"] = railShapes[n("rail_direction")%6]
		p["powered"] = tf(bit("rail_data_bit"))
		p["waterlogged"] = "false"
		return out(pick(map[string]string{"golden_rail": "powered_rail"}, name, name))
	case "tripwire_hook":
		p["facing"] = cardinal(n("direction"))
		p["attached"], p["powered"] = tf(bit("attached_bit")), "false"
		return out("tripwire_hook")
	case "trip_wire":
		p["attached"], p["disarmed"], p["powered"] = tf(bit("attached_bit")), tf(bit("disarmed_bit")), "false"
		p["north"], p["south"], p["east"], p["west"] = "false", "false", "false", "false"
		return out("tripwire")
	case "unpowered_repeater", "powered_repeater":
		p["facing"] = cardinal(n("direction"))
		p["delay"] = fmt.Sprint(n("repeater_delay") + 1)
		p["locked"], p["powered"] = "false", tf(name == "powered_repeater")
		return out("repeater")
	case "piston", "sticky_piston":
		p["facing"] = facing6(n("facing_direction"))
		p["extended"] = "false"
		return out(name)
	case "piston_arm_collision":
		p["facing"] = facing6(n("facing_direction"))
		p["short"], p["type"] = "false", "normal"
		return out("piston_head")
	case "observer":
		p["facing"] = facing6(n("facing_direction"))
		p["powered"] = "false"
		return out("observer")
	case "dispenser", "dropper":
		p["facing"] = facing6(n("facing_direction"))
		p["triggered"] = "false"
		return out(name)
	case "hopper":
		f := facing6(n("facing_direction"))
		if f == "up" {
			f = "down"
		}
		p["facing"], p["enabled"] = f, "true"
		return out("hopper")
	case "command_block":
		p["facing"] = facing6(n("facing_direction"))
		p["conditional"] = tf(bit("conditional_bit"))
		return out("command_block")

	// Furniture and containers.
	case "chest", "trapped_chest", "ender_chest":
		p["facing"] = horizontalFacing(n("facing_direction"))
		p["type"], p["waterlogged"] = "single", "false"
		return out(name)
	case "furnace", "lit_furnace", "blast_furnace", "lit_blast_furnace", "smoker", "lit_smoker":
		p["facing"] = horizontalFacing(n("facing_direction"))
		p["lit"] = tf(strings.HasPrefix(name, "lit_"))
		return out(strings.TrimPrefix(name, "lit_"))
	case "barrel":
		p["facing"] = facing6(n("facing_direction"))
		p["open"] = tf(bit("open_bit"))
		return out("barrel")
	case "ladder":
		p["facing"] = horizontalFacing(n("facing_direction"))
		p["waterlogged"] = "false"
		return out("ladder")
	case "bed":
		p["facing"] = cardinal(n("direction"))
		p["part"] = "foot"
		if bit("head_piece_bit") {
			p["part"] = "head"
		}
		p["occupied"] = "false"
		return out("red_bed") // the colour is the block entity's (Fix)
	case "anvil", "chipped_anvil", "damaged_anvil":
		// Bedrock's direction turns a quarter from Java's.
		p["facing"] = cardinal(n("direction") + 1)
		switch s("damage") {
		case "slightly_damaged":
			return out("chipped_anvil")
		case "very_damaged":
			return out("damaged_anvil")
		}
		return out(name)
	case "bell":
		p["facing"] = cardinal(n("direction"))
		p["attachment"] = map[string]string{"standing": "floor", "hanging": "ceiling", "side": "single_wall", "multiple": "double_wall"}[s("attachment")]
		p["powered"] = "false"
		return out("bell")
	case "grindstone":
		p["facing"] = cardinal(n("direction"))
		p["face"] = map[string]string{"standing": "floor", "hanging": "ceiling", "side": "wall", "multiple": "wall"}[s("attachment")]
		return out("grindstone")
	case "lectern":
		p["facing"] = cardinal(n("direction"))
		p["has_book"], p["powered"] = "false", "false"
		return out("lectern")
	case "loom", "stonecutter_block":
		p["facing"] = cardinal(n("direction"))
		return out(pick(map[string]string{"stonecutter_block": "stonecutter"}, name, name))
	case "bee_nest", "beehive":
		p["facing"] = cardinal(n("direction"))
		p["honey_level"] = fmt.Sprint(min(n("honey_level"), 5))
		return out(name)
	case "brewing_stand":
		p["has_bottle_0"], p["has_bottle_1"], p["has_bottle_2"] = tf(bit("brewing_stand_slot_a_bit")), tf(bit("brewing_stand_slot_b_bit")), tf(bit("brewing_stand_slot_c_bit"))
		return out("brewing_stand")
	case "cauldron":
		lvl := n("fill_level")
		if lvl == 0 {
			return out("cauldron")
		}
		p["level"] = fmt.Sprint((lvl + 1) / 2)
		if s("cauldron_liquid") == "lava" {
			return out("lava_cauldron")
		}
		return out("water_cauldron")
	case "composter":
		p["level"] = fmt.Sprint(min(n("composter_fill_level"), 8))
		return out("composter")
	case "scaffolding":
		p["bottom"], p["distance"], p["waterlogged"] = "false", "0", "false"
		return out("scaffolding")
	case "flower_pot":
		return out("flower_pot") // the plant is the block entity's (Fix)
	case "frame", "glow_frame":
		p["facing"] = facing6(n("facing_direction"))
		p["map"] = tf(bit("item_frame_map_bit"))
		return out(pick(map[string]string{"frame": "item_frame", "glow_frame": "glow_item_frame"}, name, name))
	case "standing_sign":
		p["rotation"] = fmt.Sprint(n("ground_sign_direction") & 15)
		p["waterlogged"] = "false"
		return out("oak_sign")
	case "spruce_standing_sign", "birch_standing_sign", "jungle_standing_sign", "acacia_standing_sign", "darkoak_standing_sign":
		p["rotation"] = fmt.Sprint(n("ground_sign_direction") & 15)
		p["waterlogged"] = "false"
		return out(strings.Replace(strings.TrimSuffix(name, "_standing_sign"), "darkoak", "dark_oak", 1) + "_sign")
	case "standing_banner":
		p["rotation"] = fmt.Sprint(n("ground_sign_direction") & 15)
		return out("white_banner") // the colour is the block entity's (Fix)
	case "wall_banner":
		p["facing"] = horizontalFacing(n("facing_direction"))
		return out("white_wall_banner")
	case "skull":
		if f := n("facing_direction"); f >= 2 {
			p["facing"] = horizontalFacing(f)
			p["powered"] = "false"
			return out("skeleton_wall_skull")
		}
		p["rotation"], p["powered"] = "0", "false"
		return out("skeleton_skull")

	// Fences, panes and walls: arms are the neighbours' (Fix), except
	// walls, whose Bedrock state has them.
	case "fence":
		fenceProps(p)
		return out(wood(s("wood_type")) + "_fence")
	case "nether_brick_fence":
		fenceProps(p)
		return out(name)
	case "cobblestone_wall":
		wallProps(p, st)
		t := s("wall_block_type")
		if t == "" {
			t = "cobblestone"
		}
		return out(t + "_wall")
	}
	if strings.HasSuffix(name, "_fence") {
		fenceProps(p)
		return out(name)
	}
	if strings.HasSuffix(name, "_wall") && !strings.HasSuffix(name, "_wall_sign") && !strings.HasSuffix(name, "_wall_banner") {
		wallProps(p, st)
		return out(name)
	}
	if strings.HasSuffix(name, "_planks") || strings.HasSuffix(name, "_bricks") || strings.HasSuffix(name, "_block") ||
		strings.HasSuffix(name, "_ore") || strings.HasSuffix(name, "_wool") {
		return out(name)
	}
	return JavaState{}, false
}

// pick is m[k], or def when m lacks k.
func pick(m map[string]string, k, def string) string {
	if v, ok := m[k]; ok {
		return v
	}
	return def
}

func fenceProps(p map[string]string) {
	p["north"], p["south"], p["east"], p["west"], p["waterlogged"] = "false", "false", "false", "false", "false"
}

func wallProps(p map[string]string, st map[string]any) {
	for _, d := range []string{"north", "south", "east", "west"} {
		v, _ := st["wall_connection_type_"+d].(string)
		p[d] = pick(map[string]string{"short": "low", "tall": "tall"}, v, "none")
	}
	post, _ := st["wall_post_bit"].(int64)
	p["up"] = fmt.Sprint(post != 0)
	p["waterlogged"] = "false"
}

func wood(t string) string {
	if t == "" {
		return "oak"
	}
	return t
}

func axis(a string) string {
	if a == "" {
		return "y"
	}
	return a
}

// facing6 is a facing_direction: 0 down, 1 up, 2 north, 3 south, 4 west,
// 5 east.
func facing6(d int) string {
	if d < 0 || d > 5 {
		return "north"
	}
	return [6]string{"down", "up", "north", "south", "west", "east"}[d]
}

// horizontalFacing is a facing_direction limited to the four sides.
func horizontalFacing(d int) string {
	if d < 2 || d > 5 {
		return "north"
	}
	return facing6(d)
}

// cardinal is a "direction": 0 south, 1 west, 2 north, 3 east.
func cardinal(d int) string { return [4]string{"south", "west", "north", "east"}[d&3] }

// face sets a button's face and facing from its facing_direction.
func face(p map[string]string, d int) {
	switch d {
	case 0:
		p["face"], p["facing"] = "ceiling", "north"
	case 1:
		p["face"], p["facing"] = "floor", "north"
	default:
		p["face"], p["facing"] = "wall", horizontalFacing(d)
	}
}

var railShapes = [10]string{"north_south", "east_west", "ascending_east", "ascending_west", "ascending_north",
	"ascending_south", "south_east", "south_west", "north_west", "north_east"}

var leverDirs = map[string][2]string{
	"down_east_west": {"ceiling", "west"}, "east": {"wall", "east"}, "west": {"wall", "west"},
	"south": {"wall", "south"}, "north": {"wall", "north"}, "up_north_south": {"floor", "north"},
	"up_east_west": {"floor", "east"}, "down_north_south": {"ceiling", "north"},
}

var slabType1 = map[string]string{"smooth_stone": "smooth_stone", "sandstone": "sandstone", "wood": "petrified_oak",
	"cobblestone": "cobblestone", "brick": "brick", "stone_brick": "stone_brick", "quartz": "quartz",
	"nether_brick": "nether_brick", "": "smooth_stone"}

var slabType2 = map[string]string{"red_sandstone": "red_sandstone", "purpur": "purpur", "prismarine_rough": "prismarine",
	"prismarine_dark": "dark_prismarine", "prismarine_brick": "prismarine_brick", "mossy_cobblestone": "mossy_cobblestone",
	"smooth_sandstone": "smooth_sandstone", "red_nether_brick": "red_nether_brick", "": "red_sandstone"}

var stairNames = map[string]string{
	"stone_stairs": "cobblestone_stairs", "normal_stone_stairs": "stone_stairs",
	"prismarine_bricks_stairs": "prismarine_brick_stairs",
}

var doorNames = map[string]string{
	"wooden_door": "oak_door", "spruce_door": "spruce_door", "birch_door": "birch_door", "jungle_door": "jungle_door",
	"acacia_door": "acacia_door", "dark_oak_door": "dark_oak_door", "iron_door": "iron_door",
	"mangrove_door": "mangrove_door", "crimson_door": "crimson_door", "warped_door": "warped_door",
}

var trapdoorNames = map[string]string{
	"trapdoor": "oak_trapdoor", "spruce_trapdoor": "spruce_trapdoor", "birch_trapdoor": "birch_trapdoor",
	"jungle_trapdoor": "jungle_trapdoor", "acacia_trapdoor": "acacia_trapdoor", "dark_oak_trapdoor": "dark_oak_trapdoor",
	"iron_trapdoor": "iron_trapdoor", "warped_trapdoor": "warped_trapdoor", "crimson_trapdoor": "crimson_trapdoor",
}

var gateNames = map[string]string{
	"fence_gate": "oak_fence_gate", "spruce_fence_gate": "spruce_fence_gate", "birch_fence_gate": "birch_fence_gate",
	"jungle_fence_gate": "jungle_fence_gate", "acacia_fence_gate": "acacia_fence_gate", "dark_oak_fence_gate": "dark_oak_fence_gate",
}

var buttonNames = map[string]string{
	"stone_button": "stone_button", "wooden_button": "oak_button", "spruce_button": "spruce_button",
	"birch_button": "birch_button", "jungle_button": "jungle_button", "acacia_button": "acacia_button",
	"dark_oak_button": "dark_oak_button", "polished_blackstone_button": "polished_blackstone_button",
}

var plateNames = map[string]string{
	"stone_pressure_plate": "stone_pressure_plate", "wooden_pressure_plate": "oak_pressure_plate",
	"spruce_pressure_plate": "spruce_pressure_plate", "birch_pressure_plate": "birch_pressure_plate",
	"jungle_pressure_plate": "jungle_pressure_plate", "acacia_pressure_plate": "acacia_pressure_plate",
	"dark_oak_pressure_plate": "dark_oak_pressure_plate", "polished_blackstone_pressure_plate": "polished_blackstone_pressure_plate",
	"heavy_weighted_pressure_plate": "heavy_weighted_pressure_plate", "light_weighted_pressure_plate": "light_weighted_pressure_plate",
}

var wallSignNames = map[string]string{
	"wall_sign": "oak_wall_sign", "spruce_wall_sign": "spruce_wall_sign", "birch_wall_sign": "birch_wall_sign",
	"jungle_wall_sign": "jungle_wall_sign", "acacia_wall_sign": "acacia_wall_sign", "darkoak_wall_sign": "dark_oak_wall_sign",
}
