package tpack

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type AtlasEntry struct {
	SpriteName          LPString8
	X, Y, Width, Height uint16
}

func NewAtlasEntryFromBinaryReader(reader *bytes.Reader) AtlasEntry {
	var nameLength uint8
	binary.Read(reader, binary.BigEndian, &nameLength)
	var spriteName = make([]byte, nameLength)
	binary.Read(reader, binary.BigEndian, &spriteName)
	var x, y, width, height uint16
	binary.Read(reader, binary.BigEndian, &x)
	binary.Read(reader, binary.BigEndian, &y)
	binary.Read(reader, binary.BigEndian, &width)
	binary.Read(reader, binary.BigEndian, &height)
	return AtlasEntry{
		SpriteName: LPString8(spriteName),
		X:          x,
		Y:          y,
		Width:      width,
		Height:     height,
	}
}

func (a AtlasEntry) TxtEncode() string {
	return fmt.Sprintf("%s\t%d\t%d\t%d\t%d\n", a.SpriteName, a.X, a.Y, a.Width, a.Height)
}

func NewAtlasEntryFromString(line string) AtlasEntry {
	entry := AtlasEntry{}
	fmt.Scanf(line, "%s\t%d\t%d\t%d\t%d", &entry.SpriteName, &entry.X, &entry.Y, &entry.Width, &entry.Height)
	return entry
}

func (a AtlasEntry) String() string {
	return fmt.Sprintf("%s %d %d %d %d", a.SpriteName, a.X, a.Y, a.Width, a.Height)
}
func (a AtlasEntry) Length() int {
	return len(a.SpriteName) + 9
}

func (a AtlasEntry) Bytes() []byte {
	writer := new(bytes.Buffer)
	binary.Write(writer, binary.BigEndian, a.SpriteName.Bytes())
	binary.Write(writer, binary.BigEndian, a.X)
	binary.Write(writer, binary.BigEndian, a.Y)
	binary.Write(writer, binary.BigEndian, a.Width)
	binary.Write(writer, binary.BigEndian, a.Height)
	return writer.Bytes()
}

func (a AtlasEntry) IsValid() bool {
	return a.SpriteName != "" && a.Width > 0 && a.Height > 0
}
