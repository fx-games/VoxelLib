// Package render draws voxel.World meshes with WebGPU (gogpu/wgpu: Metal on
// macOS, DX12/Vulkan elsewhere; files *_gpu.go, gpu.go, surface_*.go).
// Culling, sorting and buffer management live in this file, region.go and
// texarray.go.
package render

import (
	"image"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/mesh"
)

// Options tune the renderer; they may be changed between frames.
type Options struct {
	RenderDistance int // in chunks
	SkyColor       mgl32.Vec3
	FogColor       mgl32.Vec3
	Daylight       float32 // 0..1 multiplier for sky light
	// BlockBrightness 0..1 brightens block light (torches) toward
	// The reference's "Bright" gamma; 0 is the plain, moody curve.
	BlockBrightness float32
	SunDir          mgl32.Vec3
	// Fog, when non-nil, replaces the distance fog and sky colour, e.g. for
	// a camera under water or lava.
	Fog *Fog
	// Missing returns the placeholder texture; IsGreyscale decides tinting.
	Missing     func(size int) image.Image
	IsGreyscale func(image.Image) bool
	// BlockTint colours tinted blocks by position and biome (mesh.Config).
	BlockTint func(info *voxel.StateInfo, x, y, z int, biome func(x, z int) uint8) uint32
	// RenderBlocksUV maps block textures like the 1.7 reference (mesh.Config).
	RenderBlocksUV bool
	// DynamicTint is added to a vertex tint scaled by its BlockTint weight,
	// so a colour that follows the camera (e.g. water) needs no remesh.
	DynamicTint mgl32.Vec3
	// Lightmap, when non-nil, gives the light colour per (sky, block)
	// level for terrain, models, items and particles; nil uses
	// DefaultLightmap(Daylight, BlockBrightness).
	Lightmap *Lightmap
	// Sky, when non-nil, replaces the procedural sky: the frame is cleared
	// to the fog colour and these are drawn before terrain. SkyAfter is
	// drawn after translucent terrain (e.g. high clouds, rain).
	Sky, SkyAfter []SkyDraw
	// Pack, when set, renders the world with the shader pack (pack.go):
	// shadows, deferred lighting and the pack's sky instead of Sky and
	// SkyAfter. Set it every frame; nil: the plain renderer.
	Pack *PackFrame
	// SkyTextures loads SkyDraw textures by name; nil uses the block
	// texture source.
	SkyTextures TextureSource
	// Weather, when non-nil, is the rain and snow DrawWeather draws.
	Weather *Weather
}

// Lightmap is a light colour table indexed [sky][block] by light level,
// sampled with linear filtering between levels like the reference's 16x16
// lightmap texture.
type Lightmap [16][16][3]float32

// DefaultLightmap is the engine's own lighting: the brighter of a cool sky
// curve scaled by daylight and a warm block curve raised toward
// The reference's bright gamma by bright.
func DefaultLightmap(daylight, bright float32) *Lightmap {
	curve := func(l float64) float64 { return 0.04 + 0.96*(l/(4-3*l)) }
	var lm Lightmap
	for s := range 16 {
		for b := range 16 {
			sl, bl := float64(s)/15, float64(b)/15
			sv := curve(sl) * float64(daylight)
			bc := curve(bl)
			bv := bc + (0.04+0.96*(1-math.Pow(1-bl, 4))-bc)*float64(bright)
			for c, k := range [3][2]float64{{0.95, 1}, {0.97, 0.86}, {1, 0.66}} {
				lm[s][b][c] = float32(max(sv*k[0], bv*k[1], 0.04))
			}
		}
	}
	return &lm
}

// LightColor returns this frame's light colour for sky and block light
// levels (0..15), for draws lit on the CPU.
func (r *Renderer) LightColor(sky, block uint8) mgl32.Vec3 {
	return mgl32.Vec3(r.light[min(sky, 15)][min(block, 15)])
}

// frameLight sets r.light for this frame; it reports whether it changed.
func (r *Renderer) frameLight() bool {
	lm := r.Opt.Lightmap
	if lm == nil {
		if r.lightDefault != nil && r.lightDay == r.Opt.Daylight && r.lightBright == r.Opt.BlockBrightness {
			lm = r.lightDefault
		} else {
			lm = DefaultLightmap(r.Opt.Daylight, r.Opt.BlockBrightness)
			r.lightDefault, r.lightDay, r.lightBright = lm, r.Opt.Daylight, r.Opt.BlockBrightness
		}
	}
	if *lm == r.light && r.lightSet {
		return false
	}
	r.light, r.lightSet = *lm, true
	return true
}

