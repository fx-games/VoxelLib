package main

// Combat: humanoid creatures hit by an arrow turn hostile and fight back.
// Crossbow carriers (the crossbow is part of their model) keep their
// distance and shoot real arrows; everyone else runs in and swings the
// pack's 3D sword (cut out of the model that carries one), drawn in their
// hand. The player has health, a hurt flash and knockback, and respawns
// on death. Behaviour (chase, attack timing) is mobs' Hostile mode; this
// file picks weapons and clips and applies the blows.
// Hooks elsewhere: setupCombat after the asset import, aimHostiles and
// applyStrikes around the mob ticks, combatClip when choosing animations,
// drawWeapons after the creatures, drawHUD after the world, tickPlayerHealth
// each tick, and the arrow Hit handler in crossbow.go.

import (
	"image"
	"image/color"
	"log"
	"math"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/importer/mcmodel"
	"github.com/memmaker/voxellib/mobs"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/render"
)

// swordGroup is the bone holding the 3D sword in the pack's models.
const swordGroup = "sword"

const (
	maxHealth    = 20 // half-hearts, as Minecraft
	meleeDamage  = 3
	arrowDamage  = 4
	hurtTicks    = 10 // invulnerable and flashing red after a hit
	giveUpRange  = 48 // hostiles farther than this calm down
	meleeReach   = 2.2
	rangedReach  = 14
	meleeCool    = 8
	rangedCool   = 30
	shoulderFrac = 0.72 // where arrows leave a shooter, as a share of height
)

// weapon is how a species fights.
type weapon struct {
	ranged  bool
	clips   []string // attack clips, used in turn
	ready   string   // clip while hostile and not attacking ("" = normal)
	hand    int      // bone the sword is drawn on (-1 = none)
	handOff float32  // offset down that bone to the hand
	front   float64  // how far the model reaches forward (+Z), for arrow spawns
}

type combatState struct {
	swordModel int // ModelRenderer handle; -1 = none
	weapons    map[*species]*weapon
	health     int
	hurt       int // ticks left of the hurt flash / invulnerability
	playerTgt  *playerTarget
	hud        *render.Overlay
	hudHealth  int // health drawn into hud
	flash      *render.Overlay
	weaponBuf  []render.ModelInstance
	weaponMats []mgl32.Mat4
}

// playerTarget lets arrows hit the player.
type playerTarget struct{ g *game }

func (p *playerTarget) Box() collide.AABB { return p.g.player.Box() }
func (p *playerTarget) Frame() (x, y, z, yaw float64) {
	pl := p.g.player
	return pl.X, pl.Y, pl.Z, p.g.cam.Yaw
}

// setupCombat finds the sword in an imported pack.
func (g *game) setupCombat(p *mcmodel.Pack) {
	g.health, g.hudHealth = maxHealth, -1
	g.playerTgt = &playerTarget{g}
	var sword *model.Model
	for _, m := range p.Models {
		if sword = m.Subtree(swordGroup); sword != nil {
			break
		}
	}
	if sword != nil {
		paintSword(sword)
	}
	if sword == nil || g.ensureModelRenderer() != nil {
		return
	}
	var err error
	if g.swordModel, err = g.modelRend.Add(sword); err != nil {
		log.Printf("combat: %v", err)
		g.swordModel = -1
	}
}

// paintSword points each mesh at the texture that is actually painted
// under its UVs: the legion's faces use its ranged skin, where the sword
// is left blank (ranged legions don't carry it); its melee skins have it.
func paintSword(m *model.Model) {
	for i := range m.Meshes {
		me := &m.Meshes[i]
		best, bestN := me.Texture, -1
		for k, tex := range m.Textures {
			b, n := tex.Bounds(), 0
			for _, v := range me.Verts {
				x := b.Min.X + int(v.UV[0]*float32(b.Dx()))
				y := b.Min.Y + int(v.UV[1]*float32(b.Dy()))
				if _, _, _, a := tex.At(x, y).RGBA(); a > 0 {
					n++
				}
			}
			if n > bestN {
				best, bestN = k, n
			}
		}
		me.Texture = best
	}
}

