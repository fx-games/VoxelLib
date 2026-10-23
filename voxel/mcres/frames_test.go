package mcres

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"
)

// A region of an animated strip keeps every frame (campfire_fire#16:...).
func TestRegionFramesAnimated(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 16, 32))
	img.Set(0, 0, color.NRGBA{255, 0, 0, 255})  // frame 0
	img.Set(0, 16, color.NRGBA{0, 255, 0, 255}) // frame 1
	var buf bytes.Buffer
	png.Encode(&buf, img)
	p := FromFS(fstest.MapFS{
		"assets/x/textures/fire.png":        {Data: buf.Bytes()},
		"assets/x/textures/fire.png.mcmeta": {Data: []byte(`{"animation":{"frametime":2}}`)},
	})
	frames, ticks, err := p.TextureFrames("x:fire#16:0,0,8,8")
	if err != nil || len(frames) != 2 || ticks != 2 {
		t.Fatalf("got %d frames, %d ticks, %v; want 2, 2", len(frames), ticks, err)
	}
	if frames[0].Bounds().Dx() != 8 || frames[1].At(0, 0) != (color.NRGBA{0, 255, 0, 255}) {
		t.Fatalf("frame 1 region wrong: %v %v", frames[1].Bounds(), frames[1].At(0, 0))
	}
}
