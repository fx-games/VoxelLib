package main

// Guns: the TACZ default gun pack (importer/tacz, assets/tacz) in first
// person. V cycles through the guns and back to an empty hand, left click
// fires, right click aims down the iron sights, R reloads, H inspects, B
// switches the fire mode. firearm.GunState runs each gun's cycle; its clips
// play on the gun's own model, which holds the camera locators idle_view and
// iron_view. Each pellet is a hitscan ray within the gun's spread; damage
// falls off with distance and head hits multiply it.

import (
	"bytes"
	"io/fs"
	"log"
	"math"
	"math/rand"
	"os"
	"sync"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/audio"
	"github.com/memmaker/voxellib/importer/cuboid"
	"github.com/memmaker/voxellib/importer/tacz"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/render"
	firearm "github.com/memmaker/voxellib/weapon"
)

const (
	gunPackDir     = "assets/tacz/tacz_default_gun"
	gunDefaultsDir = "assets/tacz/defaults"
	// creatureHealth is the hit points of every creature (a vanilla zombie's).
	creatureHealth = 20
	// gunRange is how far a bullet reaches, in blocks.
	gunRange = 256
	// startRounds is the stock of each ammo type a new game starts with.
	startRounds = 300
	// gunKnockback is the push (blocks/tick) a creature gets from a hit.
	gunKnockback = 0.15
)

// Bones a gun shows only while it holds rounds (TACZ's GunModelConstant).
var ammoBones = []string{"bullet_in_mag", "bullet_in_barrel", "bullet_chain"}

// additionalMag is the second magazine some reload clips show; TACZ draws
// it only while the playing clip animates it.
const additionalMag = "additional_magazine"

type gunsState struct {
	gunPack   *tacz.Pack
	gunSel    int // index into gunPack.Guns; -1 = no gun in hand
	gun       firearm.GunState
	arms      gunArms
	fx        gunFX
	shells    gunShells
	hudTex    map[string]ui.Texture // HUD sprites by pack/hud path
	heatScale float32               // HeatBarOverlay's eased scale
	lensDisc  int                   // the scopes' view disc model (-1: not uploaded)
	gunViews  map[string]*gunView   // by gun ID, loaded on first use
	stock     map[string]int        // ammo ID → rounds carried
	// aim is the aiming progress 0..1 (LocalPlayerAim: 1/aim_time per
	// second), aimPrev the last tick's.
	aim, aimPrev float32
	fov          gunFOVState
	gunRand      *rand.Rand
	// gunRecoil is the last shot's camera curve, recoilTicks ticks old;
	// recoilP/recoilY are its values already applied to the camera.
	gunRecoil        firearm.Recoil
	recoilTicks      int
	recoilP, recoilY float32
	sounds           gunSounds
	// cue is the playing clip's timed sounds.
	cueClip string
	cueT    float32
	cueNext int
	// keys last tick, for presses
	triggerWas, inspectWas, modeWas bool
}

// gunView is a gun's model on the GPU with its animator.
type gunView struct {
	handle     int
	m          *model.Model
	anim       *model.Animator
	idle, iron mgl32.Mat4 // view placing the camera at idle_view / the gun's sight (Gun.Sight)
	ammo       []int      // bones of ammoBones (with descendants)
	addMag     []int      // additional_magazine and descendants
	addMagBone int
	hands      [2]int // lefthand_pos, righthand_pos (-1: no arm)
	skin       []mgl32.Mat4
	times      []float32 // layer times saved while posing between ticks
	lens       *lensView // nil: no scope
}

// gunArms draws the player's arms at a gun's hand bones as TACZ does
// (LeftHandRender/RightHandRender → PlayerRenderer.renderLeftHand/renderRightHand).
type gunArms struct {
	body  *mp.Body
	model int // body geometry with the player's skin
	skins [2][]mgl32.Mat4
	draw  [2][]mgl32.Mat4
}

// setupGuns loads the gun pack and gives the player a stock of every ammo type.
func (g *game) setupGuns() {
	g.gunSel, g.heatScale, g.lensDisc = -1, 0.25, -1 // heatScale: HeatBarOverlay.heatScale
	p, err := tacz.Load(os.DirFS(gunPackDir), os.DirFS(gunDefaultsDir))
	if err != nil {
		log.Printf("guns disabled: %v", err)
		return
	}
	g.gunPack, g.gunViews, g.stock = p, map[string]*gunView{}, map[string]int{}
	g.gunRand = rand.New(rand.NewSource(1))
	for _, a := range p.Ammo {
		g.stock[a.ID] = startRounds
	}
	g.sounds.files = p.FS
	log.Printf("guns: %d guns, %d ammo types", len(p.Guns), len(p.Ammo))
}

