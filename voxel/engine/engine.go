// Package engine is the batteries-included facade: it streams chunks around a
// viewer (generate or load from Anvil), lights them on a background worker,
// meshes dirty sections on a worker pool nearest-first, uploads results within
// a per-frame budget, unloads and saves far chunks, and renders.
//
// A game only needs: engine.New, then per frame Update(x,y,z) + Render(cam),
// and SetBlock / Raycast for interaction.
package engine

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/anvil"
	"github.com/memmaker/voxellib/voxel/mesh"
	"github.com/memmaker/voxellib/voxel/physics"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxellib/voxel/voxelmap"
	"github.com/memmaker/voxellib/worldgen"
)

// Generator fills new chunks; see package worldgen for implementations
// (worldgen/overworld, worldgen/flat) and the rules generators must follow.
type Generator interface {
	Generate(w *voxel.World, c *voxel.Chunk)
}

// Config configures an Engine.
type Config struct {
	Registry       *voxel.Registry // nil: voxel.NewRegistry()
	MinY, Height   int             // default -64, 384
	RenderDistance int             // chunks, default 12
	Generator      Generator       // used for chunks not on disk (nil: void)
	WorldDir       string          // Minecraft world folder to load/save ("" = memory only)
	ReadOnly       bool            // never write to WorldDir or SaveDir
	// Map, when set, streams dimension Dimension of a VoxelMap instead of
	// WorldDir. Per chunk, the save slot's changes win over the map's
	// regions, which win over the generator. Without a Generator, the one
	// registered (worldgen.Register) under the dimension's "generator"
	// name is used, made with the map's seed. MinY and Height default to
	// the map's (0, 256); Textures defaults to the map's.
	Map       *voxelmap.Map
	Dimension int
	// SaveDir is the save slot of a Map world ("" = nothing is saved). It
	// holds only what the player changed (the edited sections of each
	// chunk) and the game's state blob (SaveGame); see NewGame.
	SaveDir string
	// OnMapChunk, when set, gets the raw NBT ("Level" compound) of every
	// chunk read from Map: Biomes, Entities, TileEntities and so on. It
	// runs on the load workers, concurrently, and again each time the
	// chunk streams back in.
	OnMapChunk func(p voxel.ChunkPos, level anvil.Compound)
	// Bounds, when set, limits the world to these chunks (inclusive
	// corners): nothing outside is loaded, generated or drawn, e.g. a
	// hand-built level.
	Bounds        *[2]voxel.ChunkPos
	Textures      render.TextureSource
	RenderOptions render.Options
	// Workers is the size of each background pool (mesh, generation).
	// Default (NumCPU-3)/2: with the light and save workers that leaves a
	// core (and a Go P) free for the render thread; oversubscribing shows up
	// as frame hitches. Workers also run at a lower OS priority (macOS).
	Workers      int
	UploadBudget time.Duration // GPU upload time per frame, default 4ms
}

// Engine owns world, workers and renderer.
type Engine struct {
	World    *voxel.World
	Renderer *render.Renderer
	cfg      Config
	store    *anvil.Store
	delta    bool // store holds Map deltas (only edited sections)
	// Map deltas of chunks that streamed out since the last SaveGame
	// (save.go). ponytail: kept in memory until saved, a few KB per edited
	// chunk; spill to a scratch store if journeys ever edit that much.
	unsaved   map[voxel.ChunkPos]*anvil.Compound
	unsavedMu sync.Mutex
	fresh     atomic.Bool // the slot holds no save: its regions are ignored
	slotMu    sync.Mutex  // one SaveGame at a time
	stageMu   sync.Mutex  // held while chunks move from the world to unsaved
	journal   *journal    // read-only engine: a save not yet applied to the slot, read through

	center   voxel.ChunkPos
	viewer   [3]float64
	started  bool
	loaded   map[voxel.ChunkPos]bool // present in the world
	pending  map[voxel.ChunkPos]bool // load/generate requested
	stale    map[voxel.ChunkPos]bool // pending from before Regenerate: dropped on arrival
	loadReq  chan voxel.ChunkPos
	loadRes  chan *voxel.Chunk
	worldOps chan worldOp // applied in order by the light worker
	litRes   chan voxel.ChunkPos
	lit      map[voxel.ChunkPos]bool // mirror of Chunk.Lit, so the main thread needn't lock
	saveReq  chan *voxel.Chunk

	meshJobs    chan voxel.SectionPos
	meshRes     chan *mesh.Mesh
	dirty       map[voxel.SectionPos]bool
	inFlight    map[voxel.SectionPos]bool
	redo        map[voxel.SectionPos]bool
	staleMesh   map[voxel.SectionPos]bool // in flight at Regenerate: not uploaded
	mainMesher  *mesh.Mesher
	cfgMu       sync.RWMutex
	meshCfg     *mesh.Config
	dispatchBuf []meshCand

	quit chan struct{}
	wg   sync.WaitGroup
	// Stats
	MeshedSections int
	LastUploadTime time.Duration
	// Timing holds the worst duration of each Update phase since the caller
	// last cleared it (see TimingNames). Diagnostics only.
	Timing [7]time.Duration
}

