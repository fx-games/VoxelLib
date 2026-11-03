package bedrock

import "strings"

// Blocks the Java client draws with block-entity renderers (chests,
// shulker boxes, banners, skulls, the bell's body) have no block model
// with elements. These make one from the renderer's boxes and the entity
// texture's cube layout, so they draw as blocks.

// ebox is one box: block-space from/to (1/16), the texture offset u,v and
// the box's size on the texture (w along x, h along y, d along z), laid
// out like an entity model's cube: the front (north) at (u+d, v+d), the
// east side left of it, the west side and the back to its right, the top
// at (u+d, v) and the bottom beside it.
type ebox struct {
	from, to [3]float32
	u, v     float32
	w, h, d  float32
	tex      string // texture variable
	tint     bool
	inset    float32 // shrink the box (not its texture), to sit inside another
}

type emodel struct {
	textures map[string]string
	size     map[string][2]float32 // texture variable → pixel size
	boxes    []ebox
}

func (m emodel) json() map[string]any {
	var elems []any
	for _, b := range m.boxes {
		sz := m.size[b.tex]
		su, sv := 16/sz[0], 16/sz[1]
		rect := func(u0, v0, u1, v1 float32) [4]float32 { return [4]float32{u0 * su, v0 * sv, u1 * su, v1 * sv} }
		u, v, w, h, d := b.u, b.v, b.w, b.h, b.d
		top := rect(u+d+w, v+d, u+d, v)
		faces := map[string]any{
			"north": rect(u+d, v+d, u+d+w, v+d+h),
			"east":  rect(u, v+d, u+d, v+d+h),
			"west":  rect(u+d+w, v+d, u+d+w+d, v+d+h),
			"south": rect(u+d+w+d, v+d, u+d+w+d+w, v+d+h),
			"up":    top,
			"down":  rect(u+d+w+w, v+d, u+d+w, v),
		}
		out := map[string]any{}
		for f, uv := range faces {
			face := map[string]any{"uv": uv, "texture": "#" + b.tex}
			if b.tint {
				face["tintindex"] = 0
			}
			out[f] = face
		}
		from, to := b.from, b.to
		for i := range 3 {
			from[i] += b.inset
			to[i] -= b.inset
		}
		elems = append(elems, map[string]any{"from": from, "to": to, "faces": out})
	}
	return map[string]any{"textures": m.textures, "elements": elems}
}