// weaponOf picks (once) how a species fights: crossbow if its model holds
// one, else the sword, swung with the clips that suit the rig.
func (g *game) weaponOf(sp *species) *weapon {
	if w, ok := g.weapons[sp]; ok {
		return w
	}
	m := sp.model
	w := &weapon{hand: -1}
	has := func(n string) bool { _, ok := m.Clips[n]; return ok }
	pick := func(names ...string) []string {
		var out []string
		for _, n := range names {
			if has(n) {
				out = append(out, n)
			}
		}
		return out
	}
	if m.Bone(crossbowGroup) >= 0 {
		w.ranged = true
		w.clips = pick("animation.merc.shoot_heavy", "animation.mob.idleshoot")
		if has("animation.mob.idleshoot") {
			w.ready = "animation.mob.idleshoot"
		}
	} else {
		name := strings.ToLower(m.Name)
		switch {
		case strings.Contains(name, "jugg"):
			w.clips = pick("animation.mob.swing1_2h", "animation.mob.swing2_2h", "animation.mob.heavy.slam")
		case strings.Contains(name, "thief"):
			w.clips = pick("animation.mob.slashone", "animation.mob.slashtwo", "animation.mob.slashthree")
		}
		if len(w.clips) == 0 {
			w.clips = pick("animation.mob.swing1", "animation.mob.slashone", "animation.mob.swing2")
		}
		// The sword goes on the hand bone, or the end of the right arm.
		if w.hand = m.Bone("itemBone"); w.hand < 0 {
			for i, b := range m.Bones {
				if n := strings.ToLower(b.Name); strings.Contains(n, "right") && strings.Contains(n, "arm") {
					w.hand, w.handOff = i, 0.6
					break
				}
			}
		}
	}
	_, hi := m.Bounds()
	w.front = float64(hi[2])
	if g.weapons == nil {
		g.weapons = map[*species]*weapon{}
	}
	g.weapons[sp] = w
	return w
}

// fights reports whether a creature fights back when shot.
func fights(c *creature) bool {
	return c.sp.model.Tags["humanoid"] && !c.sp.static && c.dying == 0
}

// provoke turns a creature against the player.
func (g *game) provoke(c *creature) {
	if !fights(c) || c.mob.Hostile {
		return
	}
	w := g.weaponOf(c.sp)
	if len(w.clips) == 0 {
		return
	}
	m := c.mob
	m.Hostile, m.Static = true, false
	if m.RunSpeed == 0 { // gallery mobs stand still until provoked
		m.WalkSpeed, m.RunSpeed = 1.12/20, 2.24/20
	}
	m.Reach, m.Cooldown = meleeReach, meleeCool
	if w.ranged {
		m.Reach, m.Cooldown = rangedReach, rangedCool
	}
}

// aimHostiles points hostiles at the player before the mobs tick, and
// calms down those that lost track.
func (g *game) aimHostiles() {
	p := g.player
	for _, c := range g.creatures {
		m := c.mob
		if !m.Hostile || c.dying > 0 {
			continue
		}
		if math.Hypot(p.X-m.X, p.Z-m.Z) > giveUpRange {
			m.Hostile = false
			m.HomeX, m.HomeY, m.HomeZ = m.X, m.Y, m.Z
			continue
		}
		m.GoalX, m.GoalY, m.GoalZ = p.X, p.Y, p.Z
	}
}