// New creates an engine. Must be called on the render thread after
// render.Init (the renderer is created here).
func New(cfg Config) (*Engine, error) {
	if cfg.Registry == nil {
		cfg.Registry = voxel.NewRegistry()
	}
	if m := cfg.Map; m != nil {
		d := m.Dimension(cfg.Dimension)
		if d == nil {
			return nil, fmt.Errorf("engine: map has no dimension %d", cfg.Dimension)
		}
		if cfg.Height == 0 {
			cfg.MinY, cfg.Height = voxelmap.MinY, voxelmap.Height
		}
		if cfg.Textures == nil {
			cfg.Textures = m
		}
		if cfg.Generator == nil {
			if f := worldgen.Lookup(d.Generator); f != nil {
				cfg.Generator = f(m.Seed, m.State)
			} else if d.Generator != "" {
				log.Printf("engine: no generator %q registered; chunks the map lacks stay empty", d.Generator)
			}
		}
	}
	if cfg.Height == 0 {
		cfg.MinY, cfg.Height = -64, 384
	}
	if cfg.RenderDistance == 0 {
		cfg.RenderDistance = 12
	}
	if cfg.Workers <= 0 {
		cfg.Workers = max(1, (runtime.NumCPU()-3)/2)
	}
	if cfg.UploadBudget == 0 {
		cfg.UploadBudget = 4 * time.Millisecond
	}
	renderThread()
	w := voxel.NewWorld(cfg.Registry, cfg.MinY, cfg.Height)
	ro := cfg.RenderOptions
	ro.RenderDistance = cfg.RenderDistance
	r, err := render.New(w, cfg.Textures, ro)
	if err != nil {
		return nil, err
	}
	e := &Engine{
		World: w, Renderer: r, cfg: cfg,
		loaded: map[voxel.ChunkPos]bool{}, pending: map[voxel.ChunkPos]bool{}, stale: map[voxel.ChunkPos]bool{},
		loadReq: make(chan voxel.ChunkPos, 4096), loadRes: make(chan *voxel.Chunk, 4096),
		worldOps: make(chan worldOp, 4096), litRes: make(chan voxel.ChunkPos, 1<<16),
		lit: map[voxel.ChunkPos]bool{}, saveReq: make(chan *voxel.Chunk, 1024),
		meshJobs: make(chan voxel.SectionPos, cfg.Workers*2), meshRes: make(chan *mesh.Mesh, 1024),
		dirty: map[voxel.SectionPos]bool{}, inFlight: map[voxel.SectionPos]bool{}, redo: map[voxel.SectionPos]bool{},
		staleMesh:  map[voxel.SectionPos]bool{},
		mainMesher: mesh.NewMesher(), meshCfg: r.MeshConfig(), quit: make(chan struct{}),
	}
	switch {
	case cfg.Map != nil:
		if cfg.SaveDir != "" {
			st, fresh, j, err := openSlot(cfg.SaveDir, cfg.Dimension, !cfg.ReadOnly)
			if err != nil {
				r.Destroy()
				return nil, err
			}
			e.store, e.delta, e.journal = st, true, j
			e.fresh.Store(fresh)
		}
	case cfg.WorldDir != "":
		e.store = anvil.OpenStore(cfg.WorldDir, !cfg.ReadOnly)
	}
	for i := 0; i < cfg.Workers; i++ {
		e.wg.Add(2)
		go e.loadWorker()
		go e.meshWorker()
	}
	e.wg.Add(2)
	go e.lightWorker()
	go e.saveWorker()
	return e, nil
}

