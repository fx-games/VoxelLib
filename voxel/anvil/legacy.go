package anvil

import (
	"strconv"

	"github.com/memmaker/voxellib/voxel"
)

// legacyIDs maps pre-1.13 numeric ids whose metadata doesn't matter here.
var legacyIDs = map[byte]string{
	1: "stone", 2: "grass_block", 3: "dirt", 4: "cobblestone", 7: "bedrock",
	12: "sand", 13: "gravel", 14: "gold_ore", 15: "iron_ore", 16: "coal_ore", 20: "glass", 21: "lapis_ore",
	24: "sandstone", 32: "dead_bush", 37: "dandelion", 38: "poppy",
	39: "brown_mushroom", 40: "red_mushroom", 41: "gold_block", 42: "iron_block", 43: "smooth_stone", 45: "bricks",
	46: "tnt", 47: "bookshelf", 48: "mossy_cobblestone", 49: "obsidian", 51: "fire", 55: "redstone_wire",
	56: "diamond_ore", 57: "diamond_block", 58: "crafting_table", 60: "farmland",
	73: "redstone_ore", 79: "ice", 80: "snow_block", 81: "cactus", 82: "clay", 83: "sugar_cane", 84: "jukebox",
	85: "oak_fence", 86: "pumpkin", 87: "netherrack", 89: "glowstone", 98: "stone_bricks", 103: "melon",
	110: "mycelium", 129: "emerald_ore", 159: "terracotta", 172: "terracotta", 174: "packed_ice",
}

var (
	woods  = [4]string{"oak", "spruce", "birch", "jungle"}
	colors = [16]string{"white", "orange", "magenta", "light_blue", "yellow", "lime", "pink", "gray",
		"light_gray", "cyan", "purple", "blue", "brown", "green", "red", "black"}
	// facing tables of the old metadata
	stairFacing = [4]string{"east", "west", "south", "north"}
	torchFacing = [5]string{"", "east", "west", "south", "north"}
	doorFacing  = [4]string{"east", "south", "west", "north"}
	sideFacing  = map[byte]string{2: "north", 3: "south", 4: "west", 5: "east"}
)

// legacyState converts a pre-1.13 id and metadata (the 1.13 "flattening").
func legacyState(id, m byte) (string, map[string]string) {
	switch id {
	case 5:
		return woods[m&3] + "_planks", nil
	case 17:
		return woods[m&3] + "_log", map[string]string{"axis": [4]string{"y", "x", "z", "y"}[m>>2&3]}
	case 18:
		return woods[m&3] + "_leaves", nil
	case 31:
		if m == 2 {
			return "fern", nil
		}
		return "short_grass", nil
	case 35:
		return colors[m&15] + "_wool", nil
	case 8, 9, 10, 11:
		name := "water"
		if id >= 10 {
			name = "lava"
		}
		return name, map[string]string{"level": strconv.Itoa(int(m & 7))}
	case 44:
		// ponytail: stone slabs only; the other slab metas are rarer.
		t := "bottom"
		if m&8 != 0 {
			t = "top"
		}
		return "smooth_stone_slab", map[string]string{"type": t}
	case 50:
		if m >= 1 && m <= 4 {
			return "wall_torch", map[string]string{"facing": torchFacing[m]}
		}
		return "torch", nil
	case 53, 67:
		name := "oak_stairs"
		if id == 67 {
			name = "cobblestone_stairs"
		}
		half := "bottom"
		if m&4 != 0 {
			half = "top"
		}
		return name, map[string]string{"facing": stairFacing[m&3], "half": half}
	case 59:
		return "wheat", map[string]string{"age": strconv.Itoa(int(m & 7))}
	case 54:
		if f, ok := sideFacing[m]; ok {
			return "chest", map[string]string{"facing": f}
		}
		return "chest", nil // 1.2 drew unset chests facing south
	case 72:
		return "stone_pressure_plate", map[string]string{"powered": strconv.FormatBool(m&1 != 0)}
	case 61, 62:
		return "furnace", map[string]string{"facing": sideFacing[max(m, 2)]}
	case 63:
		return "oak_sign", map[string]string{"rotation": strconv.Itoa(int(m & 15))}
	case 64:
		// Each half only knows part of the door; fixDoors merges them.
		if m&8 != 0 {
			hinge := "left"
			if m&1 != 0 {
				hinge = "right"
			}
			return "oak_door", map[string]string{"half": "upper", "hinge": hinge, "facing": "east", "open": "false"}
		}
		return "oak_door", map[string]string{"half": "lower", "hinge": "left", "facing": doorFacing[m&3], "open": strconv.FormatBool(m&4 != 0)}
	case 68:
		return "oak_wall_sign", map[string]string{"facing": sideFacing[max(m, 2)]}
	case 65:
		return "ladder", map[string]string{"facing": sideFacing[max(m, 2)]}
	case 69:
		p := map[string]string{"powered": strconv.FormatBool(m&8 != 0), "face": "floor", "facing": "north"}
		switch m & 7 {
		case 1, 2, 3, 4:
			p["face"], p["facing"] = "wall", torchFacing[m&7]
		case 6:
			p["facing"] = "west"
		case 0:
			p["face"], p["facing"] = "ceiling", "west"
		case 7:
			p["face"] = "ceiling"
		}
		return "lever", p
	case 78:
		return "snow", map[string]string{"layers": strconv.Itoa(int(m&7) + 1)}
	}
	if name, ok := legacyIDs[id]; ok {
		return name, nil
	}
	return "stone", nil
}

func decodeLegacy(reg *voxel.Registry, s *voxel.Section, blocks, meta []byte) {
	// legacy order is y<<8|z<<4|x too
	var buf [voxel.SectionVolume]voxel.StateID
	cache := map[uint16]voxel.StateID{}
	for i, b := range blocks {
		var m byte
		if len(meta) == 2048 {
			m = meta[i/2] >> (4 * (i % 2)) & 15
		}
		key := uint16(b)<<4 | uint16(m)
		id, ok := cache[key]
		if !ok {
			if b != 0 {
				id = reg.State(legacyState(b, m))
			}
			cache[key] = id
		}
		buf[i] = id
	}
	s.Blocks.Pack(&buf)
	s.Recount()
}

// fixDoors gives each half of a legacy door the facing/open (from the lower
// half) and hinge (from the upper half) the other half stored.
func fixDoors(reg *voxel.Registry, ch *voxel.Chunk) {
	t := reg.Table()
	for y := ch.MinY(); y < ch.MinY()+len(ch.Sections)*16-1; y++ {
		for z := 0; z < 16; z++ {
			for x := 0; x < 16; x++ {
				lo, hi := ch.Get(x, y, z), ch.Get(x, y+1, z)
				if t[lo].Name != "minecraft:oak_door" || t[hi].Name != "minecraft:oak_door" {
					continue
				}
				pl, ph := reg.StateProps(lo), reg.StateProps(hi)
				if pl["half"] != "lower" || ph["half"] != "upper" {
					continue
				}
				pl["hinge"] = ph["hinge"]
				ph["facing"], ph["open"] = pl["facing"], pl["open"]
				ch.SetRaw(x, y, z, reg.State("oak_door", pl))
				ch.SetRaw(x, y+1, z, reg.State("oak_door", ph))
				t = reg.Table()
			}
		}
	}
}