// applyStrikes starts attack clips and applies blows and shots after the
// mobs ticked.
func (g *game) applyStrikes() {
	p := g.player
	for _, c := range g.creatures {
		m := c.mob
		if !m.Hostile || c.dying > 0 {
			continue
		}
		w := g.weaponOf(c.sp)
		if m.State == mobs.Attack && m.StateTicks == 0 { // a new attack
			c.attacks++
			clip := w.clips[c.attacks%len(w.clips)]
			c.anim.Play(clip, true)
			c.attackClip = clip
			n := int(math.Ceil(float64(c.sp.model.Clips[clip].Length) * 20))
			m.AttackTicks = max(n, 6)
			m.HitTick = max(1, m.AttackTicks*2/5)
		}
		if !m.Strike {
			continue
		}
		if w.ranged {
			g.shootAtPlayer(c, w)
			continue
		}
		dx, dz := p.X-m.X, p.Z-m.Z
		if math.Hypot(dx, dz) <= meleeReach+0.6 && math.Abs(p.Y-m.Y) < 2 {
			g.hurtPlayer(meleeDamage, m.X, m.Z)
		}
	}
}

// shootAtPlayer fires an arrow from just in front of the shooter's chest
// (outside its own mesh, so it can't hit itself) at the player's chest,
// aiming high enough to make up for the drop.
func (g *game) shootAtPlayer(c *creature, w *weapon) {
	m, p := c.mob, g.player
	fx, fz := math.Sin(m.Yaw), -math.Cos(m.Yaw)
	out := w.front + 0.3
	sx, sy, sz := m.X+fx*out, m.Y+c.sp.height*shoulderFrac, m.Z+fz*out
	tx, ty, tz := p.X, p.Y+p.Height*0.6, p.Z
	d := math.Hypot(tx-sx, tz-sz)
	t := d / projectile.Speed
	drop := 0.5 * projectile.Gravity * t * t
	yaw := math.Atan2(tx-sx, -(tz - sz))
	pitch := math.Atan2(ty-sy+drop, d)
	g.arrows.Shoot(sx, sy, sz, yaw, pitch, projectile.Speed)
}

// hurtPlayer applies damage with Minecraft's knockback away from x,z and a
// short invulnerability; at zero health the player respawns.
func (g *game) hurtPlayer(dmg int, fromX, fromZ float64) {
	if g.hurt > 0 || g.player.NoClip {
		return
	}
	p := g.player
	g.health -= dmg
	g.hurt = hurtTicks
	dx, dz := p.X-fromX, p.Z-fromZ
	if l := math.Hypot(dx, dz); l > 1e-6 {
		p.VX += dx / l * 0.4
		p.VZ += dz / l * 0.4
	}
	p.VY = math.Max(p.VY, 0.36)
	if g.health <= 0 {
		g.respawn()
	}
}

// respawn puts the player back at the spawn with full health; every
// hostile calms down.
func (g *game) respawn() {
	g.health, g.hurt = maxHealth, 0
	g.player.Teleport(g.spawnX, g.spawnY, g.spawnZ)
	for _, c := range g.creatures {
		if c.mob.Hostile {
			c.mob.Hostile = false
			c.mob.HomeX, c.mob.HomeY, c.mob.HomeZ = c.mob.X, c.mob.Y, c.mob.Z
		}
	}
	g.debugMsg = "you died"
}

func (g *game) tickPlayerHealth() {
	if g.hurt > 0 {
		g.hurt--
	}
}

// combatClip is the clip a fighting creature plays instead of its
// behaviour's ("" = no override).
func (g *game) combatClip(c *creature) string {
	m := c.mob
	if !m.Hostile {
		return ""
	}
	if m.State == mobs.Attack {
		return c.attackClip
	}
	if w := g.weaponOf(c.sp); m.State == mobs.Idle && w.ready != "" {
		return w.ready
	}
	return ""
}

