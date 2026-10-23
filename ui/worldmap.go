package ui

import (
	"fmt"
	"image"
	"math"
)

// MapMarker is a point on the world map: the player, an NPC, a place.
type MapMarker struct {
	X, Z  float64 // world blocks
	Icon  *Sprite // nil: a small square in Color
	Color uint32
	Label string // tooltip on hover
}

// Waypoint is a player-set mark.
type Waypoint struct {
	Name string
	X, Z float64
}

// WorldMapScreen shows a map image the game renders (map pixel = Scale
// blocks, pixel 0,0 = world OriginX,OriginZ), pannable with the left mouse,
// zoomable with the wheel, with markers and waypoints; a right click asks
// to set a waypoint (OnWaypoint), clicking a waypoint's mark asks to
// remove it (OnRemove).
type WorldMapScreen struct {
	Title            string
	Map              Texture
	OriginX, OriginZ float64
	Scale            float64 // blocks per map pixel
	Markers          func() []MapMarker
	Waypoints        func() []Waypoint
	WaypointIcon     *Sprite
	OnWaypoint       func(x, z float64)
	OnRemove         func(i int)
	// CenterX, CenterZ: where the view starts (the player); Zoom: screen px
	// per map px (0: 1).
	CenterX, CenterZ float64
	Zoom             float64

	view     image.Rectangle
	dragging bool
	lastX    int
	lastY    int
	inited   bool
}

func (m *WorldMapScreen) Init(ctx *Context) {
	m.view = image.Rect(10, 22, ctx.W-10, ctx.H-20)
	if !m.inited {
		m.inited = true
		if m.Zoom == 0 {
			m.Zoom = 1
		}
		if m.Scale == 0 {
			m.Scale = 1
		}
	}
}

func (m *WorldMapScreen) Update() {}

// toScreen maps world x,z to GUI pixels.
func (m *WorldMapScreen) toScreen(x, z float64) (float64, float64) {
	cx := float64(m.view.Min.X+m.view.Max.X) / 2
	cy := float64(m.view.Min.Y+m.view.Max.Y) / 2
	return cx + (x-m.CenterX)/m.Scale*m.Zoom, cy + (z-m.CenterZ)/m.Scale*m.Zoom
}

func (m *WorldMapScreen) toWorld(sx, sy int) (float64, float64) {
	cx := float64(m.view.Min.X+m.view.Max.X) / 2
	cy := float64(m.view.Min.Y+m.view.Max.Y) / 2
	return m.CenterX + (float64(sx)-cx)/m.Zoom*m.Scale, m.CenterZ + (float64(sy)-cy)/m.Zoom*m.Scale
}

func (m *WorldMapScreen) Draw(ctx *Context, mx, my int, partial float32) {
	c, f := ctx.Canvas, ctx.Font
	ctx.DrawBackground()
	v := m.view
	DrawRect(c, v.Min.X-1, v.Min.Y-1, v.Max.X+1, v.Max.Y+1, 0xFF000000)
	c.SetScissor(v)
	if m.Map != nil {
		w, h := m.Map.Size()
		x0, y0 := m.toScreen(m.OriginX, m.OriginZ)
		x1, y1 := m.toScreen(m.OriginX+float64(w)*m.Scale, m.OriginZ+float64(h)*m.Scale)
		// Draw only the visible part, so huge maps stay exact in float32.
		vx0, vy0 := math.Max(x0, float64(v.Min.X)), math.Max(y0, float64(v.Min.Y))
		vx1, vy1 := math.Min(x1, float64(v.Max.X)), math.Min(y1, float64(v.Max.Y))
		if vx1 > vx0 && vy1 > vy0 {
			u := func(sx float64) float32 { return float32((sx - x0) / (x1 - x0)) }
			t := func(sy float64) float32 { return float32((sy - y0) / (y1 - y0)) }
			c.Quad(Quad{Tex: m.Map, X0: float32(vx0), Y0: float32(vy0), X1: float32(vx1), Y1: float32(vy1),
				U0: u(vx0), V0: t(vy0), U1: u(vx1), V1: t(vy1), Top: 0xFFFFFFFF, Bottom: 0xFFFFFFFF})
		}
	}
	var hover []string
	near := func(sx, sy float64) bool { return math.Abs(sx-float64(mx)) <= 4 && math.Abs(sy-float64(my)) <= 4 }
	if m.Waypoints != nil {
		for _, w := range m.Waypoints() {
			sx, sy := m.toScreen(w.X, w.Z)
			if m.WaypointIcon != nil {
				m.WaypointIcon.Draw(c, float32(sx)-m.WaypointIcon.W/2, float32(sy)-m.WaypointIcon.H/2)
			} else {
				DrawRect(c, int(sx)-2, int(sy)-2, int(sx)+2, int(sy)+2, 0xFF000000)
				DrawRect(c, int(sx)-1, int(sy)-1, int(sx)+1, int(sy)+1, 0xFFFFFF55)
			}
			if near(sx, sy) {
				hover = append(hover, w.Name)
			}
		}
	}
	if m.Markers != nil {
		for _, mk := range m.Markers() {
			sx, sy := m.toScreen(mk.X, mk.Z)
			if mk.Icon != nil {
				mk.Icon.Draw(c, float32(sx)-mk.Icon.W/2, float32(sy)-mk.Icon.H/2)
			} else {
				col := mk.Color
				if col == 0 {
					col = 0xFFFFFFFF
				}
				DrawRect(c, int(sx)-2, int(sy)-2, int(sx)+2, int(sy)+2, 0xFF000000)
				DrawRect(c, int(sx)-1, int(sy)-1, int(sx)+1, int(sy)+1, col)
			}
			if mk.Label != "" && near(sx, sy) {
				hover = append(hover, mk.Label)
			}
		}
	}
	c.SetScissor(image.Rectangle{})
	f.DrawCentered(c, ctx.T(m.Title), ctx.W/2, 8, 0xFFFFFF)
	if inside(mx, my, v.Min.X, v.Min.Y, v.Dx(), v.Dy()) {
		wx, wz := m.toWorld(mx, my)
		s := fmt.Sprintf("%d, %d", int(math.Floor(wx)), int(math.Floor(wz)))
		f.Draw(c, s, v.Min.X, v.Max.Y+5, 0xFFFFFF, true)
	}
	if len(hover) > 0 {
		DrawTooltip(ctx, hover, mx, my)
	}
}

