package png_chunks

import (
	"encoding/binary"
	"io"
)

// grAb is the private PNG chunk SLADE (and GZDoom itself) writes a sprite's
// hotspot into: two big-endian signed 32-bit ints, X then Y, in pixels from
// the top-left corner. It is how a mod tells the engine where a weapon's grip
// sits, or where a monster's feet are, on a picture the engine would
// otherwise only know the width and height of.

// ReadGrabOffset reads a PNG's grAb chunk, if it has one.
func ReadGrabOffset(infile io.ReadSeeker) (x, y int, ok bool) {
	body := ReadChunkFromPNG(infile, "grAb")
	if len(body) < 8 {
		return 0, 0, false
	}
	x = int(int32(binary.BigEndian.Uint32(body[0:4])))
	y = int(int32(binary.BigEndian.Uint32(body[4:8])))
	return x, y, true
}

// InjectGrabChunk writes a hotspot into a PNG as it is copied from infile to
// outfile, the same way SLADE would have. Used for a picture converted from
// Doom's own legacy format, whose header carries the same offset under a
// different encoding, so the two formats read the same way downstream.
func InjectGrabChunk(infile io.Reader, outfile io.Writer, x, y int) error {
	body := make([]byte, 8)
	binary.BigEndian.PutUint32(body[0:4], uint32(int32(x)))
	binary.BigEndian.PutUint32(body[4:8], uint32(int32(y)))
	return InjectChunkIntoPNG(infile, outfile, "grAb", body)
}
