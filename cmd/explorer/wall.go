package main

import (
	"errors"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/voxel/render"
)

// The battle royale energy wall, drawn like the reference's world border
// (RenderGlobal.renderWorldBorder): each side within the render distance
// as a 256-high sheet (round the viewer's height) of a scrolling force-field texture, blended
// additively, fading in as the viewer comes near (alpha (1 - d/far)^4),
// blue while it stands and red while it closes in.

const forcefield = "voxellib:forcefield"

// wallTextures serves the force field to the renderer's SkyDraws and
// everything else from the source it had.
type wallTextures struct {
	next render.TextureSource
	img  image.Image
}

func (w *wallTextures) Texture(name string) (image.Image, error) {
	if name == forcefield {
		return w.img, nil
	}
	if w.next == nil {
		return nil, errors.New("no texture source")
	}
	return w.next.Texture(name)
}

// forcefieldImage is a 32x32 tile of thin diagonal lines in both
// directions, white (the vertex colour tints it), brighter where they
// cross: an energy lattice.
func forcefieldImage() image.Image {
	const n = 32
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			a := 0.0
			if (x+y)%8 == 0 {
				a += 0.7
			}
			if (x-y+n)%8 == 0 {
				a += 0.7
			}
			if (x+y)%16 == 4 && (x-y+n)%16 == 4 {
				a = 1
			}
			img.SetNRGBA(x, y, color.NRGBA{255, 255, 255, uint8(math.Min(1, a) * 255)})
		}
	}
	return img
}

// drawWall draws the zone's wall for the camera.
func (g *game) drawWall(z mp.Zone, closing bool) {
	r := g.Eng.Renderer
	if _, ok := r.Opt.SkyTextures.(*wallTextures); !ok {
		r.Opt.SkyTextures = &wallTextures{next: r.Opt.SkyTextures, img: forcefieldImage()}
		if r.Opt.SkyTextures.(*wallTextures).next == nil {
			r.Opt.SkyTextures.(*wallTextures).next = g.Config.Engine.Textures
		}
	}
	far := float64(r.Opt.RenderDistance * 16)
	cx, cy, cz := g.Cam.X, g.Cam.Y, g.Cam.Z
	// The closest distance to a side (WorldBorder.getClosestDistance).
	d := math.Min(math.Min(cx-z.MinX, z.MaxX-cx), math.Min(cz-z.MinZ, z.MaxZ-cz))
	if d > far {
		return
	}
	alpha := float32(math.Pow(1-math.Max(0, d)/far, 4))
	col := [4]float32{0x20 / 255.0, 0xA0 / 255.0, 1, alpha}
	if closing {
		col = [4]float32{1, 0x30 / 255.0, 0x30 / 255.0, alpha}
	}
	scroll := float32(time.Now().UnixMilli()%3000) / 3000
	// 256 high like the reference's 0 to 256, but round the viewer: worlds
	// here reach below 0. The texture's v spans 128 like the reference's,
	// on whole 2-block repeats so it doesn't slide as the viewer climbs.
	bottom := math.Floor(cy/2)*2 - 128
	top := bottom + 256
	var verts []render.SkyVertex
	// side adds the sheet along one side from a to b (world, along the
	// side), at the fixed coordinate c; x tells which axis it runs on.
	side := func(c, a, b float64, alongZ bool) {
		a, b = math.Max(a, math.Floor(along(alongZ, cx, cz)-far)), math.Min(b, math.Ceil(along(alongZ, cx, cz)+far))
		if b <= a {
			return
		}
		u0, u1 := scroll, scroll+float32(b-a)*0.5
		pos := func(t, y float64) [3]float32 {
			if alongZ {
				return [3]float32{float32(c - cx), float32(y - cy), float32(t - cz)}
			}
			return [3]float32{float32(t - cx), float32(y - cy), float32(c - cz)}
		}
		verts = append(verts,
			render.SkyVertex{Pos: pos(a, top), UV: [2]float32{u0, scroll}, Color: col},
			render.SkyVertex{Pos: pos(b, top), UV: [2]float32{u1, scroll}, Color: col},
			render.SkyVertex{Pos: pos(b, bottom), UV: [2]float32{u1, scroll + 128}, Color: col},
			render.SkyVertex{Pos: pos(a, bottom), UV: [2]float32{u0, scroll + 128}, Color: col})
	}
	if z.MaxX-cx < far {
		side(z.MaxX, z.MinZ, z.MaxZ, true)
	}
	if cx-z.MinX < far {
		side(z.MinX, z.MinZ, z.MaxZ, true)
	}
	if z.MaxZ-cz < far {
		side(z.MaxZ, z.MinX, z.MaxX, false)
	}
	if cz-z.MinZ < far {
		side(z.MinZ, z.MinX, z.MaxX, false)
	}
	if len(verts) == 0 {
		return
	}
	g.wallDraw[0] = render.SkyDraw{Texture: forcefield, Additive: true, Verts: verts, Transform: mgl32.Ident4()}
	r.DrawSkyDraws(&g.Cam, g.wallDraw[:])
}

func along(alongZ bool, x, z float64) float64 {
	if alongZ {
		return z
	}
	return x
}
