package render

import "image"

// Overlay draws a 2D image over the frame in pixel coordinates, for HUD
// panels such as a help screen. The image is uploaded by SetImage and
// stays on the GPU until replaced.
type Overlay struct {
	gpu  gpuOverlay
	w, h int
}

// NewOverlay creates an empty overlay.
func NewOverlay() (*Overlay, error) {
	o := &Overlay{}
	if err := o.initGPU(); err != nil {
		return nil, err
	}
	return o, nil
}

// SetImage uploads the image to draw, into the same texture while the
// size stays (HUDs repaint every frame).
func (o *Overlay) SetImage(img image.Image) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	o.setPixels(toNRGBARect(img).Pix, w, h, w == o.w && h == o.h)
	o.w, o.h = w, h
}

// Size returns the image size in pixels.
func (o *Overlay) Size() (w, h int) { return o.w, o.h }

// Draw blends the image at pixel position x,y (top-left origin), scaled by
// an integer factor, on a framebuffer of fbW x fbH pixels.
func (o *Overlay) Draw(x, y, scale, fbW, fbH int) {
	if !o.hasImage() || fbW == 0 || fbH == 0 {
		return
	}
	nx := func(px int) float32 { return float32(px)/float32(fbW)*2 - 1 }
	ny := func(py int) float32 { return 1 - float32(py)/float32(fbH)*2 }
	o.drawRect([4]float32{nx(x), ny(y + o.h*scale), nx(x + o.w*scale), ny(y)})
}
