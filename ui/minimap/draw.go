package minimap

import (
	"fmt"
	"math"
	"time"

	"github.com/memmaker/voxellib/ui"
)

// maxPerKind is the reference's entity cap per kind (entityAmount 1).
const maxPerKind = 100

// fbSide is the side of the reference's scaling and rotation frame
// buffers in their pixels.
const fbSide = 512

// origin is the map's top-left corner in units for a GUI scale: the
// reference's (int)(x * mapScale) + 9 with mapScale = scale / minimapScale
// in float (X, Y in GUI px).
func (c *Config) origin(guiScale int, unit float64) (int, int) {
	ms := float32(guiScale) / float32(unit)
	return int(float32(c.X)*ms) + 9, int(float32(c.Y)*ms) + 9
}

// Draw draws the map, its frame, dots, compass, waypoints, crosshair and
// coordinates for the view Update last followed, as MinimapFBORenderer
// does: the map texture scaled by the zoom into a 512 px picture (nearest,
// bilinear while the zoom moves), that one turned and moved by the
// player's place inside the block into a second (bilinear with
// AntiAliasing), the dots and compass drawn on it, and its view square
// drawn bilinearly onto the screen at whole units. A map picture pixel is
// unit/2 screen px. The canvas must implement ui.Targets.
func (m *Map) Draw(ctx *ui.Context, v *View) {
	tc, ok := ctx.Canvas.(ui.Targets)
	if m.img == nil || !ok {
		return
	}
	if m.dirty || m.tex == nil {
		m.tex = ui.UpdateTexture(ctx.Canvas, m.tex, m.img)
		m.dirty = false
	}
	if m.scal == nil {
		m.scal, m.rot = tc.NewTarget(fbSide, fbSide), tc.NewTarget(fbSide, fbSide)
	}
	c := ctx.Canvas
	sc := float64(ctx.Scale)
	u := m.unit
	f := u / sc       // GUI px per unit
	half := m.mapHalf // the map's side in units
	viewW := 2 * half // ... in map-picture px
	zoom := m.animZoom(time.Now())
	zooming := float64(int(zoom)) != zoom

	locked := m.Cfg.LockNorth || half > 180
	angle := 90.0
	if !locked {
		angle = -90 - float64(v.Yaw)
	}
	a := angle * math.Pi / 180
	ps, pc := math.Sin(math.Pi-a), math.Cos(math.Pi-a)
	white := uint32(0xFFFFFFFF)

	// The scaling buffer: black, then the map with the player's block's
	// corner at 256, 256 (its texture coordinates taken modulo the store).
	xf, zf := math.Floor(v.X), math.Floor(v.Z)
	n := float64(m.n)
	ux, uz := float64(int(xf)&(m.n-1)), float64(int(zf)&(m.n-1))
	r := fbSide / 2 / zoom
	tc.SetTarget(m.scal, 0xFF000000)
	c.Quad(ui.Quad{Tex: m.tex, X1: fbSide, Y1: fbSide,
		U0: float32((ux - r) / n), V0: float32((uz - r) / n), U1: float32((ux + r) / n), V1: float32((uz + r) / n),
		Top: white, Bottom: white, Linear: zooming})

	// The rotation buffer: the scaling buffer turned about the view's
	// centre, moved by the player's place inside the block.
	hw := float64(viewW) / 2
	tc.SetTarget(m.rot, 0)
	x0, y0 := hw-fbSide/2-(v.X-xf)*zoom, hw-fbSide/2-(v.Z-zf)*zoom
	q := ui.Quad{Tex: m.scal, X0: float32(x0), Y0: float32(y0), X1: float32(x0 + fbSide), Y1: float32(y0 + fbSide),
		U1: 1, V1: 1, Top: white, Bottom: white, Linear: m.Cfg.AntiAliasing}
	if !locked {
		q.Rot, q.RX, q.RY = float32(math.Atan2(pc, ps)*180/math.Pi), float32(hw), float32(hw)
	}
	c.Quad(q)
	sheet := ctx.Tex(m.Cfg.Sheet)
	m.drawDots(c, sheet, v, hw, hw, 1, zoom, ps, pc)
	if m.Cfg.Compass {
		m.drawCompass(ctx, hw, hw, 1, ps, pc, float64(viewW/2-7-3))
	}
	tc.SetTarget(nil, 0)

	// Its view square onto the screen, half the view in units.
	ox, oy := m.Cfg.origin(ctx.Scale, u)
	sx, sy := ox-9, oy-9
	gx, gy := float64(ox)*f, float64(oy)*f
	side := float64(half) * f
	t := float32(viewW) / fbSide
	c.Quad(ui.Quad{Tex: m.rot, X0: float32(gx), Y0: float32(gy), X1: float32(gx + side), Y1: float32(gy + side),
		U1: t, V1: t, Top: white, Bottom: white, Linear: true})

	cx, cy := gx+side/2, gy+side/2
	m.drawFrame(c, sheet, f, sx, sy, half)
	m.drawWaypoints(ctx, v, cx, cy, u, sc, zoom, ps, pc, float64(int(float64(half)*u/2))+3*u)
	if !locked {
		m.drawCrosshair(c, cx, cy, f/2)
	}
	if m.Cfg.ShowCoords {
		size := half + 18
		bx, by, bz := int(math.Floor(v.X)), int(math.Floor(v.Y)), int(math.Floor(v.Z))
		lines := []string{fmt.Sprintf("%d, %d, %d", bx, by, bz)}
		if ctx.Font.Width(lines[0]) >= size {
			lines = []string{fmt.Sprintf("%d, %d", bx, bz), fmt.Sprint(by)}
		}
		for i, s := range lines {
			x := sx + size/2 - ctx.Font.Width(s)/2
			y := sy + size + i*10
			ctx.Font.DrawScaled(c, s, 0, 0, float32(f), float32(x), float32(y), 0xFFFFFFFF, true)
		}
	}
}