func (e *Engine) loadWorker() {
	defer e.wg.Done()
	workerThread()
	for {
		select {
		case <-e.quit:
			return
		case p := <-e.loadReq:
			c := e.produce(p)
			select {
			case e.loadRes <- c:
			case <-e.quit:
				return
			}
		}
	}
}

func (e *Engine) produce(p voxel.ChunkPos) *voxel.Chunk {
	if e.cfg.Map != nil {
		return e.produceMap(p)
	}
	if e.store != nil {
		nbt, err := e.store.Read(p.X, p.Z)
		if err == nil {
			c, err := anvil.DecodeChunk(e.World, nbt)
			if err == nil {
				c.Pos = p
				return c
			}
			log.Printf("engine: chunk %v: %v", p, err)
		} else if !errors.Is(err, anvil.ErrNoChunk) {
			log.Printf("engine: chunk %v: %v", p, err)
		}
	}
	c := e.World.NewChunk(p)
	if e.cfg.Generator != nil {
		e.cfg.Generator.Generate(e.World, c)
	}
	return c
}

// worldOp is a batch of structural world changes. The main thread never
// takes the world write lock while streaming: lighting holds it for
// milliseconds at a time, so every insert, unload and light pass goes through
// this one goroutine, in submission order.
type worldOp struct {
	add    []*voxel.Chunk
	remove []voxel.ChunkPos
	light  []voxel.ChunkPos // nearest first
}

func (e *Engine) lightWorker() {
	defer e.wg.Done()
	workerThread()
	for {
		select {
		case <-e.quit:
			return
		case op := <-e.worldOps:
			if len(op.remove) > 0 && e.delta {
				// Staged here, under stageMu, so a chunk is always either
				// in the world or staged when SaveGame looks (save.go).
				e.stageMu.Lock()
				for _, c := range e.World.RemoveChunks(op.remove) {
					if c.Modified {
						e.stage(c)
					}
				}
				e.stageMu.Unlock()
			} else if len(op.remove) > 0 {
				for _, c := range e.World.RemoveChunks(op.remove) {
					if c.Modified && e.store != nil {
						select {
						case e.saveReq <- c:
						case <-e.quit:
							return
						}
					}
				}
			}
			if len(op.add) > 0 {
				e.World.AddChunks(op.add)
			}
			for _, p := range op.light {
				if !e.World.TryLightChunk(p) {
					continue
				}
				select {
				case e.litRes <- p:
				case <-e.quit:
					return
				}
			}
		}
	}
}

func (e *Engine) saveWorker() {
	defer e.wg.Done()
	workerThread()
	for {
		select {
		case <-e.quit:
			return
		case c := <-e.saveReq:
			e.save(c)
		}
	}
}

func (e *Engine) save(c *voxel.Chunk) {
	if e.store == nil || e.cfg.ReadOnly {
		return
	}
	if e.delta {
		e.stage(c)
		return
	}
	e.World.RLock()
	nbt := anvil.EncodeChunk(e.World, c)
	e.World.RUnlock()
	if err := e.store.Write(c.Pos.X, c.Pos.Z, nbt); err != nil {
		log.Printf("engine: save %v: %v", c.Pos, err)
	}
}

func (e *Engine) meshWorker() {
	defer e.wg.Done()
	workerThread()
	m := mesh.NewMesher()
	for {
		select {
		case <-e.quit:
			return
		case p := <-e.meshJobs:
			m.Snap.Capture(e.World, p)
			e.cfgMu.RLock()
			cfg := e.meshCfg
			e.cfgMu.RUnlock()
			res := m.Build(e.World.Reg.Table(), cfg)
			select {
			case e.meshRes <- res:
			case <-e.quit:
				return
			}
		}
	}
}

func (e *Engine) inRange(p voxel.ChunkPos, r int) bool {
	dx, dz := p.X-e.center.X, p.Z-e.center.Z
	return dx >= -r && dx <= r && dz >= -r && dz <= r
}

// TimingNames labels the entries of Engine.Timing.
var TimingNames = [7]string{"unload", "request", "add", "lit", "dirty", "upload", "dispatch"}

func (e *Engine) mark(i int, t *time.Time) {
	now := time.Now()
	e.Timing[i] = max(e.Timing[i], now.Sub(*t))
	*t = now
}

