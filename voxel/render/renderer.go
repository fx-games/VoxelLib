package render

import (
	"image"
	"math"
	"sort"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/mesh"
)

// Options tune the renderer; they may be changed between frames.
type Options struct {
	RenderDistance int // in chunks
	SkyColor       mgl32.Vec3
	FogColor       mgl32.Vec3
	Daylight       float32 // 0..1 multiplier for sky light
	SunDir         mgl32.Vec3
	// Missing returns the placeholder texture; IsGreyscale decides tinting.
	Missing     func(size int) image.Image
	IsGreyscale func(image.Image) bool
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

// Renderer owns all GPU state for one world.
type Renderer struct {
	Opt   Options
	Stats Stats

	world      *voxel.World
	src        TextureSource
	tex        *textureArray
	texVersion uint64
	cfg        *mesh.Config

	terrain, sky, lines *program
	ebo                 uint32
	emptyVAO            uint32
	lineVAO, lineVBO    uint32

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

	counts []int32
	bases  []int32
	ptrs   []unsafe.Pointer

	frame   uint64
	retired []retired
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

// New creates a renderer. A GL 3.3 core context must be current; gl.Init is
// called here.
func New(w *voxel.World, src TextureSource, opt Options) (*Renderer, error) {
	if err := gl.Init(); err != nil {
		return nil, err
	}
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
	var err error
	if r.terrain, err = newProgram(terrainVS, terrainFS); err != nil {
		return nil, err
	}
	if r.sky, err = newProgram(skyVS, skyFS); err != nil {
		return nil, err
	}
	if r.lines, err = newProgram(lineVS, lineFS); err != nil {
		return nil, err
	}
	// Shared quad index buffer (0,1,2, 2,3,0) + 4k, like Minecraft's
	// sequential index buffer: every section reuses it via base vertex.
	idx := make([]uint32, maxQuadsPerDraw*6)
	for q := 0; q < maxQuadsPerDraw; q++ {
		b := uint32(q * 4)
		copy(idx[q*6:], []uint32{b, b + 1, b + 2, b + 2, b + 3, b})
	}
	gl.GenBuffers(1, &r.ebo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.ebo)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(idx)*4, gl.Ptr(idx), gl.STATIC_DRAW)
	gl.GenVertexArrays(1, &r.emptyVAO)

	gl.GenVertexArrays(1, &r.lineVAO)
	gl.GenBuffers(1, &r.lineVBO)
	gl.BindVertexArray(r.lineVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.lineVBO)
	// Static: unit-cube edges (24 vertices) then the crosshair (4). Both are
	// placed with uMVP; rewriting a buffer the GPU still reads stalls macOS.
	var lv []float32
	corner := func(i int) []float32 { return []float32{float32(i & 1), float32(i >> 1 & 1), float32(i >> 2 & 1)} }
	for _, e := range [12][2]int{{0, 1}, {2, 3}, {4, 5}, {6, 7}, {0, 2}, {1, 3}, {4, 6}, {5, 7}, {0, 4}, {1, 5}, {2, 6}, {3, 7}} {
		lv = append(append(lv, corner(e[0])...), corner(e[1])...)
	}
	lv = append(lv, -1, 0, 0, 1, 0, 0, 0, -1, 0, 0, 1, 0)
	gl.BufferData(gl.ARRAY_BUFFER, len(lv)*4, gl.Ptr(lv), gl.STATIC_DRAW)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, 12, 0)
	gl.BindVertexArray(0)

	r.reloadTextures()
	return r, nil
}

func (r *Renderer) reloadTextures() {
	names, ver := r.world.Reg.TextureNames()
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
	r.cfg = &mesh.Config{Tintable: r.tex.tintable}
}

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
	var reg *region
	for _, pg := range r.regions[key] {
		if a, b, ok := pg.allocPair(s.n, s.transN); ok {
			reg, s.off, s.tOff = pg, a, b
			break
		}
	}
	if reg == nil {
		reg = newRegion(key, r.ebo, max(pageVerts, s.n+s.transN))
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
		gl.BindBuffer(gl.ARRAY_BUFFER, s.reg.vbo)
		gl.BufferSubData(gl.ARRAY_BUFFER, s.tOff*vertexBytes, len(data)*4, gl.Ptr(data))
	}
	s.sortPos = [3]float64{r.camX, r.camY, r.camZ}
}

