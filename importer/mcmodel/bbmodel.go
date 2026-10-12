package mcmodel

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"strconv"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

type bbFile struct {
	Name       string `json:"name"`
	Resolution struct {
		Width, Height float32
	} `json:"resolution"`
	Textures []struct {
		Name     string  `json:"name"`
		ID       string  `json:"id"`
		UUID     string  `json:"uuid"`
		Source   string  `json:"source"`
		UVWidth  float32 `json:"uv_width"`
		UVHeight float32 `json:"uv_height"`
	} `json:"textures"`
	Elements []struct {
		UUID       string     `json:"uuid"`
		Type       string     `json:"type"`
		From       [3]float32 `json:"from"`
		To         [3]float32 `json:"to"`
		Origin     [3]float32 `json:"origin"`
		Rotation   [3]float32 `json:"rotation"`
		Inflate    float32    `json:"inflate"`
		Visibility *bool      `json:"visibility"`
		Faces      map[string]struct {
			UV       [4]float32      `json:"uv"`
			Texture  json.RawMessage `json:"texture"`
			Rotation int             `json:"rotation"`
		} `json:"faces"`
	} `json:"elements"`
	Groups     []bbGroup         `json:"groups"`
	Outliner   []json.RawMessage `json:"outliner"`
	Animations []struct {
		Name      string `json:"name"`
		Loop      string `json:"loop"`
		Length    float32
		Animators map[string]struct {
			Name      string `json:"name"`
			Type      string `json:"type"`
			Keyframes []struct {
				Channel       string                       `json:"channel"`
				Time          float32                      `json:"time"`
				Interpolation string                       `json:"interpolation"`
				DataPoints    []map[string]json.RawMessage `json:"data_points"`
			} `json:"keyframes"`
		} `json:"animators"`
	} `json:"animations"`
}

type bbGroup struct {
	UUID     string            `json:"uuid"`
	Name     string            `json:"name"`
	Origin   [3]float32        `json:"origin"`
	Rotation [3]float32        `json:"rotation"`
	Children []json.RawMessage `json:"children"`
}

