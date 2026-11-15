package voxel

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/memmaker/voxellib/resloc"
)

// StateID is a block state id, global to a Registry. 0 is always air in the default namespace.
// Like the reference's global palette, every distinct name+properties combination
// gets its own id, so hot loops work on small integers and flat lookup tables.
type StateID uint16

const Air StateID = 0

// Shape is how a block is turned into geometry.
type Shape uint8

const (
	ShapeNone   Shape = iota // air, invisible
	ShapeCube                // full 16^3 cube
	ShapeCross               // two diagonal quads (flowers, grass)
	ShapeLiquid              // cube whose top is lowered when not covered by liquid
	ShapeBoxes               // StateInfo.Boxes (stairs, slabs, doors, torches, ...)
)

// Layer is the render pass, as the reference's RenderType: solid has no blending and
// no alpha test (cheapest), cutout alpha-tests, translucent blends and is sorted.
type Layer uint8

const (
	LayerSolid Layer = iota
	LayerCutout
	LayerTranslucent
	LayerCount
)

// Tint selects a colour multiplied into a face, as the reference's BlockColors.
type Tint uint8

const (
	TintNone Tint = iota
	TintGrass
	TintFoliage
	TintWater
	TintBirch
	TintSpruce
	TintFixed // BlockDef.TintColor
)

// BlockDef describes a block type. Textures are resource names in the reference's
// namespace, e.g. "block/stone", indexed by Face.
type BlockDef struct {
	Name          string // namespaced, resloc.Default+":stone"
	Shape         Shape
	Layer         Layer
	Textures      [6]string
	Tint          Tint
	TintMask      uint8 // faces (1<<Face) that get Tint; 0 means all
	Opaque        bool  // full opaque cube: hides neighbour faces, casts AO, blocks light
	Collides      bool
	Replaceable   bool // placing a block may overwrite it (air, water, grass)
	LightEmission uint8
	LightOpacity  uint8  // light lost passing through, 0..15
	CullSelf      bool   // faces between two of the same block are hidden (glass, water)
	Connects      bool   // fence: grows arms toward opaque and connecting neighbours (see Connected)
	Unknown       bool   // created on demand for a name nobody registered
	TintColor     uint32 // 0xRRGGBB for TintFixed
	// Boxes, when set, are a ShapeBoxes block's elements (e.g. from a JSON
	// block model) instead of the built-in shapes picked by name.
	Boxes []Box
	// Collision, when non-nil, replaces the collision of the render shape:
	// these boxes collide (an empty list: nothing does). Collides should be
	// false then, or the full cube collides too.
	Collision []Box
	// Level is a liquid's level for blocks that encode it in the name
	// rather than a "level" property (see StateInfo.Level).
	Level uint8
	// Tags label the block for neighbours' Part conditions.
	Tags []string
	// Parts are elements that depend on the neighbours (the reference's
	// multipart): a ShapeBoxes block draws Boxes plus every Part whose
	// conditions hold (see Connected).
	Parts []Part
	// Retex swaps a ShapeCube's face textures by its neighbours: each face
	// takes the first entry that names it and whose When holds (the reference's
	// world-aware getIcon, as snowy grass, stone and podzol sides).
	Retex []Retex
	// NeighbourLight lights the block with the brightest light of the
	// blocks above and beside it instead of its own (the reference's
	// useNeighborBrightness: stairs, slabs, farmland, which block light).
	NeighbourLight bool
	// PlantOffset shifts a plant (ShapeCross, or ShapeBoxes for plant
	// models) by a position hash (RenderBlocks.renderCrossedSquares and
	// friends).
	PlantOffset PlantOffset
	// Scatter, when set, adds a mod's scattered fallen-leaf quads
	// (ShapeBoxes).
	Scatter *Scatter
	// UVRandom marks faces (1<<Face) whose texture the mesher turns by a
	// position-dependent uvRotate (the 1.7 reference RenderBlocks), as a mod's
	// randomly rotated natural blocks (see mesh.RenderBlocksSides).
	UVRandom uint8
	// UVSeed, when set, is per face the side index whose hash turns it
	// (nil: mesh.RenderBlocksSides).
	UVSeed *[6]uint8
	// UVRotate is a fixed RenderBlocks uvRotate (0..3) per face (logs
	// lying along x or z), for faces not in UVRandom.
	UVRotate [6]uint8
	// Connect, when set, picks every face's texture by its in-plane
	// neighbours (connected textures; overrides Textures and box textures).
	Connect *Connect
	// Strength, when set, is the block's hardness and blast resistance
	// (nil: the vanilla block of its name, see StateInfo.Strength).
	Strength *Strength
}