// drawDots is renderEntityListToFBO for the plain dots: rounded offset
// plus the remainder, an 11x11 dot, dimmed by height.
func (m *Map) drawDots(c ui.Canvas, sheet ui.Texture, v *View, cx, cy, k, zoom, ps, pc float64) {
	cull := float64(m.mapHalf + 48)
	maxDist := cull * cull / (zoom * zoom)
	for kind := KindOther; kind <= KindPlayer; kind++ {
		shown := 0
		for _, e := range v.Entities {
			if e.Kind != kind || e.Hidden {
				continue
			}
			dx, dz, dh := e.X-v.X, e.Z-v.Z, v.Y-e.Y
			if dx*dx > maxDist || dz*dz > maxDist || dh*dh > float64(m.Cfg.HeightLimit*m.Cfg.HeightLimit) {
				continue
			}
			if shown++; shown > maxPerKind {
				break
			}
			tx := ps*dx*zoom - pc*dz*zoom
			ty := pc*dx*zoom + ps*dz*zoom
			col := m.entityColour(m.Cfg.KindColour[kind], float32(dh))
			x, y := cx+(tx-5.5)*k, cy+(ty-5.5)*k
			c.Quad(ui.Quad{Tex: sheet, X0: float32(x), Y0: float32(y), X1: float32(x + 11*k), Y1: float32(y + 11*k),
				U0: 0, V0: 117. / 256, U1: 11. / 256, V1: 128. / 256, Top: col, Bottom: col, Linear: m.Cfg.SmoothDots})
		}
	}
}

// entityColour is MinimapRadar.getEntityColour's height dimming.
func (m *Map) entityColour(col uint32, offh float32) uint32 {
	hl := float32(m.Cfg.HeightLimit)
	level := max(hl-offh, 0)
	b := float32(1)
	if level <= float32(m.Cfg.HeightLimit/2) && m.Cfg.EntityDepth {
		b = 0.25 + 0.5*level/hl
	}
	if b >= 1 {
		return col
	}
	r, g, bl, a := int(col>>16&255), int(col>>8&255), int(col&255), 255
	if m.caving != -1 {
		a = int(float32(a) * b)
	} else {
		r, g, bl = int(float32(r)*b), int(float32(g)*b), int(float32(bl)*b)
	}
	return uint32(a)<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(bl)
}

// edge is WaypointsGuiRenderer.translatePosition for a square: the offset
// clamped to the square of half side spec, rounded, minus one.
func edge(spec, ps, pc, offx, offy, zoom float64) (float64, float64) {
	y := (pc*offx + ps*offy) * zoom
	x := (ps*offx - pc*offy) * zoom
	bx, by := x, y
	if x > spec {
		bx, by = spec, y*spec/x
	} else if x < -spec {
		bx, by = -spec, -y*spec/x
	}
	if by > spec {
		by, bx = spec, x*spec/y
	} else if by < -spec {
		by, bx = -spec, -x*spec/y
	}
	return math.Round(bx) - 1, math.Round(by) - 1
}