// Fog is a fog override: colour plus start and end distance in blocks;
// the factor is linear between them (GL_LINEAR).
type Fog struct {
	Color      mgl32.Vec3
	Start, End float32
}

// CurrentFog returns the fog the terrain is drawn with: Opt.Fog if set,
// else distance fog ending at the render distance, darkened with daylight
// as in the reference game. Other passes (items) use it to match the terrain.
func (r *Renderer) CurrentFog() Fog {
	if r.Opt.Fog != nil {
		return *r.Opt.Fog
	}
	far := float32(r.Opt.RenderDistance * 16)
	return Fog{Color: r.Opt.FogColor.Mul(0.04 + 0.96*r.Opt.Daylight), Start: far * 0.75, End: far}
}

// Stats describes the last rendered frame.
type Stats struct {
	Sections     int // sections with GPU data
	Regions      int
	Visited      int // sections reached by the occlusion BFS
	Visible      int // non-empty sections drawn
	DrawCalls    int
	DrawRanges   int
	Quads        int
	VertexMemory int // bytes reserved in region buffers
}

type section struct {
	pos     voxel.SectionPos
	reg     *region
	off, n  int // opaque + cutout vertices
	tOff    int // translucent vertices
	ranges  [voxel.LayerCount][mesh.GroupCount]mesh.Range
	transN  int
	vis     mesh.VisGraph
	empty   bool
	trans   *mesh.Mesh
	sortPos [3]float64
	quads   int
}

// drawBatch is one multidraw: ranges counts[first:first+n] / bases[...] of a
// region page, drawn with the given layer's state.
type drawBatch struct {
	reg      *region
	layer    voxel.Layer
	first, n int
}

// Renderer owns all GPU state for one world.
type Renderer struct {
	Opt   Options
	Stats Stats

	world      *voxel.World
	src        TextureSource
	tex        *textureArray
	start      time.Time // animation clock
	texVersion uint64
	cfg        *mesh.Config

	gpu    gpuRenderer // backend state
	damage gpuDamage   // breaking crack textures (SetDamageTextures)

	regions  map[[3]int][]*region // pages per region key
	sections map[voxel.SectionPos]*section
	columns  map[voxel.ChunkPos]int

	// occlusion BFS scratch
	visitGen         uint32
	visited          []uint32
	visDim           int
	queue            []bfsNode
	visible          []*section
	camX, camY, camZ float64

	// draw lists for this frame: index counts and base vertices per range
	counts  []int32
	bases   []int32
	batches []drawBatch

	frame   uint64
	retired []retired
	// terrainGen counts changes to the drawn terrain (meshes, translucent
	// order, animated textures): the pack's shadow cache key.
	terrainGen uint64

	light                 Lightmap // this frame's
	lightSet              bool
	lightDefault          *Lightmap // cached DefaultLightmap
	lightDay, lightBright float32
	skyTex                map[string]*skyTexture
	skyVerts              []SkyVertex
	mergeBuf              []int64
	regionOrder           []*region
	weatherDraws          []SkyDraw // DrawWeather, reused
	weatherFog            Fog
}

// retire frees a span once in-flight frames can no longer read it.
func (r *Renderer) retire(reg *region, off, n int) {
	if n > 0 {
		r.retired = append(r.retired, retired{reg, off, n, r.frame})
	}
}

func (r *Renderer) releaseRetired() {
	i := 0
	for ; i < len(r.retired) && r.retired[i].frame+retireFrames <= r.frame; i++ {
		if rt := r.retired[i]; !rt.reg.dead {
			rt.reg.arena.release(rt.off, rt.n)
		}
	}
	r.retired = append(r.retired[:0], r.retired[i:]...)
}

type bfsNode struct {
	pos   voxel.SectionPos
	entry int8 // face we came in through, -1 for the start
	dirs  uint8
}

// maxQuadsPerDraw sizes the shared quad index buffer.
const maxQuadsPerDraw = 1 << 15