// ParseBBModel reads a Blockbench project (.bbmodel; any Bedrock, GeckoLib
// or generic entity format) into a model plus the animations embedded in
// it, already bound to the model's clips. Textures embedded as data URIs
// are decoded; cubes become rigid parts of their group's bone.
func ParseBBModel(data []byte) (*model.Model, []*Animation, error) {
	var f bbFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, nil, fmt.Errorf("mcmodel: %w", err)
	}
	m := &model.Model{Name: f.Name, Clips: map[string]*model.Clip{}, Tags: map[string]bool{}}
	resW, resH := f.Resolution.Width, f.Resolution.Height
	if resW == 0 {
		resW, resH = 16, 16
	}

	// Textures.
	texIndex := map[string]int{}
	texUV := []mgl32.Vec2{}
	for i, t := range f.Textures {
		img, err := decodeDataURI(t.Source)
		if err != nil {
			img = image.NewNRGBA(image.Rect(0, 0, 1, 1)) // keep indices stable
		}
		uw, uh := t.UVWidth, t.UVHeight
		if uw == 0 {
			uw, uh = resW, resH
		}
		texUV = append(texUV, mgl32.Vec2{uw, uh})
		m.Textures = append(m.Textures, img)
		texIndex[strconv.Itoa(i)] = i
		if t.ID != "" {
			texIndex[t.ID] = i
		}
		if t.UUID != "" {
			texIndex[t.UUID] = i
		}
	}

	// Bones from the outliner (groups may be inline or in "groups").
	groups := map[string]bbGroup{}
	for _, g := range f.Groups {
		groups[g.UUID] = g
	}
	elemBone := map[string]int{}
	origins := []mgl32.Vec3{}
	var walk func(raw []json.RawMessage, parent int)
	walk = func(raw []json.RawMessage, parent int) {
		for _, r := range raw {
			var uuid string
			if json.Unmarshal(r, &uuid) == nil {
				elemBone[uuid] = parent
				continue
			}
			var g bbGroup
			if json.Unmarshal(r, &g) != nil {
				continue
			}
			if full, ok := groups[g.UUID]; ok && g.Name == "" {
				full.Children = g.Children
				g = full
			}
			o := mgl32.Vec3{g.Origin[0], g.Origin[1], g.Origin[2]}.Mul(1.0 / 16)
			t := o
			if parent >= 0 {
				t = o.Sub(origins[parent])
			}
			m.Bones = append(m.Bones, model.Bone{Name: g.Name, Parent: parent, T: t, R: mgl32.QuatIdent(),
				S: mgl32.Vec3{1, 1, 1}, UseEuler: true, Euler: mgl32.Vec3{g.Rotation[0], g.Rotation[1], g.Rotation[2]}})
			origins = append(origins, o)
			walk(g.Children, len(m.Bones)-1)
		}
	}
	walk(f.Outliner, -1)
	root := -1
	boneFor := func(uuid string) int {
		if b, ok := elemBone[uuid]; ok && b >= 0 {
			return b
		}
		if root < 0 {
			m.Bones = append(m.Bones, model.Bone{Name: "root", Parent: -1, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, UseEuler: true})
			origins = append(origins, mgl32.Vec3{})
			root = len(m.Bones) - 1
		}
		return root
	}

	// Cubes.
	meshOf := map[int]int{} // texture -> mesh
	for _, e := range f.Elements {
		if (e.Type != "" && e.Type != "cube") || (e.Visibility != nil && !*e.Visibility) {
			continue
		}
		bone := boneFor(e.UUID)
		if bone > 255 {
			continue
		}
		lo := mgl32.Vec3{min(e.From[0], e.To[0]), min(e.From[1], e.To[1]), min(e.From[2], e.To[2])}.Sub(mgl32.Vec3{e.Inflate, e.Inflate, e.Inflate})
		hi := mgl32.Vec3{max(e.From[0], e.To[0]), max(e.From[1], e.To[1]), max(e.From[2], e.To[2])}.Add(mgl32.Vec3{e.Inflate, e.Inflate, e.Inflate})
		rot := model.EulerZYX(mgl32.Vec3{e.Rotation[0], e.Rotation[1], e.Rotation[2]}).Mat4()
		org := mgl32.Vec3{e.Origin[0], e.Origin[1], e.Origin[2]}
		xf := mgl32.Scale3D(1.0/16, 1.0/16, 1.0/16).Mul4(mgl32.Translate3D(org[0], org[1], org[2])).Mul4(rot).Mul4(mgl32.Translate3D(-org[0], -org[1], -org[2]))
		for dir, face := range e.Faces {
			if len(face.Texture) == 0 || string(face.Texture) == "null" || string(face.Texture) == "false" {
				continue
			}
			ti, ok := texIndex[strings.Trim(string(face.Texture), `"`)]
			if !ok {
				continue
			}
			corners, normal, ok := faceCorners(dir, lo, hi)
			if !ok {
				continue
			}
			mi, ok := meshOf[ti]
			if !ok {
				mi = len(m.Meshes)
				meshOf[ti] = mi
				m.Meshes = append(m.Meshes, model.Mesh{Texture: ti})
			}
			me := &m.Meshes[mi]
			u1, v1, u2, v2 := face.UV[0]/texUV[ti][0], face.UV[1]/texUV[ti][1], face.UV[2]/texUV[ti][0], face.UV[3]/texUV[ti][1]
			uvs := [4]mgl32.Vec2{{u1, v1}, {u2, v1}, {u2, v2}, {u1, v2}}
			k := ((face.Rotation/90)%4 + 4) % 4
			n := rot.Mul4x1(normal.Vec4(0)).Vec3()
			base := uint32(len(me.Verts))
			for i, c := range corners {
				v := model.Vertex{Pos: mgl32.TransformCoordinate(c, xf), Normal: n, UV: uvs[(i-k+4)%4]}
				v.Joints[0], v.Weights[0] = uint8(bone), 1
				me.Verts = append(me.Verts, v)
			}
			me.Indices = append(me.Indices, base, base+3, base+2, base, base+2, base+1)
		}
	}
	if len(m.Bones) == 0 {
		return nil, nil, fmt.Errorf("mcmodel: no geometry")
	}
	// Cube coordinates are stored before their group's rotation is applied,
	// so the bind pose is the unrotated model: bone space = model - pivot.
	for i := range m.Bones {
		o := origins[i]
		m.Bones[i].InvBind = mgl32.Translate3D(-o[0], -o[1], -o[2])
	}
	attachArmor(m)

	// Embedded animations.
	var anims []*Animation
	for _, a := range f.Animations {
		an := &Animation{Name: a.Name, Length: a.Length, Loop: model.LoopOnce, Bones: map[string]*BoneAnim{}}
		switch a.Loop {
		case "loop":
			an.Loop = model.LoopRepeat
		case "hold":
			an.Loop = model.LoopHold
		}
		for _, at := range a.Animators {
			if at.Type != "" && at.Type != "bone" {
				continue
			}
			ba := an.Bones[at.Name]
			if ba == nil {
				ba = &BoneAnim{}
				an.Bones[at.Name] = ba
			}
			for _, kf := range at.Keyframes {
				var dst **Track
				var mul [3]float32
				switch kf.Channel {
				case "rotation":
					dst, mul = &ba.Rotation, rotMul
				case "position":
					dst, mul = &ba.Position, posMul
				case "scale":
					dst, mul = &ba.Scale, sclMul
				default:
					continue
				}
				if *dst == nil {
					*dst = &Track{mul: mul}
				}
				k := key{t: kf.Time}
				switch kf.Interpolation {
				case "catmullrom":
					k.interp = model.CatmullRom
				case "step":
					k.interp = model.Step
				}
				if len(kf.DataPoints) == 0 {
					continue
				}
				var err error
				if k.pre, err = bbPoint(kf.DataPoints[0]); err != nil {
					return nil, nil, fmt.Errorf("mcmodel: %s: %w", a.Name, err)
				}
				k.post = k.pre
				if len(kf.DataPoints) > 1 {
					if k.post, err = bbPoint(kf.DataPoints[1]); err != nil {
						return nil, nil, err
					}
				}
				(*dst).keys = append((*dst).keys, k)
			}
		}
		for _, ba := range an.Bones {
			for _, tr := range []*Track{ba.Rotation, ba.Position, ba.Scale} {
				if tr != nil {
					tr.sort()
					if an.Length == 0 {
						an.Length = max(an.Length, tr.end())
					}
				}
			}
		}
		anims = append(anims, an)
		if c, ok := an.Bind(m); ok {
			m.Clips[an.Name] = c
		}
	}
	return m, anims, nil
}

