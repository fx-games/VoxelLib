package render

import (
	"log"

	"github.com/go-gl/mathgl/mgl32"
)

// SkyDraw is one camera-centred sky draw (sky planes, sun, moon, clouds,
// rain): coloured or textured geometry drawn with depth test and no depth
// writes, in Options.Sky or Options.SkyAfter order.
type SkyDraw struct {
	// Texture names a texture of Options.SkyTextures; "" draws untextured.
	// UVs repeat.
	Texture string
	// Additive blends (SRC_ALPHA, ONE); else alpha blending.
	Additive bool
	// Fog, when set, mixes toward its colour by eye distance (linear).
	Fog *Fog
	// AlphaTest discards fragments with alpha at or below it (when > 0).
	AlphaTest float32
	// Far is the projection's far plane; 0 uses the terrain camera's.
	Far float32
	// Fan makes Verts a triangle fan; else every 4 Verts are a quad.
	Fan bool
	// Transform maps Verts to camera-relative world space; zero means
	// identity.
	Transform mgl32.Mat4
	Verts     []SkyVertex
}

// SkyVertex is a sky vertex: position, texture coordinate and an RGBA
// colour multiplied with the texture (glColor4f per vertex).
type SkyVertex struct {
	Pos   [3]float32
	UV    [2]float32
	Color [4]float32
}

// triangles appends d's vertices as a triangle list.
func (d *SkyDraw) triangles(out []SkyVertex) []SkyVertex {
	v := d.Verts
	if d.Fan {
		for i := 2; i < len(v); i++ {
			out = append(out, v[0], v[i-1], v[i])
		}
		return out
	}
	for i := 0; i+3 < len(v); i += 4 {
		out = append(out, v[i], v[i+1], v[i+2], v[i+2], v[i+3], v[i])
	}
	return out
}

// skyTexture returns the uploaded texture for name, loading it on first
// use; nil if it can't be loaded.
func (r *Renderer) skyTexture(name string) *skyTexture {
	if t, ok := r.skyTex[name]; ok {
		return t
	}
	if r.skyTex == nil {
		r.skyTex = map[string]*skyTexture{}
	}
	src := r.Opt.SkyTextures
	if src == nil {
		src = r.src
	}
	img, err := src.Texture(name)
	if err != nil {
		log.Printf("render: sky texture %s: %v", name, err)
		r.skyTex[name] = nil
		return nil
	}
	t := uploadSkyTexture(img)
	r.skyTex[name] = t
	return t
}
