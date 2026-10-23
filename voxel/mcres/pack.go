// Package mcres loads Minecraft Java resource packs: textures (with .mcmeta
// animation strips), blockstates and block models. Packs may be directories,
// .zip files or the game's client .jar; several packs stack like in-game, the
// first one that has a file wins.
package mcres

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/memmaker/voxellib/voxel"
)

// Pack is a stack of resource sources, highest priority first.
type Pack struct {
	sources []fs.FS
	closers []io.Closer

	mu     sync.Mutex
	models map[string]*model
}

// Open opens each path (directory, .zip or .jar) and stacks them.
func Open(paths ...string) (*Pack, error) {
	p := &Pack{models: map[string]*model{}}
	for _, ph := range paths {
		st, err := os.Stat(ph)
		if err != nil {
			p.Close()
			return nil, err
		}
		if st.IsDir() {
			p.sources = append(p.sources, os.DirFS(ph))
			continue
		}
		zr, err := zip.OpenReader(ph)
		if err != nil {
			p.Close()
			return nil, fmt.Errorf("mcres: %s: %w", ph, err)
		}
		p.sources = append(p.sources, zr)
		p.closers = append(p.closers, zr)
	}
	return p, nil
}

// FromFS builds a pack from already opened file systems (e.g. embed.FS).
func FromFS(fss ...fs.FS) *Pack { return &Pack{sources: fss, models: map[string]*model{}} }

func (p *Pack) Close() {
	for _, c := range p.closers {
		c.Close()
	}
	p.closers = nil
}

func (p *Pack) open(name string) (fs.File, error) {
	for _, s := range p.sources {
		if f, err := s.Open(name); err == nil {
			return f, nil
		}
	}
	return nil, fs.ErrNotExist
}

func (p *Pack) readFile(name string) ([]byte, error) {
	f, err := p.open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// splitID turns "minecraft:block/stone" into ("minecraft", "block/stone").
func splitID(id string) (string, string) {
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[:i], id[i+1:]
	}
	return "minecraft", id
}

// Texture loads a texture by resource name ("block/stone"). Animated strips
// are cut to their first frame; see TextureFrames for all of them.
func (p *Pack) Texture(name string) (image.Image, error) {
	frames, _, err := p.TextureFrames(name)
	if err != nil {
		return nil, err
	}
	return frames[0], nil
}

// TextureFrames loads a texture and, for animated ones (a vertical strip of
// square frames with a .png.mcmeta "animation" section), returns the frames
// in play order and the ticks (1/20 s) each one is shown. Still textures
// return one frame and 0.
//
// A name ending in "#W:x,y,w,h" is the w×h region at x,y of a sheet laid out
// W pixels wide (entity textures; scaled for higher resolution packs).
func (p *Pack) TextureFrames(name string) ([]image.Image, int, error) {
	if base, region, ok := strings.Cut(name, "#"); ok {
		frames, ticks, err := p.TextureFrames(base)
		if err != nil {
			return nil, 0, err
		}
		var sw, x, y, w, h int
		if _, err := fmt.Sscanf(region, "%d:%d,%d,%d,%d", &sw, &x, &y, &w, &h); err != nil || sw <= 0 {
			return nil, 0, fmt.Errorf("mcres: bad region %q", name)
		}
		// the region of every frame: an animated icon stays animated
		out := make([]image.Image, len(frames))
		for i, img := range frames {
			b := img.Bounds()
			k := float64(b.Dx()) / float64(sw)
			r := image.Rect(int(float64(x)*k), int(float64(y)*k), int(float64(x+w)*k), int(float64(y+h)*k)).Add(b.Min)
			o := image.NewNRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
			draw.Draw(o, o.Bounds(), img, r.Min, draw.Src)
			out[i] = o
		}
		return out, ticks, nil
	}
	ns, rel := splitID(name)
	file := path.Join("assets", ns, "textures", rel+".png")
	f, err := p.open(file)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, 0, fmt.Errorf("mcres: %s: %w", name, err)
	}
	meta, _ := p.readFile(file + ".mcmeta")
	frames, ticks := Frames(img, meta)
	return frames, ticks, nil
}