// Part is a group of elements drawn when every condition holds. A Replace
// part replaces everything gathered before it; within a Group only the
// first matching part counts. A part with Set adds nothing but raises that
// block-local flag for the parts after it (Cond.Flag): several such parts
// make an "any of" across different neighbours.
type Part struct {
	When      []Cond
	Boxes     []Box
	Collision []Box // nil: Boxes collide
	Replace   bool
	Group     string
	Set       string
	set       uint32
}

// Retex is one neighbour-dependent set of face textures (see
// BlockDef.Retex); an empty Tex keeps that face's texture.
type Retex struct {
	When []Cond
	Tex  [6]string
	ids  [6]uint16
}

// Cond holds when the neighbour on Face (Dy blocks higher or lower: a
// diagonal neighbour, e.g. redstone wire climbing a block) passes any of
// the tests.
//
// With Flag, the tests name flags raised by earlier parts (Part.Set)
// instead of neighbour tags, and Face/Dy are unused.
//
// With Pos, the tests are on the block's own world position (Face/Dy
// unused), as the 1.7 reference's RenderBlocks.renderBlockFire picks icons:
// "sum" holds when (x+y+z)&1 == 1, "half" when (x/2+y/2+z/2)&1 == 1
// (Java's truncating division).
type Cond struct {
	Face Face
	Dy   int8
	Flag bool
	Pos  bool
	Any  []TagTest
}

// posTests are the Cond.Pos test names, by TagTest.id.
var posTests = []string{"sum", "half"}

// TagTest is true when the neighbour carries Tag (or, with Not, doesn't).
// Built-in tags: "air" (the neighbour is air) and "opaque" (an opaque full
// cube).
type TagTest struct {
	Tag string
	Not bool
	id  uint16
}

// StateInfo is the flattened, baked view of one state the mesher and physics
// read in their inner loops.
type StateInfo struct {
	Def      *BlockDef
	Name     string
	Props    string // canonical "k=v,k=v"
	Tex      [6]uint16
	Shape    Shape
	Layer    Layer
	Tint     Tint
	TintMask uint8
	Opaque   bool
	Collides bool
	// Step is a stair or a slab: it collides as its boxes (World.Boxes),
	// low enough to walk up without a jump, and a pathfinder stands on
	// it as on a full block (World.Footing).
	Step bool
	// Climb is a ladder or vine: a body in it climbs (World.Climbable).
	Climb    bool
	Emission uint8
	Opacity  uint8
	CullSelf bool
	// Level is a liquid's level in the reference: 0 a source (8/9 high), 1..7
	// flowing and ever shallower ((8-Level)/9); the mesher slopes the
	// surface between neighbours.
	Level uint8
	// Boxes are a ShapeBoxes state's elements; Variants holds the
	// neighbour-dependent ones (Connected picks).
	Boxes     []Box
	Variants  map[string][]Box
	Connects  bool
	Collision []Box // BlockDef.Collision
	TintColor uint32
	// Cull, when set, is per face of a cube the side whose opaque
	// neighbour hides it (FaceCount: never hidden), from its model's
	// cullfaces (see FaceResolver). Nil: each face by the neighbour it faces.
	Cull *[6]Face
	// Tags and Parts as in BlockDef, tags interned (see Registry.tagID).
	Tags           []uint16
	Parts          []Part
	Retex          []Retex
	NeighbourLight bool
	PlantOffset    PlantOffset
	Scatter        *Scatter
	UVRandom       uint8
	UVSeed         *[6]uint8
	UVRotate       [6]uint8
	Connect        *ConnectInfo // BlockDef.Connect
	// Strength is BlockDef.Strength, else the vanilla block's of the
	// name, else DefaultStrength.
	Strength Strength
}

// Model is a state's geometry as a resource pack's JSON block model gives
// it (see respack): the one source of block shapes. A ShapeCube or
// ShapeCross has its face textures (and a cube its face rules); ShapeBoxes
// its elements as boxes, with Tex the texture standing for the block
// (particle) on every face.
type Model struct {
	Shape Shape
	Tex   [6]string
	Rules FaceRules // ShapeCube
	Boxes []Box     // ShapeBoxes
}

