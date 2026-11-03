package bedrock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/voxel/anvil"
)

// ExportOptions configures Export.
type ExportOptions struct {
	Jar  string // Java client jar: blockstates, models and textures
	Out  string // VoxelMap directory to write
	Name string // the map's name ("" = the world's, formatting codes removed)
	Logf func(format string, args ...any)
}

// Export converts the overworld of a Bedrock world into a VoxelMap (see
// voxel/voxelmap): each Bedrock block becomes the Java Edition state that
// looks the same, drawn with the jar's models and textures, which are
// copied into the map. Blocks the Java client draws as block entities
// (chests, shulker boxes, banners, skulls) get generated models. Chunks
// past the world's edge come from the "minecraft:flat" generator
// (worldgen/flat), like Bedrock's own flat worlds.
//
// The map keeps y 0..255 of the world (VoxelMap's height).
func Export(w *World, o ExportOptions) error {
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	j, err := OpenJar(o.Jar)
	if err != nil {
		return err
	}
	defer j.Close()
	e := &exporter{w: w, j: j, o: o, secs: map[[3]int]*[4096]uint16{}, index: map[string]uint16{},
		bes: w.BlockEntities(0), sturdy: map[uint16]bool{}}
	e.intern(JavaState{Name: "air"})
	if err := e.read(); err != nil {
		return err
	}
	e.fix()
	if err := os.MkdirAll(o.Out, 0o755); err != nil {
		return err
	}
	keys, err := e.blocks()
	if err != nil {
		return err
	}
	if err := e.regions(keys); err != nil {
		return err
	}
	if err := e.copyAssets(); err != nil {
		return err
	}
	return e.mapJSON()
}

type exporter struct {
	w      *World
	j      *Jar
	o      ExportOptions
	secs   map[[3]int]*[4096]uint16 // chunk x, section y, chunk z → state per y<<8|z<<4|x
	states []JavaState
	index  map[string]uint16
	bes    map[[3]int]map[string]any
	sturdy map[uint16]bool
	gen    map[string]emodel // generated models by id
}

func (e *exporter) intern(s JavaState) uint16 {
	k := s.String()
	if i, ok := e.index[k]; ok {
		return i
	}
	i := uint16(len(e.states))
	e.states = append(e.states, s)
	e.index[k] = i
	return i
}

// read translates every section of y 0..255.
func (e *exporter) read() error {
	cache := map[string]uint16{}
	unknown := map[string]int{}
	water := e.intern(JavaState{Name: "water", Props: map[string]string{"level": "0"}})
	for _, p := range e.w.SubChunks() {
		if p.Dim != 0 || p.Y < 0 || p.Y > 15 {
			continue
		}
		layers, err := e.w.SubChunk(p)
		if err != nil {
			return fmt.Errorf("section %v: %w", p, err)
		}
		if len(layers) == 0 {
			continue
		}
		tr := func(s *Storage) []uint16 {
			ids := make([]uint16, len(s.Palette))
			for i, b := range s.Palette {
				k := b.Key()
				id, ok := cache[k]
				if !ok {
					js, known := Translate(b)
					if !known { // a block both editions name alike
						js = JavaState{Name: strings.TrimPrefix(b.Name, "minecraft:")}
						known = true
					}
					if known && js.Name != "" && (js.Name == "air" || e.j.HasBlock(js.Name)) {
						id = e.intern(js)
					} else {
						unknown[k]++
					}
					cache[k] = id
				}
				ids[i] = id
			}
			return ids
		}
		var sec [4096]uint16
		any := false
		l0 := layers[0]
		ids := tr(l0)
		var ids1 []uint16
		if len(layers) > 1 {
			ids1 = tr(layers[1])
		}
		for x := range 16 {
			for z := range 16 {
				for y := range 16 {
					bi := x<<8 | z<<4 | y
					s := ids[l0.Index[bi]]
					if s == 0 && ids1 != nil && e.states[ids1[layers[1].Index[bi]]].Name == "water" {
						s = water // water held in the second layer
					}
					if s != 0 {
						sec[y<<8|z<<4|x] = s
						any = true
					}
				}
			}
		}
		if any {
			e.secs[[3]int{p.X, p.Y, p.Z}] = &sec
		}
	}
	names := make([]string, 0, len(unknown))
	for k := range unknown {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		e.o.Logf("bedrock: no Java block for %s (left as air)", k)
	}
	e.o.Logf("bedrock: %d sections, %d Java states", len(e.secs), len(e.states))
	return nil
}

