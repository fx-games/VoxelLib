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
	// freed holds sectors of replaced chunks: the header pointing at them
	// may not be on disk yet, so they are reused only after the next fsync.
	freed [][2]int // offset, count
}

// writeFault, when set by a test, fails a chunk write at a step ("data":
// after the chunk's sectors are written, before its header entry).
var writeFault func(step string) error

// ErrNoChunk is returned when a region slot is empty.
var ErrNoChunk = errors.New("anvil: chunk not present")

// OpenRegion opens a region file: read-only, or for writing (creating it if
// needed) when create is set.
func OpenRegion(path string, create bool) (*Region, error) {
	flag := os.O_RDONLY // never write to a world only read from
	if create {
		flag = os.O_RDWR | os.O_CREATE
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
	return r.WriteChunks(map[[2]int]Compound{{x, z}: c})
}

// WriteChunks stores several chunks (keyed by x, z within the region) behind
// a single fsync.
func (r *Region) WriteChunks(cs map[[2]int]Compound) error {
	type put struct{ idx, off, need int }
	payloads := make(map[[2]int][]byte, len(cs))
	for k, c := range cs {
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
		if (len(payload)+sectorSize-1)/sectorSize > 255 {
			return errors.New("anvil: chunk too large for region sector table")
		}
		payloads[k] = payload
	}

	// Crash safety: a chunk is never overwritten in place. Its new copy
	// goes to free sectors, is fsynced, and only then does the 4-byte
	// header entry (one sector, written atomically) switch to it; the old
	// sectors are reused only after a later fsync made that switch durable.
	// A crash at any point leaves the old or the new chunk, never a torn
	// one. A batch shares one fsync.
	r.mu.Lock()
	defer r.mu.Unlock()
	var puts []put
	undo := func() {
		for _, p := range puts {
			r.release([2]int{p.off, p.need})
		}
	}
	for k, payload := range payloads {
		need := (len(payload) + sectorSize - 1) / sectorSize
		off := r.alloc(need)
		for s := off; s < off+need; s++ {
			for s >= len(r.used) {
				r.used = append(r.used, false)
			}
			r.used[s] = true
		}
		puts = append(puts, put{(k[1]&31)<<5 | (k[0] & 31), off, need})
		padded := make([]byte, need*sectorSize)
		copy(padded, payload)
		if _, err := r.f.WriteAt(padded, int64(off)*sectorSize); err != nil {
			undo()
			return err
		}
	}
	if writeFault != nil {
		if err := writeFault("data"); err != nil {
			return err
		}
	}
	if err := r.f.Sync(); err != nil {
		undo()
		return err
	}
	// Everything written before this fsync, the headers that dropped the
	// freed sectors included, is durable now.
	for _, fr := range r.freed {
		r.release(fr)
	}
	r.freed = r.freed[:0]
	now := uint32(time.Now().Unix())
	for _, p := range puts {
		if o := r.offsets[p.idx]; o != 0 {
			r.freed = append(r.freed, [2]int{int(o >> 8), int(o & 0xff)})
		}
		r.offsets[p.idx] = uint32(p.off)<<8 | uint32(p.need)
		r.stamps[p.idx] = now
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], r.offsets[p.idx])
		if _, err := r.f.WriteAt(hdr[:], int64(p.idx*4)); err != nil {
			return err
		}
		binary.BigEndian.PutUint32(hdr[:], r.stamps[p.idx])
		if _, err := r.f.WriteAt(hdr[:], int64(sectorSize+p.idx*4)); err != nil {
			return err
		}
	}
	return nil
}

