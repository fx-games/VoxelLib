package app

import (
	"image"
	"math"

	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/render"
)

// Entity is anything that moves in the world and is drawn with a model:
// mobs, NPCs, mounts, props. The app keeps the previous tick's pose for
// interpolation, ticks it and draws it with App.Models.
type Entity struct {
	ID                  int
	X, Y, Z             float64 // feet centre
	PrevX, PrevY, PrevZ float64
	VX, VY, VZ          float64 // blocks per tick, for the game's physics
	Yaw, Pitch          float64 // radians, as render.ModelInstance
	PrevYaw, PrevPitch  float64
	Width, Height       float64 // box for picking (0 width: not pickable)
	// Bounds, when set, is the pick box in place of Width x Height: a flat
	// hanging entity's, which may be inside out along its thin axis until
	// the player's pick border grows it.
	Bounds *collide.AABB

	Model int // render.ModelRenderer handle in App.Models; -1 = not drawn
	// Tick runs each tick after the previous pose was saved.
	Tick func(e *Entity)
	// Pose runs before drawing, with the interpolated pose already in in
	// (X, Y, Z, Yaw, Pitch, light): animate (in.Skin), tilt, fade, tint.
	// Return false to skip drawing this frame.
	Pose func(e *Entity, alpha float64, in *render.ModelInstance) bool
	// Data is the game's own state.
	Data    any
	Removed bool // set to remove it at the end of the tick
}

// Box is the entity's collision box.
func (e *Entity) Box() collide.AABB {
	if e.Bounds != nil {
		return *e.Bounds
	}
	return collide.Box(e.X, e.Y, e.Z, e.Width, e.Height)
}

// Lerp interpolates the pose between the last two ticks (yaw the short
// way round).
func (e *Entity) Lerp(alpha float64) (x, y, z, yaw, pitch float64) {
	return e.PrevX + (e.X-e.PrevX)*alpha, e.PrevY + (e.Y-e.PrevY)*alpha, e.PrevZ + (e.Z-e.PrevZ)*alpha,
		lerpAngle(e.PrevYaw, e.Yaw, alpha), lerpAngle(e.PrevPitch, e.Pitch, alpha)
}

// Teleport moves the entity without interpolating from its old place
// (story NPCs should walk; this is for spawning and loading).
func (e *Entity) Teleport(x, y, z float64) {
	e.X, e.Y, e.Z = x, y, z
	e.PrevX, e.PrevY, e.PrevZ = x, y, z
}

func lerpAngle(a, b, t float64) float64 {
	d := math.Remainder(b-a, 2*math.Pi)
	return a + d*t
}

// Entities is the entity registry.
type Entities struct {
	List   []*Entity
	nextID int
	insts  []render.ModelInstance
}

// Add registers e (its previous pose starts at the current one) and
// gives it an ID.
func (es *Entities) Add(e *Entity) *Entity {
	es.nextID++
	e.ID = es.nextID
	e.PrevX, e.PrevY, e.PrevZ, e.PrevYaw, e.PrevPitch = e.X, e.Y, e.Z, e.Yaw, e.Pitch
	es.List = append(es.List, e)
	return e
}

// ByID finds an entity, or nil.
func (es *Entities) ByID(id int) *Entity {
	for _, e := range es.List {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// tick saves each previous pose, ticks, and drops removed entities.
func (es *Entities) tick() {
	for _, e := range es.List {
		e.PrevX, e.PrevY, e.PrevZ, e.PrevYaw, e.PrevPitch = e.X, e.Y, e.Z, e.Yaw, e.Pitch
		if e.Tick != nil && !e.Removed {
			e.Tick(e)
		}
	}
	es.sweep()
}

func (es *Entities) sweep() {
	live := es.List[:0]
	for _, e := range es.List {
		if !e.Removed {
			live = append(live, e)
		}
	}
	clear(es.List[len(live):])
	es.List = live
}

// buildEntities poses every entity with a model within the render
// distance for this frame's views (drawWorld).
func (a *App) buildEntities(alpha float64) {
	es := &a.Entities
	es.insts = es.insts[:0]
	if len(es.List) == 0 {
		return
	}
	far := float64(a.Eng.Renderer.Opt.RenderDistance * 16)
	for _, e := range es.List {
		if e.Model < 0 || e.Removed {
			continue
		}
		x, y, z, yaw, pitch := e.Lerp(alpha)
		if dx, dz := x-a.Cam.X, z-a.Cam.Z; dx*dx+dz*dz > far*far {
			continue
		}
		sky, blk := a.Light(x, y+e.Height/2, z)
		// Posed in place in the reused slice: a local would escape to Pose.
		es.insts = append(es.insts, render.ModelInstance{Model: e.Model, X: x, Y: y, Z: z, Yaw: yaw, Pitch: pitch, Sky: sky, Block: blk})
		if e.Pose != nil && !e.Pose(e, alpha, &es.insts[len(es.insts)-1]) {
			es.insts = es.insts[:len(es.insts)-1]
		}
	}
}

// drawWorld draws for cam what the engine's terrain pass leaves out: the
// block outline and cracks, the entities (buildEntities), the game's
// Hooks.DrawWorld and the weather; for the main view and each scope view.
func (a *App) drawWorld(cam *render.Camera, alpha float64) {
	if h, ent, ok := a.Target(); ok && ent == nil && !a.ScreenOpen() {
		if a.Hooks.SelectBox != nil {
			if b, ok := a.Hooks.SelectBox(h); ok {
				a.Eng.Renderer.DrawBoxOutline(cam, b)
			}
		} else {
			a.Eng.Renderer.DrawBlockOutline(cam, h.Block.X, h.Block.Y, h.Block.Z)
		}
	}
	a.drawDamage(cam, alpha)
	a.Models.Draw(cam, a.Entities.insts)
	if a.Hooks.DrawWorld != nil {
		a.Hooks.DrawWorld(cam, alpha)
	}
	a.Eng.Renderer.DrawWeather(cam) // Options.Weather, after particles as in 1.7
}

// DrawScopeView renders the world again with fov (vertical, radians), as
// the main view draws it (terrain, then drawWorld), into the scope view
// (render.BeginScopeView) inside clip, for a hand-pass
// HandStencil.ScopeView draw this frame. Call it from Hooks.DrawHand,
// before that hand draw.
func (a *App) DrawScopeView(fov float64, clip image.Rectangle, alpha float64) {
	if !render.BeginScopeView(clip) {
		return
	}
	cam := a.Cam
	cam.FOV = fov
	a.Eng.Renderer.RenderView(&cam)
	a.drawWorld(&cam, alpha)
	render.EndScopeView()
}
