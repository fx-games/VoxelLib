package tpack

import "bytes"

type TrimmedAtlasMetadata []TrimmedAtlasEntry

func NewTrimmedAtlasMetadataFromBytes(data []byte) TrimmedAtlasMetadata {
	a := make(TrimmedAtlasMetadata, 0)
	reader := bytes.NewReader(data)
	for reader.Len() > 0 {
		entry := NewTrimmedAtlasEntryFromReader(reader)
		a = append(a, entry)
	}
	return a
}

func (a TrimmedAtlasMetadata) HasIndex(index int) bool {
	return index >= 0 && index < len(a)
}

func (a TrimmedAtlasMetadata) LastValidIndex() int {
	return len(a) - 1
}

func (a TrimmedAtlasMetadata) Name(index int) string {
	return string(a[index].SpriteName)
}

func (a TrimmedAtlasMetadata) Bytes() []byte {
	var buffer []byte
	for _, entry := range a {
		buffer = append(buffer, entry.Bytes()...)
	}
	return buffer
}

func (a TrimmedAtlasMetadata) UVSize(index int) (int, int) {
	return int(a[index].TrimRectWidth), int(a[index].TrimRectHeight)
}

func (a TrimmedAtlasMetadata) UVPosition(index int) (int, int) {
	return int(a[index].X), int(a[index].Y)
}

func (a TrimmedAtlasMetadata) UV(index int) (minU int, minV int, maxU int, maxV int) {
	width, height := a.UVSize(index)
	x, y := a.UVPosition(index)
	return x, y, x + width, y + height
}

func (a TrimmedAtlasMetadata) SizeForLayout(index int) (int, int) {
	return int(a[index].SourceWidth), int(a[index].SourceHeight)
}
func (a TrimmedAtlasMetadata) PositionOffsetForLayout(index int) (int, int) {
	return int(a[index].TrimRectX), int(a[index].TrimRectY)
}

// Hotspot is where the picture's own artist put its anchor, in the same
// top-left-of-the-untrimmed-picture space SizeForLayout describes: how far
// right and down from that corner a weapon's grip, or a monster's feet, sit.
func (a TrimmedAtlasMetadata) Hotspot(index int) (int, int) {
	return int(a[index].OffsetX), int(a[index].OffsetY)
}
