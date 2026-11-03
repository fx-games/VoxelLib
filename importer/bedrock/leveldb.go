// Package bedrock reads Minecraft Bedrock Edition worlds: Mojang's LevelDB
// (tables with zlib or raw deflate blocks, plus the write-ahead log), the
// little-endian NBT, level.dat and the palette sub-chunks.
package bedrock

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DB is a LevelDB database read whole into memory: every live key and
// its newest value. Bedrock worlds are small enough.
type DB struct {
	kv map[string][]byte
}

type entry struct {
	seq  uint64
	del  bool
	data []byte
}

// OpenDB reads the db directory: every table (.ldb, .sst) and log (.log),
// the newest write of each key winning.
func OpenDB(dir string) (*DB, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	all := map[string]entry{}
	put := func(k []byte, e entry) {
		if o, ok := all[string(k)]; !ok || e.seq >= o.seq {
			all[string(k)] = e
		}
	}
	for _, f := range files {
		name := filepath.Join(dir, f.Name())
		switch {
		case strings.HasSuffix(f.Name(), ".ldb"), strings.HasSuffix(f.Name(), ".sst"):
			if err := readTable(name, put); err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name(), err)
			}
		case strings.HasSuffix(f.Name(), ".log"):
			if err := readLog(name, put); err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name(), err)
			}
		}
	}
	db := &DB{kv: map[string][]byte{}}
	for k, e := range all {
		if !e.del {
			db.kv[k] = e.data
		}
	}
	return db, nil
}

// Get returns a key's value, or nil.
func (db *DB) Get(key []byte) []byte { return db.kv[string(key)] }

// Each calls f for every key.
func (db *DB) Each(f func(key, value []byte)) {
	for k, v := range db.kv {
		f([]byte(k), v)
	}
}

// Len is the number of keys.
func (db *DB) Len() int { return len(db.kv) }

const tableMagic = 0xdb4775248b80fb57

// readTable reads an SSTable: the footer's index block lists the data
// blocks; each entry's internal key ends in seq<<8|type.
func readTable(name string, put func([]byte, entry)) error {
	b, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	if len(b) < 48 || binary.LittleEndian.Uint64(b[len(b)-8:]) != tableMagic {
		return errors.New("not a table")
	}
	foot := b[len(b)-48:]
	_, n := handle(foot) // metaindex
	idx, _ := handle(foot[n:])
	index, err := block(b, idx)
	if err != nil {
		return err
	}
	return entries(index, func(_, v []byte) error {
		h, _ := handle(v)
		data, err := block(b, h)
		if err != nil {
			return err
		}
		return entries(data, func(k, v []byte) error {
			if len(k) < 8 {
				return errors.New("short key")
			}
			tag := binary.LittleEndian.Uint64(k[len(k)-8:])
			put(k[:len(k)-8], entry{seq: tag >> 8, del: tag&0xFF == 0, data: append([]byte(nil), v...)})
			return nil
		})
	})
}

type blockHandle struct{ off, size uint64 }

func handle(p []byte) (blockHandle, int) {
	off, n1 := binary.Uvarint(p)
	size, n2 := binary.Uvarint(p[max(n1, 0):])
	return blockHandle{off, size}, n1 + n2
}

// block returns a block's contents, decompressed by its trailer's type:
// 0 none, 2 zlib, 4 raw deflate (Mojang's).
func block(b []byte, h blockHandle) ([]byte, error) {
	end := h.off + h.size
	if end+5 > uint64(len(b)) {
		return nil, errors.New("block out of range")
	}
	raw := b[h.off:end]
	switch b[end] {
	case 0:
		return raw, nil
	case 2:
		r, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(r)
	case 4:
		return io.ReadAll(flate.NewReader(bytes.NewReader(raw)))
	}
	return nil, fmt.Errorf("block compression %d", b[end])
}

// entries walks a block's prefix-compressed entries.
func entries(blk []byte, f func(k, v []byte) error) error {
	if len(blk) < 4 {
		return errors.New("short block")
	}
	nr := binary.LittleEndian.Uint32(blk[len(blk)-4:])
	end := len(blk) - 4 - 4*int(nr)
	if end < 0 {
		return errors.New("bad restarts")
	}
	var key []byte
	for p := 0; p < end; {
		shared, n1 := binary.Uvarint(blk[p:])
		unshared, n2 := binary.Uvarint(blk[p+n1:])
		vlen, n3 := binary.Uvarint(blk[p+n1+n2:])
		p += n1 + n2 + n3
		if n1 <= 0 || n2 <= 0 || n3 <= 0 || int(shared) > len(key) || p+int(unshared)+int(vlen) > end {
			return errors.New("bad entry")
		}
		key = append(key[:shared], blk[p:p+int(unshared)]...)
		p += int(unshared)
		if err := f(key, blk[p:p+int(vlen)]); err != nil {
			return err
		}
		p += int(vlen)
	}
	return nil
}

// readLog replays the write-ahead log: 32 KiB blocks of records (full, or
// first/middle/last pieces), each a write batch.
func readLog(name string, put func([]byte, entry)) error {
	b, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	const blockSize = 32 << 10
	var rec []byte
	for p := 0; p+7 <= len(b); {
		if left := blockSize - p%blockSize; left < 7 {
			p += left // trailer padding
			continue
		}
		n := int(binary.LittleEndian.Uint16(b[p+4:]))
		typ := b[p+6]
		if typ == 0 && n == 0 { // preallocated zeros: the end
			break
		}
		if p+7+n > len(b) {
			break // torn write
		}
		data := b[p+7 : p+7+n]
		p += 7 + n
		switch typ {
		case 1:
			rec = append(rec[:0], data...)
		case 2:
			rec = append(rec[:0], data...)
			continue
		case 3:
			rec = append(rec, data...)
			continue
		case 4:
			rec = append(rec, data...)
		default:
			continue
		}
		if err := batch(rec, put); err != nil {
			return err
		}
	}
	return nil
}

// batch applies a write batch: seq, count, then put/delete records.
func batch(rec []byte, put func([]byte, entry)) error {
	if len(rec) < 12 {
		return errors.New("short batch")
	}
	seq := binary.LittleEndian.Uint64(rec)
	count := binary.LittleEndian.Uint32(rec[8:])
	p := 12
	str := func() ([]byte, bool) {
		l, n := binary.Uvarint(rec[p:])
		if n <= 0 || p+n+int(l) > len(rec) {
			return nil, false
		}
		s := rec[p+n : p+n+int(l)]
		p += n + int(l)
		return s, true
	}
	for i := range count {
		if p >= len(rec) {
			return errors.New("short batch")
		}
		tag := rec[p]
		p++
		k, ok := str()
		if !ok {
			return errors.New("bad batch key")
		}
		e := entry{seq: seq + uint64(i), del: tag == 0}
		if tag == 1 {
			v, ok := str()
			if !ok {
				return errors.New("bad batch value")
			}
			e.data = append([]byte(nil), v...)
		}
		put(append([]byte(nil), k...), e)
	}
	return nil
}