// quadIndices is the shared quad index buffer (0,1,2, 2,3,0) + 4k, like
// The reference's sequential index buffer: every section reuses it via base
// vertex.
func quadIndices() []uint32 {
	idx := make([]uint32, maxQuadsPerDraw*6)
	for q := 0; q < maxQuadsPerDraw; q++ {
		b := uint32(q * 4)
		copy(idx[q*6:], []uint32{b, b + 1, b + 2, b + 2, b + 3, b})
	}
	return idx
}

// lineVertices are the static line vertices: unit-cube edges (24 vertices)
// then the crosshair (4). Both are placed with an MVP matrix.
func lineVertices() []float32 {
	var lv []float32
	corner := func(i int) []float32 { return []float32{float32(i & 1), float32(i >> 1 & 1), float32(i >> 2 & 1)} }
	for _, e := range [12][2]int{{0, 1}, {2, 3}, {4, 5}, {6, 7}, {0, 2}, {1, 3}, {4, 6}, {5, 7}, {0, 4}, {1, 5}, {2, 6}, {3, 7}} {
		lv = append(append(lv, corner(e[0])...), corner(e[1])...)
	}
	return append(lv, -1, 0, 0, 1, 0, 0, 0, -1, 0, 0, 1, 0)
}

// New creates a renderer; Init must have been called.
func New(w *voxel.World, src TextureSource, opt Options) (*Renderer, error) {
	r := &Renderer{Opt: opt, world: w, src: src,
		regions: map[[3]int][]*region{}, sections: map[voxel.SectionPos]*section{}, columns: map[voxel.ChunkPos]int{}}
	if r.Opt.RenderDistance == 0 {
		r.Opt.RenderDistance = 12
	}
	if r.Opt.Daylight == 0 {
		r.Opt.Daylight = 1
	}
	if r.Opt.SkyColor == (mgl32.Vec3{}) {
		r.Opt.SkyColor = mgl32.Vec3{0.47, 0.65, 1.0}
		r.Opt.FogColor = mgl32.Vec3{0.75, 0.85, 1.0}
	}
	if r.Opt.SunDir == (mgl32.Vec3{}) {
		r.Opt.SunDir = mgl32.Vec3{0.4, 0.8, -0.45}.Normalize()
	}
	if err := r.initGPU(); err != nil {
		return nil, err
	}
	r.start = time.Now()
	r.reloadTextures()
	return r, nil
}

func (r *Renderer) reloadTextures() {
	names, ver := r.world.Reg.TextureNames()
	if r.tex != nil && r.tex.add(names) { // new names into spare layers
		r.texVersion = ver
		r.cfg = &mesh.Config{Tintable: r.tex.tintable, BlockTint: r.Opt.BlockTint, RenderBlocksUV: r.Opt.RenderBlocksUV}
		return
	}
	if r.tex != nil {
		r.tex.delete()
	}
	missing := r.Opt.Missing
	if missing == nil {
		missing = func(s int) image.Image { return image.NewNRGBA(image.Rect(0, 0, s, s)) }
	}
	grey := r.Opt.IsGreyscale
	if grey == nil {
		grey = func(image.Image) bool { return false }
	}
	r.tex = newTextureArray(names, r.src, missing, grey)
	r.texVersion = ver
	r.cfg = &mesh.Config{Tintable: r.tex.tintable, BlockTint: r.Opt.BlockTint, RenderBlocksUV: r.Opt.RenderBlocksUV}
}

// AnimTick is the texture animation tick (20 Hz since the renderer
// started): an animated texture shows frame (AnimTick/ticksPerFrame)%frames.
func (r *Renderer) AnimTick() int { return int(time.Since(r.start) / (50 * time.Millisecond)) }

// MeshConfig returns the per-texture data the mesher needs. It changes when
// textures are added, so fetch it per batch.
func (r *Renderer) MeshConfig() *mesh.Config { return r.cfg }

// SyncTextures rebuilds the texture array if the registry learned new
// texture names (e.g. unknown blocks from a loaded world). Returns true if it
// did, in which case meshes using new textures should be rebuilt.
func (r *Renderer) SyncTextures() bool {
	if r.world.Reg.TextureVersion() == r.texVersion {
		return false
	}
	r.reloadTextures()
	return true
}