func (e *exporter) at(x, y, z int) uint16 {
	if y < 0 || y > 255 {
		return 0
	}
	s := e.secs[[3]int{x >> 4, y >> 4, z >> 4}]
	if s == nil {
		return 0
	}
	return s[(y&15)<<8|(z&15)<<4|x&15]
}

func (e *exporter) set(x, y, z int, id uint16) {
	if s := e.secs[[3]int{x >> 4, y >> 4, z >> 4}]; s != nil {
		s[(y&15)<<8|(z&15)<<4|x&15] = id
	}
}

func (e *exporter) state(x, y, z int) JavaState { return e.states[e.at(x, y, z)] }

var dirs = map[string][2]int{"north": {0, -1}, "south": {0, 1}, "west": {-1, 0}, "east": {1, 0}}

// clockwise turns a horizontal facing a quarter clockwise seen from above.
func clockwise(f string) string {
	return map[string]string{"north": "east", "east": "south", "south": "west", "west": "north"}[f]
}

func counterClockwise(f string) string { return clockwise(clockwise(clockwise(f))) }

func opposite(f string) string { return clockwise(clockwise(f)) }

// fix sets what Bedrock leaves to the neighbours or to block entities:
// stair corners, fence, pane and bar arms, door halves, vines under a
// ceiling, the upper halves of tall plants, chest pairs, bed and banner
// colours, potted plants and skulls.
func (e *exporter) fix() {
	type pos struct{ x, y, z int }
	var todo []pos
	for k, s := range e.secs {
		for i, id := range s {
			if id != 0 && fixable(e.states[id].Name) {
				todo = append(todo, pos{k[0]<<4 | i&15, k[1]<<4 | i>>8, k[2]<<4 | i>>4&15})
			}
		}
	}
	type change struct {
		p  pos
		id uint16
	}
	var out []change
	for _, p := range todo {
		s := e.state(p.x, p.y, p.z)
		n := e.fixOne(p.x, p.y, p.z, s)
		if n.String() != s.String() {
			out = append(out, change{p, e.intern(n)})
		}
	}
	for _, c := range out { // after all reads: fixes read the original neighbours
		e.set(c.p.x, c.p.y, c.p.z, c.id)
	}
	e.o.Logf("bedrock: %d blocks shaped by their neighbours or block entities", len(out))
}

func fixable(n string) bool {
	return strings.HasSuffix(n, "_stairs") || strings.HasSuffix(n, "_fence") || isPane(n) ||
		strings.HasSuffix(n, "_door") || n == "vine" || isTallPlant(n) || n == "chest" || n == "trapped_chest" ||
		strings.HasSuffix(n, "_bed") || strings.HasSuffix(n, "_banner") || n == "flower_pot" || strings.HasSuffix(n, "_skull")
}

func isPane(n string) bool { return strings.HasSuffix(n, "_pane") || n == "iron_bars" }

func isTallPlant(n string) bool {
	switch n {
	case "sunflower", "lilac", "tall_grass", "large_fern", "rose_bush", "peony":
		return true
	}
	return false
}

func cloneProps(p map[string]string) map[string]string {
	c := make(map[string]string, len(p))
	for k, v := range p {
		c[k] = v
	}
	return c
}

