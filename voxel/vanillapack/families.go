package vanillapack

import (
	"fmt"
	"sort"
	"strings"

	"github.com/memmaker/voxellib/voxel/blockmodel"
)

// block is a generated blockstate and the block models it adds.
type block struct {
	state  blockstate
	models map[string]*blockmodel.Model // by id below block/
}

type blockstate struct {
	Variants  map[string]any `json:"variants,omitempty"`
	Multipart []part         `json:"multipart,omitempty"`
}

type part struct {
	When  any `json:"when,omitempty"`
	Apply any `json:"apply"`
}

type variant struct {
	Model  string `json:"model"`
	X      int    `json:"x,omitempty"`
	Y      int    `json:"y,omitempty"`
	UVLock bool   `json:"uvlock,omitempty"`
	Weight int    `json:"weight,omitempty"`
}

func newBlock() *block { return &block{models: map[string]*blockmodel.Model{}} }

// v is a variant drawing block model id.
func v(id string) variant { return variant{Model: "minecraft:block/" + id} }

// rot turns the variant by x and y degrees (any multiple of 90).
func (v variant) rot(x, y int) variant {
	v.X, v.Y = (x%360+360)%360, (y%360+360)%360
	return v
}

func (v variant) lock() variant { v.UVLock = true; return v }

// lockTurned locks the UVs of a turned variant.
func (v variant) lockTurned() variant {
	v.UVLock = v.X != 0 || v.Y != 0
	return v
}

// model adds block model id: parent's elements with textures "var=id"
// ("block/x" ids; "#var" refers to another variable).
func (b *block) model(id, parent string, tex ...string) {
	m := &blockmodel.Model{Textures: map[string]string{}}
	if parent != "" {
		m.Parent = "minecraft:block/" + parent
	}
	for _, t := range tex {
		k, val, _ := strings.Cut(t, "=")
		if !strings.HasPrefix(val, "#") && !strings.Contains(val, ":") {
			val = "minecraft:" + val
		}
		m.Textures[k] = val
	}
	b.models[id] = m
}

// variant sets the variant (several: picked at random) of the state key.
func (b *block) variant(key string, vs ...variant) {
	if b.state.Variants == nil {
		b.state.Variants = map[string]any{}
	}
	if len(vs) == 1 {
		b.state.Variants[key] = vs[0]
	} else {
		b.state.Variants[key] = vs
	}
}

// part adds a multipart case: the variants drawn while when holds (nil:
// always).
func (b *block) part(when any, vs ...variant) {
	p := part{When: when, Apply: vs[0]}
	if when == nil {
		p.When = nil
	}
	if len(vs) > 1 {
		p.Apply = vs
	}
	b.state.Multipart = append(b.state.Multipart, p)
}

type props map[string]string

// key is the variant key of the properties: "k=v,..." by name.
func (p props) key() string {
	ks := make([]string, 0, len(p))
	for k, v := range p {
		ks = append(ks, k+"="+v)
	}
	sort.Strings(ks)
	return strings.Join(ks, ",")
}

// each calls f with every combination of the properties in spec
// ("facing=east,north,south,west half=bottom,top").
func each(spec string, f func(p props)) {
	type prop struct {
		name string
		vals []string
	}
	var ps []prop
	for _, w := range strings.Fields(spec) {
		k, vs, _ := strings.Cut(w, "=")
		ps = append(ps, prop{k, strings.Split(vs, ",")})
	}
	var rec func(i int, p props)
	rec = func(i int, p props) {
		if i == len(ps) {
			q := props{}
			for k, v := range p {
				q[k] = v
			}
			f(q)
			return
		}
		for _, val := range ps[i].vals {
			p[ps[i].name] = val
			rec(i+1, p)
		}
	}
	rec(0, props{})
}

const (
	horizontal = "east,north,south,west"
	all6       = "down,east,north,south,up,west"
)

// yOf is the y turn of a model facing north to face f.
func yOf(f string) int {
	return map[string]int{"north": 0, "east": 90, "south": 180, "west": 270}[f]
}

