// Command genbasepack writes ui/basepack's textures: the GUI a game has
// before it brings its own (or a client jar's), drawn here in code.
//
//	go run ./cmd/genbasepack   # from the repository root
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const out = "ui/basepack/assets/minecraft/textures"

func main() {
	write("font/ascii.png", fontSheet())
	write("gui/widgets.png", widgets())
	write("gui/options_background.png", background())
}

func write(name string, img image.Image) {
	p := filepath.Join(out, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(p)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

// glyphs are printable ASCII (0x20..0x7E) as five columns, bit 0 the top
// row and bit 7 a descender row: Adafruit GFX's glcdfont (BSD licence).
const glyphs = `00 00 00 00 00|00 00 5F 00 00|00 07 00 07 00|14 7F 14 7F 14|24 2A 7F 2A 12|23 13 08 64 62|36 49 56 20 50|00 08 07 03 00|
00 1C 22 41 00|00 41 22 1C 00|2A 1C 7F 1C 2A|08 08 3E 08 08|00 80 70 30 00|08 08 08 08 08|00 00 60 60 00|20 10 08 04 02|
3E 51 49 45 3E|00 42 7F 40 00|72 49 49 49 46|21 41 49 4D 33|18 14 12 7F 10|27 45 45 45 39|3C 4A 49 49 31|41 21 11 09 07|
36 49 49 49 36|46 49 49 29 1E|00 00 14 00 00|00 40 34 00 00|00 08 14 22 41|14 14 14 14 14|00 41 22 14 08|02 01 59 09 06|
3E 41 5D 59 4E|7C 12 11 12 7C|7F 49 49 49 36|3E 41 41 41 22|7F 41 41 41 3E|7F 49 49 49 41|7F 09 09 09 01|3E 41 41 51 73|
7F 08 08 08 7F|00 41 7F 41 00|20 40 41 3F 01|7F 08 14 22 41|7F 40 40 40 40|7F 02 1C 02 7F|7F 04 08 10 7F|3E 41 41 41 3E|
7F 09 09 09 06|3E 41 51 21 5E|7F 09 19 29 46|26 49 49 49 32|03 01 7F 01 03|3F 40 40 40 3F|1F 20 40 20 1F|3F 40 38 40 3F|
63 14 08 14 63|03 04 78 04 03|61 59 49 4D 43|00 7F 41 41 41|02 04 08 10 20|00 41 41 41 7F|04 02 01 02 04|40 40 40 40 40|
00 03 07 08 00|20 54 54 78 40|7F 28 44 44 38|38 44 44 44 28|38 44 44 28 7F|38 54 54 54 18|00 08 7E 09 02|18 A4 A4 9C 78|
7F 08 04 04 78|00 44 7D 40 00|20 40 40 3D 00|7F 10 28 44 00|00 41 7F 40 00|7C 04 78 04 78|7C 08 04 04 78|38 44 44 44 38|
FC 18 24 24 18|18 24 24 18 FC|7C 08 04 04 08|48 54 54 54 24|04 04 3F 44 24|3C 40 40 20 7C|1C 20 40 20 1C|3C 40 30 40 3C|
44 28 10 28 44|4C 90 90 90 7C|44 64 54 4C 44|00 08 36 41 00|00 00 77 00 00|00 41 36 08 00|02 01 02 04 02`

// fontSheet is ascii.png: 16×16 cells of 8×8, the glyph in the cell's top
// left, white on transparent (Font measures each glyph's width).
func fontSheet() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for i, g := range strings.Split(strings.ReplaceAll(glyphs, "\n", ""), "|") {
		c := 0x20 + i
		cx, cy := c%16*8, c/16*8
		for x, col := range strings.Fields(g) {
			bits, _ := strconv.ParseUint(col, 16, 8)
			for y := 0; y < 8; y++ {
				if bits>>y&1 != 0 {
					img.SetNRGBA(cx+x, cy+y, color.NRGBA{255, 255, 255, 255})
				}
			}
		}
	}
	return img
}

// widgets is widgets.png's buttons: 200×20 at y 46 (disabled), 66
// (normal) and 86 (hover), bevelled slate.
func widgets() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	rng := rand.New(rand.NewSource(1))
	for k, base := range []uint8{60, 105, 125} {
		y0 := 46 + k*20
		for y := 0; y < 20; y++ {
			for x := 0; x < 200; x++ {
				v := int(base) + rng.Intn(9) - 4
				switch {
				case x == 0 || y == 0 || x == 199 || y == 19:
					v = 0
				case x == 1 || y == 1:
					v += 45
				case x == 198 || y == 17 || y == 18:
					v -= 40
				}
				v = max(0, min(255, v))
				tint := uint8(max(0, min(255, v+map[bool]int{true: 18}[k == 2])))
				img.SetNRGBA(x, y0+y, color.NRGBA{uint8(v), uint8(v), tint, 255})
			}
		}
	}
	return img
}

// background is options_background.png: a dark flagstone tile.
func background() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	rng := rand.New(rand.NewSource(2))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			v := 70 + rng.Intn(24)
			if y == 7 || y == 15 || x == 15 || x == 7 && y < 7 || x == 3 && y > 7 { // mortar
				v = 40
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(v), uint8(v - 4), uint8(v - 10), 255})
		}
	}
	return img
}