func (e *exporter) fixOne(x, y, z int, s JavaState) JavaState {
	p := cloneProps(s.Props)
	n := JavaState{s.Name, p}
	nb := func(f string) JavaState { d := dirs[f]; return e.state(x+d[0], y, z+d[1]) }
	be := e.bes[[3]int{x, y, z}]
	switch name := s.Name; {
	case strings.HasSuffix(name, "_stairs"):
		p["shape"] = e.stairShape(x, y, z, s)
	case strings.HasSuffix(name, "_fence"):
		for f := range dirs {
			o := nb(f)
			p[f] = fmt.Sprint(sameFence(name, o.Name) ||
				strings.HasSuffix(o.Name, "_fence_gate") && axisOf(o.Props["facing"]) == axisOf(clockwise(f)) ||
				!connectException(o.Name) && e.isSturdy(e.index[o.String()]))
		}
	case isPane(name):
		for f := range dirs {
			o := nb(f)
			p[f] = fmt.Sprint(isPane(o.Name) || isWall(o.Name) || !connectException(o.Name) && e.isSturdy(e.index[o.String()]))
		}
	case strings.HasSuffix(name, "_door"):
		if p["half"] == "upper" {
			if lo := e.state(x, y-1, z); lo.Name == name {
				p["facing"], p["open"] = lo.Props["facing"], lo.Props["open"]
			}
		} else if up := e.state(x, y+1, z); up.Name == name {
			p["hinge"] = up.Props["hinge"]
		}
	case name == "vine":
		if e.isSturdy(e.at(x, y+1, z)) {
			p["up"] = "true"
		}
	case isTallPlant(name):
		if p["half"] == "upper" {
			if lo := e.state(x, y-1, z); isTallPlant(lo.Name) {
				n.Name = lo.Name
			}
		}
	case name == "chest" || name == "trapped_chest":
		if be != nil && be["pairx"] != nil {
			dx, dz := int(Int(be, "pairx"))-x, int(Int(be, "pairz"))-z
			f := p["facing"]
			switch [2]int{dx, dz} {
			case dirs[clockwise(f)]:
				p["type"] = "left"
			case dirs[counterClockwise(f)]:
				p["type"] = "right"
			}
		}
	case strings.HasSuffix(name, "_bed"):
		if be != nil {
			n.Name = dyeColors[int(Int(be, "color"))&15] + "_bed"
		}
	case strings.HasSuffix(name, "_banner"):
		if be != nil && be["Base"] != nil { // Bedrock numbers the colours from black
			n.Name = dyeColors[15-int(Int(be, "Base"))&15] + strings.TrimPrefix(name, "white")
		}
	case name == "flower_pot":
		if plant, ok := be["PlantBlock"].(map[string]any); ok {
			st, _ := plant["states"].(map[string]any)
			if js, ok := Translate(BlockState{Name: Str(plant, "name"), States: st}); ok && e.j.HasBlock("potted_"+js.Name) {
				n = JavaState{Name: "potted_" + js.Name, Props: map[string]string{}}
			}
		}
	case strings.HasSuffix(name, "_skull"):
		if be != nil {
			kind := [...]string{"skeleton", "wither_skeleton", "zombie", "player", "creeper", "skeleton", "piglin"}[min(max(int(Int(be, "SkullType")), 0), 6)]
			wall := strings.Contains(name, "_wall_")
			n.Name = kind + "_skull"
			if kind == "player" || kind == "zombie" || kind == "creeper" || kind == "piglin" {
				n.Name = kind + "_head"
			}
			if wall {
				n.Name = strings.Replace(n.Name, "_skull", "_wall_skull", 1)
				n.Name = strings.Replace(n.Name, "_head", "_wall_head", 1)
			} else if r, ok := be["Rotation"].(float64); ok {
				p["rotation"] = fmt.Sprint(int(r/22.5+16.5) & 15)
			}
		}
	}
	return n
}

var dyeColors = [16]string{"white", "orange", "magenta", "light_blue", "yellow", "lime", "pink", "gray",
	"light_gray", "cyan", "purple", "blue", "brown", "green", "red", "black"}

func axisOf(f string) string {
	if f == "east" || f == "west" {
		return "x"
	}
	return "z"
}

