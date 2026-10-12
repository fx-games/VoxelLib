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
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/anvil"
	"github.com/memmaker/voxellib/voxel/mesh"
	"github.com/memmaker/voxellib/voxel/physics"
	"github.com/memmaker/voxellib/voxel/render"
)

// Generator fills new chunks. *gen.Generator implements it.
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
	ReadOnly       bool            // never write to WorldDir
	Textures       render.TextureSource
	RenderOptions  render.Options
	// Workers is the size of each background pool (mesh, generation).
	// Default (NumCPU-2)/2 so the render thread and the light worker always
	// find a free core; oversubscribing shows up as frame hitches.
	Workers      int
	UploadBudget time.Duration // GPU upload time per frame, default 4ms
}

// Engine owns world, workers and renderer.
type Engine struct {
	World    *voxel.World
	Renderer *render.Renderer
	cfg      Config
	store    *anvil.Store

	center   voxel.ChunkPos
	viewer   [3]float64
	started  bool
	loaded   map[voxel.ChunkPos]bool // present in the world
	pending  map[voxel.ChunkPos]bool // load/generate requested
	loadReq  chan voxel.ChunkPos
	loadRes  chan *voxel.Chunk
	lightReq chan voxel.ChunkPos
	litRes   chan voxel.ChunkPos
	lit      map[voxel.ChunkPos]bool // mirror of Chunk.Lit, so the main thread needn't lock
	saveReq  chan *voxel.Chunk

	meshJobs   chan voxel.SectionPos
	meshRes    chan *mesh.Mesh
	dirty      map[voxel.SectionPos]bool
	inFlight   map[voxel.SectionPos]bool
	redo       map[voxel.SectionPos]bool
	mainMesher *mesh.Mesher
	cfgMu      sync.RWMutex
	meshCfg    *mesh.Config

	quit chan struct{}
	wg   sync.WaitGroup
	// Stats
	MeshedSections int
	LastUploadTime time.Duration
	// Timing holds the worst duration of each Update phase since the caller
	// last cleared it (see TimingNames). Diagnostics only.
	Timing [7]time.Duration
}

// New creates an engine. Must be called on the GL thread with a current
// context (the renderer is created here).
func New(cfg Config) (*Engine, error) {
	if cfg.Registry == nil {
		cfg.Registry = voxel.NewRegistry()
	}
	if cfg.Height == 0 {
		cfg.MinY, cfg.Height = -64, 384
	}
	if cfg.RenderDistance == 0 {
		cfg.RenderDistance = 12
	}
	if cfg.Workers <= 0 {
		cfg.Workers = max(1, (runtime.NumCPU()-2)/2)
	}
	if cfg.UploadBudget == 0 {
		cfg.UploadBudget = 4 * time.Millisecond
	}
	w := voxel.NewWorld(cfg.Registry, cfg.MinY, cfg.Height)
	ro := cfg.RenderOptions
	ro.RenderDistance = cfg.RenderDistance
	r, err := render.New(w, cfg.Textures, ro)
	if err != nil {
		return nil, err
	}
	e := &Engine{
		World: w, Renderer: r, cfg: cfg,
		loaded: map[voxel.ChunkPos]bool{}, pending: map[voxel.ChunkPos]bool{},
		loadReq: make(chan voxel.ChunkPos, 4096), loadRes: make(chan *voxel.Chunk, 4096),
		lightReq: make(chan voxel.ChunkPos, 1<<16), litRes: make(chan voxel.ChunkPos, 1<<16),
		lit: map[voxel.ChunkPos]bool{}, saveReq: make(chan *voxel.Chunk, 1024),
		meshJobs: make(chan voxel.SectionPos, cfg.Workers*2), meshRes: make(chan *mesh.Mesh, 1024),
		dirty: map[voxel.SectionPos]bool{}, inFlight: map[voxel.SectionPos]bool{}, redo: map[voxel.SectionPos]bool{},
		mainMesher: mesh.NewMesher(), meshCfg: r.MeshConfig(), quit: make(chan struct{}),
	}
	if cfg.WorldDir != "" {
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

func (e *Engine) lightWorker() {
	defer e.wg.Done()
	for {
		select {
		case <-e.quit:
			return
		case p := <-e.lightReq:
			if e.World.TryLightChunk(p) {
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
	e.World.RLock()
	nbt := anvil.EncodeChunk(e.World, c)
	e.World.RUnlock()
	if err := e.store.Write(c.Pos.X, c.Pos.Z, nbt); err != nil {
		log.Printf("engine: save %v: %v", c.Pos, err)
	}
}

func (e *Engine) meshWorker() {
	defer e.wg.Done()
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
// the GL thread.
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
			if e.inRange(ch.Pos, R+3) {
				batch = append(batch, ch)
			}
		default:
			break recv
		}
	}
	if len(batch) > 0 {
		e.World.AddChunks(batch)
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
		for _, p := range req {
			select {
			case e.lightReq <- p:
			default:
			}
		}
	}
	e.mark(2, &t)
	for n := 0; n < 4096; n++ {
		select {
		case p := <-e.litRes:
			if e.loaded[p] {
				e.lit[p] = true
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

func (e *Engine) dist2(p voxel.ChunkPos) int {
	dx, dz := p.X-e.center.X, p.Z-e.center.Z
	return dx*dx + dz*dz
}

func (e *Engine) neighboursLoaded(p voxel.ChunkPos) bool {
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if !e.loaded[p.Add(dx, dz)] {
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
	for _, c := range e.World.RemoveChunks(far) {
		if c.Modified && e.store != nil {
			e.saveReq <- c
		}
	}
	for _, p := range far {
		delete(e.loaded, p)
		delete(e.lit, p)
		e.Renderer.RemoveColumn(p)
	}
}

// dispatch sends dirty, renderable sections to the workers, nearest first.
func (e *Engine) dispatch() {
	if len(e.dirty) == 0 {
		return
	}
	R := e.cfg.RenderDistance
	list := make([]voxel.SectionPos, 0, len(e.dirty))
	for p := range e.dirty {
		if !e.inRange(p.Chunk(), R) {
			delete(e.dirty, p)
			continue
		}
		list = append(list, p)
	}
	vx, vy, vz := e.viewer[0], e.viewer[1], e.viewer[2]
	d := func(p voxel.SectionPos) float64 {
		dx, dy, dz := float64(p.X*16+8)-vx, float64(p.Y*16+8)-vy, float64(p.Z*16+8)-vz
		return dx*dx + dy*dy*2 + dz*dz
	}
	sort.Slice(list, func(i, j int) bool { return d(list[i]) < d(list[j]) })
	for _, p := range list {
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

// Spawn reads the spawn point from WorldDir/level.dat if present.
func (e *Engine) Spawn() (x, y, z int, ok bool) {
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

// Close stops workers and saves every modified chunk.
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
	if e.store != nil && !e.cfg.ReadOnly {
		for p := range e.loaded {
			if c := e.World.Chunk(p); c != nil && c.Modified {
				e.save(c)
			}
		}
		e.store.Close()
	}
	e.Renderer.Destroy()
}
