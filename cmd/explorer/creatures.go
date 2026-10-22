package main

import (
	"log"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/mobs"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/render"
)

// species is an imported model ready to spawn.
type species struct {
	model  *model.Model
	handle int
	kind   mobs.Kind
	clips  [mobs.NumStates]string // clip per mobs.State ("" = fall back to idle)
	width  float64
	height float64
	static bool       // props: stay put
	shape  *shapeInfo // for exact arrow hits, built on first use
}

type creature struct {
	ent  *app.Entity // drawn by the app; its pose follows mob
	sp   *species
	mob  *mobs.Mob
	anim *model.Animator
	skin []mgl32.Mat4
	// debugClip, when set (K key), overrides the behaviour's animation.
	debugClip string
	// attacks counts attacks (picks the next clip); attackClip is playing.
	attacks    int
	attackClip string
	// dying counts ticks since a fatal hit (0 = alive); see crossbow.go.
	dying int
	// drawn is the pose last drawn (skin is its animation), for exact
	// arrow hits against what the player saw.
	drawn                        bool
	drawX, drawY, drawZ, drawYaw float64
	drawTilt                     float64 // death tilt as drawn
	drawFade                     float32
	fallYaw                      float64     // direction a corpse falls
	drawFrame, poseFrame         int         // pose is the drawn skin, rebuilt when stale
	pose                         *model.Pose // deformed vertices for hit tests
}

// creaturesState is the explorer's creature glue: imported models →
// render.ModelRenderer, behaviour from package mobs.
type creaturesState struct {
	species    []*species
	creatures  []*creature
	spawnLeft  int
	mobRand    *rand.Rand
	lastAnim   time.Time
	animDT     float32 // animation time step this frame
	modelInsts []render.ModelInstance
	lineup     bool   // debug: species gallery in front of the player
	lineupAnim string // debug: clip forced on the gallery
}

// filterSpecies keeps only species whose name contains f.
func (g *game) filterSpecies(f string) {
	if f == "" {
		return
	}
	var keep []*species
	for _, sp := range g.species {
		if strings.Contains(sp.model.Name, f) {
			keep = append(keep, sp)
		}
	}
	g.species = keep
}

// spawnLineup places one creature per species in a row 4 blocks in front
// of the player, facing it, standing still (debug gallery).
func (g *game) spawnLineup() {
	p := g.Player
	fx, fz := math.Sin(g.Cam.Yaw), -math.Cos(g.Cam.Yaw)
	rx, rz := -fz, fx
	n := len(g.species)
	for i, sp := range g.species {
		off := (float64(i) - float64(n-1)/2) * 1.6
		x, z := p.X+fx*4+rx*off, p.Z+fz*4+rz*off
		y, _ := g.Eng.SurfaceY(int(math.Floor(x)), int(math.Floor(z)))
		m := mobs.New(mobs.Walker, x, float64(y), z, sp.width, sp.height)
		m.WalkSpeed, m.RunSpeed, m.Range = 0, 0, 0
		m.Static = true
		m.Yaw = g.Cam.Yaw + math.Pi
		m.PrevYaw = m.Yaw
		g.addCreature(&creature{sp: sp, mob: m, anim: model.NewAnimator(sp.model), debugClip: g.lineupAnim})
	}
	g.spawnLeft = 0
}

// addSpecies registers imported models. clipFor maps a behaviour state to a
// clip name for a model ("" if it has none).
func (g *game) addSpecies(ms []*model.Model, clipFor func(m *model.Model, s mobs.State) string) {
	for _, m := range ms {
		h, err := g.Models.Add(m)
		if err != nil {
			log.Print(err)
			continue
		}
		lo, hi := m.Bounds()
		sp := &species{model: m, handle: h, kind: mobs.Walker,
			width:  math.Min(0.9, math.Max(0.3, float64(max(hi[0]-lo[0], hi[2]-lo[2]))*0.8)),
			height: math.Max(0.3, float64(hi[1])),
		}
		if m.Tags["flying"] {
			sp.kind = mobs.Flyer
		}
		for s := mobs.Idle; s <= mobs.Trick; s++ { // Attack clips: combat.go
			sp.clips[s] = clipFor(m, s)
		}
		g.species = append(g.species, sp)
	}
}

// clipByName finds a clip whose name matches the state ("Walk" matches
// "Walk", "walk", "animation.mob.walk"...).
func clipByName(m *model.Model, s mobs.State) string {
	want := strings.ToLower(s.String())
	best := ""
	for _, n := range m.ClipNames() {
		ln := strings.ToLower(n)
		last := ln[strings.LastIndex(ln, ".")+1:]
		if last == want {
			return n
		}
		if best == "" && strings.Contains(last, want) {
			best = n
		}
	}
	return best
}

