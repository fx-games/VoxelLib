package tpack

import "image"

type Byter interface {
	Bytes() []byte
}

type NamedImage struct {
	image.Image
	Name string
	// OffsetX and OffsetY are the picture's own hotspot, in pixels from its
	// top-left corner, before it is trimmed: where a weapon's grip sits, or
	// where a monster's feet are. A DECORATE animation's whole sense of
	// motion - a chainsaw swinging, a pistol kicking back - is often carried
	// entirely in how this moves from one frame's picture to the next, not
	// in anything the state script itself writes.
	OffsetX, OffsetY int
}

func (i NamedImage) Pixels() []uint8 {
	return AsRGBA(i).Pix
}

type LPString8 string

func (s LPString8) String() string {
	return string(s)
}
func (s LPString8) Bytes() []byte {
	length := uint8(len(s))
	return append([]byte{length}, []byte(s)...)
}
