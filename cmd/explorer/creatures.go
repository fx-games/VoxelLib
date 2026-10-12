package main

import (
	"log"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/go-gl/mathgl/mgl32"
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
	clips  [5]string // clip per mobs.State ("" = fall back to idle)
	width  float64
	height float64
	static bool // props: stay put
}

type creature struct {
	sp   *species
	mob  *mobs.Mob
	anim *model.Animator
	skin []mgl32.Mat4
	// debugClip, when set (K key), overrides the behaviour's animation.
	debugClip string
}

// creaturesState is the explorer's creature glue: imported models →
// render.ModelRenderer, behaviour from package mobs.
type creaturesState struct {
	modelRend  *render.ModelRenderer
	species    []*species
	creatures  []*creature
	spawnLeft  int
	mobRand    *rand.Rand
	lastAnim   time.Time
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
	p := g.player
	fx, fz := math.Sin(g.cam.Yaw), -math.Cos(g.cam.Yaw)
	rx, rz := -fz, fx
	n := len(g.species)
	for i, sp := range g.species {
		off := (float64(i) - float64(n-1)/2) * 1.6
		x, z := p.X+fx*4+rx*off, p.Z+fz*4+rz*off
		y, _ := g.eng.SurfaceY(int(math.Floor(x)), int(math.Floor(z)))
		m := mobs.New(mobs.Walker, x, float64(y), z, sp.width, sp.height)
		m.WalkSpeed, m.RunSpeed, m.Range = 0, 0, 0
		m.Static = true
		m.Yaw = g.cam.Yaw + math.Pi
		m.PrevYaw = m.Yaw
		cr := &creature{sp: sp, mob: m, anim: model.NewAnimator(sp.model), debugClip: g.lineupAnim}
		g.creatures = append(g.creatures, cr)
	}
	g.spawnLeft = 0
}

// ensureModelRenderer creates the model renderer on first use.
func (g *game) ensureModelRenderer() error {
	if g.modelRend != nil {
		return nil
	}
	var err error
	g.modelRend, err = render.NewModelRenderer(g.eng.Renderer)
	return err
}

// addSpecies registers imported models. clipFor maps a behaviour state to a
// clip name for a model ("" if it has none).
func (g *game) addSpecies(ms []*model.Model, clipFor func(m *model.Model, s mobs.State) string) {
	if err := g.ensureModelRenderer(); err != nil {
		log.Printf("models disabled: %v", err)
		return
	}
	for _, m := range ms {
		h, err := g.modelRend.Add(m)
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
		for s := mobs.Idle; s <= mobs.Trick; s++ {
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
	if len(g.species) == 0 || g.frozen || g.findGround {
		return
	}
	p := g.player
	if g.lineup {
		g.lineup = false
		g.spawnLineup()
	}
	for tries := 0; g.spawnLeft > 0 && tries < 8; tries++ {
		a := g.mobRand.Float64() * 2 * math.Pi
		r := 4 + g.mobRand.Float64()*16
		x, z := p.X+math.Cos(a)*r, p.Z+math.Sin(a)*r
		if !g.eng.ChunkReady(voxel.ChunkPos{X: int(math.Floor(x)) >> 4, Z: int(math.Floor(z)) >> 4}) {
			continue
		}
		y, ok := g.eng.SurfaceY(int(math.Floor(x)), int(math.Floor(z)))
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
		g.creatures = append(g.creatures, cr)
		g.spawnLeft--
	}
	all := make([]*mobs.Mob, 0, len(g.creatures)+1)
	for _, c := range g.creatures {
		c.mob.Tick(g.terrain, g.mobRand)
		all = append(all, c.mob)
	}
	// The player shoves mobs aside but isn't shoved back.
	all = append(all, &mobs.Mob{X: p.X, Y: p.Y, Z: p.Z, Width: p.Width, Height: p.Height, Static: true})
	mobs.Separate(all, g.terrain)
}

// cycleAnimation (K) steps the nearest creature through all its clips,
// then back to normal behaviour.
func (g *game) cycleAnimation() {
	var best *creature
	bd := 1e9
	for _, c := range g.creatures {
		if d := math.Hypot(c.mob.X-g.player.X, c.mob.Z-g.player.Z); d < bd {
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

func (g *game) drawCreatures(alpha float64) {
	if len(g.creatures) == 0 {
		return
	}
	now := time.Now()
	dt := float32(now.Sub(g.lastAnim).Seconds())
	if g.lastAnim.IsZero() || dt > 0.25 {
		dt = 0
	}
	g.lastAnim = now
	g.modelInsts = g.modelInsts[:0]
	for _, c := range g.creatures {
		x, y, z, yaw := c.mob.Lerp(alpha)
		if math.Hypot(x-g.cam.X, z-g.cam.Z) > float64(g.eng.Renderer.Opt.RenderDistance*16) {
			continue
		}
		clip := c.debugClip
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
		c.anim.Update(dt)
		if c.debugClip != "" && c.anim.Done() {
			c.anim.Play(clip, true) // loop one-shots while previewing
		}
		c.skin = c.anim.Skin(c.skin)
		sky, blk := g.light(x, y+c.sp.height/2, z)
		g.modelInsts = append(g.modelInsts, render.ModelInstance{Model: c.sp.handle, X: x, Y: y, Z: z, Yaw: yaw, Skin: c.skin, Sky: sky, Block: blk})
	}
	g.modelRend.Draw(&g.cam, g.modelInsts)
}