// Frames splits a texture into its animation frames in play order and the
// ticks (1/20 s) each is shown, by its .png.mcmeta (nil: none), as
// TextureFrames does. Still textures return one frame and 0.
func Frames(img image.Image, mcmeta []byte) ([]image.Image, int) {
	b := img.Bounds()
	if b.Dy() <= b.Dx() || b.Dy()%b.Dx() != 0 {
		return []image.Image{img}, 0
	}
	n := b.Dy() / b.Dx()
	strip := make([]image.Image, n)
	for i := range strip {
		fr := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dx()))
		draw.Draw(fr, fr.Bounds(), img, image.Pt(b.Min.X, b.Min.Y+i*b.Dx()), draw.Src)
		strip[i] = fr
	}
	var meta struct {
		Animation *struct {
			Frametime int               `json:"frametime"`
			Frames    []json.RawMessage `json:"frames"`
		} `json:"animation"`
	}
	if mcmeta != nil {
		json.Unmarshal(mcmeta, &meta)
	}
	if meta.Animation == nil {
		return strip[:1], 0 // a tall texture without animation metadata
	}
	ft := max(meta.Animation.Frametime, 1)
	if len(meta.Animation.Frames) == 0 {
		return strip, ft
	}
	// Explicit order: entries are an index or {"index":i,"time":t}; per-frame
	// times are approximated by repeating the frame.
	var out []image.Image
	for _, raw := range meta.Animation.Frames {
		var idx, t int
		var obj struct{ Index, Time int }
		if json.Unmarshal(raw, &idx) == nil {
			t = ft
		} else if json.Unmarshal(raw, &obj) == nil {
			idx, t = obj.Index, obj.Time
			if t == 0 {
				t = ft
			}
		}
		if idx < 0 || idx >= n {
			continue
		}
		for r := 0; r < max(1, t/ft); r++ {
			out = append(out, strip[idx])
		}
	}
	if len(out) == 0 {
		return strip, ft
	}
	return out, ft
}