// gunOn reports whether a gun is in hand.
func (g *game) gunOn() bool { return g.gunPack != nil && g.gunSel >= 0 }

// nextGun (V) takes out the next gun, or puts the last one away.
func (g *game) nextGun() {
	if g.gunPack == nil {
		return
	}
	g.selectGun(g.gunSel + 1)
}

// selectGunID takes out the gun with this ID.
func (g *game) selectGunID(id string) {
	if g.gunPack == nil {
		return
	}
	for i, spec := range g.gunPack.Guns {
		if spec.ID == id {
			g.selectGun(i)
			return
		}
	}
	log.Printf("no gun %q", id)
}

// selectGun takes out gun i of the pack (out of range: no gun).
func (g *game) selectGun(i int) {
	if i < 0 || i >= len(g.gunPack.Guns) {
		g.gunSel = -1
		return
	}
	spec := &g.gunPack.Guns[i]
	v, err := g.gunView(spec)
	if err != nil {
		log.Printf("gun %s: %v", spec.ID, err)
		g.gunSel = -1
		return
	}
	g.equipped = false // the crossbow goes away
	g.gunSel = i
	g.gun = firearm.GunState{Gun: spec, Ammo: spec.Mag, ClipLen: func(c string) float32 {
		if cl := v.m.Clips[c]; cl != nil {
			return cl.Length
		}
		return 0
	}}
	g.sounds.preload(spec)
	g.gunEvents(g.gun.Draw())
}

func (g *game) gunView(spec *firearm.Gun) (*gunView, error) {
	if v := g.gunViews[spec.ID]; v != nil {
		return v, nil
	}
	m, lens, err := g.gunPack.Model(spec.ID)
	if err != nil {
		return nil, err
	}
	h, err := g.Models.Add(m)
	if err != nil {
		return nil, err
	}
	v := &gunView{handle: h, m: m, anim: model.NewAnimator(m)}
	if lens != nil {
		if v.lens, err = g.lensView(lens); err != nil {
			return nil, err
		}
	}
	v.anim.FadeTime = 0
	v.anim.Play(firearm.ClipIdle, true)
	v.anim.Layers = []model.Layer{mainTrack: {}}
	world := m.BindWorld()
	v.idle, v.iron = cameraAt(world, m.Bone("idle_view")), cameraAt(world, m.Bone(spec.Sight))
	for _, n := range ammoBones {
		v.ammo = append(v.ammo, m.Descendants(m.Bone(n))...)
	}
	v.addMagBone = m.Bone(additionalMag)
	v.addMag = m.Descendants(v.addMagBone)
	v.hands = [2]int{m.Bone("lefthand_pos"), m.Bone("righthand_pos")}
	g.gunViews[spec.ID] = v
	return v, nil
}

// cameraAt is the view matrix that puts the camera at bone b's bind pose.
func cameraAt(world []mgl32.Mat4, b int) mgl32.Mat4 {
	if b < 0 {
		return mgl32.Translate3D(0, 0, -0.5)
	}
	return world[b].Inv()
}

// tickGun runs the held gun's cycle for one tick.
func (g *game) tickGun() {
	if !g.gunOn() {
		return
	}
	g.aimPrev = g.aim
	g.recoilTicks++
	aiming := g.Input.Down(app.ActUse) && !g.ScreenOpen() && g.gun.Phase == firearm.Ready
	if aiming {
		g.aim = min(g.aim+g.aimStep(), 1)
	} else {
		g.aim = max(g.aim-g.aimStep(), 0)
	}
	in := firearm.GunInput{
		Trigger:  g.Input.Down(app.ActAttack) && !g.ScreenOpen(),
		Reload:   g.Input.Down("reload"),
		Inspect:  g.Input.Down("inspect"),
		NextMode: g.Input.Down("firemode"),
	}
	in.Pressed = in.Trigger && !g.triggerWas
	g.triggerWas = in.Trigger
	in.Inspect = in.Inspect && !g.inspectWas
	g.inspectWas = g.Input.Down("inspect")
	in.NextMode = in.NextMode && !g.modeWas
	g.modeWas = g.Input.Down("firemode")
	stock := g.stock[g.gun.Gun.Ammo]
	ev := g.gun.Tick(1.0/app.TickRate, in, &stock)
	g.stock[g.gun.Gun.Ammo] = stock
	g.gunEvents(ev)

	v := g.gunViews[g.gun.Gun.ID]
	g.tickGunTracks(v, 1.0/app.TickRate)
	g.cueT += 1.0 / app.TickRate
	cues := g.gun.Gun.Cues[g.cueClip]
	for ; g.cueNext < len(cues) && cues[g.cueNext].T <= g.cueT; g.cueNext++ {
		g.sounds.play(cues[g.cueNext].Sound, 1)
	}
}

