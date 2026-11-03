package vanillapack

import (
	"encoding/json"
	"strconv"
	"strings"
)

// block generates the blockstate of a block name and the models it adds,
// or nil for a name it doesn't know.
func (p *Pack) block(name string) *block {
	if b, ok := p.blocks[name]; ok {
		return b
	}
	p.blocks[name] = nil // a family asking for itself finds nothing
	b := p.generate(name)
	p.blocks[name] = b
	return b
}

func (p *Pack) generate(name string) *block {
	if spec, ok := specials[name]; ok {
		return special(name, spec)
	}
	if m := p.templates[name]; m != nil && len(m.Elements) > 0 {
		return single(name) // a model of its own textures (cauldron, flower_pot)
	}
	if spec, ok := singles[name]; ok {
		return singleSpec(name, spec)
	}
	if rest, ok := strings.CutPrefix(name, "waxed_"); ok {
		return p.block(rest) // drawn like the unwaxed block, with its models
	}
	if rest, ok := strings.CutPrefix(name, "infested_"); ok {
		return p.block(rest)
	}
	if rest, ok := strings.CutPrefix(name, "potted_"); ok {
		b := single(name)
		b.model(name, "flower_pot_cross", "plant=block/"+rest)
		return b
	}
	for _, f := range families {
		if stem, ok := strings.CutSuffix(name, f.suffix); ok {
			if b := f.gen(p, name, stem); b != nil {
				return b
			}
		}
	}
	if p.hasTex(name) {
		return cubeAll(name, "block/"+name)
	}
	return nil
}

// singleSpec reads a one-model block written "parent var=tex ...", with $
// standing for block/<name>; "-" is no parent.
func singleSpec(name, spec string) *block {
	ws := strings.Fields(strings.ReplaceAll(spec, "$", "block/"+name))
	parent := ws[0]
	if parent == "-" {
		parent = ""
	}
	b := single(name)
	b.model(name, parent, ws[1:]...)
	return b
}