// Upload stores (or replaces) a section mesh on the GPU.
func (r *Renderer) Upload(m *mesh.Mesh) {
	r.terrainGen++
	s := r.sections[m.Pos]
	if s == nil {
		s = &section{pos: m.Pos}
		r.sections[m.Pos] = s
		r.columns[m.Pos.Chunk()]++
	}
	r.free(s)
	s.vis, s.empty, s.ranges, s.trans = m.Vis, m.Empty, m.Ranges, nil
	s.quads = m.Quads()
	if m.Empty {
		return
	}
	dataN := len(m.Data) / mesh.VertexWords
	s.transN = len(m.Trans) / mesh.VertexWords
	s.n = dataN
	key := mesh.RegionOf(m.Pos)
	reg, a, b, ok := fit(r.regions[key], s.n, s.transN)
	s.off, s.tOff = a, b
	if !ok {
		reg = r.newRegion(key, max(minPageVerts, s.n+s.transN))
		r.regions[key] = append(r.regions[key], reg)
		s.off, s.tOff, _ = reg.allocPair(s.n, s.transN)
	}
	s.reg = reg
	reg.sections[mesh.RegionIndex(m.Pos)] = s
	reg.upload(s.off, m.Data)
	if s.transN > 0 {
		s.trans = m
		r.sortSection(s, true)
	}
}

// sortSection uploads the translucent quads sorted for the camera. fresh
// means s.tOff was just allocated and is safe to write.
func (r *Renderer) sortSection(s *section, fresh bool) {
	o := s.pos.Origin()
	cam := [3]float32{float32(r.camX - float64(o.X)), float32(r.camY - float64(o.Y)), float32(r.camZ - float64(o.Z))}
	data := s.trans.SortTranslucent(cam)
	// Write the new order to a fresh span; the old one may still be drawn.
	if fresh {
		s.reg.upload(s.tOff, data)
	} else if off, ok := s.reg.arena.alloc(s.transN); ok {
		r.retire(s.reg, s.tOff, s.transN)
		s.tOff = off
		s.reg.upload(off, data)
	} else {
		s.reg.overwrite(s.tOff, data)
	}
	s.sortPos = [3]float64{r.camX, r.camY, r.camZ}
	r.terrainGen++
}

func (r *Renderer) free(s *section) {
	if s.reg == nil {
		return
	}
	r.terrainGen++
	r.retire(s.reg, s.off, s.n)
	r.retire(s.reg, s.tOff, s.transN)
	delete(s.reg.sections, mesh.RegionIndex(s.pos))
	if len(s.reg.sections) == 0 {
		s.reg.destroy()
		pages := r.regions[s.reg.key]
		for i, pg := range pages {
			if pg == s.reg {
				pages = append(pages[:i], pages[i+1:]...)
				break
			}
		}
		if len(pages) == 0 {
			delete(r.regions, s.reg.key)
		} else {
			r.regions[s.reg.key] = pages
		}
	}
	s.reg, s.n, s.transN = nil, 0, 0
}

// RemoveSection frees a section's GPU data.
func (r *Renderer) RemoveSection(p voxel.SectionPos) {
	if s := r.sections[p]; s != nil {
		r.free(s)
		delete(r.sections, p)
		if r.columns[p.Chunk()]--; r.columns[p.Chunk()] <= 0 {
			delete(r.columns, p.Chunk())
		}
	}
}

// RemoveColumn frees every section of a chunk column.
func (r *Renderer) RemoveColumn(c voxel.ChunkPos) {
	for sy := r.world.MinSection(); sy < r.world.MinSection()+r.world.SectionCount(); sy++ {
		r.RemoveSection(voxel.SectionPos{X: c.X, Y: sy, Z: c.Z})
	}
}

// HasSection reports whether a mesh for p was uploaded.
func (r *Renderer) HasSection(p voxel.SectionPos) bool { return r.sections[p] != nil }

