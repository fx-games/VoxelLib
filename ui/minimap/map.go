package minimap

import (
	"image"
	"math"
	"time"

	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxellib/voxel"
)

// Config is the minimap's settings, the reference's names in brackets.
type Config struct {
	X, Y      int       // top-left corner in GUI px (the minimap interface's position)
	Zooms     []float64 // blocks are Zooms[Zoom] screen px (zooms, zoom)
	Zoom      int
	Size      int // half the map's side in its units, 0 auto (minimapSize)
	LockNorth bool
	CaveMaps  int // 0 off, else the cave roof's radius + 1 (caveMaps)
	CaveZoom  int // the cave view zooms by 1+CaveZoom (caveZoom)
	CaveDepth int // blocks below the player the cave view shows (caveMapsDepth)
	Lighting  bool
	Slopes    int // 0 off, 1 two-tone, 2 lit (terrainSlopes)
	Shadow    [3]float32
	// The map turns with bilinear sampling (antiAliasing); the dots are
	// sampled bilinearly (smoothDots).
	AntiAliasing, SmoothDots bool
	ShowCoords               bool
	Compass                  bool   // letters on the map's edge with a background (compassLocation 1)
	CompassRGB               uint32 // 0xRRGGBB
	CompassKeys              [4]string
	HeightLimit              int  // entities further above or below are not shown (heightLimit)
	EntityDepth              bool // dim entities below or above (showEntityHeight)
	// Colours of the entity kinds (0xAARRGGBB), by Entity.Kind.
	KindColour [5]uint32
	PlayerRGB  uint32 // the crosshair's colour
	// Sheet is the GUI sheet with the frame, the dots (u 0, v 117, 11x11)
	// and the crosshair-less arrow; addressed as 256x256.
	Sheet string
}

// Entity kinds, in the reference's draw order.
const (
	KindOther   = 0
	KindItem    = 1
	KindLiving  = 2
	KindHostile = 3
	KindPlayer  = 4
)

// Entity is a dot on the map.
type Entity struct {
	X, Y, Z float64 // interpolated position (Y: the feet)
	Kind    int
	Hidden  bool // sneaking or invisible
}

// Waypoint is a coloured mark with a symbol.
type Waypoint struct {
	X, Z   int
	Symbol string
	RGB    uint32
}

// View is what the map follows this frame.
type View struct {
	X, Y, Z    float64 // the camera entity, interpolated (Y: the feet)
	Yaw        float32
	Sun        float32 // the sun brightness factor, 0..1
	Entities   []Entity
	Waypoints  []Waypoint
	WaypointsA uint8 // waypoint opacity on the map
}

// chunksPerFrame is how many chunks Update rewrites per call.
const chunksPerFrame = 4

// Map is one minimap.
type Map struct {
	Cfg Config
	Src *Source

	n         int // store side in blocks, a power of two
	surf      []surfPixel
	lit       []uint32 // surface: alpha<<24 | rgb
	cave      []cavePixel
	tags      []voxel.ChunkPos
	have      []bool
	cavL      []int  // the cave level a chunk was written at
	retry     []bool // a cave chunk read a neighbour not stored for its level
	caveMode  bool
	img       *image.NRGBA
	tex       ui.Texture
	scal, rot ui.Texture // the reference's scaling and rotation frame buffers
	dirty     bool
	day       uint8 // daylight the image holds, in 1/255 steps (0 none yet)
	dayF      float32
	level     int
	caving    int
	cursor    int
	zoom      float64
	zoomTime  time.Time
	mapHalf   int     // half the side (units) the store was sized for
	unit      float64 // screen px per map unit (minimapScale)
	lastFrame time.Time
}

func (m *Map) target() float64 {
	z := m.Cfg.Zooms[min(m.Cfg.Zoom, len(m.Cfg.Zooms)-1)]
	if m.Cfg.CaveZoom > 0 && m.caving != -1 {
		z *= float64(1 + m.Cfg.CaveZoom)
	}
	return min(z, m.Cfg.Zooms[len(m.Cfg.Zooms)-1])
}

