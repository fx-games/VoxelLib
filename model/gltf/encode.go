package gltf

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"sort"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

// Encode writes m as a self-contained .gltf (buffer and PNG textures as
// data URIs): its bones as nodes, one skin over all of them, a primitive
// per mesh, and its keyframed clips. Load reads it back with one bone
// more, the node that holds the skinned mesh.
func Encode(m *model.Model) ([]byte, error) {
	var buf bytes.Buffer
	type acc struct {
		BufferView    int       `json:"bufferView"`
		ComponentType int       `json:"componentType"`
		Count         int       `json:"count"`
		Type          string    `json:"type"`
		Min           []float32 `json:"min,omitempty"`
		Max           []float32 `json:"max,omitempty"`
	}
	type view struct {
		Buffer     int `json:"buffer"`
		ByteOffset int `json:"byteOffset"`
		ByteLength int `json:"byteLength"`
	}
	var accs []acc
	var views []view
	add := func(data any, comp, count int, typ string) int {
		for buf.Len()%4 != 0 {
			buf.WriteByte(0)
		}
		off := buf.Len()
		binary.Write(&buf, binary.LittleEndian, data)
		views = append(views, view{0, off, buf.Len() - off})
		accs = append(accs, acc{BufferView: len(views) - 1, ComponentType: comp, Count: count, Type: typ})
		return len(accs) - 1
	}
	const f32, u16, u32 = 5126, 5123, 5125

	type node struct {
		Name        string    `json:"name,omitempty"`
		Children    []int     `json:"children,omitempty"`
		Mesh        *int      `json:"mesh,omitempty"`
		Skin        *int      `json:"skin,omitempty"`
		Translation []float32 `json:"translation,omitempty"`
		Rotation    []float32 `json:"rotation,omitempty"`
		Scale       []float32 `json:"scale,omitempty"`
	}
	nodes := make([]node, len(m.Bones))
	var roots []int
	invBind := make([]float32, 0, 16*len(m.Bones))
	joints := make([]int, len(m.Bones))
	for i, b := range m.Bones {
		r := b.R
		if b.UseEuler {
			r = model.EulerZYX(b.Euler)
		}
		s := b.S
		if s == (mgl32.Vec3{}) {
			s = mgl32.Vec3{1, 1, 1}
		}
		nodes[i] = node{Name: b.Name, Translation: b.T[:], Rotation: []float32{r.V[0], r.V[1], r.V[2], r.W}, Scale: s[:]}
		if b.Parent >= 0 {
			nodes[b.Parent].Children = append(nodes[b.Parent].Children, i)
		} else {
			roots = append(roots, i)
		}
		ib := b.InvBind
		if ib == (mgl32.Mat4{}) {
			ib = mgl32.Ident4()
		}
		invBind = append(invBind, ib[:]...)
		joints[i] = i
	}

	type prim struct {
		Attributes map[string]int `json:"attributes"`
		Indices    int            `json:"indices"`
		Material   *int           `json:"material,omitempty"`
	}
	var prims []prim
	for _, me := range m.Meshes {
		n := len(me.Verts)
		pos, nor, uv := make([]float32, 0, 3*n), make([]float32, 0, 3*n), make([]float32, 0, 2*n)
		jo, we := make([]uint16, 0, 4*n), make([]float32, 0, 4*n)
		lo, hi := mgl32.Vec3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}, mgl32.Vec3{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
		for _, v := range me.Verts {
			pos, nor, uv = append(pos, v.Pos[:]...), append(nor, v.Normal[:]...), append(uv, v.UV[:]...)
			jo, we = append(jo, v.Joints[:]...), append(we, v.Weights[:]...)
			for k := range 3 {
				lo[k], hi[k] = min(lo[k], v.Pos[k]), max(hi[k], v.Pos[k])
			}
		}
		p := prim{Attributes: map[string]int{
			"POSITION":   add(pos, f32, n, "VEC3"),
			"NORMAL":     add(nor, f32, n, "VEC3"),
			"TEXCOORD_0": add(uv, f32, n, "VEC2"),
			"JOINTS_0":   add(jo, u16, n, "VEC4"),
			"WEIGHTS_0":  add(we, f32, n, "VEC4"),
		}}
		accs[p.Attributes["POSITION"]].Min, accs[p.Attributes["POSITION"]].Max = lo[:], hi[:]
		p.Indices = add(me.Indices, u32, len(me.Indices), "SCALAR")
		if me.Texture >= 0 && me.Texture < len(m.Textures) {
			t := me.Texture
			p.Material = &t
		}
		prims = append(prims, p)
	}
	mesh, skin := 0, 0
	nodes = append(nodes, node{Name: m.Name, Mesh: &mesh, Skin: &skin})
	roots = append(roots, len(nodes)-1)
	ibAcc := add(invBind, f32, len(m.Bones), "MAT4")

	type channel struct {
		Sampler int `json:"sampler"`
		Target  struct {
			Node int    `json:"node"`
			Path string `json:"path"`
		} `json:"target"`
	}
	type sampler struct {
		Input         int    `json:"input"`
		Output        int    `json:"output"`
		Interpolation string `json:"interpolation"`
	}
	type anim struct {
		Name     string    `json:"name"`
		Channels []channel `json:"channels"`
		Samplers []sampler `json:"samplers"`
	}
	var anims []anim
	names := make([]string, 0, len(m.Clips))
	for n := range m.Clips {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		c := m.Clips[name]
		a := anim{Name: name}
		for _, ch := range c.Channels {
			k, ok := ch.Track.(*model.Keyframes)
			if !ok || ch.Additive {
				return nil, fmt.Errorf("gltf: clip %q: only absolute keyframes encode", name)
			}
			path, comps, typ := "translation", 3, "VEC3"
			switch ch.Target {
			case model.Rotation:
				path, comps, typ = "rotation", 4, "VEC4"
			case model.Scale:
				path = "scale"
			}
			var out []float32
			for _, v := range k.Values {
				out = append(out, v[:comps]...)
			}
			interp := "LINEAR"
			if k.Interp == model.Step {
				interp = "STEP"
			}
			in := add(k.Times, f32, len(k.Times), "SCALAR")
			if len(k.Times) > 0 {
				accs[in].Min, accs[in].Max = []float32{k.Times[0]}, []float32{k.Times[len(k.Times)-1]}
			}
			a.Samplers = append(a.Samplers, sampler{in, add(out, f32, len(k.Values), typ), interp})
			var cc channel
			cc.Sampler, cc.Target.Node, cc.Target.Path = len(a.Samplers)-1, ch.Bone, path
			a.Channels = append(a.Channels, cc)
		}
		anims = append(anims, a)
	}

	type img struct {
		URI string `json:"uri"`
	}
	type src struct {
		Source  int `json:"source"`
		Sampler int `json:"sampler"`
	}
	type mat struct {
		PBR struct {
			BaseColorTexture struct {
				Index int `json:"index"`
			} `json:"baseColorTexture"`
			Metallic float32 `json:"metallicFactor"`
		} `json:"pbrMetallicRoughness"`
		AlphaMode string `json:"alphaMode"`
	}
	var imgs []img
	var texs []src
	var mats []mat
	for i, t := range m.Textures {
		var p bytes.Buffer
		if err := png.Encode(&p, t); err != nil {
			return nil, err
		}
		imgs = append(imgs, img{"data:image/png;base64," + base64.StdEncoding.EncodeToString(p.Bytes())})
		texs = append(texs, src{i, 0})
		var mt mat
		mt.PBR.BaseColorTexture.Index, mt.AlphaMode = i, "MASK"
		mats = append(mats, mt)
	}
	doc := map[string]any{
		"asset":       map[string]string{"version": "2.0", "generator": "voxellib gltf.Encode"},
		"scene":       0,
		"scenes":      []map[string]any{{"nodes": roots}},
		"nodes":       nodes,
		"meshes":      []map[string]any{{"name": m.Name, "primitives": prims}},
		"skins":       []map[string]any{{"joints": joints, "inverseBindMatrices": ibAcc}},
		"accessors":   accs,
		"bufferViews": views,
		"buffers":     []map[string]any{{"byteLength": buf.Len(), "uri": "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())}},
		"samplers":    []map[string]int{{"magFilter": 9728, "minFilter": 9728}}, // nearest: pixel art
	}
	if len(anims) > 0 {
		doc["animations"] = anims
	}
	if len(imgs) > 0 {
		doc["images"], doc["textures"], doc["materials"] = imgs, texs, mats
	}
	return json.MarshalIndent(doc, "", " ")
}
