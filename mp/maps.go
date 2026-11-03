package mp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/anvil"
	"github.com/memmaker/voxellib/voxel/voxelmap"
	"github.com/memmaker/voxellib/worldgen"
)

// MapsDir holds the VoxelMaps a game can be played on: generator
// "map:<name>" streams MapsDir/<name>. Every player needs the same map.
var MapsDir = "maps"

const mapPrefix = "map:"

// MapName is the map a generator name streams ("" for a terrain generator).
func MapName(gen string) string {
	if n, ok := strings.CutPrefix(gen, mapPrefix); ok {
		return n
	}
	return ""
}

// MapInfo is a playable map: its generator name ("map:minecity") and title.
type MapInfo struct{ Gen, Title string }

// Maps lists the VoxelMaps in MapsDir, by title.
func Maps() []MapInfo {
	ents, _ := os.ReadDir(MapsDir)
	var out []MapInfo
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(MapsDir, e.Name(), "map.json"))
		if err != nil {
			continue
		}
		var m struct{ Name string }
		json.Unmarshal(b, &m)
		if m.Name == "" {
			m.Name = e.Name()
		}
		out = append(out, MapInfo{Gen: mapPrefix + e.Name(), Title: m.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

// Heights are the build limits of a generator's world: a map's are the
// format's (0, 256).
func Heights(gen string) (minY, height int) {
	if MapName(gen) != "" {
		return voxelmap.MinY, voxelmap.Height
	}
	return -64, 384
}

// Maps open once per registry (opening registers the map's blocks).
var openMaps = struct {
	sync.Mutex
	m map[*voxel.Registry]map[string]*voxelmap.Map
}{m: map[*voxel.Registry]map[string]*voxelmap.Map{}}

// OpenMap opens MapsDir/<name> into reg, or returns the map opened there
// before.
func OpenMap(reg *voxel.Registry, name string) (*voxelmap.Map, error) {
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return nil, fmt.Errorf("mp: bad map name %q", name)
	}
	openMaps.Lock()
	defer openMaps.Unlock()
	if m := openMaps.m[reg][name]; m != nil {
		return m, nil
	}
	m, err := voxelmap.Open(filepath.Join(MapsDir, name), reg)
	if err != nil {
		return nil, fmt.Errorf("mp: map %s: %w", name, err)
	}
	if openMaps.m[reg] == nil {
		openMaps.m[reg] = map[string]*voxelmap.Map{}
	}
	openMaps.m[reg][name] = m
	return m, nil
}

// MapGen streams a map's spawn dimension: its chunks where it has them,
// the dimension's own generator elsewhere. The world must have the map's
// heights.
type MapGen struct {
	Map  *voxelmap.Map
	dim  int
	fill worldgen.Generator
}

// NewMapGen makes the generator of a map.
func NewMapGen(m *voxelmap.Map) *MapGen {
	g := &MapGen{Map: m, dim: m.Spawn.Dim}
	if d := m.Dimension(g.dim); d != nil {
		if f := worldgen.Lookup(d.Generator); f != nil {
			g.fill = f(m.Seed, m.State)
		}
	}
	return g
}

func (g *MapGen) Generate(w *voxel.World, c *voxel.Chunk) {
	src, _, err := g.Map.ReadChunk(w, g.dim, c.Pos)
	if err != nil {
		if !errors.Is(err, anvil.ErrNoChunk) {
			log.Printf("mp: map chunk %v: %v", c.Pos, err)
		}
		if g.fill != nil {
			g.fill.Generate(w, c)
		}
		return
	}
	for i, s := range src.Sections {
		c.SetSection(i, s)
	}
	c.Height, c.Signs, c.Biomes = src.Height, src.Signs, src.Biomes
}

// FindLand is the map's spawn.
func (g *MapGen) FindLand(x, z int) (int, int) { return g.Map.Spawn.X, g.Map.Spawn.Z }