func (g *game) gunEvents(ev []firearm.GunEvent) {
	v := g.gunViews[g.gun.Gun.ID]
	for _, e := range ev {
		if e.Clip != "" {
			g.playGunClip(v, e.Clip)
		}
		if e.Shot {
			g.shoot()
		}
		if e.Dry {
			g.sounds.play(g.gun.Gun.Dry, 1)
		}
	}
}

// TACZ's default state machine tracks, as Animator layers over the base
// track's looping static_idle (the Animator's Clip): the main track
// (layer mainTrack) plays draw, bolt, reload, inspect and put away once
// and stops, overriding what it keys; each shot runs "shoot" on its own
// gun kick track, blended (added) over the pose.
const (
	mainTrack = 0
	// mainTransition is runAnimation's transition for main track clips
	// other than draw (0).
	mainTransition = 0.2
)

// playGunClip starts a clip on its track.
func (g *game) playGunClip(v *gunView, clip string) {
	c := v.m.Clips[clip]
	switch clip {
	case firearm.ClipIdle:
		v.anim.Layers[mainTrack].Clip = nil // the base track shows through
		return
	case firearm.ClipShoot:
		if c != nil {
			v.anim.Layers = append(v.anim.Layers, model.Layer{Clip: c, Blend: true})
		}
		return
	}
	if clip == firearm.ClipDraw {
		v.anim.FadeFrom(0)
	} else {
		v.anim.FadeFrom(mainTransition)
	}
	v.anim.Layers[mainTrack] = model.Layer{Clip: c}
	g.cueClip, g.cueT, g.cueNext = clip, 0, 0
}

// tickGunTracks advances the tracks by dt; finished clips stop (PLAY_ONCE_STOP).
func (g *game) tickGunTracks(v *gunView, dt float32) {
	v.anim.Update(dt)
	ls := v.anim.Layers
	for i := range ls {
		ls[i].Time += dt
	}
	if ls[mainTrack].Done() {
		ls[mainTrack].Clip = nil
	}
	kept := ls[:mainTrack+1]
	for _, l := range ls[mainTrack+1:] {
		if !l.Done() {
			kept = append(kept, l)
		}
	}
	v.anim.Layers = kept
}

// aimFrac is how far the sights are raised, 0..1, between ticks.
func (g *game) aimFrac(alpha float64) float32 {
	return g.aimPrev + (g.aim-g.aimPrev)*float32(alpha)
}

// aimStep is a tick's aiming progress: the gun's aim time (0: at once).
func (g *game) aimStep() float32 {
	if t := g.gun.Gun.Aim; t > 0 {
		return 1 / (t * app.TickRate)
	}
	return 1
}

