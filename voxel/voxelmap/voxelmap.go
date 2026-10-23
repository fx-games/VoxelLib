// Package voxelmap reads a VoxelMap: a directory holding a converted
// pre-1.13 map, with its unchanged Anvil regions, so a game can
// stream it through voxel/engine without the original game.
//
//	map.json        {"name", "format", "seed", "spawn": {"dim","x","y","z"},
//	                 "dimensions": [{"id", "name", "regions", "generator"}]}
//	ids.json        {"<numeric id>": "<registry name>"} (blocks only)
//	blocks.json     [{"key", "shape", "textures", "layer", "opacity", "emission",
//	                  "collision", "tint", ...}], see Block
//	models/**.json  Java JSON block models
//	textures/**.png block textures (+ .png.mcmeta for animations)
//	<regions>/      r.X.Z.mca of a dimension, legacy numeric ids
//
// A block state is named by its block key "<registry name>#<meta>", e.g.
// "stone#0" in the default namespace; a blocks.json key "<name>#*" covers every meta not
// listed on its own. Each key becomes a voxel.BlockDef of that name, so
// Registry.StateString of a map block is its key.
//
// Resource ids "ns:path" (textures, models) live at textures/ns/path.png and
// models/ns/path.json; ids without a namespace, and default-namespace ids, may
// also sit directly at textures/path.png.
package voxelmap

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/memmaker/voxellib/resloc"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/anvil"
	"github.com/memmaker/voxellib/voxel/respack"
)

// MinY and Height are the build limits of a pre-1.13 world: worlds that
// stream a map must use them.
const MinY, Height = 0, 256

// Air is the block key of air.
const Air = resloc.Default + ":air#0"

// Dimension is one entry of map.json's dimensions.
type Dimension struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Regions   string `json:"regions"`   // region folder, relative to the map
	Generator string `json:"generator"` // name a game registered with worldgen.Register
}

// Map is an opened VoxelMap bound to one registry.
type Map struct {
	Dir        string
	Name       string      `json:"name"`
	Format     int         `json:"format"`
	Seed       int64       `json:"seed"`
	Spawn      Spawn       `json:"spawn"`
	Dimensions []Dimension `json:"dimensions"`
	// IDs maps numeric block ids to registry names (ids.json).
	IDs map[uint16]string `json:"-"`
	// Blocks holds blocks.json by key, including the raw entry for fields
	// the engine doesn't use (sounds, flags, ...).
	Blocks map[string]*Block `json:"-"`

	reg    *voxel.Registry
	pack   *respack.Pack
	cache  [4096 * 16]atomic.Uint32 // id<<4|meta → StateID+1
	mu     sync.Mutex
	stores map[int]*anvil.Store
}

// Spawn is the map's starting point.
type Spawn struct {
	Dim     int `json:"dim"`
	X, Y, Z int
}

// Open reads a VoxelMap and registers a block definition for every key of
// blocks.json in reg.
func Open(dir string, reg *voxel.Registry) (*Map, error) {
	m := &Map{Dir: dir, reg: reg, IDs: map[uint16]string{}, Blocks: map[string]*Block{}, stores: map[int]*anvil.Store{}}
	if err := readJSON(filepath.Join(dir, "map.json"), m); err != nil {
		return nil, err
	}
	var ids map[string]string
	if err := readJSON(filepath.Join(dir, "ids.json"), &ids); err != nil {
		return nil, err
	}
	for k, name := range ids {
		n, err := strconv.Atoi(k)
		if err != nil || n < 0 || n >= 4096 {
			return nil, fmt.Errorf("voxelmap: ids.json: bad id %q", k)
		}
		m.IDs[uint16(n)] = name
	}
	var raws []json.RawMessage
	if err := readJSON(filepath.Join(dir, "blocks.json"), &raws); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, raw := range raws {
		b := &Block{Raw: raw}
		if err := json.Unmarshal(raw, b); err != nil {
			return nil, fmt.Errorf("voxelmap: blocks.json: %w", err)
		}
		m.Blocks[b.Key] = b
	}
	m.pack = respack.FromFS(resFS{os.DirFS(dir)})
	m.register()
	return m, nil
}

func readJSON(file string, v any) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("voxelmap: %s: %w", filepath.Base(file), err)
	}
	return nil
}

// register defines every key; "#*" entries for the metas without their own.
func (m *Map) register() {
	for key, b := range m.Blocks {
		name, meta, _ := strings.Cut(key, "#")
		meta, _, _ = strings.Cut(meta, "|") // a tile-entity variant (Block.TE)
		if meta != "*" {
			n, _ := strconv.Atoi(meta)
			m.reg.Register(m.def(key, b, n))
			continue
		}
		for n := 0; n < 16; n++ {
			k := name + "#" + strconv.Itoa(n)
			if _, ok := m.Blocks[k]; !ok {
				m.reg.Register(m.def(k, b, n))
			}
		}
	}
}

