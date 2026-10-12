package anvil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const sectorSize = 4096

// Region is one r.X.Z.mca file: 32x32 chunks, a sector table and timestamps.
type Region struct {
	mu      sync.Mutex
	f       *os.File
	offsets [1024]uint32 // sector offset << 8 | sector count
	stamps  [1024]uint32
	used    []bool // sector allocation map
}

// ErrNoChunk is returned when a region slot is empty.
var ErrNoChunk = errors.New("anvil: chunk not present")

// OpenRegion opens (creating if create is set) a region file.
func OpenRegion(path string, create bool) (*Region, error) {
	flag := os.O_RDWR
	if create {
		flag |= os.O_CREATE
	}
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		// fall back to read-only (e.g. a world on a read-only volume)
		f, err = os.Open(path)
		if err != nil {
			return nil, err
		}
	}
	r := &Region{f: f}
	st, _ := f.Stat()
	if st.Size() < 2*sectorSize {
		if create {
			if _, err := f.WriteAt(make([]byte, 2*sectorSize), 0); err != nil {
				f.Close()
				return nil, err
			}
		} else if st.Size() == 0 {
			return r, nil
		}
	}
	var hdr [2 * sectorSize]byte
	if _, err := f.ReadAt(hdr[:], 0); err != nil && !errors.Is(err, io.EOF) {
		f.Close()
		return nil, err
	}
	for i := 0; i < 1024; i++ {
		r.offsets[i] = binary.BigEndian.Uint32(hdr[i*4:])
		r.stamps[i] = binary.BigEndian.Uint32(hdr[sectorSize+i*4:])
	}
	st, _ = f.Stat()
	r.used = make([]bool, (st.Size()+sectorSize-1)/sectorSize)
	r.used[0], r.used[1] = true, true
	for _, o := range r.offsets {
		off, n := int(o>>8), int(o&0xff)
		for s := off; s < off+n && s < len(r.used); s++ {
			r.used[s] = true
		}
	}
	return r, nil
}

func (r *Region) Close() error { return r.f.Close() }

// Has reports whether a chunk (region-local 0..31) is stored.
func (r *Region) Has(x, z int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.offsets[(z&31)<<5|(x&31)] != 0
}

// ReadChunk returns the decoded NBT of a chunk at region-local x,z.
func (r *Region) ReadChunk(x, z int) (Compound, error) {
	r.mu.Lock()
	o := r.offsets[(z&31)<<5|(x&31)]
	if o == 0 {
		r.mu.Unlock()
		return nil, ErrNoChunk
	}
	off, n := int64(o>>8)*sectorSize, int(o&0xff)*sectorSize
	buf := make([]byte, n)
	_, err := r.f.ReadAt(buf, off)
	r.mu.Unlock()
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(buf) < 5 {
		return nil, errors.New("anvil: short chunk")
	}
	length := int(binary.BigEndian.Uint32(buf))
	if length < 1 || length+4 > len(buf) {
		return nil, fmt.Errorf("anvil: bad chunk length %d", length)
	}
	comp := buf[4]
	data := bytes.NewReader(buf[5 : 4+length])
	var rd io.Reader
	switch comp {
	case 1:
		gz, err := gzip.NewReader(data)
		if err != nil {
			return nil, err
		}
		rd = gz
	case 2:
		zr, err := zlib.NewReader(data)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		rd = zr
	case 3:
		rd = data
	default:
		return nil, fmt.Errorf("anvil: unsupported compression %d (external/LZ4 chunks are not supported)", comp)
	}
	c, _, err := ReadNBT(rd)
	return c, err
}

// WriteChunk stores a chunk (zlib), reusing its sectors when it still fits.
func (r *Region) WriteChunk(x, z int, c Compound) error {
	var body bytes.Buffer
	zw := zlib.NewWriter(&body)
	if err := WriteNBT(zw, "", c); err != nil {
		return err
	}
	zw.Close()
	payload := make([]byte, 5+body.Len())
	binary.BigEndian.PutUint32(payload, uint32(body.Len()+1))
	payload[4] = 2
	copy(payload[5:], body.Bytes())
	need := (len(payload) + sectorSize - 1) / sectorSize
	if need > 255 {
		return errors.New("anvil: chunk too large for region sector table")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	idx := (z&31)<<5 | (x & 31)
	o := r.offsets[idx]
	off, n := int(o>>8), int(o&0xff)
	for s := off; s < off+n && s < len(r.used); s++ {
		r.used[s] = false
	}
	if o == 0 || need > n {
		off = r.alloc(need)
	}
	for s := off; s < off+need; s++ {
		for s >= len(r.used) {
			r.used = append(r.used, false)
		}
		r.used[s] = true
	}
	padded := make([]byte, need*sectorSize)
	copy(padded, payload)
	if _, err := r.f.WriteAt(padded, int64(off)*sectorSize); err != nil {
		return err
	}
	r.offsets[idx] = uint32(off)<<8 | uint32(need)
	r.stamps[idx] = uint32(time.Now().Unix())
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], r.offsets[idx])
	if _, err := r.f.WriteAt(hdr[:], int64(idx*4)); err != nil {
		return err
	}
	binary.BigEndian.PutUint32(hdr[:], r.stamps[idx])
	_, err := r.f.WriteAt(hdr[:], int64(sectorSize+idx*4))
	return err
}

func (r *Region) alloc(n int) int {
	run := 0
	for s := 2; s < len(r.used); s++ {
		if r.used[s] {
			run = 0
			continue
		}
		run++
		if run == n {
			return s - n + 1
		}
	}
	return len(r.used) - run
}

// Store gives chunk-level access to a world's region directory, keeping
// region files open.
type Store struct {
	Dir     string
	mu      sync.Mutex
	regions map[[2]int]*Region
	write   bool
}

// OpenStore opens <worldDir>/region (or worldDir itself if it already
// contains .mca files). write enables saving.
func OpenStore(worldDir string, write bool) *Store {
	dir := filepath.Join(worldDir, "region")
	if m, _ := filepath.Glob(filepath.Join(worldDir, "*.mca")); len(m) > 0 {
		dir = worldDir
	}
	if write {
		os.MkdirAll(dir, 0o755)
	}
	return &Store{Dir: dir, regions: map[[2]int]*Region{}, write: write}
}

func (s *Store) region(cx, cz int, create bool) (*Region, error) {
	key := [2]int{cx >> 5, cz >> 5}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.regions[key]; ok && r != nil {
		return r, nil
	}
	path := filepath.Join(s.Dir, fmt.Sprintf("r.%d.%d.mca", key[0], key[1]))
	if !create {
		if _, err := os.Stat(path); err != nil {
			return nil, ErrNoChunk
		}
	}
	r, err := OpenRegion(path, create)
	if err != nil {
		return nil, err
	}
	s.regions[key] = r
	return r, nil
}

// Read returns the NBT of chunk cx,cz or ErrNoChunk.
func (s *Store) Read(cx, cz int) (Compound, error) {
	r, err := s.region(cx, cz, false)
	if err != nil {
		return nil, err
	}
	return r.ReadChunk(cx&31, cz&31)
}

// Write stores the NBT of chunk cx,cz.
func (s *Store) Write(cx, cz int, c Compound) error {
	if !s.write {
		return errors.New("anvil: store is read-only")
	}
	r, err := s.region(cx, cz, true)
	if err != nil {
		return err
	}
	return r.WriteChunk(cx&31, cz&31, c)
}

// Close closes all open region files.
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, r := range s.regions {
		r.Close()
		delete(s.regions, k)
	}
}