func bbPoint(p map[string]json.RawMessage) ([3]Expr, error) {
	var out [3]Expr
	for i, c := range []string{"x", "y", "z"} {
		raw, ok := p[c]
		if !ok {
			out[i] = num(0)
			continue
		}
		e, err := parseScalar(raw)
		if err != nil {
			return out, err
		}
		out[i] = e
	}
	return out, nil
}

// faceCorners returns a cube face's corners as top-left, top-right,
// bottom-right, bottom-left seen from outside (matching UV corners), and
// its normal. Coordinates are Blockbench pixels.
func faceCorners(dir string, lo, hi mgl32.Vec3) ([4]mgl32.Vec3, mgl32.Vec3, bool) {
	x0, y0, z0, x1, y1, z1 := lo[0], lo[1], lo[2], hi[0], hi[1], hi[2]
	switch dir {
	case "north":
		return [4]mgl32.Vec3{{x1, y1, z0}, {x0, y1, z0}, {x0, y0, z0}, {x1, y0, z0}}, mgl32.Vec3{0, 0, -1}, true
	case "south":
		return [4]mgl32.Vec3{{x0, y1, z1}, {x1, y1, z1}, {x1, y0, z1}, {x0, y0, z1}}, mgl32.Vec3{0, 0, 1}, true
	case "east":
		return [4]mgl32.Vec3{{x1, y1, z1}, {x1, y1, z0}, {x1, y0, z0}, {x1, y0, z1}}, mgl32.Vec3{1, 0, 0}, true
	case "west":
		return [4]mgl32.Vec3{{x0, y1, z0}, {x0, y1, z1}, {x0, y0, z1}, {x0, y0, z0}}, mgl32.Vec3{-1, 0, 0}, true
	case "up":
		return [4]mgl32.Vec3{{x0, y1, z0}, {x1, y1, z0}, {x1, y1, z1}, {x0, y1, z1}}, mgl32.Vec3{0, 1, 0}, true
	case "down":
		return [4]mgl32.Vec3{{x0, y0, z1}, {x1, y0, z1}, {x1, y0, z0}, {x0, y0, z0}}, mgl32.Vec3{0, -1, 0}, true
	}
	return [4]mgl32.Vec3{}, mgl32.Vec3{}, false
}

func decodeDataURI(s string) (image.Image, error) {
	i := strings.Index(s, ",")
	if !strings.HasPrefix(s, "data:") || i < 0 {
		return nil, fmt.Errorf("not a data URI")
	}
	data, err := base64.StdEncoding.DecodeString(s[i+1:])
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}
