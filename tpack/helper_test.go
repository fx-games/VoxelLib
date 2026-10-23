package tpack

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/memmaker/voxellib/tpack/png_chunks"
	"github.com/stretchr/testify/assert"
)

// grabbed builds a one pixel PNG carrying the grAb chunk given.
func grabbed(t *testing.T, x, y int) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, 1, 1))
	picture.Set(0, 0, color.RGBA{1, 2, 3, 255})
	plain := new(bytes.Buffer)
	if err := png.Encode(plain, picture); err != nil {
		t.Fatal(err)
	}
	withChunk := new(bytes.Buffer)
	if err := png_chunks.InjectGrabChunk(plain, withChunk, x, y); err != nil {
		t.Fatal(err)
	}
	return withChunk.Bytes()
}

// A hotspot that lands further out than Doom's whole screen is a corrupt
// chunk, not an anchor. Brutal Doom ships one: TRACA5 says (27, -1232) on a
// 32x32 tracer, which drew the tracer three voxels off the shot that made it.
func TestACorruptHotspotIsNotBelieved(t *testing.T) {
	x, y := OffsetsFromPNG(grabbed(t, 27, -1232))
	assert.Equal(t, 0, x)
	assert.Equal(t, 0, y)

	// A real one, however far off its own picture, is left alone: a weapon
	// frame's hotspot is subtracted from the middle of the 320x200 space and
	// is routinely a couple of hundred pixels negative.
	x, y = OffsetsFromPNG(grabbed(t, -250, 25))
	assert.Equal(t, -250, x)
	assert.Equal(t, 25, y)
}