// singles are the one-model blocks whose model doesn't follow from the
// name.
var singles = map[string]string{
	"air": "- particle=missingno", "cave_air": "- particle=missingno", "void_air": "- particle=missingno",
	"water": "- particle=$_still", "lava": "- particle=$_still", "bubble_column": "- particle=block/water_still",
	"barrier": "- particle=item/barrier", "structure_void": "- particle=item/structure_void",
	"moving_piston": "- particle=block/piston_side", "decorated_pot": "- particle=block/terracotta",
	"end_gateway": "- particle=block/obsidian", "end_portal": "- particle=block/obsidian", "ender_chest": "- particle=block/obsidian",
	"chest": "- particle=block/oak_planks", "trapped_chest": "- particle=block/oak_planks", "conduit": "- particle=$",
	"bush": "tinted_cross cross=$", "fern": "tinted_cross cross=$", "short_grass": "tinted_cross cross=$", "sugar_cane": "tinted_cross cross=$",
	"bamboo_sapling": "tinted_cross cross=block/bamboo_stage0",
	"ancient_debris": "cube_column end=$_top side=$_side", "lodestone": "cube_column end=$_top side=$_side",
	"melon": "cube_column end=$_top side=$_side", "pumpkin": "cube_column end=$_top side=$_side",
	"quartz_block": "cube_column end=$_top side=$_side", "target": "cube_column end=$_top side=$_side",
	"blackstone": "cube_column end=$_top side=$", "chiseled_quartz_block": "cube_column end=$_top side=$",
	"chiseled_tuff": "cube_column end=$_top side=$", "chiseled_tuff_bricks": "cube_column end=$_top side=$",
	"torch": "template_torch torch=$", "soul_torch": "template_torch torch=$", "copper_torch": "template_torch torch=$",
	"azalea": "template_azalea side=$_side top=$_top", "flowering_azalea": "template_azalea side=$_side top=$_top",
	"chiseled_red_sandstone": "cube_column end=block/red_sandstone_top side=$", "cut_red_sandstone": "cube_column end=block/red_sandstone_top side=$",
	"chiseled_sandstone": "cube_column end=block/sandstone_top side=$", "cut_sandstone": "cube_column end=block/sandstone_top side=$",
	"copper_chest": "- particle=block/copper_block", "copper_golem_statue": "- particle=block/copper_block",
	"exposed_copper_chest": "- particle=block/exposed_copper", "exposed_copper_golem_statue": "- particle=block/exposed_copper",
	"weathered_copper_chest": "- particle=block/weathered_copper", "weathered_copper_golem_statue": "- particle=block/weathered_copper",
	"oxidized_copper_chest": "- particle=block/oxidized_copper", "oxidized_copper_golem_statue": "- particle=block/oxidized_copper",
	"crimson_nylium": "cube_bottom_top bottom=block/netherrack side=$_side top=$", "warped_nylium": "cube_bottom_top bottom=block/netherrack side=$_side top=$",
	"firefly_bush": "cross_emissive cross=$ cross_emissive=$_emissive", "open_eyeblossom": "cross_emissive cross=$ cross_emissive=$_emissive",
	"potted_azalea_bush":           "template_potted_azalea_bush plant=$_plant side=$_side top=$_top",
	"potted_flowering_azalea_bush": "template_potted_azalea_bush plant=$_plant side=$_side top=$_top",
	"red_sandstone":                "cube_bottom_top bottom=$_bottom side=$ top=$_top", "sandstone": "cube_bottom_top bottom=$_bottom side=$ top=$_top",
	"reinforced_deepslate": "cube_bottom_top bottom=$_bottom side=$_side top=$_top", "tnt": "cube_bottom_top bottom=$_bottom side=$_side top=$_top",
	"beacon":                 "block beacon=$ glass=block/glass obsidian=block/obsidian particle=block/glass",
	"bookshelf":              "cube_column end=block/oak_planks side=$",
	"cactus":                 "block bottom=$_bottom particle=$_side side=$_side top=$_top",
	"cartography_table":      "cube down=block/dark_oak_planks east=$_side3 north=$_side3 particle=$_side3 south=$_side1 up=$_top west=$_side2",
	"crafting_table":         "cube down=block/oak_planks east=$_side north=$_front particle=$_front south=$_side up=$_top west=$_front",
	"fletching_table":        "cube down=block/birch_planks east=$_side north=$_front particle=$_front south=$_front up=$_top west=$_side",
	"smithing_table":         "cube down=$_bottom east=$_side north=$_front particle=$_front south=$_front up=$_top west=$_side",
	"dragon_egg":             "block all=$ particle=$",
	"dried_kelp_block":       "block down=block/dried_kelp_bottom east=block/dried_kelp_side north=block/dried_kelp_side particle=block/dried_kelp_side south=block/dried_kelp_side up=block/dried_kelp_top west=block/dried_kelp_side",
	"enchanting_table":       "block bottom=$_bottom particle=$_bottom side=$_side top=$_top",
	"honey_block":            "block down=$_bottom particle=$_top side=$_side up=$_top",
	"jukebox":                "cube_top side=$_side top=$_top",
	"lava_cauldron":          "template_cauldron_full bottom=block/cauldron_bottom content=block/lava_still inside=block/cauldron_inner particle=block/cauldron_side side=block/cauldron_side top=block/cauldron_top",
	"magma_block":            "cube_all all=block/magma",
	"mangrove_roots":         "block particle=#side side=$_side top=$_top",
	"moss_carpet":            "carpet wool=block/moss_block",
	"potted_crimson_roots":   "flower_pot_cross plant=block/crimson_roots_pot",
	"potted_warped_roots":    "flower_pot_cross plant=block/warped_roots_pot",
	"potted_fern":            "tinted_flower_pot_cross plant=block/fern",
	"potted_open_eyeblossom": "flower_pot_cross_emissive cross_emissive=block/open_eyeblossom_emissive plant=block/open_eyeblossom",
	"powder_snow":            "block particle=#texture texture=$",
	"seagrass":               "template_seagrass texture=$",
	"smooth_quartz":          "cube_all all=block/quartz_block_bottom",
	"smooth_red_sandstone":   "cube_all all=block/red_sandstone_top",
	"smooth_sandstone":       "cube_all all=block/sandstone_top",
	"snow_block":             "cube_all all=block/snow",
	"spawner":                "cube_all_inner_faces all=$",
	"spore_blossom":          "block base=$_base flower=$ particle=$",
}