func sameFence(a, b string) bool {
	if a == b {
		return true
	}
	wooden := func(n string) bool { return strings.HasSuffix(n, "_fence") && n != "nether_brick_fence" }
	return wooden(a) && wooden(b)
}

func isWall(n string) bool { return strings.HasSuffix(n, "_wall") }

// connectException: blocks fences and panes never attach to (the
// reference's Block.isExceptionForConnection).
func connectException(n string) bool {
	return strings.HasSuffix(n, "_leaves") || n == "barrier" || n == "carved_pumpkin" || n == "jack_o_lantern" ||
		n == "melon" || n == "pumpkin" || strings.HasSuffix(n, "shulker_box")
}

// stairShape is the reference's StairBlock.getStairsShape.
func (e *exporter) stairShape(x, y, z int, s JavaState) string {
	f, half := s.Props["facing"], s.Props["half"]
	isStairs := func(o JavaState) bool { return strings.HasSuffix(o.Name, "_stairs") && o.Props["half"] == half }
	at := func(dir string) JavaState { d := dirs[dir]; return e.state(x+d[0], y, z+d[1]) }
	canTake := func(dir string) bool {
		o := at(dir)
		return !strings.HasSuffix(o.Name, "_stairs") || o.Props["facing"] != f || o.Props["half"] != half
	}
	if o := at(f); isStairs(o) {
		of := o.Props["facing"]
		if axisOf(of) != axisOf(f) && canTake(opposite(of)) {
			if of == counterClockwise(f) {
				return "outer_left"
			}
			return "outer_right"
		}
	}
	if o := at(opposite(f)); isStairs(o) {
		of := o.Props["facing"]
		if axisOf(of) != axisOf(f) && canTake(of) {
			if of == counterClockwise(f) {
				return "inner_left"
			}
			return "inner_right"
		}
	}
	return "straight"
}

// isSturdy reports a state whose model is a full cube.
func (e *exporter) isSturdy(id uint16) bool {
	if id == 0 {
		return false
	}
	if v, ok := e.sturdy[id]; ok {
		return v
	}
	v := false
	if as, err := e.j.Models(e.states[id]); err == nil && len(as) == 1 {
		if m, err := e.j.Resolve(as[0].Model); err == nil {
			_, _, cube := m.FullCube()
			v = cube || m.Solid()
		}
	}
	e.sturdy[id] = v
	return v
}

// blocks allocates the numeric ids, writes ids.json and blocks.json and
// returns each state's id<<4|meta.
func (e *exporter) blocks() ([]uint16, error) {
	order := make([]uint16, 0, len(e.states))
	for i := range e.states {
		if i != 0 {
			order = append(order, uint16(i))
		}
	}
	// The flat generator's layers come first in their names (meta 0).
	first := map[string]bool{"bedrock": true, "dirt": true, "grass_block[snowy=false]": true}
	sort.Slice(order, func(a, b int) bool {
		sa, sb := e.states[order[a]], e.states[order[b]]
		if sa.Name != sb.Name {
			return sa.Name < sb.Name
		}
		fa, fb := first[sa.String()], first[sb.String()]
		if fa != fb {
			return fa
		}
		return sa.String() < sb.String()
	})
	keys := make([]uint16, len(e.states))
	ids := map[string]string{"0": "minecraft:air"}
	var entries []map[string]any
	entries = append(entries, map[string]any{"key": "minecraft:air#*", "shape": "none", "opacity": 0, "collision": "none"})
	next := uint16(1)
	var cur string
	var curID, meta uint16
	nth := 0 // ids a name has had: past 16 states it takes more
	newID := func(name string) {
		curID, next, meta = next, next+1, 0
		ns := "minecraft"
		if nth > 0 {
			ns = fmt.Sprintf("minecraft~%d", nth)
		}
		ids[fmt.Sprint(curID)] = ns + ":" + name
	}
	for _, i := range order {
		s := e.states[i]
		var m uint16
		switch {
		case s.Name == "water" || s.Name == "lava": // meta is the level
			if s.Name != cur {
				cur, nth = s.Name, 0
				newID(s.Name)
			}
			lvl, _ := strconv.Atoi(s.Props["level"])
			m = uint16(lvl & 15)
		default:
			if s.Name != cur {
				cur, nth = s.Name, 0
				newID(s.Name)
			} else if meta == 16 {
				nth++
				newID(s.Name)
			}
			m, meta = meta, meta+1
		}
		if curID >= 4096 {
			return nil, fmt.Errorf("bedrock: more than 4096 block ids")
		}
		keys[i] = curID<<4 | m
		ent, err := e.entry(s)
		if err != nil {
			e.o.Logf("bedrock: %s: %v (drawn as a missing-texture cube)", s, err)
			ent = map[string]any{}
		}
		ent["key"] = fmt.Sprintf("%s#%d", ids[fmt.Sprint(curID)], m)
		ent["java"] = s.String()
		entries = append(entries, ent)
	}
	if err := writeJSON(filepath.Join(e.o.Out, "ids.json"), ids); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(e.o.Out, "blocks.json"), entries); err != nil {
		return nil, err
	}
	e.o.Logf("bedrock: %d block ids, %d entries", next-1, len(entries))
	return keys, nil
}

