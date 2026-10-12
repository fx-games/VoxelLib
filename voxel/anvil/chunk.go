package anvil

import (
	"errors"
	"math/bits"
	"sort"

	"github.com/memmaker/voxellib/voxel"
)

// DataVersion written into saved chunks (1.21). Newer game versions upgrade
// it on load through their DataFixers.
const DataVersion = 3953

const (
	dvNoSpanning   = 2529 // 20w17a (1.16): entries no longer span longs
	dvSectionsRoot = 2844 // 21w43a (1.18): no Level wrapper, block_states
)

// DecodeChunk converts chunk NBT into a voxel.Chunk shaped for w. Sections
// outside the world's height are dropped. Light is only trusted from chunks
// voxellib lit and saved itself (the litTag): those load with Chunk.Lit
// set and skip relighting. Anything else is relit by the world, so data
// from any version is consistent.
func DecodeChunk(w *voxel.World, root Compound) (*voxel.Chunk, error) {
	dv := root.Int("DataVersion")
	level := root
	if lv := root.Compound("Level"); lv != nil {
		level = lv
	}
	cx, cz := level.Int("xPos"), level.Int("zPos")
	ch := w.NewChunk(voxel.ChunkPos{X: cx, Z: cz})
	secs := level.List("sections")
	if len(secs.Items) == 0 {
		secs = level.List("Sections")
	}
	if len(secs.Items) == 0 && !level.Has("sections") && !level.Has("Sections") {
		return nil, errors.New("anvil: chunk has no sections")
	}
	reg := w.Reg
	trusted := level.Int(litTag) == 1 && len(secs.Items) == len(ch.Sections)
	for _, it := range secs.Items {
		sc, ok := it.(Compound)
		if !ok {
			continue
		}
		sy := sc.Int("Y")
		s := ch.Section(sy * 16)
		if s == nil {
			continue
		}
		sky, blk := sc.Bytes("SkyLight"), sc.Bytes("BlockLight")
		if len(sky) == 2048 && len(blk) == 2048 {
			copy(s.Sky, sky)
			copy(s.Block, blk)
		} else {
			trusted = false
		}
		var palNBT List
		var data []int64
		if bs := sc.Compound("block_states"); bs != nil {
			palNBT, data = bs.List("palette"), bs.Longs("data")
		} else if sc.Has("Palette") {
			palNBT, data = sc.List("Palette"), sc.Longs("BlockStates")
		} else if blocks := sc.Bytes("Blocks"); len(blocks) == 4096 {
			decodeLegacy(reg, s, blocks, sc.Bytes("Data"))
			continue
		} else {
			continue
		}
		pal := make([]voxel.StateID, len(palNBT.Items))
		for i, p := range palNBT.Items {
			pc, _ := p.(Compound)
			pal[i] = stateFromNBT(reg, pc)
		}
		ud := make([]uint64, len(data))
		for i, v := range data {
			ud[i] = uint64(v)
		}
		spanning := dv < dvNoSpanning
		var b uint
		if spanning {
			b = uint(len(ud) * 64 / voxel.SectionVolume)
		} else if len(pal) > 1 {
			b = uint(max(4, bits.Len(uint(len(pal)-1))))
		}
		s.Blocks.LoadPacked(pal, ud, b, spanning)
		s.Recount()
	}
	ch.RecomputeHeights(reg.Table())
	ch.Lit = trusted
	return ch, nil
}

func stateFromNBT(reg *voxel.Registry, c Compound) voxel.StateID {
	name := c.String("Name")
	if name == "" {
		return voxel.Air
	}
	var props map[string]string
	if pc := c.Compound("Properties"); len(pc) > 0 {
		props = make(map[string]string, len(pc))
		for k, v := range pc {
			if s, ok := v.(string); ok {
				props[k] = s
			}
		}
	}
	switch name {
	case "minecraft:cave_air", "minecraft:void_air":
		return voxel.Air
	case "minecraft:grass": // renamed in 1.20.3
		name = "minecraft:short_grass"
	}
	return reg.State(name, props)
}

// litTag marks chunk NBT whose light voxellib computed (see DecodeChunk).
const litTag = "voxellib:lit"