// crosses are the plants drawn as two crossed planes of their texture.
var crosses = strings.Fields(`allium azure_bluet blue_orchid brain_coral brown_mushroom bubble_coral
	cactus_flower closed_eyeblossom cobweb cornflower crimson_fungus crimson_roots dandelion dead_brain_coral
	dead_bubble_coral dead_bush dead_fire_coral dead_horn_coral dead_tube_coral fire_coral golden_dandelion
	hanging_roots horn_coral kelp kelp_plant lily_of_the_valley nether_sprouts orange_tulip oxeye_daisy
	pink_tulip poppy red_mushroom red_tulip short_dry_grass tall_dry_grass torchflower tube_coral
	twisting_vines twisting_vines_plant warped_fungus warped_roots weeping_vines weeping_vines_plant
	white_tulip wither_rose`)

func init() {
	for _, n := range crosses {
		singles[n] = "cross cross=$"
	}
}

// family makes the blocks named <stem><suffix>, or returns nil.
type family struct {
	suffix string
	gen    func(p *Pack, name, stem string) *block
}

// families are tried in order: longer suffixes first (set in init: they
// read other blocks).
var families []family

func init() {
	families = []family{
		{"_wall_hanging_sign", func(p *Pack, name, wood string) *block {
			b := facingY(name, 180)
			b.model(name, "template_wall_hanging_sign", "all=block/"+wood+"_hanging_sign", "particle="+strippedLog(wood))
			return b
		}},
		{"_hanging_sign", func(p *Pack, name, wood string) *block {
			return hangingSign(name, "block/"+name, strippedLog(wood))
		}},
		{"_wall_sign", func(p *Pack, name, wood string) *block {
			b := facingY(name, 180)
			b.model(name, "template_wall_sign", "all=block/"+wood+"_sign", "particle=block/"+planks(wood))
			return b
		}},
		{"_sign", func(p *Pack, name, wood string) *block { return sign(name, "block/"+name, "block/"+planks(wood)) }},
		{"_wall_banner", func(p *Pack, name, _ string) *block { return banner() }},
		{"_banner", func(p *Pack, name, _ string) *block { return banner() }},
		{"_wall_head", skull}, {"_head", func(p *Pack, name, stem string) *block {
			if name == "piston_head" {
				return nil
			}
			return skull(p, name, stem)
		}}, {"_wall_skull", skull}, {"_skull", skull},
		{"_shulker_box", func(p *Pack, name, _ string) *block { return singleSpec(name, "- particle=$") }},
		{"shulker_box", func(p *Pack, name, _ string) *block { return singleSpec(name, "- particle=$") }},
		{"_candle_cake", func(p *Pack, name, color string) *block { return candleCake(name, color+"_candle") }},
		{"candle_cake", func(p *Pack, name, _ string) *block { return candleCake(name, "candle") }},
		{"candle", func(p *Pack, name, _ string) *block { return candle(name) }},
		{"_bed", func(p *Pack, name, _ string) *block { return bed(name) }},
		{"_stained_glass_pane", func(p *Pack, name, color string) *block {
			g := "block/" + color + "_stained_glass"
			return pane(name, g, g+"_pane_top")
		}},
		{"glass_pane", func(p *Pack, name, _ string) *block { return pane(name, "block/glass", "block/glass_pane_top") }},
		{"_bars", func(p *Pack, name, _ string) *block { return bars(name) }},
		{"_stairs", func(p *Pack, name, stem string) *block {
			if t, ok := p.baseTex(p.baseOf(stem)); ok {
				return stairs(name, t)
			}
			return nil
		}},
		{"_slab", func(p *Pack, name, stem string) *block {
			base := p.baseOf(stem)
			if t, ok := p.baseTex(base); ok {
				return slab(p, name, base, t)
			}
			return nil
		}},
		{"_fence_gate", func(p *Pack, name, stem string) *block {
			if t, ok := p.baseTex(p.baseOf(stem)); ok {
				return fenceGate(name, t.side)
			}
			return nil
		}},
		{"_fence", func(p *Pack, name, stem string) *block {
			if t, ok := p.baseTex(p.baseOf(stem)); ok {
				return fence(name, t.side)
			}
			return nil
		}},
		{"_wall", func(p *Pack, name, stem string) *block {
			if t, ok := p.baseTex(p.baseOf(stem)); ok {
				return wall(name, t.side)
			}
			return nil
		}},
		{"_door", func(p *Pack, name, _ string) *block { return door(name) }},
		{"_trapdoor", func(p *Pack, name, stem string) *block {
			fixed := stem == "oak" || stem == "dark_oak" || stem == "iron" || strings.HasSuffix(stem, "copper")
			return trapdoor(name, !fixed)
		}},
		{"_button", func(p *Pack, name, stem string) *block {
			if t, ok := p.baseTex(p.baseOf(stem)); ok {
				return button(name, t.side)
			}
			return nil
		}},
		{"_pressure_plate", func(p *Pack, name, stem string) *block {
			switch stem {
			case "light_weighted":
				return pressurePlate(name, "block/gold_block", true)
			case "heavy_weighted":
				return pressurePlate(name, "block/iron_block", true)
			}
			if t, ok := p.baseTex(p.baseOf(stem)); ok {
				return pressurePlate(name, t.side, false)
			}
			return nil
		}},
		{"_carpet", func(p *Pack, name, color string) *block {
			b := single(name)
			b.model(name, "carpet", "wool=block/"+color+"_wool")
			return b
		}},
		{"_glazed_terracotta", func(p *Pack, name, _ string) *block {
			b := facingY(name, 180)
			b.model(name, "template_glazed_terracotta", "pattern=block/"+name)
			return b
		}},
		{"_concrete_powder", func(p *Pack, name, _ string) *block {
			b := newBlock()
			randomY(b, "", name)
			b.model(name, "cube_all", "all=block/"+name)
			return b
		}},
		{"_coral_wall_fan", func(p *Pack, name, stem string) *block {
			b := facingY(name, 0)
			b.model(name, "coral_wall_fan", "fan=block/"+stem+"_coral_fan")
			return b
		}},
		{"_coral_fan", func(p *Pack, name, _ string) *block { return singleSpec(name, "coral_fan fan=$") }},
		{"_sapling", func(p *Pack, name, _ string) *block { return singleSpec(name, "cross cross=$") }},
		{"_leaves", func(p *Pack, name, stem string) *block {
			if stem == "azalea" || stem == "flowering_azalea" {
				return cubeAll(name, "block/"+name)
			}
			return singleSpec(name, "leaves all=$")
		}},
		{"_wall_torch", func(p *Pack, name, stem string) *block {
			t := "torch"
			if stem != "" {
				t = stem + "_torch"
			}
			b := facingY(name, -90)
			b.model(name, "template_torch_wall", "torch=block/"+t)
			return b
		}},
		{"wall_torch", func(p *Pack, name, _ string) *block {
			b := facingY(name, -90)
			b.model(name, "template_torch_wall", "torch=block/torch")
			return b
		}},
		{"lantern", func(p *Pack, name, _ string) *block {
			if name == "sea_lantern" {
				return nil
			}
			return lantern(name)
		}},
		{"_shelf", func(p *Pack, name, wood string) *block { return shelf(name, wood) }},
		{"_log", log}, {"_stem", log},
		{"_wood", wood}, {"_hyphae", wood},
	}
}