func (r *Region) release(fr [2]int) {
	for s := fr[0]; s < fr[0]+fr[1] && s < len(r.used); s++ {
		if s >= 2 {
			r.used[s] = false
		}
	}
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

// maxOpen bounds the region files a Store keeps open; the least recently
// used idle one is closed beyond it (macOS allows 256 open files per
// process by default, and a long journey crosses many regions).
const maxOpen = 64

// Store gives chunk-level access to a world's region directory, keeping up
// to maxOpen region files open. It is safe for concurrent use.
type Store struct {
	Dir     string
	mu      sync.Mutex
	regions map[[2]int]*openRegion
	clock   uint64
	write   bool
}

// openRegion is a cached region file. refs counts the readers and writers
// using it right now: only an idle one (refs 0) is ever closed.
type openRegion struct {
	r     *Region
	refs  int
	used  uint64 // Store.clock at the last use
	dirty bool   // written since the last fsync
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
	return &Store{Dir: dir, regions: map[[2]int]*openRegion{}, write: write}
}

// acquire returns the open region of chunk cx,cz; release it when done.
func (s *Store) acquire(cx, cz int, create bool) (*openRegion, error) {
	key := [2]int{cx >> 5, cz >> 5}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clock++
	if o, ok := s.regions[key]; ok {
		o.refs++
		o.used = s.clock
		return o, nil
	}
	path := filepath.Join(s.Dir, fmt.Sprintf("r.%d.%d.mca", key[0], key[1]))
	if !create {
		if _, err := os.Stat(path); err != nil {
			return nil, ErrNoChunk
		}
	}
	// ponytail: opening (a header read) happens under the store lock; fine
	// at 64 files, move it out if region opens ever show in profiles.
	r, err := OpenRegion(path, s.write)
	if err != nil {
		return nil, err
	}
	o := &openRegion{r: r, refs: 1, used: s.clock}
	s.regions[key] = o
	s.evict()
	return o, nil
}

func (s *Store) release(o *openRegion) {
	s.mu.Lock()
	o.refs--
	s.evict()
	s.mu.Unlock()
}

// evict closes least recently used idle regions while more than maxOpen are
// open (busy ones may keep it above maxOpen until released). Caller holds mu.
func (s *Store) evict() {
	// ponytail: linear LRU scan, fine for 64 entries.
	for len(s.regions) > maxOpen {
		var lru [2]int
		var old *openRegion
		for k, o := range s.regions {
			if o.refs == 0 && (old == nil || o.used < old.used) {
				lru, old = k, o
			}
		}
		if old == nil {
			return
		}
		if old.dirty {
			old.r.f.Sync() // see Sync: written data must stay durable
		}
		old.r.Close()
		delete(s.regions, lru)
	}
}

// Read returns the NBT of chunk cx,cz or ErrNoChunk.
func (s *Store) Read(cx, cz int) (Compound, error) {
	o, err := s.acquire(cx, cz, false)
	if err != nil {
		return nil, err
	}
	defer s.release(o)
	return o.r.ReadChunk(cx&31, cz&31)
}

// Write stores the NBT of chunk cx,cz.
func (s *Store) Write(cx, cz int, c Compound) error {
	if !s.write {
		return errors.New("anvil: store is read-only")
	}
	o, err := s.acquire(cx, cz, true)
	if err != nil {
		return err
	}
	defer s.release(o)
	s.mu.Lock()
	o.dirty = true
	s.mu.Unlock()
	return o.r.WriteChunk(cx&31, cz&31, c)
}

// WriteAll stores many chunks, keyed by chunk x, z, with one fsync per
// region file instead of one per chunk.
func (s *Store) WriteAll(cs map[[2]int]Compound) error {
	if !s.write {
		return errors.New("anvil: store is read-only")
	}
	byRegion := map[[2]int]map[[2]int]Compound{}
	for k, c := range cs {
		rk := [2]int{k[0] >> 5, k[1] >> 5}
		if byRegion[rk] == nil {
			byRegion[rk] = map[[2]int]Compound{}
		}
		byRegion[rk][[2]int{k[0] & 31, k[1] & 31}] = c
	}
	for rk, batch := range byRegion {
		o, err := s.acquire(rk[0]<<5, rk[1]<<5, true)
		if err != nil {
			return err
		}
		s.mu.Lock()
		o.dirty = true
		s.mu.Unlock()
		err = o.r.WriteChunks(batch)
		s.release(o)
		if err != nil {
			return err
		}
	}
	return nil
}

// Sync flushes every write so far to stable storage (fsync), including
// writes to region files closed since.
func (s *Store) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.regions {
		if o.dirty {
			if err := o.r.f.Sync(); err != nil {
				return err
			}
			o.dirty = o.refs > 0 // a writer in progress may still write

		}
	}
	return nil
}

// Close closes all open region files.
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, o := range s.regions {
		if o.dirty {
			o.r.f.Sync()
		}
		o.r.Close()
		delete(s.regions, k)
	}
}