// EncodeChunk converts a chunk into 1.18+ chunk NBT.
func EncodeChunk(w *voxel.World, ch *voxel.Chunk) Compound {
	reg := w.Reg
	secs := List{Type: TagCompound}
	var buf [voxel.SectionVolume]voxel.StateID
	for i, s := range ch.Sections {
		sy := w.MinSection() + i
		s.Blocks.Unpack(&buf)
		index := map[voxel.StateID]int{}
		var pal []voxel.StateID
		for _, id := range buf {
			if _, ok := index[id]; !ok {
				index[id] = len(pal)
				pal = append(pal, id)
			}
		}
		palNBT := List{Type: TagCompound}
		for _, id := range pal {
			pc := Compound{"Name": reg.Info(id).Name}
			if props := reg.StateProps(id); len(props) > 0 {
				pp := Compound{}
				for k, v := range props {
					pp[k] = v
				}
				pc["Properties"] = pp
			}
			palNBT.Items = append(palNBT.Items, pc)
		}
		bs := Compound{"palette": palNBT}
		if len(pal) > 1 {
			b := uint(max(4, bits.Len(uint(len(pal)-1))))
			per := 64 / b
			data := make([]int64, (voxel.SectionVolume+int(per)-1)/int(per))
			for k, id := range buf {
				data[uint(k)/per] |= int64(uint64(index[id]) << ((uint(k) % per) * b))
			}
			bs["data"] = data
		}
		sc := Compound{
			"Y":            int8(sy),
			"block_states": bs,
			"biomes":       Compound{"palette": List{Type: TagString, Items: []any{"minecraft:plains"}}},
			"SkyLight":     append([]byte(nil), s.Sky...),
			"BlockLight":   append([]byte(nil), s.Block...),
		}
		secs.Items = append(secs.Items, sc)
	}
	root := Compound{
		"DataVersion":    int32(DataVersion),
		"xPos":           int32(ch.Pos.X),
		"zPos":           int32(ch.Pos.Z),
		"yPos":           int32(w.MinSection()),
		"Status":         "minecraft:full",
		"LastUpdate":     int64(0),
		"InhabitedTime":  int64(0),
		"isLightOn":      int8(0), // let Minecraft relight with its own engine
		"sections":       secs,
		"block_entities": List{Type: TagCompound},
	}
	if ch.Lit {
		root[litTag] = int8(1)
	}
	return root
}

// legacy numeric ids (pre-1.13) for the common terrain blocks.
var legacyIDs = map[byte]string{
	1: "stone", 2: "grass_block", 3: "dirt", 4: "cobblestone", 5: "oak_planks", 7: "bedrock",
	8: "water", 9: "water", 10: "lava", 11: "lava", 12: "sand", 13: "gravel", 14: "gold_ore",
	15: "iron_ore", 16: "coal_ore", 17: "oak_log", 18: "oak_leaves", 20: "glass", 21: "lapis_ore",
	24: "sandstone", 31: "short_grass", 32: "dead_bush", 35: "white_wool", 37: "dandelion", 38: "poppy",
	39: "brown_mushroom", 40: "red_mushroom", 41: "gold_block", 42: "iron_block", 45: "bricks",
	47: "bookshelf", 48: "mossy_cobblestone", 49: "obsidian", 56: "diamond_ore", 57: "diamond_block",
	58: "crafting_table", 73: "redstone_ore", 79: "ice", 80: "snow_block", 81: "cactus", 82: "clay",
	83: "sugar_cane", 86: "pumpkin", 89: "glowstone", 98: "stone_bricks", 103: "melon",
	110: "mycelium", 129: "emerald_ore", 159: "terracotta", 172: "terracotta", 174: "packed_ice",
}

func decodeLegacy(reg *voxel.Registry, s *voxel.Section, blocks, meta []byte) {
	// legacy order is y<<8|z<<4|x too
	var buf [voxel.SectionVolume]voxel.StateID
	cache := map[byte]voxel.StateID{0: voxel.Air}
	for i, b := range blocks {
		id, ok := cache[b]
		if !ok {
			if name, ok2 := legacyIDs[b]; ok2 {
				id = reg.Default(name)
			} else {
				id = reg.Default("stone")
			}
			cache[b] = id
		}
		buf[i] = id
	}
	s.Blocks.Pack(&buf)
	s.Recount()
}

// SortedKeys is a helper for deterministic debugging output.
func SortedKeys(c Compound) []string {
	ks := make([]string, 0, len(c))
	for k := range c {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