// xyOf turns a model facing north to face any of six directions (up
// models face up after x 270).
func xyOf(f string) (int, int) {
	switch f {
	case "up":
		return 270, 0
	case "down":
		return 90, 0
	}
	return 0, yOf(f)
}

// single is a block of one model, drawn the same in every state.
func single(model string) *block {
	b := newBlock()
	b.variant("", v(model))
	return b
}

// cubeAll is a cube of one texture.
func cubeAll(name, tex string) *block {
	b := single(name)
	b.model(name, "cube_all", "all="+tex)
	return b
}

// shapeTex are the bottom, top and side textures a slab, stair or wall
// takes from its full block.
type shapeTex struct{ bottom, top, side string }

// baseTex reads the textures of a full block from its generated model.
func (p *Pack) baseTex(base string) (shapeTex, bool) {
	b := p.block(base)
	if b == nil {
		return shapeTex{}, false
	}
	m := b.models[base]
	if m == nil {
		return shapeTex{}, false
	}
	t := func(k string) string { return strings.TrimPrefix(m.Textures[k], "minecraft:") }
	switch strings.TrimPrefix(m.Parent, "minecraft:") {
	case "block/cube_all":
		return shapeTex{t("all"), t("all"), t("all")}, true
	case "block/cube_column":
		return shapeTex{t("end"), t("end"), t("side")}, true
	case "block/cube_bottom_top":
		return shapeTex{t("bottom"), t("top"), t("side")}, true
	}
	return shapeTex{}, false
}

// baseOf finds the full block a shape of name (with its suffix cut off)
// is made of.
func (p *Pack) baseOf(stem string) string {
	if b, ok := shapeBases[stem]; ok {
		return b
	}
	for _, c := range []string{stem, stem + "s", stem + "_planks", stem + "_block"} {
		if _, ok := p.baseTex(c); ok {
			return c
		}
	}
	return ""
}

// shapeBases are the full blocks of shapes whose name doesn't tell it.
var shapeBases = map[string]string{
	"petrified_oak": "oak_planks",
	"smooth_stone":  "smooth_stone",
}

func slab(p *Pack, name, base string, t shapeTex) *block {
	b := newBlock()
	tex := []string{"bottom=" + t.bottom, "top=" + t.top, "side=" + t.side}
	b.model(name, "slab", tex...)
	b.model(name+"_top", "slab_top", tex...)
	b.variant("type=bottom", v(name))
	b.variant("type=top", v(name+"_top"))
	b.variant("type=double", v(base))
	return b
}

func stairs(name string, t shapeTex) *block {
	b := newBlock()
	tex := []string{"bottom=" + t.bottom, "top=" + t.top, "side=" + t.side}
	b.model(name, "stairs", tex...)
	b.model(name+"_inner", "inner_stairs", tex...)
	b.model(name+"_outer", "outer_stairs", tex...)
	each("facing="+horizontal+" half=bottom,top shape=inner_left,inner_right,outer_left,outer_right,straight", func(p props) {
		y := yOf(p["facing"]) - 90
		x := 0
		left := strings.HasSuffix(p["shape"], "_left")
		right := strings.HasSuffix(p["shape"], "_right")
		if p["half"] == "top" {
			x = 180
			if right {
				y += 90
			}
		} else if left {
			y -= 90
		}
		m := name
		if s, _, ok := strings.Cut(p["shape"], "_"); ok {
			m += "_" + s
		}
		b.variant(p.key(), v(m).rot(x, y).lockTurned())
	})
	return b
}

func wall(name, tex string) *block {
	b := newBlock()
	b.model(name+"_post", "template_wall_post", "wall="+tex)
	b.model(name+"_side", "template_wall_side", "wall="+tex)
	b.model(name+"_side_tall", "template_wall_side_tall", "wall="+tex)
	b.model(name+"_inventory", "wall_inventory", "wall="+tex)
	b.part(map[string]string{"up": "true"}, v(name+"_post"))
	for _, h := range []string{"low", "tall"} {
		m := name + "_side"
		if h == "tall" {
			m += "_tall"
		}
		for _, f := range []string{"north", "east", "south", "west"} {
			b.part(map[string]string{f: h}, v(m).rot(0, yOf(f)).lock())
		}
	}
	return b
}

