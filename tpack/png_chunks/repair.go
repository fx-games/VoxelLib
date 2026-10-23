package png_chunks

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Doom mods ship PNGs that GZDoom reads happily and Go's decoder refuses.
// Brutal Doom Community Expansion has both kinds: files whose chunk checksums
// do not match what is in them, and files carrying a tRNS chunk on an image
// that already has an alpha channel, which the spec does not allow.
//
// Neither makes the picture unreadable - the pixels are fine - so rather than
// lose forty-five weapons to it, this rewrites the container: the illegal
// chunks come off and every checksum is worked out again. The image data is
// untouched.

// Repair rewrites a PNG so a strict decoder will read it, and reports whether
// anything had to change.
func Repair(data []byte) (repaired []byte, changed bool, err error) {
	if !bytes.HasPrefix(data, pngSig) {
		return nil, false, fmt.Errorf("not a PNG")
	}

	var out bytes.Buffer
	out.Write(pngSig)

	colourType := byte(0)
	offset := len(pngSig)
	for offset+8 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		if length < 0 || offset+12+length > len(data) {
			return nil, false, fmt.Errorf("a chunk runs off the end")
		}
		name := string(data[offset+4 : offset+8])
		body := data[offset+8 : offset+8+length]
		stored := binary.BigEndian.Uint32(data[offset+8+length : offset+12+length])
		offset += 12 + length

		if name == "IHDR" && len(body) >= 10 {
			colourType = body[9]
		}
		// tRNS is a palette's transparency. An image that already carries an
		// alpha channel has no business with one, and Go says so.
		if name == "tRNS" && (colourType == 4 || colourType == 6) {
			changed = true
			continue
		}

		computed := pngCRC(append([]byte(name), body...))
		if computed != stored {
			changed = true
		}
		binary.Write(&out, binary.BigEndian, uint32(length))
		out.WriteString(name)
		out.Write(body)
		binary.Write(&out, binary.BigEndian, computed)

		if name == "IEND" {
			break
		}
	}
	return out.Bytes(), changed, nil
}

// RepairReader is Repair over a stream.
func RepairReader(in io.Reader) ([]byte, bool, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return nil, false, err
	}
	return Repair(data)
}
