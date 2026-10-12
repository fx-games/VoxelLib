package voxel

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// StateID is a block state id, global to a Registry. 0 is always minecraft:air.
// Like Minecraft's global palette, every distinct name+properties combination
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
)

// Layer is the render pass, as Minecraft's RenderType: solid has no blending and
// no alpha test (cheapest), cutout alpha-tests, translucent blends and is sorted.
type Layer uint8

const (
	LayerSolid Layer = iota
	LayerCutout
	LayerTranslucent
	LayerCount
)

// Tint selects a colour multiplied into a face, as Minecraft's BlockColors.
type Tint uint8

const (
	TintNone Tint = iota
	TintGrass
	TintFoliage
	TintWater
	TintBirch
	TintSpruce
)

// BlockDef describes a block type. Textures are resource names in Minecraft's
// namespace, e.g. "block/stone", indexed by Face.
type BlockDef struct {
	Name          string // namespaced, "minecraft:stone"
	Shape         Shape
	Layer         Layer
	Textures      [6]string
	Tint          Tint
	TintMask      uint8 // faces (1<<Face) that get Tint; 0 means all
	Opaque        bool  // full opaque cube: hides neighbour faces, casts AO, blocks light
	Collides      bool
	Replaceable   bool // placing a block may overwrite it (air, water, grass)
	LightEmission uint8
	LightOpacity  uint8 // light lost passing through, 0..15
	CullSelf      bool  // faces between two of the same block are hidden (glass, water)
	Unknown       bool  // created on demand for a name nobody registered
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
	Emission uint8
	Opacity  uint8
	CullSelf bool
}

// ModelResolver lets a resource pack decide shape and face textures of a state
// (see mcres). It returns ok=false to keep the BlockDef's defaults.
type ModelResolver func(name string, props map[string]string) (shape Shape, tex [6]string, ok bool)

// Registry owns block definitions, state ids and the texture name table. It is
// safe for concurrent use; Table() is lock free.
type Registry struct {
	mu         sync.Mutex
	defs       map[string]*BlockDef
	stateByKey map[string]StateID
	table      atomic.Pointer[[]StateInfo]
	texNames   []string
	texIndex   map[string]uint16
	texVersion atomic.Uint64
	Resolver   ModelResolver
	// Missing is the texture name used for faces without one.
	Missing string
}

// NewRegistry creates a registry with air (id 0) and the built-in Minecraft
// block set registered.
func NewRegistry() *Registry {
	r := &Registry{
		defs:       map[string]*BlockDef{},
		stateByKey: map[string]StateID{},
		texIndex:   map[string]uint16{},
		Missing:    "block/missing",
	}
	empty := []StateInfo{}
	r.table.Store(&empty)
	r.texture(r.Missing)
	r.Register(&BlockDef{Name: "minecraft:air", Shape: ShapeNone, Replaceable: true})
	r.State("minecraft:air", nil)
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

// Namespaced adds "minecraft:" when no namespace is given.
func Namespaced(name string) string {
	if strings.IndexByte(name, ':') < 0 {
		return "minecraft:" + name
	}
	return name
}

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
// is what most Minecraft blocks use.
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

func (r *Registry) bake(def *BlockDef, canon string, props map[string]string) StateInfo {
	info := StateInfo{
		Def: def, Name: def.Name, Props: canon,
		Shape: def.Shape, Layer: def.Layer, Tint: def.Tint, TintMask: def.TintMask,
		Opaque: def.Opaque, Collides: def.Collides, Emission: def.LightEmission,
		Opacity: def.LightOpacity, CullSelf: def.CullSelf,
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
	if r.Resolver != nil {
		if shape, t, ok := r.Resolver(def.Name, props); ok {
			info.Shape = shape
			tex = t
			if def.Unknown && shape != ShapeCube {
				info.Opaque, info.Collides, info.Opacity, info.Layer = false, false, 0, LayerCutout
			}
		}
	}
	if props["waterlogged"] == "true" && info.Shape != ShapeCube {
		info.Layer = LayerTranslucent
	}
	for f := range tex {
		info.Tex[f] = r.texture(tex[f])
	}
	return info
}

// texture interns a texture name; caller holds mu (or is the constructor).
func (r *Registry) texture(name string) uint16 {
	if name == "" {
		name = r.Missing
	}
	if i, ok := r.texIndex[name]; ok {
		return i
	}
	i := uint16(len(r.texNames))
	r.texNames = append(r.texNames, name)
	r.texIndex[name] = i
	r.texVersion.Add(1)
	return i
}

// Texture returns (interning) the layer index of a texture name.
func (r *Registry) Texture(name string) uint16 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.texture(name)
}

// TextureNames returns the texture table (index = texture layer) and its
// version, which changes whenever a texture is added.
func (r *Registry) TextureNames() ([]string, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.texNames...), r.texVersion.Load()
}

func (r *Registry) TextureVersion() uint64 { return r.texVersion.Load() }

// StateString formats a state as "minecraft:name[k=v]".
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

// ParseStateString splits "minecraft:oak_log[axis=x]" into name and props.
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