// entry is the blocks.json entry of a state (without its key).
func (e *exporter) entry(s JavaState) (map[string]any, error) {
	ent := map[string]any{}
	name := s.Name
	var texs []string
	translucent := false
	switch name {
	case "water", "lava":
		ent["shape"] = "liquid"
		ent["textures"] = map[string]string{"all": "minecraft:block/" + name + "_still", "side": "minecraft:block/" + name + "_flow"}
		ent["collision"] = "none"
		if name == "water" {
			ent["tint"], ent["opacity"] = "water", 1
		} else {
			ent["emission"] = 15
		}
		e.j.Alpha("minecraft:block/" + name + "_still")
		e.j.Alpha("minecraft:block/" + name + "_flow")
		return ent, nil
	}
	if id, m, ok := entityModel(s, func(t string) [2]float32 { sz := e.j.Size(t); return [2]float32{float32(sz.X), float32(sz.Y)} }); ok && name != "bell" {
		if e.gen == nil {
			e.gen = map[string]emodel{}
		}
		e.gen[id] = m
		ent["shape"] = "model:" + id
		if y := entityTurn(s); y != 0 {
			ent["rotation"] = map[string]any{"y": y}
		}
		for _, t := range m.textures {
			texs = append(texs, t)
		}
		if strings.HasSuffix(name, "_banner") {
			c := strings.TrimSuffix(strings.TrimSuffix(name, "_banner"), "_wall")
			ent["tint"] = dyeTint[c]
		}
	} else {
		as, err := e.j.Models(s)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(name, "item_frame") { // an entity in Java: turned here
			f := s.Props["facing"]
			for i := range as {
				switch f {
				case "up":
					as[i].X = 270
				case "down":
					as[i].X = 90
				default:
					as[i].Y = yTurn(f)
				}
			}
		}
		if len(as) == 0 {
			ent["shape"] = "none"
			return ent, nil
		}
		m, err := e.j.Resolve(as[0].Model)
		if err != nil {
			return nil, err
		}
		if len(as) == 1 && len(m.Elements) == 0 { // barrier, light, structure_void: nothing to draw
			ent["shape"] = "none"
			if name == "barrier" {
				ent["collision"] = "full"
			}
			return ent, nil
		}
		translucent = m.Translucent
		tex6, tinted, cube := m.FullCube()
		switch {
		case len(as) == 1 && m.Cross():
			t := m.Tex("#cross")
			ent["shape"], ent["textures"] = "cross", map[string]string{"cross": t}
			texs = append(texs, t)
			if !tinted {
				for _, el := range m.Elements {
					for _, f := range el.Faces {
						tinted = tinted || f.TintIndex != nil && *f.TintIndex >= 0
					}
				}
			}
		case len(as) == 1 && cube:
			ent["shape"], ent["textures"] = "cube", rotateFaces(tex6, as[0].X, as[0].Y)
			texs = append(texs, tex6[:]...)
		default:
			ent["shape"] = "model:" + as[0].Model
			if a := as[0]; a.X != 0 || a.Y != 0 || a.UVLock {
				ent["rotation"] = map[string]any{"x": a.X, "y": a.Y, "uvlock": a.UVLock}
			}
			texs = append(texs, m.FaceTextures()...)
			var parts []any
			for _, a := range as[1:] {
				pm, err := e.j.Resolve(a.Model)
				if err != nil {
					return nil, err
				}
				texs = append(texs, pm.FaceTextures()...)
				translucent = translucent || pm.Translucent
				parts = append(parts, map[string]any{"model": a.Model, "x": a.X, "y": a.Y})
			}
			if name == "bell" { // the body is the renderer's
				id, bm, _ := entityModel(s, func(t string) [2]float32 { sz := e.j.Size(t); return [2]float32{float32(sz.X), float32(sz.Y)} })
				if e.gen == nil {
					e.gen = map[string]emodel{}
				}
				e.gen[id] = bm
				parts = append(parts, map[string]any{"model": id, "y": yTurn(s.Props["facing"]),
					"textures": bm.textures})
			}
			if len(parts) > 0 {
				ent["parts"] = parts
			}
			if len(as) == 1 && m.Solid() {
				ent["flags"], ent["opacity"] = []string{"opaque"}, 15
			}
		}
		if tinted {
			ent["tint"] = tintOf(name)
		}
	}
	layer := alphaOpaque
	for _, t := range texs {
		if a := e.j.Alpha(t); a != alphaMissing {
			layer = max(layer, a)
		}
	}
	if translucent {
		layer = alphaTranslucent
	}
	ent["layer"] = [...]string{"solid", "cutout", "translucent"}[layer]
	if strings.HasSuffix(name, "_leaves") || name == "cobweb" {
		ent["opacity"] = 1
	}
	if l := emission(s); l > 0 {
		ent["emission"] = l
	}
	if c := collision(s); c != nil {
		ent["collision"] = c
	}
	return ent, nil
}

