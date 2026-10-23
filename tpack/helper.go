package tpack

import (
	"bytes"
	"github.com/memmaker/voxellib/tpack/png_chunks"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
)

// clampInt16 matches GZDoom's own tolerance for a grAb chunk's numbers: it
// reads them as 32 bit and zeroes anything outside int16 range rather than
// letting a corrupt or hand-edited chunk wrap into something nonsensical.
func clampInt16(v int) int16 {
	if v < -32768 || v > 32767 {
		return 0
	}
	return int16(v)
}

// OffsetsFromPNG reads a picture's own hotspot out of its grAb chunk. A picture
// without one has no hotspot, which is GZDoom's own answer for it: a weapon
// frame authored on the full 320x200 canvas - which is most of the ones that
// have no chunk - lands exactly where its artist meant it to, because zero is
// what that canvas was drawn against.
//
// Guessing instead (centred horizontally, anchored at the bottom) reads as
// harmless and is not: for a weapon sprite the hotspot is not a pivot, it is
// subtracted from the middle of Doom's 320x200 weapon space, and a guess of
// (width/2, height) throws a full-canvas frame most of a screen up and to the
// left. That is where Brutal Doom's minigun went.
func OffsetsFromPNG(data []byte) (x, y int) {
	grabX, grabY, _ := png_chunks.ReadGrabOffset(bytes.NewReader(data))
	if !offsetIsPlausible(grabX, grabY) {
		return 0, 0
	}
	return grabX, grabY
}

// offsetIsPlausible rejects a grAb chunk that cannot be describing a Doom
// offset at all. Doom measures both numbers against its own 320x200 screen -
// a weapon frame's hotspot is subtracted from the middle of it, and a world
// sprite's is a handful of pixels around its own art - so an offset further
// out than a whole screen is not an anchor anybody authored, it is a corrupt
// chunk.
//
// There is exactly one in the mods migrated so far, and it is instructive:
// Brutal Doom's tracer ships TRACA5 with grAb (27, -1232) on a 32x32 picture,
// which drew the tracer about three voxels away from the shot that made it.
// Nobody ever saw it in GZDoom, because the same mod replaces that actor's
// sprite with an MD3 model and the lump is never rendered.
//
// Zero is the fallback rather than a guess at the middle for the reason the
// doc comment above gives: for a weapon frame the hotspot is not a pivot, and
// a guess throws a full-canvas frame most of a screen away.
func offsetIsPlausible(x, y int) bool {
	const doomScreenWidth, doomScreenHeight = 320, 200
	return x >= -doomScreenWidth && x <= doomScreenWidth &&
		y >= -doomScreenHeight && y <= doomScreenHeight
}

func writeImageToPNG(image image.Image, dstPath string) error {
	outFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer outFile.Close()
	return png.Encode(outFile, image)
}

func writePNGWithEmbeddedAtlas(image image.Image, atlasEntries Byter, output io.Writer) error {
	atlasData := atlasEntries.Bytes()

	var byteBuffer *bytes.Buffer
	byteBuffer = new(bytes.Buffer)

	err := png.Encode(byteBuffer, image)
	if err != nil {
		return err
	}

	return png_chunks.InjectChunkIntoPNG(byteBuffer, output, "atls", atlasData)
}

func makeEven(value int) int {
	if value%2 == 0 {
		return value
	}
	return value + 1
}

func AsRGBA(img image.Image) *image.RGBA {
	switch typedImage := img.(type) {
	case *image.RGBA:
		return typedImage
	default:
		bounds := typedImage.Bounds()
		nrgba := image.NewRGBA(bounds)
		draw.Draw(nrgba, bounds, typedImage, bounds.Min, draw.Src)
		return nrgba
	}
}

func getTrimRect(img image.Image) image.Rectangle {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	pixels := AsRGBA(img).Pix
	minX, minY, maxX, maxY := detectOpaqueEdges(pixels, width, height)
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

func detectOpaqueEdges(pixels []uint8, textureWidth int, textureHeight int) (int, int, int, int) {
	// find the first non-transparent pixel on each side
	minX := textureWidth
	minY := textureHeight

	maxX := 0
	maxY := 0

	for y := 0; y < textureHeight; y++ {
		for x := 0; x < textureWidth; x++ {
			if pixels[(y*textureWidth+x)*4+3] > 0 {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	return minX, minY, maxX, maxY
}

// TrimRect is where a picture actually has pixels, which is the box the packer
// cuts it down to. Exported because a picture's own opaque bounds are what
// something standing in the world is the size of, and that is worked out once,
// away from the packing.
func TrimRect(img image.Image) image.Rectangle {
	return getTrimRect(img)
}