// entityModel returns the generated model for a state (facing north, the
// caller turns it) and its id, or false for an ordinary block.
func entityModel(s JavaState, size func(tex string) [2]float32) (id string, m emodel, ok bool) {
	name := s.Name
	one := func(tex string) (map[string]string, map[string][2]float32) {
		return map[string]string{"t": tex, "particle": tex}, map[string][2]float32{"t": size(tex)}
	}
	switch {
	case name == "chest" || name == "trapped_chest":
		base := "minecraft:entity/chest/" + map[string]string{"chest": "normal", "trapped_chest": "trapped"}[name]
		typ := s.Props["type"]
		id = "voxellib:block/" + name + "_" + typ
		switch typ {
		case "left": // the other half is east (for a chest facing north)
			m.textures, m.size = one(base + "_left")
			m.boxes = []ebox{
				{from: [3]float32{1, 0, 1}, to: [3]float32{16, 10, 15}, u: 0, v: 19, w: 15, h: 10, d: 14, tex: "t"},
				{from: [3]float32{1, 9, 1}, to: [3]float32{16, 14, 15}, u: 0, v: 0, w: 15, h: 5, d: 14, tex: "t"},
				{from: [3]float32{15, 7, 0}, to: [3]float32{16, 11, 1}, u: 0, v: 0, w: 1, h: 4, d: 1, tex: "t"},
			}
		case "right":
			m.textures, m.size = one(base + "_right")
			m.boxes = []ebox{
				{from: [3]float32{0, 0, 1}, to: [3]float32{15, 10, 15}, u: 0, v: 19, w: 15, h: 10, d: 14, tex: "t"},
				{from: [3]float32{0, 9, 1}, to: [3]float32{15, 14, 15}, u: 0, v: 0, w: 15, h: 5, d: 14, tex: "t"},
				{from: [3]float32{0, 7, 0}, to: [3]float32{1, 11, 1}, u: 0, v: 0, w: 1, h: 4, d: 1, tex: "t"},
			}
		default:
			id = "voxellib:block/" + name
			m.textures, m.size = one(base)
			m.boxes = []ebox{
				{from: [3]float32{1, 0, 1}, to: [3]float32{15, 10, 15}, u: 0, v: 19, w: 14, h: 10, d: 14, tex: "t"},
				{from: [3]float32{1, 9, 1}, to: [3]float32{15, 14, 15}, u: 0, v: 0, w: 14, h: 5, d: 14, tex: "t"},
				{from: [3]float32{7, 7, 0}, to: [3]float32{9, 11, 1}, u: 0, v: 0, w: 2, h: 4, d: 1, tex: "t"},
			}
		}
		// The bottom's top and the lid's underside meet inside: keep the
		// bottom one pixel lower so the lid's edge doesn't flicker.
		m.boxes[0].to[1] = 9
		m.boxes[0].h = 9
		return id, m, true
	case name == "shulker_box" || strings.HasSuffix(name, "_shulker_box"):
		tex := "minecraft:entity/shulker/shulker"
		if c := strings.TrimSuffix(name, "_shulker_box"); c != name {
			tex += "_" + c
		}
		m.textures, m.size = one(tex)
		m.boxes = []ebox{
			{from: [3]float32{0, 4, 0}, to: [3]float32{16, 16, 16}, u: 0, v: 0, w: 16, h: 12, d: 16, tex: "t"},
			{from: [3]float32{0, 0, 0}, to: [3]float32{16, 8, 16}, u: 0, v: 28, w: 16, h: 8, d: 16, tex: "t", inset: 0.02},
		}
		return "voxellib:block/" + name, m, true
	case strings.HasSuffix(name, "_banner"):
		// The renderer draws the banner at 2/3 scale: a 20x40 flag under a
		// 20x2 bar, on a 42-high pole when standing.
		const k = 2.0 / 3
		// Packs without banner_base (Pixel Perfection) have the pole and bar
		// in base, laid out the same.
		wood := "minecraft:entity/banner/banner_base"
		if size(wood)[0] == 0 {
			wood = "minecraft:entity/banner/base"
		}
		m.textures = map[string]string{"flag": "minecraft:entity/banner/base", "wood": wood,
			"particle": "minecraft:block/oak_planks"}
		m.size = map[string][2]float32{"flag": size("minecraft:entity/banner/base"), "wood": size(wood)}
		x0, x1 := float32(8-10*k), float32(8+10*k)
		if strings.HasSuffix(name, "_wall_banner") {
			// Against the south wall, hanging into the block below.
			top := float32(16 - 0.5)
			m.boxes = []ebox{
				{from: [3]float32{x0, top - 2*k, 16 - 2*k}, to: [3]float32{x1, top, 16}, u: 0, v: 42, w: 20, h: 2, d: 2, tex: "wood"},
				{from: [3]float32{x0, top - 2*k - 40*k, 16 - 3*k}, to: [3]float32{x1, top - 2*k, 16 - 2*k}, u: 0, v: 0, w: 20, h: 40, d: 1, tex: "flag", tint: true},
			}
			return "voxellib:block/wall_banner", m, true
		}
		p0, p1 := float32(8-k), float32(8+k)
		top := float32(42 * k)
		m.boxes = []ebox{
			{from: [3]float32{p0, 0, p0}, to: [3]float32{p1, top, p1}, u: 44, v: 0, w: 2, h: 42, d: 2, tex: "wood"},
			{from: [3]float32{x0, top - 2*k, p0}, to: [3]float32{x1, top, p1}, u: 0, v: 42, w: 20, h: 2, d: 2, tex: "wood"},
			{from: [3]float32{x0, top - 2*k - 40*k, p0 - k}, to: [3]float32{x1, top - 2*k, p0}, u: 0, v: 0, w: 20, h: 40, d: 1, tex: "flag", tint: true},
		}
		return "voxellib:block/banner", m, true
	case strings.HasSuffix(name, "_skull") || strings.HasSuffix(name, "_head"):
		kind := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(name, "_skull"), "_head"), "_wall")
		tex := map[string]string{"skeleton": "minecraft:entity/skeleton/skeleton", "wither_skeleton": "minecraft:entity/skeleton/wither_skeleton",
			"zombie": "minecraft:entity/zombie/zombie", "creeper": "minecraft:entity/creeper/creeper",
			"player": "minecraft:entity/player/wide/steve", "piglin": "minecraft:entity/piglin/piglin"}[kind]
		if tex == "" {
			tex = "minecraft:entity/skeleton/skeleton"
		}
		m.textures, m.size = one(tex)
		b := ebox{from: [3]float32{4, 0, 4}, to: [3]float32{12, 8, 12}, u: 0, v: 0, w: 8, h: 8, d: 8, tex: "t"}
		id = "voxellib:block/" + kind + "_skull"
		if strings.Contains(name, "_wall_") {
			b.from, b.to = [3]float32{4, 4, 8}, [3]float32{12, 12, 16}
			id = "voxellib:block/" + kind + "_wall_skull"
		}
		m.boxes = []ebox{b}
		return id, m, true
	case name == "bell":
		m.textures, m.size = one("minecraft:entity/bell/bell_body")
		m.boxes = []ebox{
			{from: [3]float32{5, 6, 5}, to: [3]float32{11, 13, 11}, u: 0, v: 0, w: 6, h: 7, d: 6, tex: "t"},
			{from: [3]float32{4, 4, 4}, to: [3]float32{12, 6, 12}, u: 0, v: 13, w: 8, h: 2, d: 8, tex: "t"},
		}
		return "voxellib:block/bell_body", m, true
	}
	return "", m, false
}

// entityTurn is the y rotation that turns a generated (north-facing)
// model to the state.
func entityTurn(s JavaState) int {
	if r, ok := s.Props["rotation"]; ok { // standing banners and skulls: 0 faces south
		n := 0
		for _, c := range r {
			n = n*10 + int(c-'0')
		}
		return (180 + (n+2)/4%4*90) % 360 // the nearest quarter turn
	}
	return yTurn(s.Props["facing"])
}

// yTurn is the blockstate y rotation of a north-facing model.
func yTurn(facing string) int {
	return map[string]int{"east": 90, "south": 180, "west": 270}[facing]
}