// drawCompass is drawCompass with the background, in map-picture px.
func (m *Map) drawCompass(ctx *ui.Context, cx, cy, k, ps, pc, spec float64) {
	col := m.Cfg.CompassRGB & 0xFFFFFF
	for i, key := range m.Cfg.CompassKeys {
		s := ctx.T(key)
		var offx, offy float64
		switch i {
		case 0:
			offy = -10000
		case 1:
			offx = 10000
		case 2:
			offy = 10000
		case 3:
			offx = -10000
		}
		px, py := edge(spec, ps, pc, offx, offy, 1)
		halfW := ctx.Font.Width(s)/2 - 1
		add := float64(max(halfW-3, 0))
		x0, y0 := cx+px*k, cy+py*k
		ctx.Canvas.Quad(ui.Quad{X0: float32(x0 + (-4-add)*k), Y0: float32(y0 + (-4-add)*k), X1: float32(x0 + (5+add)*k), Y1: float32(y0 + (5+add)*k),
			Top: 0x90000000 | col, Bottom: 0x90000000 | col})
		ctx.Font.DrawScaled(ctx.Canvas, s, float32(x0), float32(y0), float32(k), float32(-halfW+1), -2, 0xFF000000|col, false)
		ctx.Font.DrawScaled(ctx.Canvas, s, float32(x0), float32(y0), float32(k), float32(-halfW), -3, 0xFFFFFFFF, false)
	}
}

// drawFrame is the square frame from the sheet (minimapFrame 0), in units.
func (m *Map) drawFrame(c ui.Canvas, sheet ui.Texture, f float64, sx, sy, half int) {
	part := func(x, y, u, v, w, h int) {
		ui.DrawTexScaled(c, sheet, float32(float64(x)*f), float32(float64(y)*f), float32(float64(w)*f), float32(float64(h)*f),
			float32(u), float32(v), float32(w), float32(h), 256, 256, 0xFFFFFFFF)
	}
	right := sx + 9 + half + 4 - 17
	bottom := sy + 9 + half + 4 - 15
	part(sx+9-4, sy+9-4, 0, 0, 17, 15)
	part(right, sy+9-4, 0, 15, 17, 15)
	part(sx+9-4, bottom, 0, 30, 17, 15)
	part(right, bottom, 0, 45, 17, 15)
	start := sx + 9 - 4 + 17
	nh := int(math.Ceil(float64(right-start) / 16))
	for i := 0; i < nh; i++ {
		x, w := start+i*16, 16
		if i == nh-1 && x+w > right {
			w = right - x
		}
		part(x, sy+9-4, 0, 60, w, 4)
		part(x, sy+9-4+half+9-5, 0, 64, w, 4)
	}
	vstart := sy + 9 - 4 + 15
	nv := int(math.Ceil(float64(bottom-vstart) / 5))
	for i := 0; i < nv; i++ {
		y, h := vstart+i*5, 5
		if i == nv-1 && y+h > bottom {
			h = bottom - y
		}
		part(sx+9-4, y, 0, 68, 4, h)
		part(sx+9-4+half+9-5, y, 0, 73, 4, h)
	}
}

// drawWaypoints is renderWaypointsList: a box with the symbol, clamped to
// just outside the map, in screen px around the centre.
func (m *Map) drawWaypoints(ctx *ui.Context, v *View, cx, cy, u, sc, zoom, ps, pc, spec float64) {
	for _, w := range v.Waypoints {
		px, py := edge(spec, ps, pc, float64(w.X)+0.5-v.X, float64(w.Z)+0.5-v.Z, zoom*u/2)
		x0, y0 := cx+px/sc, cy+py/sc
		g := u / sc
		j := ctx.Font.Width(w.Symbol) / 2
		add := float64(max(j-4, 0))
		col := uint32(v.WaypointsA)<<24 | w.RGB&0xFFFFFF
		ctx.Canvas.Quad(ui.Quad{X0: float32(x0 + (-4-add)*g), Y0: float32(y0 + (-4-add)*g), X1: float32(x0 + (5+add)*g), Y1: float32(y0 + (5+add)*g), Top: col, Bottom: col})
		ctx.Font.DrawScaled(ctx.Canvas, w.Symbol, float32(x0), float32(y0), float32(g), float32(1-j), -3, 0xFFFFFFFF, true)
	}
}

// drawCrosshair is the view-following map's centre mark: an inverted
// cross, then the player-coloured plus, in half units (h GUI px each).
func (m *Map) drawCrosshair(c ui.Canvas, cx, cy, h float64) {
	rect := func(x0, y0, x1, y1 float64, col uint32) {
		c.Quad(ui.Quad{X0: float32(cx + x0*h), Y0: float32(cy + y0*h), X1: float32(cx + x1*h), Y1: float32(cy + y1*h), Top: col, Bottom: col})
	}
	c.SetInvert(true)
	rect(-5, -1, 5, 1, 0xFFFFFFFF)
	rect(-1, 3, 1, 5, 0xFFFFFFFF)
	rect(-1, -5, 1, -3, 0xFFFFFFFF)
	c.SetInvert(false)
	col := 0xFF000000 | m.Cfg.PlayerRGB
	rect(1, -1, 3, 1, col)
	rect(-3, -1, -1, 1, col)
	rect(-1, 1, 1, 3, col)
	rect(-1, -3, 1, -1, col)
}