func (r *Renderer) free(s *section) {
	if s.reg == nil {
		return
	}
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

// cullVisible runs Minecraft's section occlusion graph: a BFS from the camera
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
			if !inRange(np) || !inFrustum(np) {
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
		cam.Far = float64(r.Opt.RenderDistance*16) * 1.5
	}
	vp := cam.ViewProj()
	fr := makeFrustum(vp)

	gl.Enable(gl.DEPTH_TEST)
	gl.DepthFunc(gl.LEQUAL)
	gl.Enable(gl.CULL_FACE)
	gl.CullFace(gl.BACK)
	gl.FrontFace(gl.CCW)

	// Sky and fog darken with daylight, as in Minecraft.
	dl := r.Opt.Daylight
	skyCol := r.Opt.SkyColor.Mul(0.04 + 0.96*dl)
	fogCol := r.Opt.FogColor.Mul(0.04 + 0.96*dl)
	gl.DepthMask(false)
	r.sky.use()
	inv := vp.Inv()
	gl.UniformMatrix4fv(r.sky.loc("uInvViewProj"), 1, false, &inv[0])
	gl.Uniform3fv(r.sky.loc("uFogColor"), 1, &fogCol[0])
	gl.Uniform3fv(r.sky.loc("uSkyColor"), 1, &skyCol[0])
	gl.Uniform3fv(r.sky.loc("uSunDir"), 1, &r.Opt.SunDir[0])
	gl.BindVertexArray(r.emptyVAO)
	gl.DrawArrays(gl.TRIANGLES, 0, 3)
	gl.DepthMask(true)

	r.cullVisible(&fr)

	p := r.terrain
	p.use()
	gl.UniformMatrix4fv(p.loc("uViewProj"), 1, false, &vp[0])
	gl.Uniform1f(p.loc("uDaylight"), r.Opt.Daylight)
	gl.Uniform3fv(p.loc("uFogColor"), 1, &fogCol[0])
	far := float32(r.Opt.RenderDistance * 16)
	gl.Uniform2f(p.loc("uFog"), far*0.6, far)
	gl.Uniform1i(p.loc("uTex"), 0)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D_ARRAY, r.tex.id)

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
	order := make([]*region, 0, len(r.regions))
	bucket := map[*region][]*section{}
	for _, s := range r.visible {
		if s.reg == nil {
			continue
		}
		if _, ok := bucket[s.reg]; !ok {
			order = append(order, s.reg)
		}
		bucket[s.reg] = append(bucket[s.reg], s)
		st.Quads += s.quads
	}

	gl.Disable(gl.BLEND)
	for layer := voxel.LayerSolid; layer < voxel.LayerTranslucent; layer++ {
		cut := float32(0)
		if layer == voxel.LayerCutout {
			cut = 0.5
		}
		gl.Uniform1f(p.loc("uAlphaCut"), cut)
		for _, reg := range order {
			r.counts, r.bases = r.counts[:0], r.bases[:0]
			for _, s := range bucket[reg] {
				o := s.pos.Origin()
				ox, oy, oz := float64(o.X)-r.camX, float64(o.Y)-r.camY, float64(o.Z)-r.camZ
				for g := 0; g < mesh.GroupCount; g++ {
					rg := s.ranges[layer][g]
					if rg.Count == 0 || !groupVisible(g, ox, oy, oz) {
						continue
					}
					r.counts = append(r.counts, int32(rg.Count/4*6))
					r.bases = append(r.bases, int32(s.off+rg.First))
				}
			}
			r.multiDraw(reg)
		}
	}

	// Translucent: far to near, re-sorting nearby sections when the camera
	// moved, depth writes off.
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.DepthMask(false)
	gl.Uniform1f(p.loc("uAlphaCut"), 0.01)
	resorts := 0
	var cur *region
	r.counts, r.bases = r.counts[:0], r.bases[:0]
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
			r.multiDraw(cur)
			r.counts, r.bases = r.counts[:0], r.bases[:0]
		}
		cur = s.reg
		r.counts = append(r.counts, int32(s.transN/4*6))
		r.bases = append(r.bases, int32(s.tOff))
	}
	if cur != nil {
		r.multiDraw(cur)
	}
	gl.DepthMask(true)
	gl.Disable(gl.BLEND)
	gl.BindVertexArray(0)
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

func (r *Renderer) multiDraw(reg *region) {
	n := len(r.counts)
	if n == 0 {
		return
	}
	if len(r.ptrs) < n {
		r.ptrs = make([]unsafe.Pointer, n*2)
	}
	o := reg.key
	origin := mgl32.Vec3{
		float32(float64(o[0]*mesh.RegionX*16) - r.camX),
		float32(float64(o[1]*mesh.RegionY*16) - r.camY),
		float32(float64(o[2]*mesh.RegionZ*16) - r.camZ),
	}
	gl.Uniform3fv(r.terrain.loc("uRegionOrigin"), 1, &origin[0])
	gl.BindVertexArray(reg.vao)
	gl.MultiDrawElementsBaseVertex(gl.TRIANGLES, &r.counts[0], gl.UNSIGNED_INT, &r.ptrs[0], int32(n), &r.bases[0])
	r.Stats.DrawCalls++
	r.Stats.DrawRanges += n
}

// DrawBlockOutline draws a wireframe box around a block (Minecraft's
// selection outline).
func (r *Renderer) DrawBlockOutline(cam *Camera, x, y, z int) {
	const e = 0.002
	o := mgl32.Vec3{float32(float64(x)-cam.X) - e, float32(float64(y)-cam.Y) - e, float32(float64(z)-cam.Z) - e}
	m := cam.ViewProj().Mul4(mgl32.Translate3D(o[0], o[1], o[2])).Mul4(mgl32.Scale3D(1+2*e, 1+2*e, 1+2*e))
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	r.drawLines(0, 24, m, mgl32.Vec4{0, 0, 0, 0.6})
	gl.Disable(gl.BLEND)
}

// DrawCrosshair draws a screen-centre cross; aspect = width/height.
func (r *Renderer) DrawCrosshair(aspect float32) {
	const s = 0.025
	gl.Disable(gl.DEPTH_TEST)
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.ONE_MINUS_DST_COLOR, gl.ZERO)
	r.drawLines(24, 4, mgl32.Scale3D(s/aspect, s, 1), mgl32.Vec4{1, 1, 1, 1})
	gl.Disable(gl.BLEND)
	gl.Enable(gl.DEPTH_TEST)
}

func (r *Renderer) drawLines(first, n int32, mvp mgl32.Mat4, col mgl32.Vec4) {
	r.lines.use()
	gl.UniformMatrix4fv(r.lines.loc("uMVP"), 1, false, &mvp[0])
	gl.Uniform4fv(r.lines.loc("uColor"), 1, &col[0])
	gl.BindVertexArray(r.lineVAO)
	gl.LineWidth(1)
	gl.DrawArrays(gl.LINES, first, n)
	gl.BindVertexArray(0)
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
	gl.DeleteBuffers(1, &r.ebo)
}