// Update advances streaming for a viewer at x,y,z. Call once per frame on
// the render thread.
func (e *Engine) Update(x, y, z float64) {
	e.viewer = [3]float64{x, y, z}
	t := time.Now()
	c := voxel.ChunkPos{X: int(math.Floor(x)) >> 4, Z: int(math.Floor(z)) >> 4}
	R := e.cfg.RenderDistance
	if c != e.center || !e.started {
		e.started = true
		e.center = c
		e.unloadFar(R + 3)
	}
	e.mark(0, &t)
	e.requestChunks(R + 1)
	e.mark(1, &t)

	// arrivals, inserted under a single world lock
	var batch []*voxel.Chunk
recv:
	for len(batch) < 256 {
		select {
		case ch := <-e.loadRes:
			delete(e.pending, ch.Pos)
			if e.stale[ch.Pos] {
				delete(e.stale, ch.Pos) // made before Regenerate: requested again
				continue
			}
			if e.inRange(ch.Pos, R+3) {
				batch = append(batch, ch)
			}
		default:
			break recv
		}
	}
	if len(batch) > 0 {
		for _, ch := range batch {
			e.loaded[ch.Pos] = true
		}
		// Ask the light worker about each new column and its neighbours,
		// nearest first; it lights those whose 3x3 is now complete.
		seen := map[voxel.ChunkPos]bool{}
		var req []voxel.ChunkPos
		for _, ch := range batch {
			for dx := -1; dx <= 1; dx++ {
				for dz := -1; dz <= 1; dz++ {
					p := ch.Pos.Add(dx, dz)
					if !seen[p] && e.loaded[p] && !e.lit[p] && e.neighboursLoaded(p) {
						seen[p] = true
						req = append(req, p)
					}
				}
			}
		}
		sort.Slice(req, func(i, j int) bool { return e.dist2(req[i]) < e.dist2(req[j]) })
		e.worldOps <- worldOp{add: batch, light: req}
	}
	e.mark(2, &t)
	// Bounded: each lit column re-dirties 9 columns of sections.
	for n := 0; n < 64; n++ {
		select {
		case p := <-e.litRes:
			if e.loaded[p] {
				e.lit[p] = true
				// Lighting dirtied the 3x3 sections before this message
				// arrived; dispatch may have dropped them as not ready.
				e.dirtyAround(p)
			}
			continue
		default:
		}
		break
	}

	if e.Renderer.SyncTextures() {
		e.cfgMu.Lock()
		e.meshCfg = e.Renderer.MeshConfig()
		e.cfgMu.Unlock()
	}

	e.mark(3, &t)
	for _, p := range e.World.TakeDirty() {
		e.dirty[p] = true
	}
	e.mark(4, &t)
	e.uploadResults()
	e.mark(5, &t)
	e.dispatch()
	e.mark(6, &t)
}

// requestChunks queues missing chunks within radius, nearest first.
func (e *Engine) requestChunks(r int) {
	var want []voxel.ChunkPos
	for dz := -r; dz <= r; dz++ {
		for dx := -r; dx <= r; dx++ {
			p := e.center.Add(dx, dz)
			if !e.inBounds(p) {
				continue
			}
			if !e.loaded[p] && !e.pending[p] {
				want = append(want, p)
			}
		}
	}
	sort.Slice(want, func(i, j int) bool { return e.dist2(want[i]) < e.dist2(want[j]) })
	for _, p := range want {
		select {
		case e.loadReq <- p:
			e.pending[p] = true
		default:
			return // queue full; retried on the next centre change / tick
		}
	}
}

// inBounds reports whether a chunk is inside Config.Bounds (or there are
// none). Neighbours outside count as present: there is nothing there.
func (e *Engine) inBounds(p voxel.ChunkPos) bool {
	b := e.cfg.Bounds
	return b == nil || p.X >= b[0].X && p.X <= b[1].X && p.Z >= b[0].Z && p.Z <= b[1].Z
}

func (e *Engine) dist2(p voxel.ChunkPos) int {
	dx, dz := p.X-e.center.X, p.Z-e.center.Z
	return dx*dx + dz*dz
}

func (e *Engine) neighboursLoaded(p voxel.ChunkPos) bool {
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if n := p.Add(dx, dz); !e.loaded[n] && e.inBounds(n) {
				return false
			}
		}
	}
	return true
}

// ChunkReady reports whether a column and its neighbours are lit, i.e. its
// meshes are final. Lock free; call from the main thread.
func (e *Engine) ChunkReady(p voxel.ChunkPos) bool {
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if !e.lit[p.Add(dx, dz)] {
				return false
			}
		}
	}
	return true
}

