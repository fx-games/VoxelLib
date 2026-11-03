package bedrock

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// World is a Bedrock world directory (level.dat and db/).
type World struct {
	Name           string
	Seed           int64
	SpawnX, SpawnY int
	SpawnZ         int
	Level          map[string]any // level.dat
	DB             *DB
}

// Open reads a world directory.
func Open(dir string) (*World, error) {
	b, err := os.ReadFile(filepath.Join(dir, "level.dat"))
	if err != nil {
		return nil, err
	}
	if len(b) < 8 {
		return nil, errors.New("bedrock: short level.dat")
	}
	r := nbtReader{p: b[8:]} // version, length
	_, lvl := r.root()
	if r.err != nil {
		return nil, fmt.Errorf("bedrock: level.dat: %w", r.err)
	}
	db, err := OpenDB(filepath.Join(dir, "db"))
	if err != nil {
		return nil, err
	}
	w := &World{Name: Str(lvl, "LevelName"), Seed: Int(lvl, "RandomSeed"), Level: lvl, DB: db,
		SpawnX: int(Int(lvl, "SpawnX")), SpawnY: int(Int(lvl, "SpawnY")), SpawnZ: int(Int(lvl, "SpawnZ"))}
	if n, err := os.ReadFile(filepath.Join(dir, "levelname.txt")); err == nil && w.Name == "" {
		w.Name = strings.TrimSpace(string(n))
	}
	return w, nil
}

// Chunk key tags.
const (
	tagSubChunk    = 0x2f
	tagBlockEntity = 0x31
)

// SubChunkPos is a 16³ section: chunk x, z, section y (blocks y*16..),
// dimension (0 overworld, 1 nether, 2 end).
type SubChunkPos struct{ X, Z, Y, Dim int }

// SubChunks lists the world's sections, sorted.
func (w *World) SubChunks() []SubChunkPos {
	var out []SubChunkPos
	w.DB.Each(func(k, _ []byte) {
		var p SubChunkPos
		switch len(k) {
		case 10:
		case 14:
			p.Dim = int(int32(binary.LittleEndian.Uint32(k[8:])))
		default:
			return
		}
		if k[len(k)-2] != tagSubChunk {
			return
		}
		p.X, p.Z = int(int32(binary.LittleEndian.Uint32(k))), int(int32(binary.LittleEndian.Uint32(k[4:])))
		p.Y = int(int8(k[len(k)-1]))
		out = append(out, p)
	})
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Dim != b.Dim {
			return a.Dim < b.Dim
		}
		if a.X != b.X {
			return a.X < b.X
		}
		if a.Z != b.Z {
			return a.Z < b.Z
		}
		return a.Y < b.Y
	})
	return out
}

// BlockState is a palette entry: the block's name ("minecraft:stone") and
// its states.
type BlockState struct {
	Name   string
	States map[string]any
}

// Key is the state as "name[k=v,...]" with the states sorted.
func (s BlockState) Key() string {
	if len(s.States) == 0 {
		return s.Name
	}
	ks := make([]string, 0, len(s.States))
	for k := range s.States {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	var b strings.Builder
	b.WriteString(s.Name)
	b.WriteByte('[')
	for i, k := range ks {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s=%v", k, s.States[k])
	}
	b.WriteByte(']')
	return b.String()
}

// Storage is one block layer of a section: the palette index of each
// block at (x<<8)|(z<<4)|y.
type Storage struct {
	Index   [4096]uint16
	Palette []BlockState
}

// At is the block at x, y, z of the section.
func (s *Storage) At(x, y, z int) BlockState { return s.Palette[s.Index[x<<8|z<<4|y]] }

// SubChunk reads a section's layers (the second, where present, holds
// water in waterlogged blocks).
func (w *World) SubChunk(p SubChunkPos) ([]*Storage, error) {
	k := binary.LittleEndian.AppendUint32(nil, uint32(int32(p.X)))
	k = binary.LittleEndian.AppendUint32(k, uint32(int32(p.Z)))
	if p.Dim != 0 {
		k = binary.LittleEndian.AppendUint32(k, uint32(int32(p.Dim)))
	}
	k = append(k, tagSubChunk, byte(int8(p.Y)))
	v := w.DB.Get(k)
	if v == nil {
		return nil, nil
	}
	return decodeSubChunk(v)
}

func decodeSubChunk(b []byte) ([]*Storage, error) {
	if len(b) < 1 {
		return nil, errors.New("bedrock: empty sub-chunk")
	}
	r := &nbtReader{p: b[1:]}
	n := 1
	switch b[0] {
	case 1:
	case 8:
		n = int(r.u8())
	case 9:
		n = int(r.u8())
		r.u8() // the section's y
	default:
		return nil, fmt.Errorf("bedrock: sub-chunk version %d", b[0])
	}
	var out []*Storage
	for range n {
		s, err := readStorage(r)
		if err != nil {
			return out, err
		}
		out = append(out, s)
	}
	return out, r.err
}

func readStorage(r *nbtReader) (*Storage, error) {
	h := r.u8()
	bits := int(h >> 1)
	s := &Storage{}
	if bits > 0 {
		per := 32 / bits
		words := (4096 + per - 1) / per
		data := r.take(4 * words)
		mask := uint32(1)<<bits - 1
		for i := range 4096 {
			wd := binary.LittleEndian.Uint32(data[4*(i/per):])
			s.Index[i] = uint16(wd >> (uint(i%per) * uint(bits)) & mask)
		}
	}
	np := 1
	if bits > 0 {
		np = int(r.i32())
	}
	if np <= 0 || np > 4096 {
		return nil, fmt.Errorf("bedrock: palette of %d", np)
	}
	for range np {
		_, c := r.root()
		if r.err != nil {
			return nil, r.err
		}
		st, _ := c["states"].(map[string]any)
		s.Palette = append(s.Palette, BlockState{Name: Str(c, "name"), States: st})
	}
	for _, i := range s.Index {
		if int(i) >= len(s.Palette) {
			return nil, errors.New("bedrock: index past the palette")
		}
	}
	return s, nil
}

// BlockEntities reads the block entities of a dimension (chests, beds,
// banners, flower pots, ...) by position.
func (w *World) BlockEntities(dim int) map[[3]int]map[string]any {
	out := map[[3]int]map[string]any{}
	w.DB.Each(func(k, v []byte) {
		d := 0
		switch len(k) {
		case 9:
		case 13:
			d = int(int32(binary.LittleEndian.Uint32(k[8:])))
		default:
			return
		}
		if k[len(k)-1] != tagBlockEntity || d != dim {
			return
		}
		r := &nbtReader{p: v}
		for len(r.p) > 0 && r.err == nil {
			_, c := r.root()
			if c != nil {
				out[[3]int{int(Int(c, "x")), int(Int(c, "y")), int(Int(c, "z"))}] = c
			}
		}
	})
	return out
}
