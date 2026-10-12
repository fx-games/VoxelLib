// Package gltf loads glTF 2.0 files (.glb, or .gltf with embedded or
// sibling buffers) into a model.Model: meshes, skins, node hierarchy,
// base colour textures and all animations (translation, rotation, scale;
// LINEAR, STEP and CUBICSPLINE samplers).
//
// Every node becomes a bone, so both skinned meshes and rigid node
// hierarchies animate. Morph targets are not supported.
package gltf

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

type doc struct {
	Scene  *int `json:"scene"`
	Scenes []struct {
		Nodes []int `json:"nodes"`
	} `json:"scenes"`
	Nodes []struct {
		Name        string    `json:"name"`
		Children    []int     `json:"children"`
		Mesh        *int      `json:"mesh"`
		Skin        *int      `json:"skin"`
		Translation []float32 `json:"translation"`
		Rotation    []float32 `json:"rotation"`
		Scale       []float32 `json:"scale"`
		Matrix      []float32 `json:"matrix"`
	} `json:"nodes"`
	Meshes []struct {
		Name       string `json:"name"`
		Primitives []struct {
			Attributes map[string]int `json:"attributes"`
			Indices    *int           `json:"indices"`
			Material   *int           `json:"material"`
			Mode       *int           `json:"mode"`
		} `json:"primitives"`
	} `json:"meshes"`
	Skins []struct {
		Joints              []int `json:"joints"`
		InverseBindMatrices *int  `json:"inverseBindMatrices"`
	} `json:"skins"`
	Animations []struct {
		Name     string `json:"name"`
		Channels []struct {
			Sampler int `json:"sampler"`
			Target  struct {
				Node *int   `json:"node"`
				Path string `json:"path"`
			} `json:"target"`
		} `json:"channels"`
		Samplers []struct {
			Input         int    `json:"input"`
			Output        int    `json:"output"`
			Interpolation string `json:"interpolation"`
		} `json:"samplers"`
	} `json:"animations"`
	Accessors []struct {
		BufferView    *int   `json:"bufferView"`
		ByteOffset    int    `json:"byteOffset"`
		ComponentType int    `json:"componentType"`
		Normalized    bool   `json:"normalized"`
		Count         int    `json:"count"`
		Type          string `json:"type"`
	} `json:"accessors"`
	BufferViews []struct {
		Buffer     int `json:"buffer"`
		ByteOffset int `json:"byteOffset"`
		ByteLength int `json:"byteLength"`
		ByteStride int `json:"byteStride"`
	} `json:"bufferViews"`
	Buffers []struct {
		URI        string `json:"uri"`
		ByteLength int    `json:"byteLength"`
	} `json:"buffers"`
	Materials []struct {
		Name string `json:"name"`
		PBR  struct {
			BaseColorTexture *struct {
				Index int `json:"index"`
			} `json:"baseColorTexture"`
		} `json:"pbrMetallicRoughness"`
	} `json:"materials"`
	Textures []struct {
		Source *int `json:"source"`
	} `json:"textures"`
	Images []struct {
		URI        string `json:"uri"`
		BufferView *int   `json:"bufferView"`
	} `json:"images"`
}