// rotateFaces turns six face textures (down, up, north, south, west, east)
// like a blockstate's x and y rotation (respack.rotateFaces).
func rotateFaces(t [6]string, rx, ry int) [6]string {
	const d, u, n, s, w, ea = 0, 1, 2, 3, 4, 5
	for ; rx > 0; rx -= 90 {
		t[u], t[n], t[d], t[s] = t[s], t[u], t[n], t[d]
	}
	for ; ry > 0; ry -= 90 {
		t[ea], t[s], t[w], t[n] = t[n], t[ea], t[s], t[w]
	}
	return t
}

// tintOf is the colour a tinted block takes (the reference's BlockColors).
func tintOf(name string) string {
	switch name {
	case "birch_leaves":
		return "#80A755"
	case "spruce_leaves":
		return "#619961"
	case "lily_pad":
		return "#208030"
	case "water", "water_cauldron":
		return "water"
	}
	if strings.HasSuffix(name, "_leaves") || name == "vine" {
		return "foliage"
	}
	return "grass"
}

// dyeTint is each dye's texture colour (the reference's DyeColor), which
// banners take.
var dyeTint = map[string]string{"white": "#F9FFFE", "orange": "#F9801D", "magenta": "#C74EBD", "light_blue": "#3AB3DA",
	"yellow": "#FED83D", "lime": "#80C71F", "pink": "#F38BAA", "gray": "#474F52", "light_gray": "#9D9D97", "cyan": "#169C9C",
	"purple": "#8932B8", "blue": "#3C44AA", "brown": "#835432", "green": "#5E7C16", "red": "#B02E26", "black": "#1D1D21"}