// cullVisible runs the reference's section occlusion graph: a BFS from the camera
// section that only crosses from an entry face to exit faces its VisGraph
// connects, never turns back toward the camera, and stays inside the frustum
// and render distance.
func (r *Renderer) cullVisible(fr *frustum) {
	R := r.Opt.RenderDistance
	dim := 2*R + 3
	nsec := r.world.SectionCount()
	if r.visDim != dim || len(r.visited) != dim*dim*nsec {
		r.visDim = dim
		r.visited = make([]uint32, dim*dim*nsec)
		r.visitGen = 0
	}
	r.visitGen++
	if r.visitGen == 0 {
		clear(r.visited)
		r.visitGen = 1
	}
	gen := r.visitGen
	ccx, ccz := int(math.Floor(r.camX))>>4, int(math.Floor(r.camZ))>>4
	minSec := r.world.MinSection()
	csy := int(math.Floor(r.camY)) >> 4
	vidx := func(p voxel.SectionPos) int {
		return ((p.Y-minSec)*dim+(p.Z-ccz+R+1))*dim + (p.X - ccx + R + 1)
	}
	inRange := func(p voxel.SectionPos) bool {
		dx, dz := p.X-ccx, p.Z-ccz
		return p.Y >= minSec && p.Y < minSec+nsec && dx >= -R && dx <= R && dz >= -R && dz <= R
	}
	inFrustum := func(p voxel.SectionPos) bool {
		ox := float32(float64(p.X*16) - r.camX)
		oy := float32(float64(p.Y*16) - r.camY)
		oz := float32(float64(p.Z*16) - r.camZ)
		return fr.box(ox, oy, oz, ox+16, oy+16, oz+16)
	}
	q := r.queue[:0]
	vis := r.visible[:0]
	push := func(p voxel.SectionPos, entry int8, dirs uint8) {
		i := vidx(p)
		if r.visited[i] == gen {
			return
		}
		r.visited[i] = gen
		q = append(q, bfsNode{p, entry, dirs})
	}
	switch {
	case csy >= minSec+nsec || csy < minSec:
		// Camera above/below the world: seed the whole boundary layer.
		sy, entry := minSec+nsec-1, int8(voxel.FaceUp)
		if csy < minSec {
			sy, entry = minSec, int8(voxel.FaceDown)
		}
		for dz := -R; dz <= R; dz++ {
			for dx := -R; dx <= R; dx++ {
				p := voxel.SectionPos{X: ccx + dx, Y: sy, Z: ccz + dz}
				if inFrustum(p) {
					push(p, entry, 0)
				}
			}
		}
	default:
		push(voxel.SectionPos{X: ccx, Y: csy, Z: ccz}, -1, 0)
	}
	visited := 0
	for head := 0; head < len(q); head++ {
		n := q[head]
		visited++
		s := r.sections[n.pos]
		gv := mesh.AllVisible
		if s != nil {
			gv = s.vis
			if !s.empty {
				vis = append(vis, s)
			}
		} else if r.columns[n.pos.Chunk()] == 0 {
			continue // column not loaded: don't see through it
		}
		for d := voxel.Face(0); d < voxel.FaceCount; d++ {
			if n.dirs&(1<<d.Opposite()) != 0 {
				continue
			}
			if n.entry >= 0 && !gv.Connected(voxel.Face(n.entry), d) {
				continue
			}
			np := n.pos.Offset(d)
			// Visited first: it is cheaper than the frustum test and push
			// would drop the node anyway.
			if !inRange(np) || r.visited[vidx(np)] == gen || !inFrustum(np) {
				continue
			}
			push(np, int8(d.Opposite()), n.dirs|1<<d)
		}
	}
	r.queue = q
	r.visible = vis
	r.Stats.Visited = visited
}

// Render draws sky and terrain for a camera.
func (r *Renderer) Render(cam *Camera) {
	r.camX, r.camY, r.camZ = cam.X, cam.Y, cam.Z
	r.frame++
	r.releaseRetired()
	if cam.Far == 0 {
		// EntityRenderer.setupCameraTransform (1.7.10): gluPerspective(fov,
		// aspect, 0.05, farPlaneDistance*2) with farPlaneDistance =
		// renderDistanceChunks*16. Clouds depth-test against this.
		cam.Far = float64(r.Opt.RenderDistance*16) * 2
	}
	vp := cam.ViewProj()
	fr := makeFrustum(vp)
	r.cullVisible(&fr)
	if r.tex.animate(r.AnimTick()) {
		r.terrainGen++
	}
	r.buildBatches()
	if r.frameLight() {
		r.uploadLight()
	}
	if r.Opt.Pack != nil && dev.enc != nil {
		r.packFrame(cam)
	}
	r.drawTerrain(cam, vp)
}

