// Package blockmodel holds Java Edition block models (the JSON the
// reference game reads from assets/<ns>/models) as Go values, with a
// compact way to write their elements in code:
//
//	blockmodel.Box(0, 0, 0, 16, 8, 16, "d #bottom c", "u #top", "n #side c")
//
// Each face is "<d|u|n|s|w|e> <texture> [uv<x1>,<y1>,<x2>,<y2>] [c|c<face>]
// [r<degrees>] [t<tintindex>]": a face without uv takes the one the
// reference derives from the element's position, "c" culls against the
// face's own side, "cn" (etc.) against another.
package blockmodel

import (
	"fmt"
	"strconv"
	"strings"
)

// Model is a block model file.
type Model struct {
	Parent           string            `json:"parent,omitempty"`
	AmbientOcclusion *bool             `json:"ambientocclusion,omitempty"`
	TextureSize      *[2]float64       `json:"texture_size,omitempty"`
	Textures         map[string]string `json:"textures,omitempty"`
	Elements         []Elem            `json:"elements,omitempty"`
}

// Elem is a model element: a box from From to To in 1/16 block.
type Elem struct {
	From          [3]float64      `json:"from"`
	To            [3]float64      `json:"to"`
	Rotation      *Rot            `json:"rotation,omitempty"`
	Shade         *bool           `json:"shade,omitempty"`
	LightEmission int             `json:"light_emission,omitempty"`
	Faces         map[string]Face `json:"faces"`
}

// Face is one face of an element.
type Face struct {
	Texture   string      `json:"texture"`
	UV        *[4]float64 `json:"uv,omitempty"`
	Cullface  string      `json:"cullface,omitempty"`
	Rotation  int         `json:"rotation,omitempty"`
	TintIndex *int        `json:"tintindex,omitempty"`
}

// Rot turns an element about Origin: by Angle about Axis, or (the newer
// form) by X, Y and Z degrees.
type Rot struct {
	Origin  [3]float64 `json:"origin"`
	Axis    string     `json:"axis,omitempty"`
	Angle   float64    `json:"angle,omitempty"`
	Rescale bool       `json:"rescale,omitempty"`
	X       float64    `json:"x,omitempty"`
	Y       float64    `json:"y,omitempty"`
	Z       float64    `json:"z,omitempty"`
}

// FaceNames are the faces in the order down, up, north, south, west, east.
var FaceNames = [6]string{"down", "up", "north", "south", "west", "east"}

func faceName(c byte) (string, bool) {
	for _, n := range FaceNames {
		if n[0] == c {
			return n, true
		}
	}
	return "", false
}

// Box makes an element from (x1, y1, z1) to (x2, y2, z2) with the given
// faces (see the package comment). A malformed face panics: models are
// written in code.
func Box(x1, y1, z1, x2, y2, z2 float64, faces ...string) Elem {
	e := Elem{From: [3]float64{x1, y1, z1}, To: [3]float64{x2, y2, z2}, Faces: map[string]Face{}}
	for _, spec := range faces {
		name, f, err := parseFace(spec)
		if err != nil {
			panic(fmt.Sprintf("blockmodel: face %q: %v", spec, err))
		}
		e.Faces[name] = f
	}
	return e
}

func parseFace(spec string) (string, Face, error) {
	fs := strings.Fields(spec)
	if len(fs) < 2 || len(fs[0]) != 1 {
		return "", Face{}, fmt.Errorf("want <face> <texture> ...")
	}
	name, ok := faceName(fs[0][0])
	if !ok {
		return "", Face{}, fmt.Errorf("no face %q", fs[0])
	}
	f := Face{Texture: fs[1]}
	for _, w := range fs[2:] {
		switch {
		case strings.HasPrefix(w, "uv"):
			var uv [4]float64
			ps := strings.Split(w[2:], ",")
			if len(ps) != 4 {
				return "", f, fmt.Errorf("uv %q", w)
			}
			for i, p := range ps {
				v, err := strconv.ParseFloat(p, 64)
				if err != nil {
					return "", f, err
				}
				uv[i] = v
			}
			f.UV = &uv
		case w == "c":
			f.Cullface = name
		case w[0] == 'c' && len(w) == 2:
			if f.Cullface, ok = faceName(w[1]); !ok {
				return "", f, fmt.Errorf("cullface %q", w)
			}
		case w[0] == 'r':
			r, err := strconv.Atoi(w[1:])
			if err != nil {
				return "", f, err
			}
			f.Rotation = r
		case w[0] == 't':
			t, err := strconv.Atoi(w[1:])
			if err != nil {
				return "", f, err
			}
			f.TintIndex = &t
		default:
			return "", f, fmt.Errorf("unknown %q", w)
		}
	}
	return name, f, nil
}

// Rot turns the element by angle degrees about axis ("x", "y" or "z")
// through (ox, oy, oz).
func (e Elem) Rot(axis string, angle, ox, oy, oz float64) Elem {
	e.Rotation = &Rot{Origin: [3]float64{ox, oy, oz}, Axis: axis, Angle: angle}
	return e
}

// RotXYZ turns the element by x, y and z degrees through (ox, oy, oz).
func (e Elem) RotXYZ(x, y, z, ox, oy, oz float64) Elem {
	e.Rotation = &Rot{Origin: [3]float64{ox, oy, oz}, X: x, Y: y, Z: z}
	return e
}

// Rescale stretches a turned element back across the block.
func (e Elem) Rescale() Elem {
	r := *e.Rotation
	r.Rescale = true
	e.Rotation = &r
	return e
}

// NoShade draws the element without the reference's per-face shading.
func (e Elem) NoShade() Elem {
	e.Shade = new(bool)
	return e
}

// Light makes the element glow at level n.
func (e Elem) Light(n int) Elem {
	e.LightEmission = n
	return e
}

// DefaultUV is the UV the reference derives for face f (0 down .. 5 east)
// of the box from a to b when the face gives none.
func DefaultUV(f int, a, b [3]float64) [4]float64 {
	switch f {
	case 0:
		return [4]float64{a[0], 16 - b[2], b[0], 16 - a[2]}
	case 1:
		return [4]float64{a[0], a[2], b[0], b[2]}
	case 2:
		return [4]float64{16 - b[0], 16 - b[1], 16 - a[0], 16 - a[1]}
	case 3:
		return [4]float64{a[0], 16 - b[1], b[0], 16 - a[1]}
	case 4:
		return [4]float64{a[2], 16 - b[1], b[2], 16 - a[1]}
	}
	return [4]float64{16 - b[2], 16 - b[1], 16 - a[2], 16 - a[1]}
}
