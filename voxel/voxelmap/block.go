package voxelmap

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/voxel"
)

// Block is one blocks.json entry.
//
//   - shape: "cube" (default), "cross", "liquid", "none" or
//     "model:<model id>" (a JSON block model under models/). A model may be
//     turned like a blockstate variant with "x"/"y" (multiples of 90°),
//     given at the top level or as "rotation": {"x":..,"y":..,"uvlock":..};
//     uvlock keeps position-laid textures in world space (voxel.LockBoxes).
//   - textures: one texture id for every face; a list of six (down, up,
//     north, south, west, east); or an object with "all", "side", "end",
//     "top"/"up", "bottom"/"down" and the face names (most specific wins).
//     For a model, the object's entries (or "all" for a single id) set the
//     model's texture variables. A cross takes "cross" or "all".
//   - layer: "solid", "cutout" or "translucent" (default by shape).
//   - opacity, emission: light 0..15 (opacity defaults to 15 for solid
//     cubes, else 0).
//   - collision: "full", "none" or a list of [x0,y0,z0,x1,y1,z1] boxes in
//     1/16 (default: full for cubes, the model's elements for models,
//     none otherwise).
//   - tint: "none", "grass", "foliage", "water" or a fixed "#RRGGBB".
//   - flags: a list of words; "replaceable" lets a placed block overwrite it
//     (default for cross, liquid and none); "opaque" makes a model or
//     non-solid block hide its neighbours' faces like a full cube;
//     "neighbour_light" lights it from its brightest neighbour;
//     "offset_grass", "offset_flower", "offset_double" and "offset_clover"
//     shift a cross plant like 1.7's tall grass, flowers, double plants and
//     LOTR clovers (voxel.BlockDef.PlantOffset).
//   - uvrandom: faces whose texture turns by a position-dependent quarter
//     turn (voxel.BlockDef.UVRandom); uvseed, six side indices (down, up,
//     north, south, west, east), picks each face's hash side
//     (voxel.BlockDef.UVSeed; default mesh.RenderBlocksSides).
//   - uvrotate: a fixed RenderBlocks uvRotate per face, six values 0..3
//     (voxel.BlockDef.UVRotate).
//   - connect: connected textures (voxel.Connect): "tag" a connecting
//     neighbour carries, "plane" an axis 0..2 no connection crosses
//     (default none), "below" a tag whose absence connects the blocks one
//     lower, "icons" texture ids and "mask" (256 indices into icons, by
//     in-plane neighbour mask, see voxel.ConnectOffsets).
//   - te: tile-entity fields that pick a variant: a block at a tile entity
//     becomes the entry "<key>|<v1>,<v2>,..." (the fields' integer values,
//     0 when absent) when blocks.json has it (Map.ReadChunk).
//   - te_below: the te fields come from the tile entity one block lower (the
//     upper half of a two-block piece whose lower half holds it).
//   - scatter: LOTR fallen-leaf quads (voxel.Scatter: "leaves", "xsize",
//     "zsize" as [min, max], "shade"); a "none" shape becomes ShapeBoxes.
//
// Every other field (sounds, ...) is left to the game in Raw.
type Block struct {
	Key      string `json:"key"`
	Shape    string `json:"shape"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Rotation *struct {
		X, Y   int
		UVLock bool
	} `json:"rotation"`
	Textures  json.RawMessage `json:"textures"`
	Layer     string          `json:"layer"`
	Opacity   *int            `json:"opacity"`
	Emission  int             `json:"emission"`
	Collision json.RawMessage `json:"collision"`
	Tint      string          `json:"tint"`
	Flags     []string        `json:"flags"`
	Tags      []string        `json:"tags"`
	Parts     []Part          `json:"parts"`
	UVRandom  []string        `json:"uvrandom"`
	UVSeed    *[6]uint8       `json:"uvseed"`
	UVRotate  [6]uint8        `json:"uvrotate"`
	Connect   *Connect        `json:"connect"`
	TE        []string        `json:"te"`
	TEBelow   bool            `json:"te_below"`
	Scatter   *voxel.Scatter  `json:"scatter"`
	Raw       json.RawMessage `json:"-"`
}

// Connect is a blocks.json "connect" entry (see Block).
type Connect struct {
	Tag   string   `json:"tag"`
	Plane *int8    `json:"plane"`
	Below string   `json:"below"`
	Icons []string `json:"icons"`
	Mask  []int    `json:"mask"` // not []uint8: JSON would want base64
}

// Part is a neighbour-dependent piece of a block (see voxel.Part): the model
// (turned by x/y, textures defaulting to the block's) is added when, for
// every face named in when ("north+up", "north+down": the diagonal
// neighbour above or below that one), the neighbour there carries the tag ("tag"),
// lacks it ("!tag"), passes any of a list of such tests, or passes each
// list of a list of lists. Built-in tags:
// "air", "opaque". replace swaps out everything gathered before; group lets
// only the first matching part of a group count. collision overrides the
// part's collision ("none" or boxes in 1/16). A cube's parts have no model:
// their textures (as a cube's: all, side, end, top, bottom, face names)
// replace its faces' while they hold (voxel.BlockDef.Retex), the first
// holding part per face. A part with set (and no model) raises that
// block-local flag for the parts after it when it holds; the when key
// "flags" tests such flags like tags (voxel.Part.Set, voxel.Cond.Flag).
// The when key "pos" tests the block's world position ("sum", "half",
// negated with "!": voxel.Cond.Pos).
type Part struct {
	When      map[string]json.RawMessage `json:"when"`
	Model     string                     `json:"model"`
	X         int                        `json:"x"`
	Y         int                        `json:"y"`
	Textures  json.RawMessage            `json:"textures"`
	Collision json.RawMessage            `json:"collision"`
	Replace   bool                       `json:"replace"`
	Group     string                     `json:"group"`
	Set       string                     `json:"set"`
}

// def builds the definition of key from entry b (meta is the key's meta).
func (m *Map) def(key string, b *Block, meta int) *voxel.BlockDef {
	d := &voxel.BlockDef{Name: key}
	shape := b.Shape
	if shape == "" {
		shape = "cube"
	}
	tex := faceTextures(b.Textures)
	switch {
	case shape == "cube":
		d.Shape, d.Textures, d.Collides = voxel.ShapeCube, tex, true
	case shape == "cross":
		d.Shape, d.Layer, d.Replaceable = voxel.ShapeCross, voxel.LayerCutout, true
		t := textureVars(b.Textures)["cross"]
		if t == "" {
			t = tex[voxel.FaceUp]
		}
		d.Textures = [6]string{t, t, t, t, t, t}
	case shape == "liquid":
		d.Shape, d.Layer, d.Textures, d.CullSelf, d.Replaceable = voxel.ShapeLiquid, voxel.LayerTranslucent, tex, true, true
		if meta < 8 {
			d.Level = uint8(meta)
		}
	case strings.HasPrefix(shape, "model:"):
		d.Shape, d.Layer = voxel.ShapeBoxes, voxel.LayerCutout
		boxes, err := m.pack.ModelBoxes(strings.TrimSuffix(strings.TrimPrefix(shape, "model:"), ".json"), textureVars(b.Textures))
		if err != nil || len(boxes) == 0 {
			logf("%s: model %s: %v; drawn as a cube", key, shape, err)
			d.Shape, d.Textures, d.Collides = voxel.ShapeCube, tex, true
			break
		}
		rx, ry, rotate := b.X, b.Y, voxel.RotateBoxes
		if b.Rotation != nil {
			rx, ry = b.Rotation.X, b.Rotation.Y
			if b.Rotation.UVLock {
				rotate = voxel.LockBoxes
			}
		}
		d.Boxes = rotate(rotate(boxes, 0, float32(rx)), 1, float32(ry))
	default: // "none"
		d.Shape, d.Replaceable = voxel.ShapeNone, true
	}
	if d.Shape == voxel.ShapeCube {
		d.Opaque = true
		d.LightOpacity = 15
	}
	switch b.Layer {
	case "solid":
		d.Layer = voxel.LayerSolid
	case "cutout":
		d.Layer = voxel.LayerCutout
	case "translucent":
		d.Layer = voxel.LayerTranslucent
	}
	if d.Layer != voxel.LayerSolid {
		d.Opaque, d.LightOpacity = false, 0
		d.CullSelf = d.CullSelf || d.Layer == voxel.LayerTranslucent
	}
	if b.Opacity != nil {
		d.LightOpacity = uint8(min(max(*b.Opacity, 0), 15))
	}
	d.LightEmission = uint8(min(max(b.Emission, 0), 15))
	m.collision(d, b.Collision)
	switch t := b.Tint; {
	case t == "grass":
		d.Tint = voxel.TintGrass
	case t == "foliage":
		d.Tint = voxel.TintFoliage
	case t == "water":
		d.Tint = voxel.TintWater
	case strings.HasPrefix(t, "#"):
		if c, err := strconv.ParseUint(t[1:], 16, 32); err == nil {
			d.Tint, d.TintColor = voxel.TintFixed, uint32(c)
		}
	}
	for _, f := range b.Flags {
		switch f {
		case "replaceable":
			d.Replaceable = true
		case "neighbour_light":
			d.NeighbourLight = true
		case "offset_grass":
			d.PlantOffset = voxel.OffsetGrass
		case "offset_flower":
			d.PlantOffset = voxel.OffsetFlower
		case "offset_double":
			d.PlantOffset = voxel.OffsetDouble
		case "offset_clover":
			d.PlantOffset = voxel.OffsetClover
		case "opaque": // a model or cutout block that still hides its neighbours' faces
			d.Opaque = true
			if b.Opacity == nil {
				d.LightOpacity = 15
			}
		}
	}
	for _, f := range b.UVRandom {
		if v, ok := faceNames[f]; ok {
			d.UVRandom |= 1 << v
		} else {
			logf("%s: uvrandom: unknown face %q", d.Name, f)
		}
	}
	d.UVSeed, d.UVRotate = b.UVSeed, b.UVRotate
	d.Tags = b.Tags
	if c := b.Connect; c != nil {
		if len(c.Mask) != 256 {
			logf("%s: connect: mask needs 256 entries, has %d", d.Name, len(c.Mask))
		} else {
			d.Connect = &voxel.Connect{Tag: c.Tag, Plane: -1, Below: c.Below, Icons: c.Icons}
			for i, v := range c.Mask {
				d.Connect.Mask[i] = uint8(v)
			}
			if c.Plane != nil {
				d.Connect.Plane = *c.Plane
			}
		}
	}
	if b.Scatter != nil {
		if d.Shape == voxel.ShapeNone {
			d.Shape, d.Boxes, d.Textures = voxel.ShapeBoxes, []voxel.Box{}, tex
		}
		d.Scatter = b.Scatter
	}
	if len(b.Parts) > 0 {
		m.parts(d, b)
	}
	return d
}

// parts turns blocks.json parts into voxel.Parts; the block becomes
// ShapeBoxes (with no own elements if its shape drew nothing); a cube's
// parts become its Retex.
func (m *Map) parts(d *voxel.BlockDef, b *Block) {
	if d.Shape == voxel.ShapeCube {
		for _, p := range b.Parts {
			if p.Model != "" {
				logf("%s: a cube's parts only retexture (no model %s)", d.Name, p.Model)
				continue
			}
			d.Retex = append(d.Retex, voxel.Retex{When: conds(d.Name, p.When), Tex: faceTextures(p.Textures)})
		}
		return
	}
	switch d.Shape {
	case voxel.ShapeBoxes:
	case voxel.ShapeNone:
		d.Shape, d.Boxes, d.Replaceable = voxel.ShapeBoxes, []voxel.Box{}, slices.Contains(b.Flags, "replaceable")
		if d.Collision == nil {
			d.Collision = []voxel.Box{}
		}
	default:
		logf("%s: parts need a model or \"none\" shape, not %q", d.Name, b.Shape)
		return
	}
	for _, p := range b.Parts {
		if p.Set != "" {
			d.Parts = append(d.Parts, voxel.Part{When: conds(d.Name, p.When), Set: p.Set, Group: p.Group})
			continue
		}
		tex := p.Textures
		if len(tex) == 0 {
			tex = b.Textures
		}
		boxes, err := m.pack.ModelBoxes(strings.TrimSuffix(strings.TrimPrefix(p.Model, "model:"), ".json"), textureVars(tex))
		if err != nil {
			logf("%s: part model %s: %v", d.Name, p.Model, err)
			continue
		}
		boxes = voxel.RotateBoxes(voxel.RotateBoxes(boxes, 0, float32(p.X)), 1, float32(p.Y))
		vp := voxel.Part{Boxes: boxes, Replace: p.Replace, Group: p.Group}
		vp.When = conds(d.Name, p.When)
		if len(p.Collision) > 0 {
			cd := &voxel.BlockDef{Name: d.Name, Shape: voxel.ShapeBoxes}
			m.collision(cd, p.Collision)
			vp.Collision = cd.Collision
		}
		d.Parts = append(d.Parts, vp)
	}
}

// conds parses a part's when (see Part).
func conds(name string, when map[string]json.RawMessage) (cs []voxel.Cond) {
	for face, raw := range when {
		flag, pos := face == "flags", face == "pos"
		side, dy, _ := strings.Cut(face, "+") // "north+down": the block below the north one
		f, ok := faceNames[side]
		ok = flag || pos || ok && (dy == "" || f >= voxel.FaceNorth && (dy == "up" || dy == "down"))
		if !ok {
			logf("%s: part condition on unknown face %q", name, face)
			continue
		}
		// a test, a list (any of), or a list of lists (all of, each any of)
		var groups [][]string
		var one string
		var list []string
		switch {
		case json.Unmarshal(raw, &one) == nil:
			groups = [][]string{{one}}
		case json.Unmarshal(raw, &list) == nil:
			groups = [][]string{list}
		case json.Unmarshal(raw, &groups) != nil:
			logf("%s: bad part condition %s", name, raw)
		}
		for _, tests := range groups {
			c := voxel.Cond{Face: f, Dy: map[string]int8{"up": 1, "down": -1}[dy], Flag: flag, Pos: pos}
			for _, t := range tests {
				not := strings.HasPrefix(t, "!")
				c.Any = append(c.Any, voxel.TagTest{Tag: strings.TrimPrefix(t, "!"), Not: not})
			}
			cs = append(cs, c)
		}
	}
	return
}

var faceNames = map[string]voxel.Face{"down": voxel.FaceDown, "up": voxel.FaceUp, "north": voxel.FaceNorth,
	"south": voxel.FaceSouth, "west": voxel.FaceWest, "east": voxel.FaceEast}

// collision applies a blocks.json collision value to d.
func (m *Map) collision(d *voxel.BlockDef, raw json.RawMessage) {
	var word string
	var boxes [][6]float32
	switch {
	case len(raw) == 0:
		return // the shape's default
	case json.Unmarshal(raw, &word) == nil:
		switch word {
		case "full":
			d.Collides = true
			if d.Shape == voxel.ShapeBoxes {
				d.Collision = []voxel.Box{}
			}
		case "none":
			d.Collides, d.Collision = false, []voxel.Box{}
		}
	case json.Unmarshal(raw, &boxes) == nil:
		d.Collides, d.Collision = false, make([]voxel.Box, len(boxes))
		for i, b := range boxes {
			d.Collision[i] = voxel.Box{Min: [3]float32{b[0], b[1], b[2]}, Max: [3]float32{b[3], b[4], b[5]}}
		}
	default:
		logf("%s: bad collision %s", d.Name, raw)
	}
}

// textureVars reads "textures" as variable → texture id.
func textureVars(raw json.RawMessage) map[string]string {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return map[string]string{"all": one}
	}
	var vars map[string]string
	json.Unmarshal(raw, &vars)
	return vars
}

// faceTextures reads "textures" as six faces (voxel.Face order).
func faceTextures(raw json.RawMessage) [6]string {
	var six [6]string
	var list []string
	if json.Unmarshal(raw, &list) == nil && len(list) == 6 {
		copy(six[:], list)
		return six
	}
	v := textureVars(raw)
	set := func(t string, faces ...voxel.Face) {
		if t == "" {
			return
		}
		for _, f := range faces {
			six[f] = t
		}
	}
	sides := []voxel.Face{voxel.FaceNorth, voxel.FaceSouth, voxel.FaceWest, voxel.FaceEast}
	set(v["all"], voxel.FaceDown, voxel.FaceUp, voxel.FaceNorth, voxel.FaceSouth, voxel.FaceWest, voxel.FaceEast)
	set(v["side"], sides...)
	set(v["end"], voxel.FaceDown, voxel.FaceUp)
	set(v["top"], voxel.FaceUp)
	set(v["bottom"], voxel.FaceDown)
	for f := voxel.FaceDown; f <= voxel.FaceEast; f++ {
		set(v[f.String()], f)
	}
	return six
}