// shoot fires one round: its pellets, the sound and the camera kick.
func (g *game) shoot() {
	spec := g.gun.Gun
	g.sounds.play(spec.Shoot, 1)
	aim := g.aimFrac(1)
	p := g.Player
	moving := math.Hypot(p.VX, p.VZ) > 0.01
	spread := float64(spec.Spread(aim >= 1, moving, g.Input.Down(app.ActSneak)) * g.gun.SpreadFactor())
	ex, ey, ez := p.Eye()
	targets := g.creatureTargets()
	w := g.Eng.World
	boxes := func(x, y, z int, id voxel.StateID, buf []collide.AABB) []collide.AABB {
		return w.StateBoxes(id, x, y, z, buf)
	}
	for range spec.Bullet.Pellets {
		yaw, pitch := g.Cam.Yaw, g.Cam.Pitch
		// a uniform direction in the cone: angle from the axis and around it
		r := spread * math.Sqrt(g.gunRand.Float64()) * math.Pi / 180
		a := g.gunRand.Float64() * 2 * math.Pi
		yaw += r * math.Cos(a) / math.Max(math.Cos(pitch), 0.1)
		pitch += r * math.Sin(a)
		d := [3]float64{-math.Sin(yaw) * math.Cos(pitch), math.Sin(pitch), -math.Cos(yaw) * math.Cos(pitch)}
		h, ok := projectile.Hitscan(w, boxes, [3]float64{ex, ey, ez}, d, gunRange, targets)
		if !ok || h.Block {
			continue
		}
		c, isCreature := h.Target.(*creature)
		if !isCreature {
			continue
		}
		dmg := spec.DamageAt(float32(h.Dist))
		if c.isHead(h.Part) && spec.Bullet.Headshot > 0 {
			dmg *= spec.Bullet.Headshot
		}
		g.hitCreature(c, dmg, d)
	}
	g.startMuzzle(spec)
	g.ejectShells(spec, g.gunViews[spec.ID])
	g.gunRecoil = spec.NewRecoil(g.gunRand, firearm.AimRecoil(aim, spec.Zoom))
	g.recoilTicks, g.recoilP, g.recoilY = 0, 0, 0
}

// applyRecoil turns the camera by the recoil curve's change since the last
// frame, as TACZ does each camera setup.
func (g *game) applyRecoil(alpha float64) {
	p, y := g.gunRecoil.At((float32(g.recoilTicks) + float32(alpha)) / app.TickRate)
	g.Cam.Pitch = math.Max(-1.55, math.Min(1.55, g.Cam.Pitch+float64(p-g.recoilP)*math.Pi/180))
	g.Cam.Yaw += float64(y-g.recoilY) * math.Pi / 180
	g.recoilP, g.recoilY = p, y
}

// hitCreature applies a bullet's damage, killing at creatureHealth.
func (g *game) hitCreature(c *creature, dmg float32, dir [3]float64) {
	c.damage += dmg
	yaw := math.Atan2(-dir[0], -dir[2])
	if c.damage >= creatureHealth {
		log.Printf("guns: %s killed", c.sp.model.Name)
		g.kill(c, yaw)
		return
	}
	g.provoke(c)
	if m := c.mob; !m.Static {
		l := math.Hypot(dir[0], dir[2]) + 1e-9
		m.VX += dir[0] / l * gunKnockback
		m.VZ += dir[2] / l * gunKnockback
	}
}

// isHead reports whether a hit on part (a bone, -1 for none) struck the head.
func (c *creature) isHead(part int) bool {
	head := c.sp.shape.head
	return part >= 0 && part < len(head) && head[part]
}

// drawGunHand draws the held gun, its clip posed between ticks, the camera
// moving from idle_view toward iron_view while aiming.
func (g *game) drawGunHand(alpha float64) {
	v := g.gunViews[g.gun.Gun.ID]
	// Pose between ticks: every track alpha of a tick further on.
	da := float32(alpha) / app.TickRate
	ahead := func(c *model.Clip, t float32) float32 {
		if c != nil && c.Loop != model.LoopRepeat {
			return min(t+da, c.Length)
		}
		return t + da
	}
	t := v.anim.Time
	v.times = v.times[:0]
	v.anim.Time = ahead(v.anim.Clip, t)
	for i := range v.anim.Layers {
		l := &v.anim.Layers[i]
		v.times = append(v.times, l.Time)
		l.Time = ahead(l.Clip, l.Time)
	}
	v.skin = v.anim.Skin(v.skin)
	v.anim.Time = t
	for i, lt := range v.times {
		v.anim.Layers[i].Time = lt
	}
	zero := mgl32.Mat4{}
	if g.gun.Ammo == 0 && g.gun.Gun.Reload.Kind != firearm.Inventory {
		for _, b := range v.ammo {
			v.skin[b] = zero
		}
	}
	if !clipAnimates(v.anim.Layers[mainTrack].Clip, v.addMagBone) {
		for _, b := range v.addMag {
			v.skin[b] = zero
		}
	}
	f := g.fov.aim.value() // FirstPersonRenderGunEvent's AIMING_DYNAMICS
	view := lerpMat(v.idle, v.iron, f)
	view = mgl32.Translate3D(0, g.gun.Gun.IdleRaise*(1-f), 0).Mul4(view) // a scope's idle height adjust
	sky, blk := g.Light(g.Cam.X, g.Cam.Y, g.Cam.Z)
	ins := g.lensInstances(v, view, f, sky, blk)
	gunAt := len(ins)
	ins = append(ins, render.ModelInstance{Model: v.handle, Skin: v.skin, Sky: sky, Block: blk})
	a := &g.arms
	for i, b := range v.hands {
		if b < 0 || a.model < 0 {
			continue
		}
		// The hand bone's posed frame; the arm hangs in it as in the
		// reference's ModelPart space (TACZ turns by 180° about Z there,
		// which our X-mirrored, Y-up import of the geometry already is).
		hand := v.skin[b].Mul4(v.m.Bones[b].InvBind.Inv())
		a.draw[i] = a.draw[i][:0]
		for _, s := range a.skins[i] {
			a.draw[i] = append(a.draw[i], hand.Mul4(s))
		}
		ins = append(ins, render.ModelInstance{Model: a.model, Skin: a.draw[i], Sky: sky, Block: blk})
	}
	sprites := g.drawMuzzle(v, view) // world smoke first: the hand clears depth
	ins = g.shellInstances(v, view, g.fx.dt, sky, blk, ins)
	if v.lens != nil && v.lens.scope {
		for i := gunAt; i < len(ins); i++ { // not over the magnified view
			ins[i].Stencil = render.HandStencil{Test: true, Compare: render.StencilGreater, Ref: lensView127}
		}
	}
	g.Models.DrawHandInstance(float32(g.Cam.Aspect), view, ins...)
	if g.fx.rend != nil {
		g.fx.rend.DrawHand(float32(g.Cam.Aspect), view, sprites, sky, blk)
	}
}

