package vanillapack

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
)

const texDir = "assets/minecraft/textures/"

// texAliases are textures the reference's models name that the texture
// pack has under another name (or a close one).
var texAliases = map[string]string{
	"block/wildflowers_stem":         "block/wildflower_stem",
	"block/lightning_rod_on":         "block/lightning_rod",
	"block/open_eyeblossom_emissive": "block/open_eyeblossom_e",
}

// texMade are textures made here when the texture pack lacks them.
var texMade = map[string]func() image.Image{
	// the reference's is clear: the dust's colour is in its line
	"block/redstone_dust_overlay": func() image.Image { return image.NewNRGBA(image.Rect(0, 0, 16, 16)) },
	// a twisted thread in the rows the tripwire models draw (4 and 5)
	"block/tripwire": func() image.Image {
		im := image.NewNRGBA(image.Rect(0, 0, 16, 16))
		light, dark := color.NRGBA{0xdb, 0xdb, 0xdb, 0xff}, color.NRGBA{0x9c, 0x9c, 0x9c, 0xff}
		for x := range 16 {
			c1, c2 := light, dark
			if x%4 >= 2 {
				c1, c2 = dark, light
			}
			im.Set(x, 4, c1)
			im.Set(x, 5, c2)
		}
		return im
	},
}

// hidden are files of the default texture pack that don't fit the
// reference's blocks (its end_portal model draws a portal frame).
var hidden = map[string]bool{
	"assets/minecraft/models/block/end_portal.json": true,
}

// texFile serves an aliased or made texture: the file name, its contents
// (nil to read alias instead) and whether it is one.
func texFile(name string) (alias string, made []byte, ok bool) {
	rel, ok := strings.CutPrefix(name, texDir)
	if !ok {
		return "", nil, false
	}
	id, suffix, ok := strings.Cut(rel, ".")
	if !ok {
		return "", nil, false
	}
	if a, ok := texAliases[id]; ok {
		return texDir + a + "." + suffix, nil, true
	}
	if f, ok := texMade[id]; ok && suffix == "png" {
		var b bytes.Buffer
		png.Encode(&b, f())
		return "", b.Bytes(), true
	}
	return "", nil, false
}