// animZoom is MinimapProcessor.updateZoom: the zoom closes 20% of the gap
// per 1/60 s.
func (m *Map) animZoom(now time.Time) float64 {
	t := m.target()
	if m.zoom == 0 {
		m.zoom = t
	}
	off := t - m.zoom
	if off > 0.01 || off < -0.01 {
		off = float64(float32(off * math.Pow(0.8, float64(now.Sub(m.zoomTime))/float64(time.Second/60))))
	} else {
		off = 0
	}
	m.zoom = t - off
	m.zoomTime = now
	return m.zoom
}

// sizes is the reference's minimap scale (screen px per unit) and half
// map side in units for a fbW x fbH screen.
func (c *Config) sizes(fbW, fbH int) (unit float64, half int) {
	s := min(fbW, fbH)
	unit = 2
	if s >= 1500 {
		unit = float64(s / 500)
	}
	half = c.Size
	if half <= 0 {
		size := int(float64(s) / unit)
		half = min(max(55, 2*size*130/1080), 250)
	}
	return unit, half
}

func (m *Map) resize(half int, unit float64) {
	// The view turns: its diagonal must fit, with a chunk to spare
	// each way and the zoomed-out end.
	side := float64(half) * 2 * unit * math.Sqrt2 / m.Cfg.Zooms[0]
	n := 64
	for float64(n) < side+48 {
		n *= 2
	}
	m.mapHalf, m.unit = half, unit
	if n == m.n {
		return
	}
	m.n = n
	c := n / 16
	m.surf = make([]surfPixel, n*n)
	m.lit = make([]uint32, n*n)
	m.cave = make([]cavePixel, n*n)
	m.tags = make([]voxel.ChunkPos, c*c)
	m.have = make([]bool, c*c)
	m.cavL = make([]int, c*c)
	m.retry = make([]bool, c*c)
	m.img = image.NewNRGBA(image.Rect(0, 0, n, n))
	black(m.img.Pix)
	m.day = 0
}

func (m *Map) idx(wx, wz int) int { return (wz&(m.n-1))*m.n + wx&(m.n-1) }

func (m *Map) cell(cx, cz int) int {
	c := m.n / 16
	return (cz&(c-1))*c + cx&(c-1)
}

func (m *Map) written(wx, wz int) bool {
	i := m.cell(wx>>4, wz>>4)
	return m.have[i] && m.tags[i] == voxel.ChunkPos{X: wx >> 4, Z: wz >> 4}
}

// pixel implements surface for the colours.
func (m *Map) pixel(wx, wz int) *surfPixel {
	if !m.written(wx, wz) {
		return nil
	}
	return &m.surf[m.idx(wx, wz)]
}

// caveHeights is loadCave's view of the store and the world (read-locked).
func (m *Map) caveHeights(w *voxel.World) caveHeights {
	return func(wx, wz int) (int, bool, bool) {
		if m.written(wx, wz) {
			return int(m.cave[m.idx(wx, wz)].height), true, m.cavL[m.cell(wx>>4, wz>>4)] == m.caving
		}
		if c := w.ChunkUnlocked(voxel.ChunkPos{X: wx >> 4, Z: wz >> 4}); c != nil {
			return int(c.Height[(wz&15)<<4|wx&15]), false, false
		}
		return -1, false, false
	}
}

// done reports whether chunk p needs no priority write: stored, and in the
// cave view for the current level with its neighbours read (a failed one
// waits for the round robin until its north, west and north-west
// neighbours are stored for the level, as the reference retries it).
func (m *Map) done(p voxel.ChunkPos) bool {
	c := m.cell(p.X, p.Z)
	if !m.have[c] || m.tags[c] != p {
		return false
	}
	if !m.caveMode {
		return true
	}
	if m.cavL[c] != m.caving {
		return false
	}
	if !m.retry[c] {
		return true
	}
	for _, q := range [3]voxel.ChunkPos{{X: p.X, Z: p.Z - 1}, {X: p.X - 1, Z: p.Z}, {X: p.X - 1, Z: p.Z - 1}} {
		if !m.written(q.X<<4, q.Z<<4) || m.cavL[m.cell(q.X, q.Z)] != m.caving {
			return true
		}
	}
	return false
}