func fence(name, tex string) *block {
	b := newBlock()
	b.model(name+"_post", "fence_post", "texture="+tex)
	b.model(name+"_side", "fence_side", "texture="+tex)
	b.model(name+"_inventory", "fence_inventory", "texture="+tex)
	b.part(nil, v(name+"_post"))
	for _, f := range []string{"north", "east", "south", "west"} {
		b.part(map[string]string{f: "true"}, v(name+"_side").rot(0, yOf(f)).lock())
	}
	return b
}

func fenceGate(name, tex string) *block {
	b := newBlock()
	for _, s := range []string{"", "_open", "_wall", "_wall_open"} {
		b.model(name+s, "template_fence_gate"+s, "texture="+tex)
	}
	each("facing="+horizontal+" in_wall=false,true open=false,true", func(p props) {
		m := name
		if p["in_wall"] == "true" {
			m += "_wall"
		}
		if p["open"] == "true" {
			m += "_open"
		}
		b.variant(p.key(), v(m).rot(0, yOf(p["facing"])+180).lock())
	})
	return b
}

func door(name string) *block {
	b := newBlock()
	for _, h := range []string{"bottom", "top"} {
		for _, s := range []string{"left", "right"} {
			for _, o := range []string{"", "_open"} {
				m := name + "_" + h + "_" + s + o
				b.model(m, "door_"+h+"_"+s+o, "bottom=block/"+name+"_bottom", "top=block/"+name+"_top")
			}
		}
	}
	each("facing="+horizontal+" half=lower,upper hinge=left,right open=false,true", func(p props) {
		h := map[string]string{"lower": "bottom", "upper": "top"}[p["half"]]
		m := name + "_" + h + "_" + p["hinge"]
		y := yOf(p["facing"]) - 90
		if p["open"] == "true" {
			m += "_open"
			if p["hinge"] == "left" {
				y += 90
			} else {
				y -= 90
			}
		}
		b.variant(p.key(), v(m).rot(0, y))
	})
	return b
}

// trapdoor: orientable ones turn closed too, and flip open on the top
// half.
func trapdoor(name string, orientable bool) *block {
	b := newBlock()
	tpl := "template_trapdoor_"
	if orientable {
		tpl = "template_orientable_trapdoor_"
	}
	for _, s := range []string{"bottom", "top", "open"} {
		b.model(name+"_"+s, tpl+s, "texture=block/"+name)
	}
	each("facing="+horizontal+" half=bottom,top open=false,true", func(p props) {
		y := yOf(p["facing"])
		x := 0
		m := name + "_" + p["half"]
		if p["open"] == "true" {
			m = name + "_open"
			if orientable && p["half"] == "top" {
				x, y = 180, y+180
			}
		} else if !orientable {
			y = 0
		}
		b.variant(p.key(), v(m).rot(x, y))
	})
	return b
}

func button(name, tex string) *block {
	b := newBlock()
	b.model(name, "button", "texture="+tex)
	b.model(name+"_pressed", "button_pressed", "texture="+tex)
	b.model(name+"_inventory", "button_inventory", "texture="+tex)
	each("face=ceiling,floor,wall facing="+horizontal+" powered=false,true", func(p props) {
		m := name
		if p["powered"] == "true" {
			m += "_pressed"
		}
		y := yOf(p["facing"])
		va := v(m)
		switch p["face"] {
		case "ceiling":
			va = va.rot(180, y+180)
		case "floor":
			va = va.rot(0, y)
		case "wall":
			va = va.rot(90, y).lock()
		}
		b.variant(p.key(), va)
	})
	return b
}

func pressurePlate(name, tex string, weighted bool) *block {
	b := newBlock()
	b.model(name, "pressure_plate_up", "texture="+tex)
	b.model(name+"_down", "pressure_plate_down", "texture="+tex)
	if weighted {
		for i := range 16 {
			m := name + "_down"
			if i == 0 {
				m = name
			}
			b.variant(fmt.Sprintf("power=%d", i), v(m))
		}
		return b
	}
	b.variant("powered=false", v(name))
	b.variant("powered=true", v(name+"_down"))
	return b
}

