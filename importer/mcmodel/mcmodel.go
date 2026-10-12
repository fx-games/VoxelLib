// Package mcmodel imports Minecraft-style assets: Blockbench projects
// (.bbmodel), Bedrock/GeckoLib geometry (.geo.json) and animations
// (.animation.json, with Molang expressions and GeckoLib easings), and
// item textures (16x16-ish PNGs under an item folder).
//
// Import takes a folder or zip (e.g. a clone of a creator's asset repo or a
// resource pack) and returns a Pack: models with all their clips, a shared
// animation library that is bound to every model whose bone names match
// (humanoid rigs share one skeleton, so one library drives all of them),
// and item sprites ready for items/extrude.
//
// Models come out in model package conventions: blocks as units, Y up,
// facing +Z.
package mcmodel

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"path"
	"sort"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/packfs"
	"github.com/memmaker/voxellib/model"
)

// Item is an item sprite.
type Item struct {
	Name  string // file base name, e.g. "arcane_blade"
	Path  string // path inside the pack
	Image image.Image
}

// Pack is everything imported from one source.
type Pack struct {
	Models     []*model.Model
	Animations []*Animation // library from animation files and .bbmodels
	Items      []Item
	// Skipped lists files that looked like assets but failed to import.
	Skipped []string
}

// Model returns a model by name, or nil.
func (p *Pack) Model(name string) *model.Model {
	for _, m := range p.Models {
		if m.Name == name {
			return m
		}
	}
	return nil
}

// Options tune Import.
type Options struct {
	// MaxItemSize is the largest PNG edge treated as an item (default 32).
	MaxItemSize int
	// ShareAnimations binds every library animation to every model whose
	// bones match (default true via Import).
	NoShare bool
}

// Import loads a pack with default options.
func Import(src string) (*Pack, error) { return ImportWith(src, Options{}) }

// ImportWith loads a pack.
func ImportWith(src string, o Options) (*Pack, error) {
	if o.MaxItemSize == 0 {
		o.MaxItemSize = 32
	}
	files, err := packfs.List(src)
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	pngs := map[string]packfs.File{}
	for _, f := range files {
		if strings.HasSuffix(strings.ToLower(f.Name), ".png") {
			pngs[strings.ToLower(strings.TrimSuffix(path.Base(f.Name), ".png"))] = f
		}
	}
	p := &Pack{}
	seenAnim := map[string]bool{}
	addAnims := func(as []*Animation) {
		for _, a := range as {
			if !seenAnim[a.Name] {
				seenAnim[a.Name] = true
				p.Animations = append(p.Animations, a)
			}
		}
	}
	for _, f := range files {
		ln := strings.ToLower(f.Name)
		base := path.Base(ln)
		switch {
		case strings.HasSuffix(ln, ".bbmodel"):
			data, err := f.Read()
			if err != nil {
				return nil, err
			}
			m, anims, err := ParseBBModel(data)
			if err != nil {
				p.Skipped = append(p.Skipped, fmt.Sprintf("%s: %v", f.Name, err))
				continue
			}
			m.Name = modelName(f.Name)
			p.Models = append(p.Models, m)
			addAnims(anims)
		case strings.HasSuffix(ln, ".geo.json"):
			data, err := f.Read()
			if err != nil {
				return nil, err
			}
			ms, err := ParseGeometry(data, func(id string) image.Image {
				for _, n := range []string{id, strings.TrimSuffix(base, ".geo.json")} {
					if t, ok := pngs[n]; ok {
						if d, err := t.Read(); err == nil {
							if img, err := png.Decode(bytes.NewReader(d)); err == nil {
								return img
							}
						}
					}
				}
				return nil
			})
			if err != nil {
				p.Skipped = append(p.Skipped, fmt.Sprintf("%s: %v", f.Name, err))
				continue
			}
			p.Models = append(p.Models, ms...)
		case strings.HasSuffix(ln, ".json") && strings.Contains(base, "anim"):
			data, err := f.Read()
			if err != nil {
				return nil, err
			}
			as, err := ParseAnimationFile(data)
			if err != nil {
				p.Skipped = append(p.Skipped, fmt.Sprintf("%s: %v", f.Name, err))
				continue
			}
			addAnims(as)
		case strings.HasSuffix(ln, ".png") && isItemPath(ln):
			data, err := f.Read()
			if err != nil {
				return nil, err
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				p.Skipped = append(p.Skipped, fmt.Sprintf("%s: %v", f.Name, err))
				continue
			}
			if b := img.Bounds(); b.Dx() > o.MaxItemSize || b.Dy() > o.MaxItemSize {
				continue
			}
			p.Items = append(p.Items, Item{Name: strings.TrimSuffix(path.Base(f.Name), path.Ext(f.Name)), Path: f.Name, Image: img})
		}
	}
	for _, m := range p.Models {
		if !o.NoShare {
			for _, a := range p.Animations {
				if _, ok := m.Clips[a.Name]; ok {
					continue
				}
				// Share only when the rigs match: most animated bones exist.
				if c, ok := a.Bind(m); ok && 2*boneCount(c) >= len(a.Bones) {
					m.Clips[a.Name] = c
				}
			}
		}
		tag(m)
		// Minecraft entity models face north (-Z); turn them to +Z.
		m.Transform(mgl32.HomogRotate3DY(3.14159265))
	}
	return p, nil
}

func modelName(p string) string {
	b := path.Base(p)
	b = strings.TrimSuffix(b, path.Ext(b))
	return strings.TrimSuffix(b, ".geo")
}

func isItemPath(p string) bool {
	for _, seg := range strings.Split(path.Dir(p), "/") {
		if seg == "item" || seg == "items" {
			return true
		}
	}
	return false
}

// tag marks humanoid rigs (head, arms and legs) so callers can pick
// humanoid behaviour and animations.
func tag(m *model.Model) {
	has := map[string]bool{}
	for _, b := range m.Bones {
		has[canonBone(b.Name)] = true
	}
	if has["head"] && has["leftarm"] && has["rightarm"] && has["leftleg"] && has["rightleg"] {
		m.Tags["humanoid"] = true
	}
}

func boneCount(c *model.Clip) int {
	seen := map[int]bool{}
	for _, ch := range c.Channels {
		seen[ch.Bone] = true
	}
	return len(seen)
}