// Update follows the view: it finds the cave roof, rewrites a few chunks
// (unwritten ones nearest first, then round robin) and recolours the image
// when the daylight moved. Call it once per frame before Draw.
func (m *Map) Update(w *voxel.World, v *View, fbW, fbH int) {
	unit, half := m.Cfg.sizes(fbW, fbH)
	m.resize(half, unit)
	bx, by, bz := int(math.Floor(v.X)), int(math.Floor(v.Y)), int(math.Floor(v.Z))
	m.caving = -1
	if m.Cfg.CaveMaps > 0 {
		m.caving = m.Src.Caving(w, bx, by, bz, m.Cfg.CaveMaps-1)
	}
	if cave := m.caving != -1; cave != m.caveMode {
		m.caveMode = cave
		clear(m.have)
		black(m.img.Pix)
		m.dirty = true
	}
	levels := 1
	if m.Cfg.Lighting {
		levels = Levels
	}
	day := uint8(math.Round(float64(daylight(v.Sun)) * 255))
	level := int(float32(1-min(1, v.Sun)) * float32(levels-1))
	if m.caveMode && level != m.level || !m.caveMode && day != m.day {
		m.level, m.day, m.dayF = level, day, daylight(v.Sun)
		m.recolour()
	}

	// The chunk window around the player.
	r := m.n/32 - 1
	pcx, pcz := bx>>4, bz>>4
	side := 2*r + 1
	todo := make([]voxel.ChunkPos, 0, chunksPerFrame)
	w.RLock()
	for d := 0; d <= r; d++ {
		for i := -d; i <= d; i++ {
			for _, p := range [4]voxel.ChunkPos{{X: pcx + i, Z: pcz - d}, {X: pcx + i, Z: pcz + d}, {X: pcx - d, Z: pcz + i}, {X: pcx + d, Z: pcz + i}} {
				c := m.cell(p.X, p.Z)
				if m.done(p) {
					continue
				}
				if w.ChunkUnlocked(p) == nil {
					if m.have[c] { // a chunk from the far side of the wrap
						m.have[c] = false
						m.blank(p)
					}
					continue
				}
				if len(todo) < chunksPerFrame && !contains(todo, p) {
					todo = append(todo, p)
				}
			}
		}
	}
	for n := 0; len(todo) < chunksPerFrame && n < side*side; n++ {
		m.cursor = (m.cursor + 1) % (side * side)
		p := voxel.ChunkPos{X: pcx - r + m.cursor%side, Z: pcz - r + m.cursor/side}
		if !contains(todo, p) && w.ChunkUnlocked(p) != nil {
			todo = append(todo, p)
		}
	}
	for _, p := range todo {
		m.write(w, w.ChunkUnlocked(p), by, levels)
	}
	w.RUnlock()
}

func contains(a []voxel.ChunkPos, p voxel.ChunkPos) bool {
	for _, q := range a {
		if q == p {
			return true
		}
	}
	return false
}

// pixelKey is what MapBlock.equalsSlopesExcluded compares (the overlays
// aside): a written pixel that differs re-slopes its neighbours.
type pixelKey struct {
	state  voxel.StateID
	height int16
	light  uint8
	biome  int16
}