// LoadFile loads a .glb or .gltf file.
func LoadFile(path string) (*model.Model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	m, err := Load(data, func(uri string) ([]byte, error) { return os.ReadFile(filepath.Join(dir, uri)) })
	if m != nil {
		m.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return m, err
}

// Load decodes glTF from memory. external resolves relative URIs of
// buffers and images; it may be nil for self-contained files.
func Load(data []byte, external func(uri string) ([]byte, error)) (*model.Model, error) {
	var jsonPart, bin []byte
	if len(data) >= 12 && string(data[:4]) == "glTF" {
		p := 12
		for p+8 <= len(data) {
			l := int(binary.LittleEndian.Uint32(data[p:]))
			typ := string(data[p+4 : p+8])
			if p+8+l > len(data) {
				return nil, errors.New("gltf: truncated chunk")
			}
			switch typ {
			case "JSON":
				jsonPart = data[p+8 : p+8+l]
			case "BIN\x00":
				bin = data[p+8 : p+8+l]
			}
			p += 8 + l
		}
	} else {
		jsonPart = data
	}
	var d doc
	if err := json.Unmarshal(jsonPart, &d); err != nil {
		return nil, fmt.Errorf("gltf: %w", err)
	}
	l := &loader{d: &d, external: external}
	for i, b := range d.Buffers {
		switch {
		case b.URI == "" && i == 0:
			l.buffers = append(l.buffers, bin)
		default:
			buf, err := l.uri(b.URI)
			if err != nil {
				return nil, err
			}
			l.buffers = append(l.buffers, buf)
		}
	}
	return l.build()
}

type loader struct {
	d        *doc
	external func(string) ([]byte, error)
	buffers  [][]byte
}

func (l *loader) uri(u string) ([]byte, error) {
	if strings.HasPrefix(u, "data:") {
		i := strings.Index(u, ",")
		if i < 0 {
			return nil, errors.New("gltf: bad data URI")
		}
		return base64.StdEncoding.DecodeString(u[i+1:])
	}
	if l.external == nil {
		return nil, fmt.Errorf("gltf: external resource %q not available", u)
	}
	return l.external(u)
}

func (l *loader) view(i int) ([]byte, int, error) {
	if i < 0 || i >= len(l.d.BufferViews) {
		return nil, 0, fmt.Errorf("gltf: bad bufferView %d", i)
	}
	v := l.d.BufferViews[i]
	if v.Buffer >= len(l.buffers) || v.ByteOffset+v.ByteLength > len(l.buffers[v.Buffer]) {
		return nil, 0, errors.New("gltf: bufferView out of range")
	}
	return l.buffers[v.Buffer][v.ByteOffset : v.ByteOffset+v.ByteLength], v.ByteStride, nil
}

var compCount = map[string]int{"SCALAR": 1, "VEC2": 2, "VEC3": 3, "VEC4": 4, "MAT4": 16}

// floats reads an accessor as float32s (normalising integer types when
// the accessor says so), n components per element.
func (l *loader) floats(i int) ([]float32, int, error) {
	if i < 0 || i >= len(l.d.Accessors) {
		return nil, 0, fmt.Errorf("gltf: bad accessor %d", i)
	}
	a := l.d.Accessors[i]
	n := compCount[a.Type]
	if n == 0 {
		return nil, 0, fmt.Errorf("gltf: accessor type %s", a.Type)
	}
	out := make([]float32, a.Count*n)
	if a.BufferView == nil {
		return out, n, nil // all zeros (sparse not supported)
	}
	buf, stride, err := l.view(*a.BufferView)
	if err != nil {
		return nil, 0, err
	}
	size := map[int]int{5120: 1, 5121: 1, 5122: 2, 5123: 2, 5125: 4, 5126: 4}[a.ComponentType]
	if size == 0 {
		return nil, 0, fmt.Errorf("gltf: component type %d", a.ComponentType)
	}
	if stride == 0 {
		stride = size * n
	}
	for e := 0; e < a.Count; e++ {
		for c := 0; c < n; c++ {
			o := a.ByteOffset + e*stride + c*size
			if o+size > len(buf) {
				return nil, 0, errors.New("gltf: accessor out of range")
			}
			var v float32
			switch a.ComponentType {
			case 5126:
				v = math.Float32frombits(binary.LittleEndian.Uint32(buf[o:]))
			case 5121:
				v = float32(buf[o])
				if a.Normalized {
					v /= 255
				}
			case 5120:
				v = float32(int8(buf[o]))
				if a.Normalized {
					v = float32(math.Max(float64(v)/127, -1))
				}
			case 5123:
				v = float32(binary.LittleEndian.Uint16(buf[o:]))
				if a.Normalized {
					v /= 65535
				}
			case 5122:
				v = float32(int16(binary.LittleEndian.Uint16(buf[o:])))
				if a.Normalized {
					v = float32(math.Max(float64(v)/32767, -1))
				}
			case 5125:
				v = float32(binary.LittleEndian.Uint32(buf[o:]))
			}
			out[e*n+c] = v
		}
	}
	return out, n, nil
}

func (l *loader) build() (*model.Model, error) {
	d := l.d
	m := &model.Model{Clips: map[string]*model.Clip{}, Tags: map[string]bool{}}

	// Bones: every node reachable from the scene, parents first.
	boneOf := make([]int, len(d.Nodes))
	for i := range boneOf {
		boneOf[i] = -1
	}
	var roots []int
	if len(d.Scenes) > 0 {
		s := 0
		if d.Scene != nil {
			s = *d.Scene
		}
		roots = d.Scenes[s].Nodes
	} else {
		isChild := map[int]bool{}
		for _, n := range d.Nodes {
			for _, c := range n.Children {
				isChild[c] = true
			}
		}
		for i := range d.Nodes {
			if !isChild[i] {
				roots = append(roots, i)
			}
		}
	}
	var visit func(n, parent int) error
	visit = func(n, parent int) error {
		if n < 0 || n >= len(d.Nodes) || boneOf[n] >= 0 {
			return errors.New("gltf: bad node graph")
		}
		nd := d.Nodes[n]
		b := model.Bone{Name: nd.Name, Parent: parent, R: mgl32.QuatIdent(), S: mgl32.Vec3{1, 1, 1}, InvBind: mgl32.Ident4()}
		if len(nd.Matrix) == 16 {
			var mat mgl32.Mat4
			copy(mat[:], nd.Matrix)
			b.T, b.R, b.S = decompose(mat)
		}
		if len(nd.Translation) == 3 {
			b.T = mgl32.Vec3{nd.Translation[0], nd.Translation[1], nd.Translation[2]}
		}
		if len(nd.Rotation) == 4 {
			b.R = mgl32.Quat{W: nd.Rotation[3], V: mgl32.Vec3{nd.Rotation[0], nd.Rotation[1], nd.Rotation[2]}}
		}
		if len(nd.Scale) == 3 {
			b.S = mgl32.Vec3{nd.Scale[0], nd.Scale[1], nd.Scale[2]}
		}
		boneOf[n] = len(m.Bones)
		m.Bones = append(m.Bones, b)
		me := boneOf[n]
		for _, c := range nd.Children {
			if err := visit(c, me); err != nil {
				return err
			}
		}
		return nil
	}
	for _, r := range roots {
		if err := visit(r, -1); err != nil {
			return nil, err
		}
	}
	if len(m.Bones) > 255 {
		return nil, errors.New("gltf: more than 255 nodes")
	}

	// Inverse bind matrices from skins.
	for _, s := range d.Skins {
		if s.InverseBindMatrices == nil {
			continue
		}
		f, _, err := l.floats(*s.InverseBindMatrices)
		if err != nil {
			return nil, err
		}
		for j, n := range s.Joints {
			if n < len(boneOf) && boneOf[n] >= 0 && (j+1)*16 <= len(f) {
				copy(m.Bones[boneOf[n]].InvBind[:], f[j*16:])
			}
		}
	}

	// Textures.
	texOf := map[int]int{} // material -> model texture
	for mi, mat := range d.Materials {
		bt := mat.PBR.BaseColorTexture
		if bt == nil || bt.Index >= len(d.Textures) || d.Textures[bt.Index].Source == nil {
			continue
		}
		src := *d.Textures[bt.Index].Source
		if src >= len(d.Images) {
			continue
		}
		im := d.Images[src]
		var data []byte
		var err error
		if im.BufferView != nil {
			data, _, err = l.view(*im.BufferView)
		} else {
			data, err = l.uri(im.URI)
		}
		if err != nil {
			return nil, err
		}
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("gltf: image %d: %w", src, err)
		}
		texOf[mi] = len(m.Textures)
		m.Textures = append(m.Textures, img)
	}

	// Meshes: one model.Mesh per primitive, for every node using a mesh.
	for ni, nd := range d.Nodes {
		if nd.Mesh == nil || boneOf[ni] < 0 || *nd.Mesh >= len(d.Meshes) {
			continue
		}
		var joints []int
		if nd.Skin != nil && *nd.Skin < len(d.Skins) {
			joints = d.Skins[*nd.Skin].Joints
		}
		for _, p := range d.Meshes[*nd.Mesh].Primitives {
			if p.Mode != nil && *p.Mode != 4 {
				continue // triangles only
			}
			me, err := l.primitive(p.Attributes, p.Indices, joints, boneOf, boneOf[ni])
			if err != nil {
				return nil, err
			}
			me.Texture = -1
			if p.Material != nil {
				if t, ok := texOf[*p.Material]; ok {
					me.Texture = t
				}
			}
			m.Meshes = append(m.Meshes, *me)
		}
	}

	// Animations.
	for ai, an := range d.Animations {
		name := an.Name
		if name == "" {
			name = fmt.Sprintf("anim%d", ai)
		}
		c := &model.Clip{Name: name}
		for _, ch := range an.Channels {
			if ch.Target.Node == nil || ch.Sampler >= len(an.Samplers) || boneOf[*ch.Target.Node] < 0 {
				continue
			}
			var target model.Target
			switch ch.Target.Path {
			case "translation":
				target = model.Translation
			case "rotation":
				target = model.Rotation
			case "scale":
				target = model.Scale
			default:
				continue // weights (morph targets)
			}
			s := an.Samplers[ch.Sampler]
			times, _, err := l.floats(s.Input)
			if err != nil {
				return nil, err
			}
			vals, n, err := l.floats(s.Output)
			if err != nil {
				return nil, err
			}
			k := &model.Keyframes{Times: times, Quat: target == model.Rotation}
			stride := n
			off := 0
			switch s.Interpolation {
			case "STEP":
				k.Interp = model.Step
			case "CUBICSPLINE": // in-tangent, value, out-tangent: keep values
				stride, off = n*3, n
			}
			for i := range times {
				var v mgl32.Vec4
				o := i*stride + off
				if o+n > len(vals) {
					break
				}
				copy(v[:], vals[o:o+n])
				k.Values = append(k.Values, v)
			}
			k.Times = k.Times[:len(k.Values)]
			if len(times) > 0 {
				c.Length = max(c.Length, times[len(times)-1])
			}
			c.Channels = append(c.Channels, model.Channel{Bone: boneOf[*ch.Target.Node], Target: target, Track: k})
		}
		m.Clips[name] = c
	}
	return m, nil
}