// MissingTexture returns the classic magenta/black checkerboard.
func MissingTexture(size int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := color.NRGBA{0, 0, 0, 255}
			if (x < size/2) != (y < size/2) {
				c = color.NRGBA{248, 0, 248, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// IsGreyscale reports whether an image has (almost) no colour, which is how
// packs mark biome-tinted textures. Used to decide whether to tint. Vanilla
// style packs often keep a faint warm cast (Pixel Perfection's leaves are
// 175,163,140), so "almost" is a saturation below 30%; textures a pack has
// painted green are far above that.
func IsGreyscale(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a < 0x1000 {
				continue
			}
			mx, mn := max(r, g, bl), min(r, g, bl)
			if mx-mn > 0x1800 && (mx-mn)*10 > mx*3 {
				return false
			}
		}
	}
	return true
}

// ---- blockstates & models ----

type blockstate struct {
	Variants  map[string]json.RawMessage `json:"variants"`
	Multipart json.RawMessage            `json:"multipart"`
}

type variant struct {
	Model string `json:"model"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
}

type modelFace struct {
	Texture   string      `json:"texture"`
	Cullface  string      `json:"cullface"`
	UV        *[4]float32 `json:"uv"`
	Rotation  int         `json:"rotation"`  // texture turn, clockwise degrees
	TintIndex *int        `json:"tintindex"` // absent or < 0: not tinted
}

type element struct {
	From     [3]float32           `json:"from"`
	To       [3]float32           `json:"to"`
	Rotation *elemRot             `json:"rotation"`
	Faces    map[string]modelFace `json:"faces"`
	// PlantTurn: voxel.Box.PlantTurn (an extension, ignored by Minecraft).
	PlantTurn bool  `json:"plant_turn"`
	Shade     *bool `json:"shade"`
}

type elemRot struct {
	Origin  [3]float32
	Axis    string
	Angle   float32
	Rescale bool
	// Shear, Matrix: voxel.Rot.Shear and Mat (extensions, ignored by
	// Minecraft).
	Shear  [2]float32
	Matrix [9]float32
}

type model struct {
	Parent   string            `json:"parent"`
	Textures map[string]string `json:"textures"`
	Elements []element         `json:"elements"`
	AO       *bool             `json:"ambientocclusion"`
	// resolved
	chain []string
}

func (p *Pack) model(id string) (*model, error) {
	ns, rel := splitID(id)
	key := ns + ":" + rel
	p.mu.Lock()
	if m, ok := p.models[key]; ok {
		p.mu.Unlock()
		if m == nil {
			return nil, fs.ErrNotExist
		}
		return m, nil
	}
	p.mu.Unlock()
	data, err := p.readFile(path.Join("assets", ns, "models", rel+".json"))
	var m *model
	if err == nil {
		m = &model{}
		if err = json.Unmarshal(data, m); err != nil {
			m = nil
		}
	}
	p.mu.Lock()
	p.models[key] = m
	p.mu.Unlock()
	if m == nil {
		if err == nil {
			err = fs.ErrNotExist
		}
		return nil, err
	}
	return m, nil
}

// resolve walks the parent chain merging textures (child wins) and taking the
// nearest elements.
func (p *Pack) resolve(id string) (tex map[string]string, elems []element, chain []string, err error) {
	tex = map[string]string{}
	for depth := 0; id != "" && depth < 16; depth++ {
		_, rel := splitID(id)
		chain = append(chain, rel)
		if strings.HasPrefix(rel, "builtin/") {
			break
		}
		m, e := p.model(id)
		if e != nil {
			return nil, nil, chain, e
		}
		for k, v := range m.Textures {
			if _, ok := tex[k]; !ok {
				tex[k] = v
			}
		}
		if elems == nil && len(m.Elements) > 0 {
			elems = m.Elements
		}
		id = m.Parent
	}
	return
}

// flat reports a model's (or its nearest parent's) "ambientocclusion": false.
func (p *Pack) flat(id string) bool {
	for depth := 0; id != "" && depth < 16; depth++ {
		m, err := p.model(id)
		if err != nil {
			return false
		}
		if m.AO != nil {
			return !*m.AO
		}
		id = m.Parent
	}
	return false
}

func lookupTex(tex map[string]string, ref string) string {
	for i := 0; i < 16 && strings.HasPrefix(ref, "#"); i++ {
		ref = tex[ref[1:]]
	}
	if ref == "" || strings.HasPrefix(ref, "#") {
		return ""
	}
	return strings.TrimPrefix(ref, "minecraft:")
}

// pickVariant chooses the variant whose "k=v,..." key matches props.
func pickVariant(bs *blockstate, props map[string]string) (*variant, error) {
	for key, raw := range bs.Variants {
		ok := true
		if key != "" && key != "normal" {
			for _, kv := range strings.Split(key, ",") {
				k, v, _ := strings.Cut(kv, "=")
				if props[k] != v {
					ok = false
					break
				}
			}
		}
		if !ok {
			continue
		}
		var v variant
		if raw[0] == '[' {
			var vs []variant
			if err := json.Unmarshal(raw, &vs); err != nil || len(vs) == 0 {
				return nil, errors.New("bad variant list")
			}
			v = vs[0]
		} else if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return &v, nil
	}
	return nil, errors.New("no variant")
}

// rotateFaces applies a blockstate x/y rotation to a per-face texture table.
func rotateFaces(t [6]string, rx, ry int) [6]string {
	for ; rx > 0; rx -= 90 { // rotate around X: up->north->down->south
		t[voxel.FaceUp], t[voxel.FaceNorth], t[voxel.FaceDown], t[voxel.FaceSouth] =
			t[voxel.FaceSouth], t[voxel.FaceUp], t[voxel.FaceNorth], t[voxel.FaceDown]
	}
	for ; ry > 0; ry -= 90 { // rotate around Y: north->east->south->west
		t[voxel.FaceEast], t[voxel.FaceSouth], t[voxel.FaceWest], t[voxel.FaceNorth] =
			t[voxel.FaceNorth], t[voxel.FaceEast], t[voxel.FaceSouth], t[voxel.FaceWest]
	}
	return t
}

// Resolver returns a voxel.ModelResolver that reads blockstates and models
// from the pack. Full cubes and cross plants are understood; anything else
// (stairs, multipart fences, ...) falls back to the registry defaults.
func (p *Pack) Resolver() voxel.ModelResolver {
	return func(name string, props map[string]string) (voxel.Shape, [6]string, bool) {
		shape, tex, _, ok := p.resolveState(name, props)
		return shape, tex, ok
	}
}

// FaceResolver returns the voxel.FaceResolver of the cubes Resolver finds:
// each face's cullface and whether it has a tintindex, turned with the
// blockstate. Set both on a registry.
func (p *Pack) FaceResolver() voxel.FaceResolver {
	return func(name string, props map[string]string) (voxel.FaceRules, bool) {
		shape, _, rules, ok := p.resolveState(name, props)
		return rules, ok && shape == voxel.ShapeCube
	}
}

// ponytail: resolveState runs twice per state (Resolver, FaceResolver);
// models are cached, only the blockstate file is read again, once per
// state at registration.
func (p *Pack) resolveState(name string, props map[string]string) (voxel.Shape, [6]string, voxel.FaceRules, bool) {
	var none [6]string
	var rules voxel.FaceRules
	ns, rel := splitID(name)
	data, err := p.readFile(path.Join("assets", ns, "blockstates", rel+".json"))
	if err != nil {
		return 0, none, rules, false
	}
	var bs blockstate
	if json.Unmarshal(data, &bs) != nil || len(bs.Variants) == 0 {
		return 0, none, rules, false
	}
	v, err := pickVariant(&bs, props)
	if err != nil {
		return 0, none, rules, false
	}
	tex, elems, chain, err := p.resolve(v.Model)
	if err != nil {
		return 0, none, rules, false
	}
	for _, c := range chain {
		if c == "block/cross" || c == "block/tinted_cross" {
			return voxel.ShapeCross, all(lookupTex(tex, "#cross")), rules, true
		}
	}
	// First full-cube element (vanilla grass_block adds a second, tinted
	// overlay element on top; the base cube is enough here).
	for _, e := range elems {
		if e.From == [3]float32{0, 0, 0} && e.To == [3]float32{16, 16, 16} {
			var t [6]string
			for f := range rules.Cull {
				rules.Cull[f] = voxel.FaceCount
			}
			for fn, face := range e.Faces {
				f, ok := voxel.FaceByName(fn)
				if !ok {
					continue
				}
				t[f] = lookupTex(tex, face.Texture)
				if c, ok := voxel.FaceByName(face.Cullface); ok {
					rules.Cull[f] = c
				}
				if face.TintIndex != nil && *face.TintIndex >= 0 {
					rules.Tinted |= 1 << f
				}
			}
			return voxel.ShapeCube, rotateFaces(t, v.X, v.Y), rotateRules(rules, v.X, v.Y), true
		}
	}
	return 0, none, rules, false
}

// rotateRules turns face rules like rotateFaces turns textures; cullface
// directions turn along.
func rotateRules(r voxel.FaceRules, rx, ry int) voxel.FaceRules {
	var ids [6]string
	for f := range ids {
		ids[f] = string(rune('0' + f))
	}
	perm := rotateFaces(ids, rx, ry) // perm[new face] = old face
	var to [6]voxel.Face             // old face → new face
	for nf, of := range perm {
		to[of[0]-'0'] = voxel.Face(nf)
	}
	var out voxel.FaceRules
	for nf, of := range perm {
		o := of[0] - '0'
		out.Cull[nf] = r.Cull[o]
		if c := r.Cull[o]; c < voxel.FaceCount {
			out.Cull[nf] = to[c]
		}
		if r.Tinted&(1<<o) != 0 {
			out.Tinted |= 1 << nf
		}
	}
	return out
}

func all(t string) [6]string { return [6]string{t, t, t, t, t, t} }

// ModelBoxes returns the elements of block model id ("block/stairs",
// "lotr:block/x") as voxel boxes: per-face textures resolved through the
// model's texture variables, with vars (variable → texture) taking
// precedence, plus UVs (with their "rotation"), element rotations (with
// "rescale"), cullfaces and which faces take the block's tint
// ("tintindex"). Faces a model leaves out are not drawn.
func (p *Pack) ModelBoxes(id string, vars map[string]string) ([]voxel.Box, error) {
	tex, elems, _, err := p.resolve(id)
	if err != nil {
		return nil, err
	}
	for k, v := range vars {
		tex[k] = v
	}
	out := make([]voxel.Box, 0, len(elems))
	flat := p.flat(id)
	for _, e := range elems {
		b := voxel.Box{Min: e.From, Max: e.To, Skip: 0x3f, Flat: flat}
		var uv [6][4]float32
		hasUV := false
		cull := [6]voxel.Face{voxel.FaceCount, voxel.FaceCount, voxel.FaceCount, voxel.FaceCount, voxel.FaceCount, voxel.FaceCount}
		b.Cull = &cull
		for fn, face := range e.Faces {
			f, ok := voxel.FaceByName(fn)
			if !ok {
				continue
			}
			b.Skip &^= 1 << f
			b.Tex[f] = lookupTex(tex, face.Texture)
			if c, ok := voxel.FaceByName(face.Cullface); ok {
				cull[f] = c
			}
			if face.TintIndex == nil || *face.TintIndex < 0 {
				b.NoTint |= 1 << f
			}
			if q := uint8(face.Rotation/90) & 3; q != 0 {
				b.UVRot[f], hasUV = q, true
			}
			if face.UV != nil {
				uv[f], hasUV = *face.UV, true
			} else {
				uv[f] = defaultUV(f, e.From, e.To)
			}
		}
		if hasUV {
			b.UV = &uv
		}
		if r := e.Rotation; r != nil && (r.Angle != 0 || r.Shear != [2]float32{} || r.Matrix != [9]float32{}) {
			if axis := strings.Index("xyz", r.Axis); axis >= 0 {
				b.Rot = []voxel.Rot{{Axis: axis, Deg: r.Angle, Origin: r.Origin, Rescale: r.Rescale, Shear: r.Shear, Mat: r.Matrix}}
			}
		}
		b.PlantTurn = e.PlantTurn
		b.NoShade = e.Shade != nil && !*e.Shade
		out = append(out, b)
	}
	return out, nil
}

// defaultUV is the UV Minecraft derives from an element's position when a
// face gives none.
func defaultUV(f voxel.Face, a, b [3]float32) [4]float32 {
	switch f {
	case voxel.FaceDown:
		return [4]float32{a[0], 16 - b[2], b[0], 16 - a[2]}
	case voxel.FaceUp:
		return [4]float32{a[0], a[2], b[0], b[2]}
	case voxel.FaceNorth:
		return [4]float32{16 - b[0], 16 - b[1], 16 - a[0], 16 - a[1]}
	case voxel.FaceSouth:
		return [4]float32{a[0], 16 - b[1], b[0], 16 - a[1]}
	case voxel.FaceWest:
		return [4]float32{a[2], 16 - b[1], b[2], 16 - a[1]}
	}
	return [4]float32{16 - b[2], 16 - b[1], 16 - a[2], 16 - a[1]}
}
