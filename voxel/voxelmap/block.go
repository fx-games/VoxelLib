package voxelmap

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/voxel"
)

// Block is one blocks.json entry.
//
//   - shape: "cube" (default), "cross", "liquid", "none" or
//     "model:<model id>" (a JSON block model under models/). A model may be
//     turned like a blockstate variant with "x"/"y" (multiples of 90°),
//     given at the top level or as "rotation": {"x":..,"y":..}.
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
//     (default for cross, liquid and none).
//
// Every other field (sounds, ...) is left to the game in Raw.
type Block struct {
	Key       string              `json:"key"`
	Shape     string              `json:"shape"`
	X         int                 `json:"x"`
	Y         int                 `json:"y"`
	Rotation  *struct{ X, Y int } `json:"rotation"`
	Textures  json.RawMessage     `json:"textures"`
	Layer     string              `json:"layer"`
	Opacity   *int                `json:"opacity"`
	Emission  int                 `json:"emission"`
	Collision json.RawMessage     `json:"collision"`
	Tint      string              `json:"tint"`
	Flags     []string            `json:"flags"`
	Raw       json.RawMessage     `json:"-"`
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
		rx, ry := b.X, b.Y
		if b.Rotation != nil {
			rx, ry = b.Rotation.X, b.Rotation.Y
		}
		boxes = voxel.RotateBoxes(boxes, 0, float32(rx))
		d.Boxes = voxel.RotateBoxes(boxes, 1, float32(ry))
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
		if f == "replaceable" {
			d.Replaceable = true
		}
	}
	return d
}

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
