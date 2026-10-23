package tpack

import (
	"bufio"
	"bytes"
)

type AtlasMetadata []AtlasEntry

func NewAtlasMetadataFromBytes(data []byte) AtlasMetadata {
	a := make(AtlasMetadata, 0)
	reader := bytes.NewReader(data)
	for reader.Len() > 0 {
		entry := NewAtlasEntryFromBinaryReader(reader)
		a = append(a, entry)
	}
	return a
}

func NewAtlasMetadataFromTxtReader(reader *bytes.Reader) AtlasMetadata {
	scanner := bufio.NewScanner(reader)
	a := make(AtlasMetadata, 0)
	for scanner.Scan() {
		line := scanner.Text()
		entry := NewAtlasEntryFromString(line)
		if !entry.IsValid() {
			continue
		}
		a = append(a, entry)
	}
	return a
}

func (a AtlasMetadata) Bytes() []byte {
	var buffer []byte
	for _, entry := range a {
		buffer = append(buffer, entry.Bytes()...)
	}
	return buffer
}

func (a AtlasMetadata) Size(index int) (int, int) {
	return int(a[index].Width), int(a[index].Height)
}

func (a AtlasMetadata) Position(index int) (int, int) {
	return int(a[index].X), int(a[index].Y)
}

func (a AtlasMetadata) UV(index int) (minU int, minV int, maxU int, maxV int) {
	width, height := a.Size(index)
	x, y := a.Position(index)
	return x, y, x + width, y + height
}

func (a AtlasMetadata) SizeForLayout(index int) (int, int) {
	return a.Size(index)
}
func (a AtlasMetadata) PositionOffsetForLayout(index int) (int, int) {
	return 0, 0
}

func (a AtlasMetadata) String() string {
	var buffer bytes.Buffer
	for _, entry := range a {
		buffer.WriteString(entry.TxtEncode())
		buffer.WriteString("\n")
	}
	return buffer.String()
}
