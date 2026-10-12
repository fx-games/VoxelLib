// Package roblox imports Roblox-style creature/asset packs (low-poly rigged
// models sold for Roblox, Unity and Godot, like Sneznyj's "Cube Pets"):
// a folder or .zip containing glTF/GLB models, often next to FBX and
// .blend copies and a Textures/ folder.
//
// Import finds every model in whatever is dropped on it (folders, zips,
// zips inside zips), keeps one copy per model (GLB/glTF are preferred; FBX
// and .blend are skipped because they need proprietary SDKs), fills in a
// missing base colour texture from the pack, and tags winged creatures as
// "flying". The result is a list of model.Model ready for
// render.ModelRenderer and mobs.
//
// Roblox's own binary place/model files (.rbxl/.rbxm) are not supported.
package roblox

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png"
	"path"
	"sort"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/packfs"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/model/gltf"
)

// Import loads every model in src (a directory, .zip, .glb or .gltf).
func Import(src string) ([]*model.Model, error) {
	files, err := packfs.List(src)
	if err != nil {
		return nil, err
	}
	byName := map[string]packfs.File{}
	for _, f := range files {
		byName[strings.ToLower(f.Name)] = f
	}
	// Pick one file per model base name, preferring GLB over glTF.
	pick := map[string]packfs.File{}
	for _, f := range files {
		ext := strings.ToLower(path.Ext(f.Name))
		if ext != ".glb" && ext != ".gltf" {
			continue
		}
		base := strings.TrimSuffix(path.Base(f.Name), path.Ext(f.Name))
		if old, ok := pick[base]; ok && strings.ToLower(path.Ext(old.Name)) == ".glb" {
			continue
		}
		pick[base] = f
	}
	if len(pick) == 0 {
		return nil, fmt.Errorf("roblox: no glTF/GLB models in %s", src)
	}
	names := make([]string, 0, len(pick))
	for n := range pick {
		names = append(names, n)
	}
	sort.Strings(names)

	var packTex image.Image // fallback: a *BaseColor*/*Albedo* PNG in the pack
	for _, f := range files {
		n := strings.ToLower(path.Base(f.Name))
		if strings.HasSuffix(n, ".png") && (strings.Contains(n, "basecolor") || strings.Contains(n, "albedo") || strings.Contains(n, "diffuse")) {
			if data, err := f.Read(); err == nil {
				packTex, _, _ = image.Decode(bytes.NewReader(data))
			}
			break
		}
	}

	var out []*model.Model
	for _, n := range names {
		f := pick[n]
		data, err := f.Read()
		if err != nil {
			return nil, err
		}
		dir := path.Dir(f.Name)
		m, err := gltf.Load(data, func(uri string) ([]byte, error) {
			g, ok := byName[strings.ToLower(path.Join(dir, uri))]
			if !ok {
				return nil, fmt.Errorf("missing %s", uri)
			}
			return g.Read()
		})
		if err != nil {
			return nil, fmt.Errorf("roblox: %s: %w", f.Name, err)
		}
		m.Name = n
		if len(m.Textures) == 0 && packTex != nil {
			m.Textures = []image.Image{packTex}
			for i := range m.Meshes {
				m.Meshes[i].Texture = 0
			}
		}
		Tag(m)
		out = append(out, m)
	}
	return out, nil
}

// Tag sets importer hints from the rig: "flying" when the model has wing
// bones. Callers may override tags afterwards.
func Tag(m *model.Model) {
	for _, b := range m.Bones {
		if strings.Contains(strings.ToLower(b.Name), "wing") {
			m.Tags["flying"] = true
		}
	}
}

// FitHeight scales a model so its bind-pose height is h blocks and its
// feet rest at y=0 (handy for packs authored at arbitrary scales).
func FitHeight(m *model.Model, h float32) {
	lo, hi := m.Bounds()
	if hi[1] <= lo[1] {
		return
	}
	s := h / (hi[1] - lo[1])
	m.Transform(mgl32.Scale3D(s, s, s).Mul4(mgl32.Translate3D(0, -lo[1], 0)))
}