// Dimension returns the dimension with id, or nil.
func (m *Map) Dimension(id int) *Dimension {
	for i := range m.Dimensions {
		if m.Dimensions[i].ID == id {
			return &m.Dimensions[i]
		}
	}
	return nil
}

// State returns the state of a block key ("stone#0" in the default namespace). Air and
// keys of unknown blocks work too (the latter as missing-texture cubes).
func (m *Map) State(key string) voxel.StateID {
	if strings.HasPrefix(key, resloc.Default+":air#") {
		return voxel.Air
	}
	return m.reg.State(key, nil)
}

// Key returns the block key of a state ("" for states not from this map).
func (m *Map) Key(id voxel.StateID) string { return m.reg.StateString(id) }

// Legacy resolves a numeric id and meta through ids.json; it is the
// anvil.LegacyIDs of the map's regions.
func (m *Map) Legacy(id uint16, meta uint8) voxel.StateID {
	i := int(id&4095)<<4 | int(meta&15)
	if v := m.cache[i].Load(); v != 0 {
		return voxel.StateID(v - 1)
	}
	name, ok := m.IDs[id]
	if !ok {
		name = "voxelmap:id" + strconv.Itoa(int(id))
	}
	s := m.State(name + "#" + strconv.Itoa(int(meta)))
	m.cache[i].Store(uint32(s) + 1)
	return s
}

// ReadChunk loads chunk p of a dimension from the map's regions. It returns
// the chunk shaped for w (MinY 0, Height 256) and the chunk's raw "Level"
// NBT: Biomes, Entities, TileEntities and the rest, for the game. It
// returns anvil.ErrNoChunk when the map doesn't have the chunk.
func (m *Map) ReadChunk(w *voxel.World, dim int, p voxel.ChunkPos) (*voxel.Chunk, anvil.Compound, error) {
	st, err := m.store(dim)
	if err != nil {
		return nil, nil, err
	}
	root, err := st.Read(p.X, p.Z)
	if err != nil {
		return nil, nil, err
	}
	c, err := anvil.DecodeChunkIDs(w, root, m.Legacy)
	if err != nil {
		return nil, nil, err
	}
	c.Pos = p
	level := root.Compound("Level")
	if level == nil {
		level = root
	}
	m.teVariants(c, level)
	return c, level, nil
}

// teVariants swaps each block at a tile entity whose entry lists te fields
// for the variant those fields' values name, when blocks.json defines it.
func (m *Map) teVariants(c *voxel.Chunk, level anvil.Compound) {
	for _, it := range level.List("TileEntities").Items {
		t, _ := it.(anvil.Compound)
		x, y, z := t.Int("x")-c.Pos.X*16, t.Int("y"), t.Int("z")-c.Pos.Z*16
		if x < 0 || x > 15 || z < 0 || z > 15 {
			continue
		}
		key := m.Key(c.Get(x, y, z))
		b := m.Blocks[key]
		if b == nil || len(b.TE) == 0 {
			continue
		}
		for i, f := range b.TE {
			key += string("|,"[min(i, 1)]) + strconv.Itoa(t.Int(f))
		}
		if _, ok := m.Blocks[key]; ok {
			c.SetRaw(x, y, z, m.State(key))
		}
	}
}

// BaseKey strips a tile-entity variant ("|...", see Block.TE) from a key.
func BaseKey(key string) string { k, _, _ := strings.Cut(key, "|"); return k }

func (m *Map) store(dim int) (*anvil.Store, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.stores[dim]; ok {
		return s, nil
	}
	d := m.Dimension(dim)
	if d == nil {
		return nil, fmt.Errorf("voxelmap: no dimension %d", dim)
	}
	s := anvil.OpenStore(filepath.Join(m.Dir, d.Regions), false)
	m.stores[dim] = s
	return s, nil
}

// Close closes the region files.
func (m *Map) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, s := range m.stores {
		s.Close()
		delete(m.stores, k)
	}
}

// Texture implements render.TextureSource for the map's textures.
func (m *Map) Texture(name string) (image.Image, error) { return m.pack.Texture(name) }

// TextureFrames implements render.AnimatedTextureSource.
func (m *Map) TextureFrames(name string) ([]image.Image, int, error) {
	return m.pack.TextureFrames(name)
}

// Pack returns the map's models and textures as a resource pack.
func (m *Map) Pack() *respack.Pack { return m.pack }

// resFS maps the resource-pack paths mcres asks for
// (assets/<ns>/<kind>/<rest>) onto the map's layout (<kind>/<ns>/<rest>,
// else <kind>/<rest>).
type resFS struct{ fs.FS }

func (r resFS) Open(name string) (fs.File, error) {
	parts := strings.SplitN(name, "/", 4)
	if len(parts) != 4 || parts[0] != "assets" {
		return nil, fs.ErrNotExist
	}
	ns, kind, rest := parts[1], parts[2], parts[3]
	if f, err := r.FS.Open(path.Join(kind, ns, rest)); err == nil {
		return f, nil
	}
	return r.FS.Open(path.Join(kind, rest))
}

func logf(format string, args ...any) { log.Printf("voxelmap: "+format, args...) }
