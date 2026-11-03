// Package vanillapack is a resource pack of VoxelLib's own blockstates and
// block models for the vanilla Java Edition block names, so content made
// for the reference game (imported maps, block names in code) can be drawn
// without the game's client jar. The geometry follows the reference's
// models; the blockstates and models are generated here, by block family
// (cubes, columns, stairs, slabs, fences, walls, panes, doors, plants,
// torches, rails, ...), and the textures come from a pack laid out like the
// reference's, by default assets/pixelperfection (Pixel Perfection, CC
// BY-SA 4.0).
//
//	pack := vanillapack.New(os.DirFS("assets/pixelperfection"))
//	data, _ := fs.ReadFile(pack, "assets/minecraft/blockstates/oak_stairs.json")
//
// A block is generated the first time its blockstate is read; its models
// are readable from then on. Blockstates and models the texture pack has
// itself (Pixel Perfection's random and reworked blocks) take precedence. Names the families don't know become a cube of
// textures/block/<name>.png when the texture pack has one.
package vanillapack

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/memmaker/voxellib/voxel/blockmodel"
)

// DefaultTextures is the texture pack New is usually given, relative to
// the VoxelLib checkout.
const DefaultTextures = "assets/pixelperfection"

const (
	statesDir = "assets/minecraft/blockstates/"
	modelsDir = "assets/minecraft/models/"
)

// Pack is the generated pack over a texture pack.
type Pack struct {
	tex       fs.FS
	templates map[string]*blockmodel.Model
	mu        sync.Mutex
	files     map[string][]byte
	blocks    map[string]*block // generated, nil for unknown names
}

// New makes the pack: generated blockstates and models, and every file of
// tex (its textures) as is.
func New(tex fs.FS) *Pack {
	p := &Pack{tex: tex, templates: templateModels(), files: map[string][]byte{}, blocks: map[string]*block{}}
	for id, m := range p.templates {
		p.put(modelsDir+"block/"+id+".json", m)
	}
	return p
}

func (p *Pack) put(name string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	p.files[name] = b
}

// Open opens a file of the texture pack, else a generated one. The
// texture pack's own blockstates and models come first, as a resource pack
// over the reference's.
func (p *Pack) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	p.mu.Lock()
	p.generateFor(name)
	b, ok := p.files[name]
	p.mu.Unlock()
	if fixed, bad := p.unfinished(name); bad && !hidden[name] {
		if !ok || strings.HasPrefix(name, modelsDir+"block/vault") {
			b, ok = fixed, true
		}
	} else if !hidden[name] {
		f, err := p.tex.Open(name)
		if err == nil {
			return f, nil
		}
		if !ok {
			if alias, made, is := texFile(name); is {
				if made == nil {
					return p.tex.Open(alias)
				}
				b, ok = made, true
			}
		}
		if !ok {
			return nil, err
		}
	}
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &memFile{name: path.Base(name), Reader: bytes.NewReader(b), size: int64(len(b))}, nil
}

// unfinished reports whether the texture pack's model of that name has
// faces it never gave a texture (Blockbench's "#missing", in Pixel
// Perfection's lanterns and stems), and the model without them. The
// generated model is drawn instead where there is one; Pixel Perfection's
// vault, a whole redesign drawn by its own blockstate, stays.
func (p *Pack) unfinished(name string) ([]byte, bool) {
	if !strings.HasPrefix(name, modelsDir) {
		return nil, false
	}
	b, err := fs.ReadFile(p.tex, name)
	if err != nil || !bytes.Contains(b, []byte(`"#missing"`)) {
		return nil, false
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil, false
	}
	els, _ := m["elements"].([]any)
	for _, e := range els {
		faces, _ := e.(map[string]any)["faces"].(map[string]any)
		for k, f := range faces {
			if f.(map[string]any)["texture"] == "#missing" {
				delete(faces, k)
			}
		}
	}
	out, err := json.Marshal(m)
	return out, err == nil
}

// generateFor generates the block of a blockstate, so the models a
// texture pack's blockstate names are there too, and a model still missing
// from the block named like it (the double of a slab is its full block's
// model).
func (p *Pack) generateFor(name string) {
	if bn, ok := baseName(name, statesDir); ok {
		if _, done := p.files[name]; !done {
			if b := p.block(bn); b != nil {
				p.put(name, b.state)
				for id, m := range b.models {
					p.put(modelsDir+"block/"+id+".json", m)
				}
			}
		}
		return
	}
	if bn, ok := baseName(name, modelsDir+"block/"); ok {
		if _, done := p.files[name]; !done {
			if b := p.block(bn); b != nil {
				for id, m := range b.models {
					if _, ok := p.files[modelsDir+"block/"+id+".json"]; !ok {
						p.put(modelsDir+"block/"+id+".json", m)
					}
				}
			}
		}
	}
}

// baseName is the name of the .json file name in dir.
func baseName(name, dir string) (string, bool) {
	n, ok := strings.CutPrefix(name, dir)
	bn, ok2 := strings.CutSuffix(n, ".json")
	return bn, ok && ok2 && path.Base(bn) == bn
}

// hasTex reports whether the texture pack has textures/block/<name>.png.
func (p *Pack) hasTex(name string) bool {
	_, err := fs.Stat(p.tex, "assets/minecraft/textures/block/"+name+".png")
	return err == nil
}

type memFile struct {
	name string
	*bytes.Reader
	size int64
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f, nil }
func (f *memFile) Close() error               { return nil }
func (f *memFile) Name() string               { return f.name }
func (f *memFile) Size() int64                { return f.size }
func (f *memFile) Mode() fs.FileMode          { return 0o444 }
func (f *memFile) ModTime() time.Time         { return time.Time{} }
func (f *memFile) IsDir() bool                { return false }
func (f *memFile) Sys() any                   { return nil }

var _ io.ReaderAt = (*memFile)(nil)

// helpers of templates.go

var box = blockmodel.Box

var no, yes = new(bool), func() *bool { t := true; return &t }()

// tex makes a texture table from "var=texture" pairs.
func tex(pairs ...string) map[string]string {
	m := map[string]string{}
	for _, p := range pairs {
		k, v, _ := strings.Cut(p, "=")
		m[k] = v
	}
	return m
}