// setupArms builds the first-person arms with the player's skin.
func (g *game) setupArms() {
	a := &g.arms
	a.body = mp.NewBody()
	a.model = -1
	m, err := cuboid.Build(a.body.Geo, g.skins[g.skinIdx].img)
	if err == nil {
		a.model, err = g.Models.Add(m)
	}
	if err != nil {
		log.Printf("guns: arms: %v", err)
		return
	}
	a.skins[0] = a.body.FirstPersonArms(false, nil)
	a.skins[1] = a.body.FirstPersonArms(true, nil)
}

// clipAnimates reports whether clip c has a channel on bone b.
func clipAnimates(c *model.Clip, b int) bool {
	if c == nil || b < 0 {
		return false
	}
	for _, ch := range c.Channels {
		if ch.Bone == b {
			return true
		}
	}
	return false
}

func lerpMat(a, b mgl32.Mat4, f float32) mgl32.Mat4 {
	var m mgl32.Mat4
	for i := range m {
		m[i] = a[i] + (b[i]-a[i])*f
	}
	return m
}

// gunSounds decodes the held gun's sounds in the background and plays them
// once decoded (a sound still decoding is skipped).
type gunSounds struct {
	files fs.FS
	mixer *audio.Mixer
	mu    sync.Mutex
	buf   map[string]*audio.Sound
}

func (s *gunSounds) preload(spec *firearm.Gun) {
	if s.mixer == nil {
		return
	}
	var evs []string
	for _, cues := range spec.Cues {
		for _, c := range cues {
			evs = append(evs, c.Sound)
		}
	}
	evs = append(evs, spec.Shoot, spec.Dry)
	go func() {
		for _, ev := range evs {
			s.load(ev)
		}
	}()
}

func (s *gunSounds) load(ev string) {
	if ev == "" {
		return
	}
	s.mu.Lock()
	_, done := s.buf[ev]
	s.mu.Unlock()
	if done {
		return
	}
	data, err := fs.ReadFile(s.files, tacz.SoundFile(ev))
	var snd *audio.Sound
	if err == nil {
		snd, err = audio.Decode(bytes.NewReader(data))
	}
	if err != nil {
		log.Printf("gun sound %s: %v", ev, err)
	}
	s.mu.Lock()
	if s.buf == nil {
		s.buf = map[string]*audio.Sound{}
	}
	s.buf[ev] = snd // nil after an error: logged once, not retried
	s.mu.Unlock()
}

func (s *gunSounds) play(ev string, vol float32) {
	if s.mixer == nil || ev == "" {
		return
	}
	s.mu.Lock()
	snd := s.buf[ev]
	s.mu.Unlock()
	if snd != nil {
		s.mixer.Play(snd, audio.UI(vol, 1))
	}
}
