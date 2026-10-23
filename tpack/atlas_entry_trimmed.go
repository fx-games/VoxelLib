package tpack

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type TrimmedAtlasEntry struct {
	SpriteName                LPString8
	X, Y                      uint16
	SourceWidth, SourceHeight uint16

	TrimRectX, TrimRectY          uint16
	TrimRectWidth, TrimRectHeight uint16

	// OffsetX and OffsetY are the picture's own hotspot, in the same
	// top-left-of-the-untrimmed-source space as TrimRectX/Y, so a renderer
	// anchors a frame where the artist put it rather than at a fixed corner
	// or center that only happens to be right for a picture with no padding
	// trimmed off. Signed: a hotspot is often above or left of the picture's
	// own visible pixels entirely.
	OffsetX, OffsetY int16
}

func NewTrimmedAtlasEntryFromReader(reader *bytes.Reader) TrimmedAtlasEntry {
	var nameLength uint8
	binary.Read(reader, binary.BigEndian, &nameLength)
	var spriteName = make([]byte, nameLength)
	binary.Read(reader, binary.BigEndian, &spriteName)
	var x, y, sourceWidth, sourceHeight uint16
	binary.Read(reader, binary.BigEndian, &x)
	binary.Read(reader, binary.BigEndian, &y)
	binary.Read(reader, binary.BigEndian, &sourceWidth)
	binary.Read(reader, binary.BigEndian, &sourceHeight)
	var trimRectX, trimRectY, trimRectWidth, trimRectHeight uint16
	binary.Read(reader, binary.BigEndian, &trimRectX)
	binary.Read(reader, binary.BigEndian, &trimRectY)
	binary.Read(reader, binary.BigEndian, &trimRectWidth)
	binary.Read(reader, binary.BigEndian, &trimRectHeight)
	var offsetX, offsetY int16
	binary.Read(reader, binary.BigEndian, &offsetX)
	binary.Read(reader, binary.BigEndian, &offsetY)
	return TrimmedAtlasEntry{
		SpriteName:     LPString8(spriteName),
		X:              x,
		Y:              y,
		SourceWidth:    sourceWidth,
		SourceHeight:   sourceHeight,
		TrimRectX:      trimRectX,
		TrimRectY:      trimRectY,
		TrimRectWidth:  trimRectWidth,
		TrimRectHeight: trimRectHeight,
		OffsetX:        offsetX,
		OffsetY:        offsetY,
	}
}

func (e TrimmedAtlasEntry) Bytes() []byte {
	writer := new(bytes.Buffer)
	binary.Write(writer, binary.BigEndian, e.SpriteName.Bytes())
	binary.Write(writer, binary.BigEndian, e.X)
	binary.Write(writer, binary.BigEndian, e.Y)
	binary.Write(writer, binary.BigEndian, e.SourceWidth)
	binary.Write(writer, binary.BigEndian, e.SourceHeight)
	binary.Write(writer, binary.BigEndian, e.TrimRectX)
	binary.Write(writer, binary.BigEndian, e.TrimRectY)
	binary.Write(writer, binary.BigEndian, e.TrimRectWidth)
	binary.Write(writer, binary.BigEndian, e.TrimRectHeight)
	binary.Write(writer, binary.BigEndian, e.OffsetX)
	binary.Write(writer, binary.BigEndian, e.OffsetY)
	return writer.Bytes()
}

func (e TrimmedAtlasEntry) String() string {
	return fmt.Sprintf("%s %d %d %d %d %d %d %d %d %d %d", e.SpriteName, e.X, e.Y, e.SourceWidth, e.SourceHeight, e.TrimRectX, e.TrimRectY, e.TrimRectWidth, e.TrimRectHeight, e.OffsetX, e.OffsetY)
}