// buildBatches fills r.batches with the frame's multidraws: solid then
// cutout per region (near to far), then translucent far to near, re-sorting
// nearby sections when the camera moved.
func (r *Renderer) buildBatches() {
	st := &r.Stats
	st.DrawCalls, st.DrawRanges, st.Quads = 0, 0, 0
	st.Visible = len(r.visible)
	st.Sections = len(r.sections)
	st.Regions = 0
	st.VertexMemory = 0
	for _, pages := range r.regions {
		for _, reg := range pages {
			st.Regions++
			st.VertexMemory += reg.arena.cap * vertexBytes
		}
	}

	// Group visible sections by region, keeping BFS (near to far) order of
	// first appearance for good early-z rejection.
	order := r.regionOrder[:0]
	for _, s := range r.visible {
		if s.reg == nil {
			continue
		}
		if len(s.reg.visible) == 0 {
			order = append(order, s.reg)
		}
		s.reg.visible = append(s.reg.visible, s)
		st.Quads += s.quads
	}
	r.regionOrder = order
	defer func() {
		for _, reg := range order {
			clear(reg.visible)
			reg.visible = reg.visible[:0]
		}
	}()

	r.counts, r.bases, r.batches = r.counts[:0], r.bases[:0], r.batches[:0]
	flush := func(reg *region, layer voxel.Layer, first int) {
		if n := len(r.counts) - first; n > 0 {
			r.batches = append(r.batches, drawBatch{reg, layer, first, n})
			st.DrawCalls++
			st.DrawRanges += n
		}
	}
	for layer := voxel.LayerSolid; layer < voxel.LayerTranslucent; layer++ {
		for _, reg := range order {
			first := len(r.counts)
			for _, s := range reg.visible {
				o := s.pos.Origin()
				ox, oy, oz := float64(o.X)-r.camX, float64(o.Y)-r.camY, float64(o.Z)-r.camZ
				// One span from the first to the last visible group: the
				// culled groups between face away from the camera, so the
				// back-face cull drops them and the pixels are the same, at
				// far fewer draws.
				lo, hi := -1, 0
				for g := 0; g < mesh.GroupCount; g++ {
					rg := s.ranges[layer][g]
					if rg.Count == 0 || !groupVisible(g, ox, oy, oz) {
						continue
					}
					if lo < 0 {
						lo = rg.First
					}
					hi = rg.First + rg.Count
				}
				if lo >= 0 && hi-lo <= maxQuadsPerDraw*4 {
					r.counts = append(r.counts, int32((hi-lo)/4*6))
					r.bases = append(r.bases, int32(s.off+lo))
				} else if lo >= 0 {
					for g := 0; g < mesh.GroupCount; g++ {
						if rg := s.ranges[layer][g]; rg.Count > 0 && groupVisible(g, ox, oy, oz) {
							r.counts = append(r.counts, int32(rg.Count/4*6))
							r.bases = append(r.bases, int32(s.off+rg.First))
						}
					}
				}
			}
			r.mergeRanges(first)
			flush(reg, layer, first)
		}
	}

	resorts := 0
	var cur *region
	first := len(r.counts)
	for i := len(r.visible) - 1; i >= 0; i-- {
		s := r.visible[i]
		if s.transN == 0 || s.reg == nil {
			continue
		}
		dx, dy, dz := s.sortPos[0]-r.camX, s.sortPos[1]-r.camY, s.sortPos[2]-r.camZ
		if resorts < 16 && dx*dx+dy*dy+dz*dz > 1 && r.near(s, 48) {
			r.sortSection(s, false)
			resorts++
		}
		if s.reg != cur && cur != nil {
			flush(cur, voxel.LayerTranslucent, first)
			first = len(r.counts)
		}
		cur = s.reg
		r.counts = append(r.counts, int32(s.transN/4*6))
		r.bases = append(r.bases, int32(s.tOff))
	}
	if cur != nil {
		flush(cur, voxel.LayerTranslucent, first)
	}
}