// drawWeapons draws the sword in each melee creature's hand, posed with
// the hand bone as drawn this frame.
func (g *game) drawWeapons() {
	if g.swordModel < 0 {
		return
	}
	g.weaponBuf, g.weaponMats = g.weaponBuf[:0], g.weaponMats[:0]
	for _, c := range g.creatures {
		if !c.drawn || !fights(c) && c.dying == 0 {
			continue
		}
		w := g.weaponOf(c.sp)
		if w.ranged || w.hand < 0 || c.drawFade >= 1 {
			continue
		}
		// The hand bone's posed frame: skin * bind frame.
		bone := unscaled(c.boneMatrix(w.hand).Mul4(c.sp.model.Bones[w.hand].InvBind.Inv()))
		m := creatureMatrix(0, 0, 0, c.drawYaw, c.drawTilt, c.fallYaw).Mul4(bone).
			Mul4(mgl32.Translate3D(0, -w.handOff, 0)).Mul4(swordHold)
		g.weaponMats = append(g.weaponMats, m)
		sky, blk := g.light(c.drawX, c.drawY+c.sp.height/2, c.drawZ)
		g.weaponBuf = append(g.weaponBuf, render.ModelInstance{Model: g.swordModel, X: c.drawX, Y: c.drawY, Z: c.drawZ, Sky: sky, Block: blk, Fade: c.drawFade})
	}
	for i := range g.weaponBuf {
		g.weaponBuf[i].Transform = &g.weaponMats[i]
	}
	g.modelRend.Draw(&g.cam, g.weaponBuf)
}

// unscaled drops the scale from a rigid transform (bone frames carry the
// model's pixel-to-block scale), keeping rotation and translation.
func unscaled(m mgl32.Mat4) mgl32.Mat4 {
	for c := 0; c < 3; c++ {
		v := m.Col(c).Vec3().Normalize()
		m.SetCol(c, v.Vec4(0))
	}
	return m
}

// swordHold turns the cut-out sword (flat in its XY plane, handle at the
// origin, blade up the +X+Y diagonal) into the hand: blade pointing
// forward and up out of the fist.
var swordHold = mgl32.HomogRotate3DZ(mgl32.DegToRad(45))

// drawHUD draws the health hearts and the hurt flash.
func (g *game) drawHUD(fbW, fbH int) {
	if g.playerTgt == nil {
		return
	}
	if g.hud == nil {
		var err error
		if g.hud, err = render.NewOverlay(); err != nil {
			return
		}
		g.flash, _ = render.NewOverlay()
		if g.flash != nil {
			img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
			img.Set(0, 0, color.NRGBA{200, 0, 0, 90})
			g.flash.SetImage(img)
		}
	}
	if g.hudHealth != g.health {
		g.hudHealth = g.health
		g.hud.SetImage(heartsImage(g.health))
	}
	if g.hurt > 0 && g.flash != nil {
		g.flash.Draw(0, 0, max(fbW, fbH), fbW, fbH)
	}
	w, h := g.hud.Size()
	s := max(2, fbH/360)
	g.hud.Draw((fbW-w*s)/2, fbH-h*s-fbH/12, s, fbW, fbH)
}

// heartsImage draws ten Minecraft-style hearts for health half-hearts.
func heartsImage(health int) image.Image {
	const hw = 9
	shape := []string{
		".XX...XX.",
		"XOOX.XOOX",
		"XOOOXOOOX",
		"XOOOOOOOX",
		".XOOOOOX.",
		"..XOOOX..",
		"...XOX...",
		"....X....",
	}
	img := image.NewNRGBA(image.Rect(0, 0, 10*hw+9, len(shape)))
	outline := color.NRGBA{20, 0, 0, 255}
	full := color.NRGBA{220, 20, 30, 255}
	empty := color.NRGBA{60, 20, 20, 200}
	for i := 0; i < 10; i++ {
		x0 := i * (hw + 1)
		for y, row := range shape {
			for x, ch := range row {
				var c color.NRGBA
				switch {
				case ch == 'X':
					c = outline
				case ch != 'O':
					continue
				case 2*i+2 <= health, 2*i+1 == health && x < hw/2:
					c = full
				default:
					c = empty
				}
				img.Set(x0+x, y, c)
			}
		}
	}
	return img
}