func pane(name, pane, edge string) *block {
	b := newBlock()
	for _, s := range []string{"post", "side", "side_alt", "noside", "noside_alt"} {
		b.model(name+"_"+s, "template_glass_pane_"+s, "pane="+pane, "edge="+edge)
	}
	b.part(nil, v(name+"_post"))
	b.part(map[string]string{"north": "true"}, v(name+"_side"))
	b.part(map[string]string{"east": "true"}, v(name+"_side").rot(0, 90))
	b.part(map[string]string{"south": "true"}, v(name+"_side_alt"))
	b.part(map[string]string{"west": "true"}, v(name+"_side_alt").rot(0, 90))
	b.part(map[string]string{"north": "false"}, v(name+"_noside"))
	b.part(map[string]string{"east": "false"}, v(name+"_noside_alt"))
	b.part(map[string]string{"south": "false"}, v(name+"_noside_alt").rot(0, 90))
	b.part(map[string]string{"west": "false"}, v(name+"_noside").rot(0, 270))
	return b
}

func bars(name string) *block {
	b := newBlock()
	tex := "block/" + name
	for _, s := range []string{"post_ends", "post", "cap", "cap_alt", "side", "side_alt"} {
		b.model(name+"_"+s, "template_bars_"+s, "bars="+tex, "edge="+tex)
	}
	none := map[string]string{"east": "false", "north": "false", "south": "false", "west": "false"}
	only := func(f string) map[string]string {
		m := map[string]string{}
		for k := range none {
			m[k] = "false"
		}
		m[f] = "true"
		return m
	}
	b.part(nil, v(name+"_post_ends"))
	b.part(none, v(name+"_post"))
	b.part(only("north"), v(name+"_cap"))
	b.part(only("east"), v(name+"_cap").rot(0, 90))
	b.part(only("south"), v(name+"_cap_alt"))
	b.part(only("west"), v(name+"_cap_alt").rot(0, 90))
	b.part(map[string]string{"north": "true"}, v(name+"_side"))
	b.part(map[string]string{"east": "true"}, v(name+"_side").rot(0, 90))
	b.part(map[string]string{"south": "true"}, v(name+"_side_alt"))
	b.part(map[string]string{"west": "true"}, v(name+"_side_alt").rot(0, 90))
	return b
}

// pillar is a block along an axis: a column model, lying along x and z
// (turned, or its own horizontal model).
func pillar(name, parent, horiz string, tex ...string) *block {
	b := newBlock()
	b.model(name, parent, tex...)
	h := name
	if horiz != "" {
		h = name + "_horizontal"
		b.model(h, horiz, tex...)
	}
	b.variant("axis=y", v(name))
	b.variant("axis=x", v(h).rot(90, 90))
	b.variant("axis=z", v(h).rot(90, 0))
	return b
}

// facingY turns a north-facing model to the horizontal facings, offset by
// deg.
func facingY(name string, deg int) *block {
	b := newBlock()
	for _, f := range strings.Split(horizontal, ",") {
		b.variant("facing="+f, v(name).rot(0, yOf(f)+deg))
	}
	return b
}

func sign(name, tex, particle string) *block {
	b := newBlock()
	for i := range 4 {
		b.model(fmt.Sprintf("%s_rot_%d", name, i), fmt.Sprintf("template_sign_rot_%d", i), "all="+tex, "particle="+particle)
	}
	for r := range 16 {
		b.variant(fmt.Sprintf("rotation=%d", r), v(fmt.Sprintf("%s_rot_%d", name, r%4)).rot(0, r/4*90))
	}
	return b
}

func hangingSign(name, tex, particle string) *block {
	b := newBlock()
	for i := range 4 {
		b.model(fmt.Sprintf("%s_rot_%d", name, i), fmt.Sprintf("template_hanging_sign_rot_%d", i), "all="+tex, "particle="+particle)
		b.model(fmt.Sprintf("%s_attached_rot_%d", name, i), fmt.Sprintf("template_attached_hanging_sign_rot_%d", i), "all="+tex, "particle="+particle)
	}
	each("attached=false,true rotation=0,1,2,3,4,5,6,7,8,9,10,11,12,13,14,15", func(p props) {
		var r int
		fmt.Sscan(p["rotation"], &r)
		m := name + "_rot_"
		if p["attached"] == "true" {
			m = name + "_attached_rot_"
		}
		b.variant(p.key(), v(fmt.Sprintf("%s%d", m, r%4)).rot(0, r/4*90))
	})
	return b
}