// mergeRanges sorts the ranges from first on by base vertex and joins the
// ones that touch: each draw costs far more CPU than its vertices cost the
// GPU. Only for opaque layers, whose order is just early-z.
func (r *Renderer) mergeRanges(first int) {
	n := len(r.counts) - first
	if n < 2 {
		return
	}
	keys := r.mergeBuf[:0]
	for i := first; i < len(r.counts); i++ {
		keys = append(keys, int64(r.bases[i])<<32|int64(r.counts[i]))
	}
	slices.Sort(keys)
	r.mergeBuf = keys
	r.counts, r.bases = r.counts[:first], r.bases[:first]
	for _, k := range keys {
		b, c := int32(k>>32), int32(k)
		if j := len(r.counts) - 1; j >= first {
			end := r.bases[j] + r.counts[j]/6*4
			if b == end && int(r.counts[j]+c) <= maxQuadsPerDraw*6 {
				r.counts[j] += c
				continue
			}
		}
		r.counts = append(r.counts, c)
		r.bases = append(r.bases, b)
	}
}

// regionOrigin is a region's origin relative to the camera.
func (r *Renderer) regionOrigin(reg *region) mgl32.Vec3 {
	o := reg.key
	return mgl32.Vec3{
		float32(float64(o[0]*mesh.RegionX*16) - r.camX),
		float32(float64(o[1]*mesh.RegionY*16) - r.camY),
		float32(float64(o[2]*mesh.RegionZ*16) - r.camZ),
	}
}

// skyColors returns the sky's zenith and horizon colours for this frame.
func (r *Renderer) skyColors() (sky, fog mgl32.Vec3) {
	f := r.CurrentFog()
	sky = r.Opt.SkyColor.Mul(0.04 + 0.96*r.Opt.Daylight)
	if r.Opt.Fog != nil {
		sky = f.Color
	}
	return sky, f.Color
}

func (r *Renderer) near(s *section, dist float64) bool {
	o := s.pos.Origin()
	dx := float64(o.X+8) - r.camX
	dy := float64(o.Y+8) - r.camY
	dz := float64(o.Z+8) - r.camZ
	return dx*dx+dy*dy+dz*dz < dist*dist
}

// groupVisible culls a face-direction group whose faces all point away from
// the camera (ox,oy,oz = section origin relative to camera).
func groupVisible(g int, ox, oy, oz float64) bool {
	switch voxel.Face(g) {
	case voxel.FaceDown:
		return oy+16 > 0
	case voxel.FaceUp:
		return oy < 0
	case voxel.FaceNorth:
		return oz+16 > 0
	case voxel.FaceSouth:
		return oz < 0
	case voxel.FaceWest:
		return ox+16 > 0
	case voxel.FaceEast:
		return ox < 0
	}
	return true
}

// outlineMVP places the unit cube of line vertices around a block.
func outlineMVP(cam *Camera, x, y, z int) mgl32.Mat4 {
	const e = 0.002
	o := mgl32.Vec3{float32(float64(x)-cam.X) - e, float32(float64(y)-cam.Y) - e, float32(float64(z)-cam.Z) - e}
	return cam.ViewProj().Mul4(mgl32.Translate3D(o[0], o[1], o[2])).Mul4(mgl32.Scale3D(1+2*e, 1+2*e, 1+2*e))
}

// boxOutlineMVP places the unit cube of line vertices on box b, grown by
// 0.002 on every side.
func boxOutlineMVP(cam *Camera, b collide.AABB) mgl32.Mat4 {
	const e = 0.002
	return cam.ViewProj().Mul4(mgl32.Translate3D(float32(b.MinX-e-cam.X), float32(b.MinY-e-cam.Y), float32(b.MinZ-e-cam.Z))).
		Mul4(mgl32.Scale3D(float32(b.MaxX-b.MinX+2*e), float32(b.MaxY-b.MinY+2*e), float32(b.MaxZ-b.MinZ+2*e)))
}

// crosshairMVP scales the crosshair lines; aspect = width/height.
func crosshairMVP(aspect float32) mgl32.Mat4 {
	const s = 0.025
	return mgl32.Scale3D(s/aspect, s, 1)
}

// VisibleSections returns positions drawn last frame (debugging/tests).
func (r *Renderer) VisibleSections() []voxel.SectionPos {
	out := make([]voxel.SectionPos, len(r.visible))
	for i, s := range r.visible {
		out[i] = s.pos
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Y < out[b].Y })
	return out
}

// Destroy frees all GPU resources.
func (r *Renderer) Destroy() {
	for _, pages := range r.regions {
		for _, reg := range pages {
			reg.destroy()
		}
	}
	r.regions = map[[3]int][]*region{}
	r.sections = map[voxel.SectionPos]*section{}
	r.tex.delete()
	r.destroyGPU()
}