// write loads a chunk's pixels and puts their colours into the image.
func (m *Map) write(w *voxel.World, c *voxel.Chunk, playerY, levels int) {
	ci := m.cell(c.Pos.X, c.Pos.Z)
	was := m.have[ci] && m.tags[ci] == c.Pos
	m.tags[ci], m.have[ci] = c.Pos, true
	if m.caveMode {
		m.cavL[ci] = m.caving
		heights, ok := m.caveHeights(w), true
		for z := 0; z < 16; z++ {
			for x := 0; x < 16; x++ {
				i := m.idx(c.Pos.X<<4|x, c.Pos.Z<<4|z)
				if !m.Src.loadCave(c, x, z, m.caving, playerY, m.Cfg.CaveDepth, levels, m.Cfg.Slopes, m.Cfg.Shadow, heights, &m.cave[i]) {
					ok = false
				}
				m.put(i)
			}
		}
		m.retry[ci] = !ok
		return
	}
	var bottom, right [16]bool // pixel (x, 15), (15, z) changed
	x0, z0 := c.Pos.X<<4, c.Pos.Z<<4
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			px := &m.surf[m.idx(x0|x, z0|z)]
			old := pixelKey{px.state, px.height, px.light, px.biome}
			m.Src.loadSurface(c, x, z, px)
			changed := !was || old != pixelKey{px.state, px.height, px.light, px.biome}
			bottom[x] = bottom[x] || z == 15 && changed
			right[z] = right[z] || x == 15 && changed
		}
	}
	// MapWriter: slopes from the stored heights, unknown while a neighbour
	// is missing (MapPixel then takes the next pixel's).
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			px := &m.surf[m.idx(x0|x, z0|z)]
			px.slopeKnown = false
			if px.state != voxel.Air {
				fixSlopes(m, x0|x, z0|z, int(px.height), px, true)
			}
		}
	}
	for z := 0; z < 16; z++ {
		for x := 0; x < 16; x++ {
			m.colour(x0|x, z0|z)
		}
	}
	// A changed edge pixel makes the slopes of the pixels south and east
	// that read it unknown, except across a region (32 chunks) border.
	south, east := c.Pos.Z&31 != 31, c.Pos.X&31 != 31
	for x := 0; x < 16; x++ {
		if !bottom[x] || !south {
			continue
		}
		m.reslope(x0|x, z0+16)
		if x < 15 {
			m.reslope(x0|x+1, z0+16)
		} else if east {
			m.reslope(x0+16, z0+16)
		}
	}
	for z := 0; z < 15; z++ {
		if right[z] && east {
			m.reslope(x0+16, z0|z+1)
		}
	}
}

// colour recomputes a written surface pixel's colour.
func (m *Map) colour(wx, wz int) {
	i := m.idx(wx, wz)
	rgb, a := m.Src.surfaceColour(m, wx, wz, &m.surf[i], m.Cfg.Shadow)
	m.lit[i] = uint32(a)<<24 | rgb
	m.put(i)
}

// reslope forgets a written pixel's slopes and recolours it.
func (m *Map) reslope(wx, wz int) {
	if px := m.pixel(wx, wz); px != nil {
		px.slopeKnown = false
		m.colour(wx, wz)
	}
}

// black makes pixels opaque black: what the reference's frame buffer shows
// where no map texture is (it is filled black before the map is drawn).
func black(pix []byte) {
	for i := range pix {
		pix[i] = uint8(i&3/3) * 255
	}
}

// blank makes a chunk's square black.
func (m *Map) blank(p voxel.ChunkPos) {
	for z := 0; z < 16; z++ {
		i := m.idx(p.X<<4, p.Z<<4|z)
		black(m.img.Pix[i*4 : i*4+64])
	}
	m.dirty = true
}

// put writes pixel i's shown colour into the image.
func (m *Map) put(i int) {
	var rgb uint32
	if m.caveMode {
		rgb = m.cave[i].rgb[m.level]
	} else {
		rgb = shade(m.lit[i]&0xFFFFFF, uint8(m.lit[i]>>24), m.dayF)
	}
	p := m.img.Pix[i*4 : i*4+4 : i*4+4]
	if p[0] != uint8(rgb>>16) || p[1] != uint8(rgb>>8) || p[2] != uint8(rgb) || p[3] != 255 {
		p[0], p[1], p[2], p[3] = uint8(rgb>>16), uint8(rgb>>8), uint8(rgb), 255
		m.dirty = true
	}
}

func (m *Map) recolour() {
	for ci := range m.have {
		if !m.have[ci] {
			continue
		}
		cx, cz := m.tags[ci].X, m.tags[ci].Z
		for z := 0; z < 16; z++ {
			for x := 0; x < 16; x++ {
				m.put(m.idx(cx<<4|x, cz<<4|z))
			}
		}
	}
}