func (e *Engine) unloadFar(r int) {
	var far []voxel.ChunkPos
	for p := range e.loaded {
		if !e.inRange(p, r) {
			far = append(far, p)
		}
	}
	if len(far) == 0 {
		return
	}
	e.worldOps <- worldOp{remove: far}
	for _, p := range far {
		delete(e.loaded, p)
		delete(e.lit, p)
		e.Renderer.RemoveColumn(p)
	}
}

// Regenerate drops every chunk, so the world streams in again from the
// generator (or WorldDir), e.g. after the generator's input changed.
// Chunks and meshes still being made from the old world are discarded when
// they arrive. Call on the render thread, between Updates.
func (e *Engine) Regenerate() {
	for p := range e.pending {
		e.stale[p] = true
	}
	for p := range e.inFlight {
		e.staleMesh[p] = true
	}
	clear(e.redo)
	clear(e.dirty)
	all := make([]voxel.ChunkPos, 0, len(e.loaded))
	for p := range e.loaded {
		all = append(all, p)
	}
	e.worldOps <- worldOp{remove: all}
	for _, p := range all {
		e.Renderer.RemoveColumn(p)
	}
	clear(e.loaded)
	clear(e.lit)
}

// dirtyAround queues every section of the 3x3 columns around p.
func (e *Engine) dirtyAround(p voxel.ChunkPos) {
	ms, n := e.World.MinSection(), e.World.SectionCount()
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			for sy := ms; sy < ms+n; sy++ {
				e.dirty[voxel.SectionPos{X: p.X + dx, Y: sy, Z: p.Z + dz}] = true
			}
		}
	}
}

// meshCand is a dirty section and its squared distance to the viewer.
type meshCand struct {
	p voxel.SectionPos
	d float64
}

// dispatch sends dirty, renderable sections to the workers, nearest first.
func (e *Engine) dispatch() {
	// Only as many jobs as the queue takes: a backlog of thousands must not
	// cost a full sort every frame.
	free := cap(e.meshJobs) - len(e.meshJobs)
	if len(e.dirty) == 0 || free == 0 {
		return
	}
	R := e.cfg.RenderDistance
	vx, vy, vz := e.viewer[0], e.viewer[1], e.viewer[2]
	list := e.dispatchBuf[:0]
	for p := range e.dirty {
		if !e.inRange(p.Chunk(), R) {
			delete(e.dirty, p)
			continue
		}
		dx, dy, dz := float64(p.X*16+8)-vx, float64(p.Y*16+8)-vy, float64(p.Z*16+8)-vz
		list = append(list, meshCand{p, dx*dx + dy*dy*2 + dz*dz})
	}
	e.dispatchBuf = list
	// Partial selection: move the nearest candidates to the front. Dropped
	// and in-flight entries consume picks, so take a few extra.
	k := min(len(list), free*4)
	for i := 0; i < k; i++ {
		m := i
		for j := i + 1; j < len(list); j++ {
			if list[j].d < list[m].d {
				m = j
			}
		}
		list[i], list[m] = list[m], list[i]
	}
	for _, c := range list[:k] {
		p := c.p
		if !e.ChunkReady(p.Chunk()) {
			// Dropped: lighting the last neighbour re-dirties it.
			delete(e.dirty, p)
			continue
		}
		if e.inFlight[p] {
			e.redo[p] = true
			delete(e.dirty, p)
			continue
		}
		select {
		case e.meshJobs <- p:
			e.inFlight[p] = true
			delete(e.dirty, p)
		default:
			return
		}
	}
}

func (e *Engine) uploadResults() {
	start := time.Now()
	for time.Since(start) < e.cfg.UploadBudget {
		select {
		case m := <-e.meshRes:
			delete(e.inFlight, m.Pos)
			if e.staleMesh[m.Pos] {
				delete(e.staleMesh, m.Pos)
				continue
			}
			if e.redo[m.Pos] {
				delete(e.redo, m.Pos)
				e.dirty[m.Pos] = true
			}
			if !e.loaded[m.Pos.Chunk()] {
				continue
			}
			e.Renderer.Upload(m)
			e.MeshedSections++
		default:
			e.LastUploadTime = time.Since(start)
			return
		}
	}
	e.LastUploadTime = time.Since(start)
}