// emission is a state's light level (the reference's lightLevel).
func emission(s JavaState) int {
	switch s.Name {
	case "torch", "wall_torch", "end_rod":
		return 14
	case "lantern", "glowstone", "sea_lantern", "beacon", "fire", "jack_o_lantern", "shroomlight",
		"pearlescent_froglight", "ochre_froglight", "verdant_froglight", "conduit", "lava":
		return 15
	case "soul_lantern", "soul_torch", "soul_wall_torch":
		return 10
	case "redstone_lamp":
		if s.Props["lit"] == "true" {
			return 15
		}
	case "furnace", "blast_furnace", "smoker":
		if s.Props["lit"] == "true" {
			return 13
		}
	case "redstone_torch", "redstone_wall_torch":
		if s.Props["lit"] == "true" {
			return 7
		}
	case "sea_pickle":
		if s.Props["waterlogged"] == "true" {
			n, _ := strconv.Atoi(s.Props["pickles"])
			return 3 + 3*n
		}
	case "brewing_stand":
		return 1
	case "magma_block":
		return 3
	case "ender_chest", "enchanting_table":
		return 7
	}
	return 0
}

// collision overrides the shape's default collision: nil keeps it.
func collision(s JavaState) any {
	n := s.Name
	switch {
	case strings.HasSuffix(n, "torch") || strings.HasSuffix(n, "_button") || strings.HasSuffix(n, "pressure_plate") ||
		strings.HasSuffix(n, "rail") || strings.HasSuffix(n, "_sign") || strings.HasSuffix(n, "_banner") ||
		strings.HasPrefix(n, "tripwire") || n == "vine" || n == "lever" || n == "cobweb" || n == "fire" ||
		strings.HasSuffix(n, "item_frame") || n == "seagrass" || n == "kelp_plant" || n == "kelp" || n == "redstone_wire":
		return "none"
	case strings.HasSuffix(n, "_fence"): // a post and arms, 24 high
		boxes := [][6]float32{{6, 0, 6, 10, 24, 10}}
		arm := map[string][6]float32{"north": {6, 0, 0, 10, 24, 6}, "south": {6, 0, 10, 10, 24, 16},
			"west": {0, 0, 6, 6, 24, 10}, "east": {10, 0, 6, 16, 24, 10}}
		for _, f := range []string{"north", "south", "west", "east"} {
			if s.Props[f] == "true" {
				boxes = append(boxes, arm[f])
			}
		}
		return boxes
	case isWall(n):
		boxes := [][6]float32{{4, 0, 4, 12, 24, 12}}
		arm := map[string][6]float32{"north": {5, 0, 0, 11, 24, 4}, "south": {5, 0, 12, 11, 24, 16},
			"west": {0, 0, 5, 4, 24, 11}, "east": {12, 0, 5, 16, 24, 11}}
		for _, f := range []string{"north", "south", "west", "east"} {
			if s.Props[f] != "none" {
				boxes = append(boxes, arm[f])
			}
		}
		return boxes
	case strings.HasSuffix(n, "_fence_gate"):
		if s.Props["open"] == "true" {
			return "none"
		}
		if axisOf(s.Props["facing"]) == "z" {
			return [][6]float32{{0, 0, 6, 16, 24, 10}}
		}
		return [][6]float32{{6, 0, 0, 10, 24, 16}}
	case strings.HasSuffix(n, "_wall_skull") || strings.HasSuffix(n, "_wall_head"):
		return nil
	case strings.HasSuffix(n, "chest"):
		return [][6]float32{{1, 0, 1, 15, 14, 15}}
	}
	return nil
}