func bed(name string) *block {
	b := newBlock()
	t := "block/" + name
	b.model(name+"_foot", "template_bed_foot", "east="+t+"_foot_east", "south="+t+"_foot_south", "up="+t+"_foot_up", "west="+t+"_foot_west")
	b.model(name+"_head", "template_bed_head", "east="+t+"_head_east", "up="+t+"_head_up", "west="+t+"_head_west")
	each("facing="+horizontal+" part=foot,head", func(p props) {
		b.variant(p.key(), v(name+"_"+p["part"]).rot(0, yOf(p["facing"])))
	})
	return b
}

func candle(name string) *block {
	b := newBlock()
	for i, n := range []string{"one_candle", "two_candles", "three_candles", "four_candles"} {
		tpl := "template_" + strings.TrimPrefix(n, "one_")
		for _, lit := range []string{"false", "true"} {
			m, t := name+"_"+n, "block/"+name
			if lit == "true" {
				m, t = m+"_lit", t+"_lit"
			}
			b.model(m, tpl, "all="+t, "particle="+t)
			b.variant(fmt.Sprintf("candles=%d,lit=%s", i+1, lit), v(m))
		}
	}
	return b
}

func candleCake(name, candle string) *block {
	b := newBlock()
	tex := []string{"bottom=block/cake_bottom", "side=block/cake_side", "top=block/cake_top", "particle=block/cake_side"}
	b.model(name, "template_cake_with_candle", append(tex, "candle=block/"+candle)...)
	b.model(name+"_lit", "template_cake_with_candle", append(tex, "candle=block/"+candle+"_lit")...)
	b.variant("lit=false", v(name))
	b.variant("lit=true", v(name+"_lit"))
	return b
}

func doublePlant(name, parent string) *block {
	b := newBlock()
	b.model(name+"_bottom", parent, "cross=block/"+name+"_bottom")
	b.model(name+"_top", parent, "cross=block/"+name+"_top")
	b.variant("half=lower", v(name+"_bottom"))
	b.variant("half=upper", v(name+"_top"))
	return b
}

// lantern: standing and hanging.
func lantern(name string) *block {
	b := newBlock()
	b.model(name, "template_lantern", "lantern=block/"+name)
	b.model(name+"_hanging", "template_hanging_lantern", "lantern=block/"+name)
	b.variant("hanging=false", v(name))
	b.variant("hanging=true", v(name+"_hanging"))
	return b
}

// randomY draws the model turned at random by y 0, 90, 180 or 270.
func randomY(b *block, key, model string) {
	b.variant(key, v(model), v(model).rot(0, 90), v(model).rot(0, 180), v(model).rot(0, 270))
}

func shelf(name, wood string) *block {
	b := newBlock()
	for _, s := range []string{"", "_center", "_left", "_right", "_unconnected", "_unpowered"} {
		tpl := "template_shelf" + s
		if s == "" {
			tpl = "template_shelf_body"
		}
		b.model(name+s, tpl, "all=block/"+name, "particle="+strippedLog(wood))
	}
	fs := []string{"north", "east", "south", "west"}
	and := func(f string, kv ...string) map[string]any {
		cs := []any{map[string]string{"facing": f}}
		for _, c := range kv {
			k, val, _ := strings.Cut(c, "=")
			cs = append(cs, map[string]string{k: val})
		}
		return map[string]any{"AND": cs}
	}
	for _, f := range fs {
		b.part(map[string]string{"facing": f}, v(name).rot(0, yOf(f)))
	}
	for _, f := range fs {
		b.part(and(f, "powered=false"), v(name+"_unpowered").rot(0, yOf(f)))
	}
	for _, s := range []string{"unconnected", "left", "center", "right"} {
		for _, f := range fs {
			b.part(and(f, "powered=true", "side_chain="+s), v(name+"_"+s).rot(0, yOf(f)))
		}
	}
	return b
}