// Render draws the world.
func (e *Engine) Render(cam *render.Camera) { e.Renderer.Render(cam) }

// Block returns the state at a position.
func (e *Engine) Block(x, y, z int) voxel.StateID { return e.World.Block(x, y, z) }

// SetBlock changes a block and rebuilds the affected sections immediately on
// the calling thread, so edits appear in the same frame (Minecraft does the
// same for player-caused changes near the camera).
func (e *Engine) SetBlock(x, y, z int, id voxel.StateID) bool {
	if !e.World.SetBlock(x, y, z, id) {
		return false
	}
	for _, p := range e.World.TakeDirty() {
		if !e.inRange(p.Chunk(), e.cfg.RenderDistance) || !e.ChunkReady(p.Chunk()) {
			continue
		}
		if e.inFlight[p] {
			e.redo[p] = true
		}
		delete(e.dirty, p)
		e.mainMesher.Snap.Capture(e.World, p)
		e.Renderer.Upload(e.mainMesher.Build(e.World.Reg.Table(), e.meshCfg))
	}
	return true
}

// Raycast finds the targeted block from an eye position along dir.
func (e *Engine) Raycast(ox, oy, oz, dx, dy, dz, maxDist float64) (physics.Hit, bool) {
	return physics.Raycast(e.World, ox, oy, oz, dx, dy, dz, maxDist, nil)
}

// Progress reports loaded chunks and pending work, for loading screens.
func (e *Engine) Progress() (loaded, pendingChunks, pendingMeshes int) {
	return len(e.loaded), len(e.pending), len(e.dirty) + len(e.inFlight)
}

// SurfaceY returns the y above the highest collidable block of a column,
// or ok=false if the chunk isn't loaded.
func (e *Engine) SurfaceY(x, z int) (int, bool) {
	c := e.World.Chunk(voxel.ChunkPos{X: x >> 4, Z: z >> 4})
	if c == nil {
		return 0, false
	}
	t := e.World.Reg.Table()
	e.World.RLock()
	defer e.World.RUnlock()
	for y := e.World.MaxY() - 1; y >= e.World.MinY; y-- {
		if t[c.Get(x&15, y, z&15)].Collides {
			return y + 1, true
		}
	}
	return e.World.MinY, true
}

// Spawn reads the spawn point from the Map (if it is in this dimension)
// or WorldDir/level.dat.
func (e *Engine) Spawn() (x, y, z int, ok bool) {
	if m := e.cfg.Map; m != nil {
		sp := m.Spawn
		return sp.X, sp.Y, sp.Z, sp.Dim == e.cfg.Dimension
	}
	if e.cfg.WorldDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(e.cfg.WorldDir, "level.dat"))
	if err != nil {
		return
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return
	}
	root, _, err := anvil.ReadNBT(zr)
	if err != nil {
		return
	}
	d := root.Compound("Data")
	if d == nil {
		return
	}
	if sp := d.Compound("spawn"); sp != nil { // 1.21.5+
		if pos, ok2 := sp["pos"].([]int32); ok2 && len(pos) == 3 {
			return int(pos[0]), int(pos[1]), int(pos[2]), true
		}
	}
	if d.Has("SpawnX") {
		return d.Int("SpawnX"), d.Int("SpawnY"), d.Int("SpawnZ"), true
	}
	return
}

// Close stops workers and saves every modified chunk to WorldDir. A save
// slot (SaveDir) keeps only what SaveGame saved.
func (e *Engine) Close() {
	close(e.quit)
	e.wg.Wait()
	// drain queued saves
	for {
		select {
		case c := <-e.saveReq:
			e.save(c)
			continue
		default:
		}
		break
	}
	if e.store != nil {
		if !e.cfg.ReadOnly && !e.delta {
			// Includes chunks whose unload op was still queued.
			for _, p := range e.World.Chunks() {
				if c := e.World.Chunk(p); c != nil && c.Modified {
					e.save(c)
				}
			}
		}
		e.store.Close()
	}
	e.Renderer.Destroy()
}

// Flush writes every modified chunk to the world directory now (an editor's
// save), instead of waiting for unload or Close. With a save slot it only
// stages them for the next SaveGame.
func (e *Engine) Flush() {
	if e.store == nil || e.cfg.ReadOnly {
		return
	}
	for _, p := range e.World.Chunks() {
		if c := e.World.Chunk(p); c != nil && c.Modified {
			e.save(c)
		}
	}
}