// ModelResolver gives a state's model from a resource pack. ok=false keeps
// the BlockDef's own cube or cross.
type ModelResolver func(name string, props map[string]string) (m Model, ok bool)

// FaceRules are a cube model's per-face details: Cull[f] is the side whose
// opaque neighbour hides face f (a model face's cullface; FaceCount when it
// has none), Tinted the faces (1<<Face) with a tintindex.
type FaceRules struct {
	Cull   [6]Face
	Tinted uint8
}

// Registry owns block definitions, state ids and the texture name table. It is
// safe for concurrent use; Table() is lock free.
type Registry struct {
	mu         sync.Mutex
	defs       map[string]*BlockDef
	stateByKey map[string]StateID
	table      atomic.Pointer[[]StateInfo]
	textures   []TextureEntry
	texIndex   map[TextureEntry]uint16
	texVersion atomic.Uint64
	tagIndex   map[string]uint16
	Resolver   ModelResolver
	// Missing is the texture name used for faces without one.
	Missing string
}

// NewRegistry creates a registry with air (id 0) and the built-in reference
// block set registered.
func NewRegistry() *Registry {
	r := &Registry{
		defs:       map[string]*BlockDef{},
		stateByKey: map[string]StateID{},
		texIndex:   map[TextureEntry]uint16{},
		tagIndex:   map[string]uint16{"air": tagAir, "opaque": tagOpaque},
		Missing:    "block/missing",
	}
	empty := []StateInfo{}
	r.table.Store(&empty)
	r.texture(r.Missing)
	r.Register(&BlockDef{Name: resloc.Default + ":air", Shape: ShapeNone, Replaceable: true})
	r.State(resloc.Default+":air", nil)
	for _, d := range builtinBlocks() {
		r.Register(d)
	}
	return r
}

// Register adds or replaces a block definition. States already baked for that
// name keep their ids; call before loading worlds for best results.
func (r *Registry) Register(d *BlockDef) {
	d.Name = Namespaced(d.Name)
	r.mu.Lock()
	r.defs[d.Name] = d
	r.mu.Unlock()
}

// Def returns a definition by (optionally un-namespaced) name.
func (r *Registry) Def(name string) *BlockDef {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.defs[Namespaced(name)]
}