func (g *game) tickCreatures() {
	if len(g.species) == 0 || !g.Ready() {
		return
	}
	p := g.Player
	if g.lineup {
		g.lineup = false
		g.spawnLineup()
	}
	for tries := 0; g.spawnLeft > 0 && tries < 8; tries++ {
		a := g.mobRand.Float64() * 2 * math.Pi
		r := 4 + g.mobRand.Float64()*16
		x, z := p.X+math.Cos(a)*r, p.Z+math.Sin(a)*r
		if !g.Eng.ChunkReady(voxel.ChunkPos{X: int(math.Floor(x)) >> 4, Z: int(math.Floor(z)) >> 4}) {
			continue
		}
		y, ok := g.Eng.SurfaceY(int(math.Floor(x)), int(math.Floor(z)))
		if !ok {
			continue
		}
		sp := g.species[(g.spawnLeft)%len(g.species)]
		fy := float64(y)
		if sp.kind == mobs.Flyer {
			fy += 2 + g.mobRand.Float64()*3
		}
		m := mobs.New(sp.kind, x, fy, z, sp.width, sp.height)
		if sp.static {
			m.WalkSpeed, m.RunSpeed, m.Range = 0, 0, 0
			m.Static = true
		}
		m.Yaw = g.mobRand.Float64() * 2 * math.Pi
		if c := sp.model.Clips[sp.clips[mobs.Trick]]; c != nil {
			m.TrickTicks = int(c.Length * 20)
		}
		cr := &creature{sp: sp, mob: m, anim: model.NewAnimator(sp.model)}
		cr.anim.Time = g.mobRand.Float32() * 5
		g.addCreature(cr)
		g.spawnLeft--
	}
	all := make([]*mobs.Mob, 0, len(g.creatures)+1)
	g.aimHostiles()
	g.tickDying()
	for _, c := range g.creatures {
		if c.dying == 0 {
			c.mob.Tick(g.Terrain, g.mobRand)
		}
		all = append(all, c.mob)
	}
	// The player shoves mobs aside but isn't shoved back.
	all = append(all, &mobs.Mob{X: p.X, Y: p.Y, Z: p.Z, Width: p.Width, Height: p.Height, Static: true})
	g.applyStrikes()
	mobs.Separate(all, g.Terrain)
	for _, c := range g.creatures { // the app interpolates and draws the entity
		e, m := c.ent, c.mob
		e.X, e.Y, e.Z, e.Yaw = m.X, m.Y, m.Z, m.Yaw
		e.PrevX, e.PrevY, e.PrevZ, e.PrevYaw = m.PrevX, m.PrevY, m.PrevZ, m.PrevYaw
	}
}

// addCreature adds a creature and its app entity.
func (g *game) addCreature(c *creature) {
	m := c.mob
	c.ent = g.Entities.Add(&app.Entity{X: m.X, Y: m.Y, Z: m.Z, Yaw: m.Yaw, Width: m.Width, Height: m.Height,
		Model: c.sp.handle, Data: c, Pose: g.poseCreature})
	g.creatures = append(g.creatures, c)
}

// cycleAnimation (K) steps the nearest creature through all its clips,
// then back to normal behaviour.
func (g *game) cycleAnimation() {
	var best *creature
	bd := 1e9
	for _, c := range g.creatures {
		if d := math.Hypot(c.mob.X-g.Player.X, c.mob.Z-g.Player.Z); d < bd {
			best, bd = c, d
		}
	}
	if best == nil {
		return
	}
	names := best.sp.model.ClipNames()
	next := ""
	for i, n := range names {
		if n == best.debugClip && i+1 < len(names) {
			next = names[i+1]
		}
	}
	if best.debugClip == "" && len(names) > 0 {
		next = names[0]
	}
	best.debugClip = next
	g.debugMsg = best.sp.model.Name + ": " + next
	if next == "" {
		g.debugMsg = best.sp.model.Name + ": behaviour"
	}
}

// animStep sets this frame's animation time step (once per frame, before
// the entities are posed).
func (g *game) animStep() {
	now := time.Now()
	g.animDT = float32(now.Sub(g.lastAnim).Seconds())
	if g.lastAnim.IsZero() || g.animDT > 0.25 {
		g.animDT = 0
	}
	g.lastAnim = now
}

// poseCreature animates a creature's entity for drawing (app.Entity.Pose)
// and remembers the drawn pose for exact arrow hits.
func (g *game) poseCreature(e *app.Entity, alpha float64, in *render.ModelInstance) bool {
	c := e.Data.(*creature)
	if c.dying == 0 || c.skin == nil { // the dead keep their last pose
		clip := c.debugClip
		if clip == "" {
			clip = g.combatClip(c)
		}
		if clip == "" {
			clip = c.sp.clips[c.mob.State]
			if clip == "" {
				clip = c.sp.clips[mobs.Idle]
			}
		}
		if clip == "bind" { // debug: no animation
			c.anim.Clip = nil
		} else if clip != "" {
			c.anim.Play(clip, false)
		}
		c.anim.Update(g.animDT)
		if c.debugClip != "" && c.anim.Done() {
			c.anim.Play(clip, true) // loop one-shots while previewing
		}
		c.skin = c.anim.Skin(c.skin)
	}
	c.drawn, c.drawX, c.drawY, c.drawZ, c.drawYaw = true, in.X, in.Y, in.Z, in.Yaw
	c.drawTilt, c.drawFade = c.deathTilt(alpha), c.deathFade(alpha)
	c.drawFrame++
	in.Tilt, in.TiltYaw, in.Fade, in.Skin = c.drawTilt, c.fallYaw, c.drawFade, c.skin
	return true
}
