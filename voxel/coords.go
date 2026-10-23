// Package voxel is the data half of the engine: block registry, paletted chunk
// sections, the world store and the light engine. It has no rendering or OpenGL
// dependency, so it can be used headless (servers, tools, tests).
//
// Coordinates and layouts follow Minecraft: Y is up, -Z is north, chunks are
// 16x16 columns split into 16x16x16 sections, and the index of a block inside a
// section is y<<8 | z<<4 | x, which is also the order the Anvil format stores
// block states and light nibbles in.
package voxel

const (
	SectionShift  = 4
	SectionSize   = 1 << SectionShift
	SectionMask   = SectionSize - 1
	SectionVolume = SectionSize * SectionSize * SectionSize
)

// BlockPos is an absolute block coordinate.
type BlockPos struct{ X, Y, Z int }

// ChunkPos addresses a 16 x 16 column.
type ChunkPos struct{ X, Z int }

// SectionPos addresses one 16^3 section; Y is the section index, not blocks
// (block y = Y*16).
type SectionPos struct{ X, Y, Z int }

func (p BlockPos) Add(o BlockPos) BlockPos { return BlockPos{p.X + o.X, p.Y + o.Y, p.Z + o.Z} }
func (p BlockPos) Offset(f Face) BlockPos  { return p.Add(f.Normal()) }
func (p BlockPos) Chunk() ChunkPos         { return ChunkPos{p.X >> 4, p.Z >> 4} }
func (p BlockPos) Section() SectionPos     { return SectionPos{p.X >> 4, p.Y >> 4, p.Z >> 4} }

func (c ChunkPos) Add(dx, dz int) ChunkPos { return ChunkPos{c.X + dx, c.Z + dz} }

func (s SectionPos) Chunk() ChunkPos { return ChunkPos{s.X, s.Z} }
func (s SectionPos) Origin() BlockPos {
	return BlockPos{s.X << 4, s.Y << 4, s.Z << 4}
}
func (s SectionPos) Offset(f Face) SectionPos {
	n := f.Normal()
	return SectionPos{s.X + n.X, s.Y + n.Y, s.Z + n.Z}
}

// SectionIndex is the y<<8|z<<4|x index used by sections and the Anvil format.
func SectionIndex(x, y, z int) int { return (y&15)<<8 | (z&15)<<4 | (x & 15) }

// Face is a cube face / axis direction in Minecraft's Direction order.
type Face uint8

const (
	FaceDown Face = iota
	FaceUp
	FaceNorth // -Z
	FaceSouth // +Z
	FaceWest  // -X
	FaceEast  // +X
	FaceCount = 6
)

var faceNormals = [6]BlockPos{{0, -1, 0}, {0, 1, 0}, {0, 0, -1}, {0, 0, 1}, {-1, 0, 0}, {1, 0, 0}}
var faceNames = [6]string{"down", "up", "north", "south", "west", "east"}

func (f Face) Normal() BlockPos { return faceNormals[f] }
func (f Face) Opposite() Face   { return f ^ 1 }

// Off is the face's normal as an offset.
func (f Face) Off() [3]int    { n := faceNormals[f]; return [3]int{n.X, n.Y, n.Z} }
func (f Face) String() string { return faceNames[f] }

// Axis returns 0 for X, 1 for Y, 2 for Z.
func (f Face) Axis() int {
	switch f {
	case FaceWest, FaceEast:
		return 0
	case FaceDown, FaceUp:
		return 1
	}
	return 2
}

// Positive reports whether the face points along +axis.
func (f Face) Positive() bool { return f == FaceUp || f == FaceSouth || f == FaceEast }

// FaceByName parses "down", "up", ... (also accepts "bottom"/"top").
func FaceByName(s string) (Face, bool) {
	for i, n := range faceNames {
		if n == s {
			return Face(i), true
		}
	}
	switch s {
	case "bottom":
		return FaceDown, true
	case "top":
		return FaceUp, true
	}
	return 0, false
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}