// Defs lists all registered definitions sorted by name.
func (r *Registry) Defs() []*BlockDef {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*BlockDef, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Namespaced adds the default namespace (resloc.Default) when none is given.
func Namespaced(name string) string { return resloc.Qualify(name) }

// Table returns the immutable state table; index it with a StateID. Grab it
// once per mesh/batch, not per block.
func (r *Registry) Table() []StateInfo { return *r.table.Load() }

// Info returns the baked info of a state.
func (r *Registry) Info(id StateID) *StateInfo {
	t := *r.table.Load()
	if int(id) < len(t) {
		return &t[id]
	}
	return &t[0]
}

// Default returns the state of a block with no properties set.
func (r *Registry) Default(name string) StateID { return r.State(name, nil) }

// MustState is State, for literal "name[k=v]" strings.
func (r *Registry) Parse(s string) StateID {
	name, props := ParseStateString(s)
	return r.State(name, props)
}

// State returns (creating when needed) the id of name+props. Unknown names get
// a placeholder cube definition whose texture is guessed as block/<name>, which
// is what most reference blocks use.
func (r *Registry) State(name string, props map[string]string) StateID {
	name = Namespaced(name)
	key := name
	canon := canonicalProps(props)
	if canon != "" {
		key += "[" + canon + "]"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.stateByKey[key]; ok {
		return id
	}
	def := r.defs[name]
	if def == nil {
		short := name[strings.IndexByte(name, ':')+1:]
		def = &BlockDef{Name: name, Shape: ShapeCube, Opaque: true, Collides: true, LightOpacity: 15, Unknown: true}
		for f := range def.Textures {
			def.Textures[f] = "block/" + short
		}
		r.defs[name] = def
	}
	old := *r.table.Load()
	if len(old) >= 1<<16 {
		return Air
	}
	info := r.bake(def, canon, props)
	nt := make([]StateInfo, len(old), len(old)+1)
	copy(nt, old)
	nt = append(nt, info)
	id := StateID(len(old))
	r.table.Store(&nt)
	r.stateByKey[key] = id
	return id
}

// conds copies cs with their tags interned.
func (r *Registry) conds(cs []Cond, flags map[string]uint16) []Cond {
	cs = append([]Cond(nil), cs...)
	for j, c := range cs {
		c.Any = append([]TagTest(nil), c.Any...)
		for k := range c.Any {
			if c.Flag {
				c.Any[k].id = flagBit(flags, c.Any[k].Tag)
				continue
			}
			if c.Pos {
				c.Any[k].id = uint16(slices.Index(posTests, c.Any[k].Tag)) // unknown: never holds
				continue
			}
			c.Any[k].id = r.tagID(c.Any[k].Tag)
		}
		cs[j] = c
	}
	return cs
}

func (r *Registry) bake(def *BlockDef, canon string, props map[string]string) StateInfo {
	base, _, _ := strings.Cut(def.Name, "#") // a VoxelMap key: the name without its meta
	var model Model
	resolved := false
	if r.Resolver != nil && def.Boxes == nil {
		model, resolved = r.Resolver(def.Name, props)
	}
	shape := def.Shape
	if resolved {
		shape = model.Shape
	}
	info := StateInfo{
		Def: def, Name: def.Name, Props: canon,
		Shape: def.Shape, Layer: def.Layer, Tint: def.Tint, TintMask: def.TintMask,
		Opaque: def.Opaque, Collides: def.Collides, Emission: def.LightEmission,
		Opacity: def.LightOpacity, CullSelf: def.CullSelf, Connects: def.Connects,
		Collision: def.Collision, TintColor: def.TintColor, Level: def.Level,
		NeighbourLight: def.NeighbourLight, PlantOffset: def.PlantOffset, Scatter: def.Scatter, UVRandom: def.UVRandom, UVSeed: def.UVSeed, UVRotate: def.UVRotate,
		Step:     shape == ShapeBoxes && (strings.HasSuffix(base, "_stairs") || strings.HasSuffix(base, "_slab")),
		Climb:    shape == ShapeBoxes && isLadder(base) || strings.HasSuffix(base, "vine"),
		Strength: strengthOf(def, base),
	}
	for _, t := range def.Tags {
		info.Tags = append(info.Tags, r.tagID(t))
	}
	if def.Connect != nil {
		info.Connect = r.bakeConnect(def.Connect)
	}
	if len(def.Parts) > 0 {
		info.Parts = make([]Part, len(def.Parts))
		flags := map[string]uint16{}
		for i, p := range def.Parts {
			p.When = r.conds(p.When, flags)
			if p.Set != "" {
				p.set = 1 << flagBit(flags, p.Set)
			}
			p.Boxes = append([]Box(nil), p.Boxes...)
			for j := range p.Boxes {
				r.boxTextures(&p.Boxes[j])
			}
			info.Parts[i] = p
		}
	}
	for _, x := range def.Retex {
		x.When = r.conds(x.When, map[string]uint16{})
		for f, t := range x.Tex {
			if t != "" {
				x.ids[f] = r.texture(t)
			}
		}
		info.Retex = append(info.Retex, x)
	}
	switch {
	case resolved && model.Shape == ShapeBoxes:
		info.Variants = r.modelVariants(def.Name, props, model.Boxes)
	case def.Shape == ShapeBoxes && def.Boxes != nil:
		info.Variants = map[string][]Box{"": append([]Box(nil), def.Boxes...)}
	case def.Shape == ShapeBoxes:
		info.Variants = shapeVariants(def.Name, props)
	}
	if info.Variants != nil {
		for _, bs := range info.Variants {
			for i := range bs {
				r.boxTextures(&bs[i])
			}
		}
		info.Boxes = info.Variants[""]
	}
	if info.TintMask == 0 {
		info.TintMask = 0x3f
	}
	tex := def.Textures
	// Pillars (logs, basalt, ...) rotate their end texture onto the axis faces.
	switch props["axis"] {
	case "x":
		end, side := tex[FaceUp], tex[FaceNorth]
		tex = [6]string{side, side, side, side, end, end}
	case "z":
		end, side := tex[FaceUp], tex[FaceNorth]
		tex = [6]string{side, side, end, end, side, side}
	}
	// Snowy grass/podzol/mycelium swap to the snow side texture, untinted.
	if props["snowy"] == "true" {
		tex[FaceUp] = "block/snow"
		for f := FaceNorth; f <= FaceEast; f++ {
			tex[f] = "block/grass_block_snow"
		}
		info.TintMask = 0
		info.Tint = TintNone
	}
	// Horizontal facing turns a cube's north (front) texture to that side.
	if def.Shape == ShapeCube {
		turns := map[string]int{"east": 1, "south": 2, "west": 3}[props["facing"]]
		for ; turns > 0; turns-- { // north->east->south->west
			tex[FaceEast], tex[FaceSouth], tex[FaceWest], tex[FaceNorth] =
				tex[FaceNorth], tex[FaceEast], tex[FaceSouth], tex[FaceWest]
		}
	}
	// Per-state textures: upper door halves, growing crops.
	if props["half"] == "upper" {
		for f := range tex {
			tex[f] = strings.Replace(tex[f], "_bottom", "_top", 1)
		}
	}
	if a, ok := props["age"]; ok && strings.Contains(tex[0], "_stage") {
		tex = all6(tex[0][:strings.LastIndex(tex[0], "_stage")+6] + a)
	}
	if resolved {
		info.Shape, tex = model.Shape, model.Tex
		if shape != ShapeCube && (def.Unknown || def.Shape == ShapeCube) { // not a full block after all
			info.Opaque, info.Collides, info.Opacity, info.Layer = false, false, 0, LayerCutout
		}
		if shape == ShapeCube {
			info.TintMask = model.Rules.Tinted
			if fr := model.Rules; fr.Cull != [6]Face{FaceDown, FaceUp, FaceNorth, FaceSouth, FaceWest, FaceEast} {
				info.Cull = &fr.Cull
			}
		}
	}
	if props["waterlogged"] == "true" && info.Shape != ShapeCube {
		info.Layer = LayerTranslucent
	}
	if info.Shape == ShapeLiquid {
		if l, err := strconv.Atoi(props["level"]); err == nil && l > 0 && l < 8 {
			info.Level = uint8(l)
		}
	}
	for f := range tex {
		info.Tex[f] = r.texture(tex[f])
	}
	return info
}

const (
	tagAir uint16 = iota
	tagOpaque
)

// tagID interns a tag; caller holds mu.
func (r *Registry) tagID(t string) uint16 {
	if i, ok := r.tagIndex[t]; ok {
		return i
	}
	i := uint16(len(r.tagIndex))
	r.tagIndex[t] = i
	return i
}

// flagBit numbers a block-local part flag (at most 32 per block).
func flagBit(flags map[string]uint16, name string) uint16 {
	b, ok := flags[name]
	if !ok {
		b = uint16(len(flags) % 32)
		flags[name] = b
	}
	return b
}

// texture interns a texture name; caller holds mu (or is the constructor).
func (r *Registry) texture(name string) uint16 {
	if name == "" {
		name = r.Missing
	}
	return r.entry(TextureEntry{Name: name, Region: WholeTexture})
}

// TextureEntry is one texture table entry: the part Region of the texture
// Name (WholeTexture for plain textures).
type TextureEntry struct {
	Name   string
	Region UVRect
}

// UVRect is a rectangle of a texture, u0, v0, u1, v1 as fractions of it.
type UVRect [4]float32

// WholeTexture is the region of a plain texture entry.
var WholeTexture = UVRect{0, 0, 1, 1}

// MaxTextures bounds the texture table: a mesh vertex holds 12 bits of it.
const MaxTextures = 1 << 12

// entry interns a texture table entry.
func (r *Registry) entry(e TextureEntry) uint16 {
	if i, ok := r.texIndex[e]; ok {
		return i
	}
	if len(r.textures) >= MaxTextures {
		panic(fmt.Sprintf("voxel: more than %d textures and texture regions (adding %+v)", MaxTextures, e))
	}
	i := uint16(len(r.textures))
	r.textures = append(r.textures, e)
	r.texIndex[e] = i
	r.texVersion.Add(1)
	return i
}

// boxTextures interns box b's face textures into TexID. A face whose UVs
// fall between the 1/16 steps a mesh vertex holds (entity sheets: quarter
// pixels of a 64 px texture) gets a region entry for its UV rectangle
// instead, and its UVs become that region's corners (orientation kept).
func (r *Registry) boxTextures(b *Box) {
	copied := false
	for f, t := range b.Tex {
		if t == "" {
			continue
		}
		if b.UV == nil || wholeSteps(b.UV[f]) {
			b.TexID[f] = r.texture(t)
			continue
		}
		if !copied { // UV may be shared with other states' boxes
			uv := *b.UV
			b.UV, copied = &uv, true
		}
		uv := &b.UV[f]
		rect := UVRect{min(uv[0], uv[2]) / 16, min(uv[1], uv[3]) / 16, max(uv[0], uv[2]) / 16, max(uv[1], uv[3]) / 16}
		b.TexID[f] = r.entry(TextureEntry{Name: t, Region: rect})
		corner := func(lo, hi float32) (float32, float32) {
			if lo <= hi {
				return 0, 16
			}
			return 16, 0
		}
		uv[0], uv[2] = corner(uv[0], uv[2])
		uv[1], uv[3] = corner(uv[1], uv[3])
	}
}

// wholeSteps: every coordinate is a whole 1/16 step (what a vertex holds).
func wholeSteps(uv [4]float32) bool {
	for _, c := range uv {
		if c != float32(int(c)) {
			return false
		}
	}
	return true
}

// Texture returns (interning) the layer index of a texture name.
func (r *Registry) Texture(name string) uint16 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.texture(name)
}

// Textures returns the texture table (index = a box face's TexID, a
// vertex's texture) and its version, which changes whenever an entry is added.
func (r *Registry) Textures() ([]TextureEntry, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]TextureEntry(nil), r.textures...), r.texVersion.Load()
}