func (m *WorldMapScreen) waypointAt(x, y int) int {
	if m.Waypoints == nil {
		return -1
	}
	for i, w := range m.Waypoints() {
		sx, sy := m.toScreen(w.X, w.Z)
		if math.Abs(sx-float64(x)) <= 4 && math.Abs(sy-float64(y)) <= 4 {
			return i
		}
	}
	return -1
}

// zoomAt multiplies the zoom by k keeping the world point under x,y fixed.
func (m *WorldMapScreen) zoomAt(x, y int, k float64) {
	wx, wz := m.toWorld(x, y)
	m.Zoom = math.Min(math.Max(m.Zoom*k, 1.0/64), 16)
	nx, nz := m.toWorld(x, y)
	m.CenterX += wx - nx
	m.CenterZ += wz - nz
}

func (m *WorldMapScreen) Input(ctx *Context, ev Event) bool {
	in := inside(ev.X, ev.Y, m.view.Min.X, m.view.Min.Y, m.view.Dx(), m.view.Dy())
	switch ev.Type {
	case MousePress:
		if !in {
			return false
		}
		switch ev.Button {
		case 0:
			if i := m.waypointAt(ev.X, ev.Y); i >= 0 && m.OnRemove != nil && ev.Mods&ModShift != 0 {
				m.OnRemove(i)
				return true
			}
			m.dragging, m.lastX, m.lastY = true, ev.X, ev.Y
		case 1:
			if m.OnWaypoint != nil {
				m.OnWaypoint(m.toWorld(ev.X, ev.Y))
			}
		}
		return true
	case MouseMove:
		if m.dragging {
			m.CenterX -= float64(ev.X-m.lastX) / m.Zoom * m.Scale
			m.CenterZ -= float64(ev.Y-m.lastY) / m.Zoom * m.Scale
			m.lastX, m.lastY = ev.X, ev.Y
		}
		return m.dragging
	case MouseRelease:
		if m.dragging {
			m.dragging = false
			return true
		}
	case MouseScroll:
		if in {
			k := 2.0
			if ev.DY < 0 {
				k = 0.5
			}
			m.zoomAt(ev.X, ev.Y, k)
			return true
		}
	case KeyPress:
		step := 32 / m.Zoom * m.Scale
		switch ev.Key {
		case KeyLeft:
			m.CenterX -= step
		case KeyRight:
			m.CenterX += step
		case KeyUp:
			m.CenterZ -= step
		case KeyDown:
			m.CenterZ += step
		case '=', 334:
			m.zoomAt((m.view.Min.X+m.view.Max.X)/2, (m.view.Min.Y+m.view.Max.Y)/2, 2)
		case '-', 333:
			m.zoomAt((m.view.Min.X+m.view.Max.X)/2, (m.view.Min.Y+m.view.Max.Y)/2, 0.5)
		default:
			return false
		}
		return true
	}
	return false
}
