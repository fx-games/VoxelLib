package bbmodel

import (
	"encoding/json"
	"fmt"
	"image"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/resloc"
)

// ParseGeometry reads a Bedrock geometry file (format 1.12+:
// the default-namespace "geometry" key), as exported by Blockbench for Bedrock and
// GeckoLib. texture returns the PNG for a geometry identifier (without
// "geometry." prefix), or nil.
func ParseGeometry(data []byte, texture func(id string) image.Image) ([]*model.Model, error) {
	var geo []struct {
		Description struct {
			Identifier string  `json:"identifier"`
			TexW       float32 `json:"texture_width"`
			TexH       float32 `json:"texture_height"`
		} `json:"description"`
		Bones []struct {
			Name     string     `json:"name"`
			Parent   string     `json:"parent"`
			Pivot    [3]float32 `json:"pivot"`
			Rotation [3]float32 `json:"rotation"`
			Mirror   bool       `json:"mirror"`
			Cubes    []struct {
				Origin   [3]float32      `json:"origin"`
				Size     [3]float32      `json:"size"`
				Pivot    *[3]float32     `json:"pivot"`
				Rotation [3]float32      `json:"rotation"`
				Inflate  float32         `json:"inflate"`
				Mirror   *bool           `json:"mirror"`
				UV       json.RawMessage `json:"uv"`
			} `json:"cubes"`
		} `json:"bones"`
	}
	key := resloc.Qualify("geometry")
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(top[key], &geo); err != nil || len(geo) == 0 {
		return nil, fmt.Errorf("bbmodel: no %s", key)
	}
	var out []*model.Model
	for _, g := range geo {
		id := g.Description.Identifier
		if len(id) > 9 && id[:9] == "geometry." {
			id = id[9:]
		}
		tw, th := g.Description.TexW, g.Description.TexH
		if tw == 0 {
			tw, th = 64, 64
		}
		m := &model.Model{Name: id, Clips: map[string]*model.Clip{}, Tags: map[string]bool{}}
		tex := texture(id)
		if tex == nil {
			tex = image.NewNRGBA(image.Rect(0, 0, 1, 1))
		}
		m.Textures = []image.Image{tex}
		me := model.Mesh{Texture: 0}
		boneIdx := map[string]int{}
		var origins []mgl32.Vec3
		// Bedrock space mirrors X relative to Blockbench/model space.
		flip := func(v [3]float32) mgl32.Vec3 { return mgl32.Vec3{-v[0], v[1], v[2]} }
		for _, b := range g.Bones {
			parent := -1
			if pi, ok := boneIdx[b.Parent]; ok {
				parent = pi
			}
			o := flip(b.Pivot).Mul(1.0 / 16)
			t := o
			if parent >= 0 {
				t = o.Sub(origins[parent])
			}
			boneIdx[b.Name] = len(m.Bones)
			m.Bones = append(m.Bones, model.Bone{Name: b.Name, Parent: parent, T: t, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1},
				UseEuler: true, Euler: mgl32.Vec3{-b.Rotation[0], -b.Rotation[1], b.Rotation[2]}})
			origins = append(origins, o)
			bi := len(m.Bones) - 1
			for _, c := range b.Cubes {
				// Blockbench from/to: X mirrored.
				from := mgl32.Vec3{-(c.Origin[0] + c.Size[0]), c.Origin[1], c.Origin[2]}
				to := from.Add(mgl32.Vec3{c.Size[0], c.Size[1], c.Size[2]})
				inf := mgl32.Vec3{c.Inflate, c.Inflate, c.Inflate}
				lo, hi := from.Sub(inf), to.Add(inf)
				piv := from
				if c.Pivot != nil {
					piv = flip(*c.Pivot)
				}
				rot := model.EulerZYX(mgl32.Vec3{-c.Rotation[0], -c.Rotation[1], c.Rotation[2]}).Mat4()
				xf := mgl32.Scale3D(1.0/16, 1.0/16, 1.0/16).Mul4(mgl32.Translate3D(piv[0], piv[1], piv[2])).Mul4(rot).Mul4(mgl32.Translate3D(-piv[0], -piv[1], -piv[2]))
				mirror := b.Mirror
				if c.Mirror != nil {
					mirror = *c.Mirror
				}
				faces := geoFaceUVs(c.UV, c.Size, mirror)
				for dir, uv := range faces {
					corners, normal, _ := faceCorners(dir, lo, hi)
					uvs := [4]mgl32.Vec2{{uv[0] / tw, uv[1] / th}, {uv[2] / tw, uv[1] / th}, {uv[2] / tw, uv[3] / th}, {uv[0] / tw, uv[3] / th}}
					n := rot.Mul4x1(normal.Vec4(0)).Vec3()
					base := uint32(len(me.Verts))
					for i, cn := range corners {
						v := model.Vertex{Pos: mgl32.TransformCoordinate(cn, xf), Normal: n, UV: uvs[i]}
						v.Joints[0], v.Weights[0] = uint16(bi), 1
						me.Verts = append(me.Verts, v)
					}
					me.Indices = append(me.Indices, base, base+3, base+2, base, base+2, base+1)
				}
			}
		}
		m.Meshes = []model.Mesh{me}
		// Cube coordinates are stored before their group's rotation is applied,
		// so the bind pose is the unrotated model: bone space = model - pivot.
		for i := range m.Bones {
			o := origins[i]
			m.Bones[i].InvBind = mgl32.Translate3D(-o[0], -o[1], -o[2])
		}
		attachArmor(m)
		out = append(out, m)
	}
	return out, nil
}

// geoFaceUVs returns [u1,v1,u2,v2] per face for box UV ([u,v]) or per-face
// UV ({"north": {"uv": [u,v], "uv_size": [w,h]}}).
func geoFaceUVs(raw json.RawMessage, size [3]float32, mirror bool) map[string][4]float32 {
	out := map[string][4]float32{}
	var box [2]float32
	if json.Unmarshal(raw, &box) == nil {
		w, h, d := size[0], size[1], size[2]
		u, v := box[0], box[1]
		out["east"] = [4]float32{u, v + d, u + d, v + d + h}
		out["north"] = [4]float32{u + d, v + d, u + d + w, v + d + h}
		out["west"] = [4]float32{u + d + w, v + d, u + 2*d + w, v + d + h}
		out["south"] = [4]float32{u + 2*d + w, v + d, u + 2*d + 2*w, v + d + h}
		out["up"] = [4]float32{u + d + w, v + d, u + d, v}
		out["down"] = [4]float32{u + d + 2*w, v, u + d + w, v + d}
		if mirror {
			for k, f := range out {
				out[k] = [4]float32{f[2], f[1], f[0], f[3]}
			}
			out["east"], out["west"] = out["west"], out["east"]
		}
		return out
	}
	var per map[string]struct {
		UV     [2]float32 `json:"uv"`
		UVSize [2]float32 `json:"uv_size"`
	}
	if json.Unmarshal(raw, &per) == nil {
		for k, f := range per {
			out[k] = [4]float32{f.UV[0], f.UV[1], f.UV[0] + f.UVSize[0], f.UV[1] + f.UVSize[1]}
		}
	}
	return out
}