func (r *Registry) TextureVersion() uint64 { return r.texVersion.Load() }

// StateString formats a state as "name[k=v]" with the default namespace prefix.
func (r *Registry) StateString(id StateID) string {
	in := r.Info(id)
	if in.Props == "" {
		return in.Name
	}
	return in.Name + "[" + in.Props + "]"
}

// StateProps returns the property map of a state.
func (r *Registry) StateProps(id StateID) map[string]string {
	_, p := ParseStateString("x[" + r.Info(id).Props + "]")
	return p
}

// ParseStateString splits a state string such as "oak_log[axis=x]" (optionally namespaced) into name and props.
func ParseStateString(s string) (string, map[string]string) {
	i := strings.IndexByte(s, '[')
	if i < 0 {
		return s, nil
	}
	name := s[:i]
	body := strings.TrimSuffix(s[i+1:], "]")
	if body == "" {
		return name, nil
	}
	props := map[string]string{}
	for _, kv := range strings.Split(body, ",") {
		k, v, _ := strings.Cut(kv, "=")
		props[k] = v
	}
	return name, props
}

func canonicalProps(props map[string]string) string {
	if len(props) == 0 {
		return ""
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s=%s", k, props[k])
	}
	return b.String()
}

// Scatter draws a block as a mod's renderFallenLeaves does:
// Leaves (a random count in [0]..[1]) single-sided, up-facing quads of the
// block's up texture, each XSize x ZSize texels (random in their ranges)
// cut from a random spot of the icon, at a random place and turn, from a
// java.util.Random seeded by the block position; Shade scales the colour.
type Scatter struct {
	Leaves [2]int32 `json:"leaves"`
	XSize  [2]int32 `json:"xsize"`
	ZSize  [2]int32 `json:"zsize"`
	Shade  float32  `json:"shade"`
}