func log(p *Pack, name, _ string) *block {
	return pillar(name, "cube_column", "cube_column_horizontal", "end=block/"+name+"_top", "side=block/"+name)
}

func wood(p *Pack, name, stem string) *block {
	l := "block/" + stem + "_log"
	if strings.HasSuffix(name, "_hyphae") {
		l = "block/" + stem + "_stem"
	}
	return pillar(name, "cube_column", "", "end="+l, "side="+l)
}

func banner() *block {
	b := single("banner")
	b.model("banner", "", "particle=block/oak_planks")
	return b
}

func skull(p *Pack, name, _ string) *block {
	b := single("skull")
	b.model("skull", "", "particle=block/soul_sand")
	return b
}

// planks is a wood's planks block.
func planks(wood string) string { return wood + "_planks" }

// strippedLog is the texture of a wood's stripped log.
func strippedLog(wood string) string {
	switch wood {
	case "crimson", "warped":
		return "block/stripped_" + wood + "_stem"
	case "bamboo":
		return "block/stripped_bamboo_block"
	}
	return "block/stripped_" + wood + "_log"
}

// special reads a block of specials: lines "m <id> <parent|-> [ao=false]
// var=tex ..." add models, "v <key> -> <alternatives>" variants and
// "p <when|-> -> <alternatives>" multipart cases, an alternative being
// "<model> [x<deg>] [y<deg>] [L(uvlock)] [w<weight>]", several split by
// "|"; $ stands for the block's name, "like <block>" draws as another
// block's spec with this name.
func special(name, spec string) *block {
	if other, ok := strings.CutPrefix(strings.TrimSpace(spec), "like "); ok {
		spec = specials[other]
	}
	spec = strings.ReplaceAll(spec, "$", name)
	b := newBlock()
	for _, l := range strings.Split(spec, "\n") {
		ws := strings.Fields(l)
		if len(ws) == 0 {
			continue
		}
		switch ws[0] {
		case "m":
			parent := ws[2]
			if parent == "-" {
				parent = ""
			}
			tex := ws[3:]
			if len(tex) > 0 && strings.HasPrefix(tex[0], "ao=") {
				b.model(ws[1], parent, tex[1:]...)
				b.models[ws[1]].AmbientOcclusion = new(bool)
				*b.models[ws[1]].AmbientOcclusion = tex[0] == "ao=true"
				continue
			}
			b.model(ws[1], parent, tex...)
		case "v", "p":
			key, alts, ok := strings.Cut(strings.TrimSpace(l)[1:], "-> ")
			if !ok {
				panic("vanillapack: special " + name + ": " + l)
			}
			key = strings.TrimSpace(key)
			var vs []variant
			for _, a := range strings.Split(alts, "|") {
				vs = append(vs, parseVariant(a))
			}
			if ws[0] == "v" {
				b.variant(key, vs...)
				continue
			}
			var when any
			if key != "-" {
				if err := json.Unmarshal([]byte(key), &when); err != nil {
					panic("vanillapack: special " + name + ": " + err.Error())
				}
			}
			b.part(when, vs...)
		default:
			panic("vanillapack: special " + name + ": " + l)
		}
	}
	return b
}

func parseVariant(s string) variant {
	ws := strings.Fields(s)
	va := v(ws[0])
	for _, w := range ws[1:] {
		n, _ := strconv.Atoi(w[1:])
		switch w[0] {
		case 'x':
			va.X = n
		case 'y':
			va.Y = n
		case 'L':
			va.UVLock = true
		case 'w':
			va.Weight = n
		}
	}
	return va
}
