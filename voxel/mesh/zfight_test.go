package mesh_test

import (
	"encoding/json"
	"io/fs"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/memmaker/voxellib/resloc"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/mesh"
	"github.com/memmaker/voxellib/voxel/respack"
	"github.com/memmaker/voxellib/voxel/shapes"
)

// referenceFights are blocks whose reference geometry itself puts
// different texels of one texture in one plane; VoxelLib draws the
// reference's shapes, so they are reported here instead of checked.
var referenceFights = map[string]string{
	"redstone_wire": "the side strips cross the centre dot (cutout, mostly clear)",
	"tripwire":      "the strings cross at the centre (cutout, mostly clear)",
	"sea_pickle":    "the crossed tops of neighbouring pickles",
	"shulker_box":   "ShulkerModel's lid overlaps the base by 4 px on every side",
}

// referenceFight is the reason name's fights are the reference's, or "".
func referenceFight(name string) string {
	for block, why := range referenceFights {
		if name == block || strings.HasSuffix(name, "_"+block) {
			return why
		}
	}
	return ""
}

// maxCombos caps the multipart property combinations tried per block.
const maxCombos = 4096

// uvTolerance is how far (in texture units) two layers' UVs at the same
// point may differ: below one texel of a 16-texel tile.
const uvTolerance = 1.0 / 32

// TestNoZFight: wherever two faces of one block state lie in the same
// plane, face the same way, overlap and use the same texture, they show
// the same texel; else the depth test picks between them pixel by pixel
// and the surface flickers (a wall's turned arms without uvlock did).
// Every state of every VoxelLib shape is meshed alone. Different
// textures in one plane are the models' deliberate overlays (grass side,
// redstone) and are not checked, nor are referenceFights.
func TestNoZFight(t *testing.T) {
	p := respack.FromFS(shapes.FS)
	reg := voxel.NewRegistry()
	reg.Resolver = p.Resolver()
	files, err := fs.Glob(shapes.FS, "assets/"+resloc.Default+"/blockstates/*.json")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, f := range files {
		block := strings.TrimSuffix(path.Base(f), ".json")
		name := resloc.Default + ":" + block
		if referenceFight(block) != "" {
			continue
		}
		data, err := fs.ReadFile(shapes.FS, f)
		if err != nil {
			t.Fatal(err)
		}
		combos, err := stateCombos(data)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, props := range combos {
			if msg := zfight(reg, reg.State(name, props)); msg != "" {
				t.Errorf("%s%v: %s", name, props, msg)
			}
			checked++
		}
	}
	t.Logf("%d states checked", checked)
}