// PlantOffset is how the 1.7 reference moves a plant off its block's centre.
type PlantOffset uint8

const (
	OffsetNone   PlantOffset = iota
	OffsetGrass              // tall grass: x, z by ±0.25, y by -0.2..0
	OffsetFlower             // flowers: x, z by ±0.15
	OffsetDouble             // double plants: x, z by ±0.15, the same for both halves
	OffsetClover             // a mod's clovers: x, z by ±0.25 (the grass hash), y kept
)

// stairShapes are a stair's "shape" values; Connected picks one by the
// neighbouring stairs, as the key of its Variants ("" for straight).
var stairShapes = []string{"straight", "outer_left", "outer_right", "inner_left", "inner_right"}

// modelVariants is a resolved model's boxes as StateInfo.Variants. A
// stair's are those of every shape (whatever its own "shape" property),
// which its neighbours choose. Caller holds mu.
func (r *Registry) modelVariants(name string, props map[string]string, boxes []Box) map[string][]Box {
	if !strings.HasSuffix(name, "_stairs") {
		return map[string][]Box{"": boxes}
	}
	out := map[string][]Box{}
	p := maps.Clone(props)
	for _, s := range stairShapes {
		p["shape"] = s
		if m, ok := r.Resolver(name, p); ok && m.Shape == ShapeBoxes {
			key := s
			if s == stairShapes[0] {
				key = ""
			}
			out[key] = m.Boxes
		}
	}
	if out[""] == nil {
		out[""] = boxes
	}
	return out
}