// regions writes the map's chunks: legacy sections (Blocks, Add, Data)
// with plains biomes.
func (e *exporter) regions(keys []uint16) error {
	cols := map[[2]int][]int{}
	for k := range e.secs {
		cols[[2]int{k[0], k[2]}] = append(cols[[2]int{k[0], k[2]}], k[1])
	}
	dir := filepath.Join(e.o.Out, "regions", "0", "region")
	os.RemoveAll(filepath.Join(e.o.Out, "regions"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	byRegion := map[[2]int]map[[2]int]anvil.Compound{}
	biomes := make([]byte, 256)
	for i := range biomes {
		biomes[i] = 1 // plains
	}
	for c, ys := range cols {
		sort.Ints(ys)
		var secs []any
		for _, sy := range ys {
			s := e.secs[[3]int{c[0], sy, c[1]}]
			blocks, data, add := make([]byte, 4096), make([]byte, 2048), make([]byte, 2048)
			hasAdd := false
			for i, id := range s {
				k := keys[id]
				blk, m := k>>4, k&15
				blocks[i] = byte(blk)
				if blk > 255 {
					add[i/2] |= byte(blk>>8&15) << (4 * (i % 2))
					hasAdd = true
				}
				data[i/2] |= byte(m) << (4 * (i % 2))
			}
			sec := anvil.Compound{"Y": int8(sy), "Blocks": blocks, "Data": data}
			if hasAdd {
				sec["Add"] = add
			}
			secs = append(secs, sec)
		}
		level := anvil.Compound{"xPos": int32(c[0]), "zPos": int32(c[1]), "Sections": anvil.List{Type: anvil.TagCompound, Items: secs},
			"Biomes": biomes, "TerrainPopulated": int8(1)}
		r := [2]int{c[0] >> 5, c[1] >> 5}
		if byRegion[r] == nil {
			byRegion[r] = map[[2]int]anvil.Compound{}
		}
		byRegion[r][[2]int{c[0] & 31, c[1] & 31}] = anvil.Compound{"Level": level}
	}
	for r, cs := range byRegion {
		f, err := anvil.OpenRegion(filepath.Join(dir, fmt.Sprintf("r.%d.%d.mca", r[0], r[1])), true)
		if err != nil {
			return err
		}
		err = f.WriteChunks(cs)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	e.o.Logf("bedrock: %d chunks written to %s", len(cols), dir)
	return nil
}

// copyAssets copies the jar's models and textures the map uses, and
// writes the generated models.
func (e *exporter) copyAssets() error {
	for _, d := range []string{"models", "textures"} {
		os.RemoveAll(filepath.Join(e.o.Out, d))
	}
	n := 0
	for p := range e.j.Used {
		data, err := e.j.Read(p)
		if err != nil {
			return err
		}
		parts := strings.SplitN(p, "/", 4) // assets/ns/kind/rest → kind/ns/rest
		dst := filepath.Join(e.o.Out, parts[2], parts[1], parts[3])
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
		n++
	}
	for id, m := range e.gen {
		ns, rel, _ := strings.Cut(id, ":")
		dst := filepath.Join(e.o.Out, "models", ns, rel+".json")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := writeJSON(dst, m.json()); err != nil {
			return err
		}
	}
	e.o.Logf("bedrock: %d assets copied, %d models generated", n, len(e.gen))
	return nil
}

var formatting = regexp.MustCompile("§.")

func (e *exporter) mapJSON() error {
	name := e.o.Name
	if name == "" {
		name = strings.TrimSpace(formatting.ReplaceAllString(e.w.Name, ""))
	}
	x, z := e.w.SpawnX, e.w.SpawnZ
	y := 255
	for y > 0 && e.at(x, y-1, z) == 0 {
		y--
	}
	if e.w.SpawnY >= 0 && e.w.SpawnY < 256 {
		y = max(y, e.w.SpawnY)
	}
	m := map[string]any{
		"name": name, "format": 1, "seed": e.w.Seed,
		"spawn":      map[string]int{"dim": 0, "x": x, "y": y, "z": z},
		"dimensions": []map[string]any{{"id": 0, "name": "Overworld", "regions": "regions/0", "generator": "minecraft:flat"}},
	}
	return writeJSON(filepath.Join(e.o.Out, "map.json"), m)
}

func writeJSON(file string, v any) error {
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(data, '\n'), 0o644)
}