func (l *loader) primitive(attr map[string]int, indices *int, joints []int, boneOf []int, node int) (*model.Mesh, error) {
	pi, ok := attr["POSITION"]
	if !ok {
		return nil, errors.New("gltf: primitive without POSITION")
	}
	pos, _, err := l.floats(pi)
	if err != nil {
		return nil, err
	}
	nv := len(pos) / 3
	get := func(name string) ([]float32, int) {
		i, ok := attr[name]
		if !ok {
			return nil, 0
		}
		f, n, e := l.floats(i)
		if e != nil {
			err = e
		}
		return f, n
	}
	nor, _ := get("NORMAL")
	uv, _ := get("TEXCOORD_0")
	jo, jn := get("JOINTS_0")
	we, wn := get("WEIGHTS_0")
	if err != nil {
		return nil, err
	}
	me := &model.Mesh{Verts: make([]model.Vertex, nv)}
	for i := range me.Verts {
		v := &me.Verts[i]
		v.Pos = mgl32.Vec3{pos[i*3], pos[i*3+1], pos[i*3+2]}
		if len(nor) >= (i+1)*3 {
			v.Normal = mgl32.Vec3{nor[i*3], nor[i*3+1], nor[i*3+2]}
		}
		if len(uv) >= (i+1)*2 {
			v.UV = mgl32.Vec2{uv[i*2], uv[i*2+1]}
		}
		if joints != nil && jn == 4 && wn == 4 && len(jo) >= (i+1)*4 && len(we) >= (i+1)*4 {
			for k := 0; k < 4; k++ {
				j := int(jo[i*4+k])
				b := node
				if j < len(joints) && joints[j] < len(boneOf) && boneOf[joints[j]] >= 0 {
					b = boneOf[joints[j]]
				}
				v.Joints[k] = uint8(b)
				v.Weights[k] = we[i*4+k]
			}
		} else {
			// Unskinned: rigidly attached to its node (InvBind is identity
			// for non-joint nodes, so the node's world transform applies).
			v.Joints[0], v.Weights[0] = uint8(node), 1
		}
	}
	if indices != nil {
		idx, _, err := l.floats(*indices)
		if err != nil {
			return nil, err
		}
		me.Indices = make([]uint32, len(idx))
		for i, f := range idx {
			me.Indices[i] = uint32(f)
		}
	} else {
		for i := 0; i < nv; i++ {
			me.Indices = append(me.Indices, uint32(i))
		}
	}
	return me, nil
}

func decompose(m mgl32.Mat4) (t mgl32.Vec3, r mgl32.Quat, s mgl32.Vec3) {
	t = m.Col(3).Vec3()
	s = mgl32.Vec3{m.Col(0).Vec3().Len(), m.Col(1).Vec3().Len(), m.Col(2).Vec3().Len()}
	var rm mgl32.Mat3
	for c := 0; c < 3; c++ {
		col := m.Col(c).Vec3().Mul(1 / s[c])
		rm.SetCol(c, col)
	}
	r = mgl32.Mat4ToQuat(rm.Mat4())
	return
}
