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
// are cut to their first frame.
func (p *Pack) Texture(name string) (image.Image, error) {
	ns, rel := splitID(name)
	f, err := p.open(path.Join("assets", ns, "textures", rel+".png"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("mcres: %s: %w", name, err)
	}
	b := img.Bounds()
	if b.Dy() > b.Dx() && b.Dy()%b.Dx() == 0 {
		frame := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dx()))
		draw.Draw(frame, frame.Bounds(), img, b.Min, draw.Src)
		return frame, nil
	}
	return img, nil
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
// vanilla marks biome-tinted textures. Used to decide whether to tint.
func IsGreyscale(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a < 0x1000 {
				continue
			}
			mx, mn := max(r, g, bl), min(r, g, bl)
			if mx-mn > 0x1800 {
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
	Texture  string `json:"texture"`
	Cullface string `json:"cullface"`
}

type element struct {
	From     [3]float64               `json:"from"`
	To       [3]float64               `json:"to"`
	Rotation *struct{ Angle float64 } `json:"rotation"`
	Faces    map[string]modelFace     `json:"faces"`
}

type model struct {
	Parent   string            `json:"parent"`
	Textures map[string]string `json:"textures"`
	Elements []element         `json:"elements"`
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

func lookupTex(tex map[string]string, ref string) string {
	for i := 0; i < 16 && strings.HasPrefix(ref, "#"); i++ {
		ref = tex[ref[1:]]
	}
	if ref == "" || strings.HasPrefix(ref, "#") {
		return ""
	}
	_, rel := splitID(ref)
	return rel
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
		var none [6]string
		ns, rel := splitID(name)
		data, err := p.readFile(path.Join("assets", ns, "blockstates", rel+".json"))
		if err != nil {
			return 0, none, false
		}
		var bs blockstate
		if json.Unmarshal(data, &bs) != nil || len(bs.Variants) == 0 {
			return 0, none, false
		}
		v, err := pickVariant(&bs, props)
		if err != nil {
			return 0, none, false
		}
		tex, elems, chain, err := p.resolve(v.Model)
		if err != nil {
			return 0, none, false
		}
		for _, c := range chain {
			if c == "block/cross" || c == "block/tinted_cross" {
				return voxel.ShapeCross, all(lookupTex(tex, "#cross")), true
			}
		}
		// First full-cube element (vanilla grass_block adds a second, tinted
		// overlay element on top; the base cube is enough here).
		for _, e := range elems {
			if e.From == [3]float64{0, 0, 0} && e.To == [3]float64{16, 16, 16} {
				var t [6]string
				for fn, face := range e.Faces {
					if f, ok := voxel.FaceByName(fn); ok {
						t[f] = lookupTex(tex, face.Texture)
					}
				}
				return voxel.ShapeCube, rotateFaces(t, v.X, v.Y), true
			}
		}
		return 0, none, false
	}
}

func all(t string) [6]string { return [6]string{t, t, t, t, t, t} }