// stateCombos is the property sets to try for a blockstate file: each
// variant key, or for multipart every combination of the values its
// "when" clauses name (each property also absent).
func stateCombos(data []byte) ([]map[string]string, error) {
	var bs struct {
		Variants  map[string]json.RawMessage
		Multipart []struct{ When json.RawMessage }
	}
	if err := json.Unmarshal(data, &bs); err != nil {
		return nil, err
	}
	var out []map[string]string
	for k := range bs.Variants {
		props := map[string]string{}
		for _, kv := range strings.Split(k, ",") {
			if k, v, ok := strings.Cut(kv, "="); ok {
				props[k] = v
			}
		}
		out = append(out, props)
	}
	if len(bs.Multipart) == 0 {
		return out, nil
	}
	values := map[string]map[string]bool{}
	for _, part := range bs.Multipart {
		if err := whenValues(part.When, values); err != nil {
			return nil, err
		}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out = []map[string]string{{}}
	for _, k := range keys {
		var next []map[string]string
		for _, base := range out {
			next = append(next, base) // k absent
			for v := range values[k] {
				m := map[string]string{k: v}
				for bk, bv := range base {
					m[bk] = bv
				}
				next = append(next, m)
			}
		}
		if len(next) > maxCombos {
			break
		}
		out = next
	}
	return out, nil
}

// whenValues collects the property values a multipart "when" names.
func whenValues(raw json.RawMessage, values map[string]map[string]bool) error {
	if len(raw) == 0 {
		return nil
	}
	var c map[string]json.RawMessage
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	for k, v := range c {
		if k == "OR" || k == "AND" {
			var subs []json.RawMessage
			if err := json.Unmarshal(v, &subs); err != nil {
				return err
			}
			for _, s := range subs {
				if err := whenValues(s, values); err != nil {
					return err
				}
			}
			continue
		}
		var want any
		if err := json.Unmarshal(v, &want); err != nil {
			return err
		}
		if values[k] == nil {
			values[k] = map[string]bool{}
		}
		switch w := want.(type) {
		case bool:
			values[k][map[bool]string{true: "true", false: "false"}[w]] = true
		case string:
			for _, alt := range strings.Split(w, "|") {
				values[k][alt] = true
			}
		}
	}
	return nil
}

// face is one meshed axis-aligned quad: the plane it lies in, its bounds
// along the two other axes and its corners' UVs.
type face struct {
	normal int
	tex    uint16
	plane  float32
	lo, hi [2]float32
	corner [4]mesh.Vertex
}

// zfight meshes state alone and describes the first pair of faces that
// fight, or "".
func zfight(reg *voxel.Registry, state voxel.StateID) string {
	w := voxel.NewWorld(reg, 0, 16)
	w.AddChunk(w.NewChunk(voxel.ChunkPos{}))
	w.SetBlock(1, 1, 1, state)
	m := mesh.NewMesher()
	m.Snap.Capture(w, voxel.SectionPos{})
	res := m.Build(reg.Table(), nil)
	boxes := solidBoxes(reg.Info(state))
	var fs []face
	for _, data := range [][]uint32{res.Data, res.Trans} {
		for i := 0; i+4*mesh.VertexWords <= len(data); i += 4 * mesh.VertexWords {
			if f, ok := decodeFace(data[i:]); ok {
				fs = append(fs, f)
			}
		}
	}
	for i := range fs {
		for j := i + 1; j < len(fs); j++ {
			a, b := &fs[i], &fs[j]
			if a.normal != b.normal || a.tex != b.tex || a.plane != b.plane {
				continue
			}
			var pt [2]float32
			overlap := true
			for k := range 2 {
				lo, hi := max(a.lo[k], b.lo[k]), min(a.hi[k], b.hi[k])
				overlap = overlap && hi > lo
				pt[k] = (lo + hi) / 2
			}
			if !overlap || hidden(boxes, a, pt) {
				continue
			}
			ua, ub := a.uvAt(pt), b.uvAt(pt)
			if abs(ua[0]-ub[0]) > uvTolerance || abs(ua[1]-ub[1]) > uvTolerance {
				return "coplanar faces " + voxel.Face(a.normal).String() + " show different texels: uv " +
					fmtUV(ua) + " vs " + fmtUV(ub)
			}
		}
	}
	return ""
}

// hiddenDepth is how far (blocks) in front of a face the point that must
// be in the open lies: well under a texel.
const hiddenDepth = 1.0 / 256

// solidBoxes are the state's boxes as axis-aligned bounds in section
// blocks (the state sits at 1,1,1); boxes turned off the axes are left out.
func solidBoxes(info *voxel.StateInfo) [][2][3]float32 {
	var out [][2][3]float32
	for _, b := range info.Boxes {
		p, q := voxel.RotatePoint(b.Min, b.Rot), voxel.RotatePoint(b.Max, b.Rot)
		if voxel.RotateFace(voxel.FaceUp, b.Rot) >= voxel.FaceCount || voxel.RotateFace(voxel.FaceNorth, b.Rot) >= voxel.FaceCount {
			continue
		}
		var lo, hi [3]float32
		for a := range 3 {
			lo[a], hi[a] = 1+min(p[a], q[a])/16, 1+max(p[a], q[a])/16
		}
		out = append(out, [2][3]float32{lo, hi})
	}
	return out
}

// hidden reports whether in-plane point pt of f lies inside another box of
// the block just in front of the face: nobody sees what fights there.
func hidden(boxes [][2][3]float32, f *face, pt [2]float32) bool {
	var p [3]float32
	ax, na := inPlane[f.normal], normalAxis[f.normal]
	p[ax[0]], p[ax[1]] = pt[0], pt[1]
	p[na] = f.plane - hiddenDepth
	if voxel.Face(f.normal).Normal().X+voxel.Face(f.normal).Normal().Y+voxel.Face(f.normal).Normal().Z > 0 {
		p[na] = f.plane + hiddenDepth
	}
	for _, b := range boxes {
		in := true
		for a := range 3 {
			in = in && p[a] > b[0][a] && p[a] < b[1][a]
		}
		if in {
			return true
		}
	}
	return false
}

// inPlane are the two axes a face of each normal spans.
var inPlane = [6][2]int{{0, 2}, {0, 2}, {0, 1}, {0, 1}, {1, 2}, {1, 2}}

// normalAxis is the axis each face normal points along.
var normalAxis = [6]int{1, 1, 2, 2, 0, 0}

// decodeFace reads one quad; ok=false for one not along an axis.
func decodeFace(d []uint32) (face, bool) {
	var f face
	for k := range 4 {
		f.corner[k] = mesh.DecodeVertex(d[k*mesh.VertexWords:])
	}
	f.normal, f.tex = f.corner[0].Normal, f.corner[0].Tex
	if f.normal >= int(voxel.FaceCount) {
		return f, false
	}
	f.plane = f.corner[0].Pos[normalAxis[f.normal]]
	ax := inPlane[f.normal]
	for k := range 2 {
		f.lo[k], f.hi[k] = f.corner[0].Pos[ax[k]], f.corner[0].Pos[ax[k]]
		for _, c := range f.corner {
			if c.Pos[normalAxis[f.normal]] != f.plane {
				return f, false
			}
			f.lo[k], f.hi[k] = min(f.lo[k], c.Pos[ax[k]]), max(f.hi[k], c.Pos[ax[k]])
		}
	}
	return f, true
}

// uvAt is the face's UV at in-plane point pt, interpolated as the GPU does
// over the parallelogram of corners 0, 1 and 3.
func (f *face) uvAt(pt [2]float32) [2]float32 {
	ax := inPlane[f.normal]
	c0, c1, c3 := f.corner[0], f.corner[1], f.corner[3]
	e1 := [2]float32{c1.Pos[ax[0]] - c0.Pos[ax[0]], c1.Pos[ax[1]] - c0.Pos[ax[1]]}
	e3 := [2]float32{c3.Pos[ax[0]] - c0.Pos[ax[0]], c3.Pos[ax[1]] - c0.Pos[ax[1]]}
	d := [2]float32{pt[0] - c0.Pos[ax[0]], pt[1] - c0.Pos[ax[1]]}
	det := e1[0]*e3[1] - e1[1]*e3[0]
	s := (d[0]*e3[1] - d[1]*e3[0]) / det
	r := (e1[0]*d[1] - e1[1]*d[0]) / det
	var uv [2]float32
	for k := range 2 {
		uv[k] = c0.UV[k] + s*(c1.UV[k]-c0.UV[k]) + r*(c3.UV[k]-c0.UV[k])
	}
	return uv
}

func abs(x float32) float32 { return max(x, -x) }

func fmtUV(uv [2]float32) string {
	b, _ := json.Marshal(uv)
	return string(b)
}
