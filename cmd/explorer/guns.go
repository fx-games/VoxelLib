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
	"sync"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/actors"
	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/audio"
	"github.com/memmaker/voxellib/character"
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
	// creatureHealth is the hit points of every creature (a vanilla zombie's).
	creatureHealth = 20
	// startRounds is the stock of each ammo type a new game starts with.
	startRounds = 300
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
	hudTex    map[string]ui.Texture       // HUD sprites by pack/hud path
	heatScale float32                     // HeatBarOverlay's eased scale
	lensDisc  int                         // the scopes' view disc model (-1: not uploaded)
	gunViews  map[string]*gunView         // by gun ID, loaded on first use
	choices   map[string]tacz.Choice      // refitted guns' attachments by gun ID (refit.go)
	modes     map[string]firearm.FireMode // guns' last fire modes by gun ID (TACZ keeps it on the item)
	stock     map[string]int              // ammo ID → rounds carried
	// aim is the aiming progress 0..1 (LocalPlayerAim: 1/aim_time per
	// second), aimPrev the last tick's.
	aim, aimPrev firearm.AimProgress
	gunSent      mp.GunFlags // the held gun's state the server has (mp.Client.SendGun)
	fov          gunFOVState
	gunRand      *rand.Rand
	// gunRecoil is the last shot's camera curve, recoilTicks ticks old;
	// recoilP/recoilY are its values already applied to the camera.
	gunRecoil        firearm.Recoil
	recoilTicks      int
	recoilP, recoilY float32
	sounds           gunSounds
	// switching: the gun in hand is being put away, then gun nextSel comes out.
	switching bool
	nextSel   int
	// actionVoice plays the last action sound (actionName), as
	// SoundPlayManager's tmpSoundInstance.
	actionVoice *audio.Voice
	actionName  firearm.GunSound
	// cue is the playing clip's timed sounds.
	cueClip string
	cueT    float32
	cueNext int
	// keys last tick, for presses
	triggerWas bool
	hits       hitMarks        // the HUD hit marker and kill streak
	keyWas     map[string]bool // gun keys down last tick (keyPressed)
}

// gunView is a gun's model on the GPU with its animator.
type gunView struct {
	spec       firearm.Gun // the gun with its attachments
	slide      slideState
	handle     int
	m          *model.Model
	anim       *model.Animator
	idle, iron mgl32.Mat4 // view placing the camera at idle_view / the gun's sight (Gun.Sight)
	ammo       []int      // bones of ammoBones (with descendants)
	addMag     []int      // additional_magazine and descendants
	addMagBone int
	hands      [2]int // lefthand_pos, righthand_pos (-1: no arm)
	skin       []mgl32.Mat4
	times      []float32    // layer times saved while posing between ticks
	lens       *lensView    // nil: no scope
	rest       []mgl32.Mat4 // static_idle at its start: the gun in other players' hands
	restEmpty  []mgl32.Mat4 // rest with the ammo bones hidden: an empty gun (mp.PlayerView.Empty)
	thirdHand  mgl32.Mat4   // inverse of thirdperson_hand's frame in rest (identity without it)
	overheated bool         // the over_heat clip has run for the current lock
	move       movement     // the movement track's state
	camBone    int          // "camera" (-1: none)
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
	p, err := tacz.LoadDefault()
	if err != nil {
		log.Printf("guns disabled: %v", err)
		return
	}
	g.gunPack, g.gunViews, g.stock, g.choices, g.modes = p, map[string]*gunView{}, map[string]int{}, map[string]tacz.Choice{}, map[string]firearm.FireMode{}
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
	from := g.gunSel
	if g.switching {
		from = g.nextSel // pressed again while putting away
	}
	g.selectGun(from + 1)
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

// gunSpec is the pack's gun with this ID (nil: none).
func (g *game) gunSpec(id string) *firearm.Gun {
	if g.gunPack == nil {
		return nil
	}
	for i := range g.gunPack.Guns {
		if g.gunPack.Guns[i].ID == id {
			return &g.gunPack.Guns[i]
		}
	}
	return nil
}

// selectGun takes out gun i of the pack (out of range: no gun).
// selectGun switches to gun i (-1 or out of range: none). A gun in hand
// is put away first (LocalPlayerDraw: put_away, then the draw); tickGun
// takes out the next one once it is holstered.
func (g *game) selectGun(i int) {
	if g.gunOn() && g.gun.Phase != firearm.Holstered {
		g.nextSel, g.switching = i, true
		if g.gun.Phase != firearm.PuttingAway {
			g.gunEvents(g.gun.PutAway())
		}
		return
	}
	g.takeGun(i)
}

// takeGun takes out gun i at once (-1 or out of range: none).
func (g *game) takeGun(i int) {
	g.switching = false
	if g.gunPack == nil || i < 0 || i >= len(g.gunPack.Guns) {
		g.gunSel = -1
		g.sendHold()
		return
	}
	v, err := g.gunView(g.gunPack.Guns[i].ID)
	if err != nil {
		log.Printf("gun %s: %v", g.gunPack.Guns[i].ID, err)
		g.gunSel = -1
		g.sendHold()
		return
	}
	g.equipped = false // the crossbow goes away
	g.gunSel = i
	spec := &v.spec
	g.gun = firearm.GunState{Gun: spec, Ammo: spec.Mag, ClipLen: clipLen(v)}
	if m, ok := g.modes[spec.ID]; ok && !g.gun.SetFireMode(m) {
		log.Printf("gun %s: kept fire mode %v not among %v", spec.ID, m, spec.FireModes())
	}
	if err := g.resolveMode(); err != nil { // the view may hold another mode's spec
		log.Printf("gun %s: %v", spec.ID, err)
	}
	g.sendHold()
	// A draw resets the gun's synced state (ShooterDataHolder.initialData),
	// here and on the server (msgHold with another gun).
	g.aim, g.aimPrev, g.gunSent = 0, 0, 0
	g.sounds.preload(spec)
	g.gunEvents(g.gun.Draw())
}

// resolveMode rebuilds the held gun's spec for its fire mode
// (fire_mode_adjust) and attachments; the view and state stay.
func (g *game) resolveMode() error {
	id := g.gun.Gun.ID
	spec, err := g.gunPack.Gun(id, g.gunChoice(id), g.gun.FireMode())
	if err != nil {
		return err
	}
	v := g.gunViews[id]
	v.spec = spec
	g.gun.Gun = &v.spec
	return nil
}

// clipLen is GunState.ClipLen for a gun's view.
func clipLen(v *gunView) func(string) float32 {
	return func(c string) float32 {
		if cl := v.m.Clips[c]; cl != nil {
			return cl.Length
		}
		return 0
	}
}

// gunView is gun id's view with its attachments (gunChoice), built on first use.
func (g *game) gunView(id string) (*gunView, error) {
	if v := g.gunViews[id]; v != nil {
		return v, nil
	}
	v, err := g.newGunView(id, g.gunChoice(id), g.gunSpec(id).FireModes()[0])
	if err != nil {
		return nil, err
	}
	g.gunViews[id] = v
	return v, nil
}

// newGunView builds gun id's view with attachments c in fire mode mode.
func (g *game) newGunView(id string, c tacz.Choice, mode firearm.FireMode) (*gunView, error) {
	spec, err := g.gunPack.Gun(id, c, mode)
	if err != nil {
		return nil, err
	}
	m, lens, err := g.gunPack.Model(id, c)
	if err != nil {
		return nil, err
	}
	h, err := g.Models.Add(m)
	if err != nil {
		return nil, err
	}
	v := &gunView{spec: spec, handle: h, m: m, anim: model.NewAnimator(m)}
	if lens != nil {
		if v.lens, err = g.lensView(lens); err != nil {
			return nil, err
		}
	}
	v.anim.FadeTime = 0
	v.anim.Play(firearm.ClipIdle, true)
	v.anim.Layers = []model.Layer{mainTrack: {}, movementTrack: {Blend: true}, slideTrack: {Blend: true}}
	v.camBone = m.Bone(cameraBone)
	rest := model.NewAnimator(m)
	rest.Play(firearm.ClipIdle, true)
	v.rest = rest.Skin(nil)
	v.thirdHand = mgl32.Ident4() // TACZ: no positioning node, no positioning
	if b := m.Bone(thirdPersonHandBone); b >= 0 {
		v.thirdHand = v.rest[b].Mul4(m.Bones[b].InvBind.Inv()).Inv()
	}
	world := m.BindWorld()
	v.idle, v.iron = cameraAt(world, m.Bone("idle_view")), cameraAt(world, m.Bone(spec.Sight))
	for _, n := range ammoBones {
		v.ammo = append(v.ammo, m.Descendants(m.Bone(n))...)
	}
	v.restEmpty = append([]mgl32.Mat4(nil), v.rest...)
	for _, b := range v.ammo {
		v.restEmpty[b] = mgl32.Mat4{}
	}
	v.addMagBone = m.Bone(additionalMag)
	v.addMag = m.Descendants(v.addMagBone)
	v.hands = [2]int{m.Bone("lefthand_pos"), m.Bone("righthand_pos")}
	return v, nil
}

// dropGunView frees gun id's view.
func (g *game) dropGunView(id string) {
	if v := g.gunViews[id]; v != nil {
		g.freeGunView(v)
		delete(g.gunViews, id)
	}
}

// freeGunView frees a view's models (the shared lens disc stays).
func (g *game) freeGunView(v *gunView) {
	g.Models.Remove(v.handle)
	if l := v.lens; l != nil {
		for _, h := range append([]int{l.ring, l.body}, l.divisions...) {
			if h >= 0 {
				g.Models.Remove(h)
			}
		}
		for _, o := range l.oculars {
			if o.handle >= 0 {
				g.Models.Remove(o.handle)
			}
		}
	}
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
	g.Player.MoveSpeed = character.Speed{} // default unless a gun is held
	if !g.gunOn() {
		g.syncGun(false)
		return
	}
	if g.gun.Phase == firearm.Holstered && g.switching {
		g.takeGun(g.nextSel)
		if !g.gunOn() {
			g.syncGun(false)
			return
		}
	}
	g.aimPrev = g.aim
	g.recoilTicks++
	aiming := g.Input.Down(app.ActUse) && !g.ScreenOpen() && g.gun.Phase == firearm.Ready
	g.aim = g.aim.Tick(aiming, g.gun.Gun.Aim, time.Second/app.TickRate)
	g.syncGun(aiming)
	// LivingEntitySpeedModifier: the gun's weight and movement_speed.
	g.Player.MoveSpeed = character.SpeedOf(character.PlayerMoveSpeed * float64(g.gun.Gun.SpeedFactor(aiming, g.gun.Phase == firearm.Reloading)))
	in := firearm.GunInput{
		Trigger: g.Input.Down(app.ActAttack) && !g.ScreenOpen(),
		Reload:  g.Input.Down("reload"),
		Inspect: g.keyPressed("inspect"),
		Melee:   g.keyPressed("melee"),
		Unload:  g.keyPressed("unload"),
	}
	in.Pressed = in.Trigger && !g.triggerWas
	g.triggerWas = in.Trigger
	// Fire select first (TACZ handles the key before the tick's shot), its
	// spec resolved (postChangeEvent), so a shot this tick uses the new mode.
	if g.keyPressed("firemode") {
		if ev := g.gun.NextMode(); ev != nil {
			g.modes[g.gun.Gun.ID] = g.gun.FireMode()
			if err := g.resolveMode(); err != nil {
				log.Printf("gun %s: %v", g.gun.Gun.ID, err)
			}
			g.sendHold()
			g.gunEvents(ev)
		}
	}
	stock := g.stock[g.gun.Gun.Ammo]
	ev := g.gun.Tick(1.0/app.TickRate, in, &stock)
	g.stock[g.gun.Gun.Ammo] = stock
	g.gunEvents(ev)

	v := g.gunViews[g.gun.Gun.ID]
	g.stateTracks(v)
	g.tickGunTracks(v, 1.0/app.TickRate)
	g.tickMovement(v)
	g.tickSlide(v)
	g.cueT += 1.0 / app.TickRate
	cues := g.gun.Gun.Cues[g.cueClip]
	for ; g.cueNext < len(cues) && cues[g.cueNext].T <= g.cueT; g.cueNext++ {
		g.sounds.play(cues[g.cueNext].Sound, audio.UI(1, 1))
	}
}

// keyPressed reports whether a gun key went down since the last tick.
func (g *game) keyPressed(name string) bool {
	if g.keyWas == nil {
		g.keyWas = map[string]bool{}
	}
	down := g.Input.Down(name)
	was := g.keyWas[name]
	g.keyWas[name] = down
	return down && !was
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
			g.gunSound(firearm.SoundDryFire, 1, g.soundPitch())
		}
		if e.Sound != "" {
			g.actionSound(e.Sound)
		}
		if e.Melee {
			g.melee()
		}
	}
}

// Display sound volumes and pitches (SoundPlayManager).
const (
	shotSoundVolume    = 0.8
	silenceSoundVolume = 0.6
	pitchBase          = 0.9
	pitchRandomRange   = 0.125
	actionSoundVolume  = 1
)

// soundPitch is SoundPlayManager's 0.9 + random·0.125.
func (g *game) soundPitch() float32 {
	return pitchBase + g.gunRand.Float32()*pitchRandomRange
}

// gunSound plays a display sound of the held gun at the player.
func (g *game) gunSound(name firearm.GunSound, volume, pitch float32) *audio.Voice {
	return g.sounds.play(g.gun.Gun.Sounds[name], audio.UI(volume, pitch))
}

// actionSound is SoundPlayManager's tmpSoundInstance: draw, reload and
// inspect stop the last action sound first (stopPlayGunSound); bolt and
// fire select do not, and fire select is not kept.
func (g *game) actionSound(name firearm.GunSound) {
	switch name {
	case firearm.SoundFireSelect:
		g.gunSound(name, actionSoundVolume, 1)
		return
	case firearm.SoundBolt:
	default:
		g.stopActionSound()
	}
	g.actionVoice, g.actionName = g.gunSound(name, actionSoundVolume, 1), name
}

func (g *game) stopActionSound() {
	if g.actionVoice != nil {
		g.actionVoice.Stop()
	}
	g.actionVoice = nil
}

// TACZ's default state machine tracks, as Animator layers over the base
// track's looping static_idle (the Animator's Clip): the main track
// (layer mainTrack) plays draw, bolt, reload, inspect and put away once
// and stops, overriding what it keys; each shot runs "shoot" on its own
// gun kick track, blended (added) over the pose.
const (
	mainTrack     = 0
	movementTrack = 1 // blended, run by tickMovement
	slideTrack    = 2 // blended, run by tickSlide
	fixedTracks   = 3 // layers past these come and go
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

// Blending track clips the scripts run by state, not by gun event.
const (
	clipSpin              = "spin"
	clipOverHeat          = "over_heat"
	clipStaticOverHeating = "static_over_heating"
)

// stateTracks runs the blending tracks the state machines drive from the
// gun's state: over_heat_states (over_heat once on overheating,
// static_over_heating looping while overheated) and the minigun's
// spin_states (spin while the main track is idle, once more while firing).
func (g *game) stateTracks(v *gunView) {
	s := &g.gun
	if s.Overheated != v.overheated {
		v.overheated = s.Overheated
		if c := v.m.Clips[clipOverHeat]; c != nil && s.Overheated {
			v.anim.Layers = append(v.anim.Layers, model.Layer{Clip: c, Blend: true})
		}
	}
	setLoops(v, v.m.Clips[clipStaticOverHeating], b2i(s.Overheated))
	if s.Gun.Spin {
		idle := v.anim.Layers[mainTrack].Clip == nil
		setLoops(v, v.m.Clips[clipSpin], b2i(idle)+b2i(idle && s.Firing()))
	}
}

// setLoops keeps n blended layers of a looping clip (nil: none to play).
func setLoops(v *gunView, c *model.Clip, n int) {
	if c == nil {
		return
	}
	for i := range v.anim.Layers {
		if l := &v.anim.Layers[i]; i >= fixedTracks && l.Clip == c {
			if n > 0 {
				n--
			} else {
				l.Clip = nil // pruned by tickGunTracks
			}
		}
	}
	for ; n > 0; n-- {
		v.anim.Layers = append(v.anim.Layers, model.Layer{Clip: c, Blend: true})
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
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
	kept := ls[:fixedTracks]
	for _, l := range ls[fixedTracks:] {
		if !l.Done() {
			kept = append(kept, l)
		}
	}
	v.anim.Layers = kept
}

// aimFrac is how far the sights are raised, 0..1, between ticks.
func (g *game) aimFrac(alpha float64) float32 {
	return float32(g.aimPrev + (g.aim-g.aimPrev)*firearm.AimProgress(alpha))
}

// syncGun tells the server the held gun's state when it changes (the
// server's movement and aim progress use it, others see the empty gun); no
// gun in hand drops the local progress too (LocalPlayerAim.tickAimingProgress).
func (g *game) syncGun(aiming bool) {
	var f mp.GunFlags
	if !g.gunOn() {
		g.aim, g.aimPrev = 0, 0
	} else {
		f = gunFlags(&g.gun, aiming)
	}
	if g.net != nil && f != g.gunSent {
		g.net.cl.SendGun(f)
		g.gunSent = f
	}
}

// gunFlags is a gun state's mp.GunFlags.
func gunFlags(s *firearm.GunState, aiming bool) mp.GunFlags {
	var f mp.GunFlags
	if aiming {
		f |= mp.GunAiming
	}
	if s.Phase == firearm.Reloading {
		f |= mp.GunReloading
	}
	if gunEmpty(s) {
		f |= mp.GunEmpty
	}
	return f
}

// gunEmpty: the gun has no round, so its ammo bones are hidden
// (BedrockGunModel's ammoHiddenRender; an inventory-fed gun shows them).
func gunEmpty(s *firearm.GunState) bool {
	return s.Ammo == 0 && s.Gun.Reload.Kind != firearm.Inventory
}

// shotAim is the aiming progress a shot's inaccuracy uses: the server's
// (InaccuracyType.getInaccuracyType reads the synced AIMING_PROGRESS_KEY),
// the local one in single player.
func (g *game) shotAim() firearm.AimProgress {
	if g.net != nil {
		return g.net.cl.Own().Aim
	}
	return g.aim
}

// shoot fires one round: its pellets, the sound and the camera kick.
func (g *game) shoot() {
	spec := g.gun.Gun
	if g.actionName == firearm.SoundInspect { // firing interrupts inspecting
		g.stopActionSound()
	}
	if s := spec.Silence; s != nil && s.UseSilenceSound {
		g.gunSound(firearm.SoundSilence, silenceSoundVolume, g.soundPitch())
	} else {
		g.gunSound(firearm.SoundShoot, shotSoundVolume, g.soundPitch())
	}
	aim := g.aimFrac(1)
	p := g.Player
	moving := math.Hypot(p.VX, p.VZ) > 0.01
	spread := float64(spec.Spread(g.shotAim() == firearm.Aimed, moving, g.Input.Down(app.ActSneak)) * g.gun.SpreadFactor())
	ex, ey, ez := p.Eye()
	shot := mp.Shot{Gun: spec.ID}
	for range spec.Bullet.Pellets {
		yaw, pitch := g.Cam.Yaw, g.Cam.Pitch
		// a uniform direction in the cone: angle from the axis and around it
		r := spread * math.Sqrt(g.gunRand.Float64()) * math.Pi / 180
		a := g.gunRand.Float64() * 2 * math.Pi
		yaw += r * math.Cos(a) / math.Max(math.Cos(pitch), 0.1)
		pitch += r * math.Sin(a)
		shot.Pellets = append(shot.Pellets, mp.Look{Yaw: yaw, Pitch: pitch})
	}
	if g.net != nil {
		g.net.cl.SendShot(shot) // the server deals the damage
	} else {
		g.hitscan(shot, [3]float64{ex, ey, ez})
	}
	g.startMuzzle(spec, nil)
	g.ejectShells(spec, g.gunViews[spec.ID])
	g.gunRecoil = spec.NewRecoil(g.gunRand, firearm.AimRecoil(aim, spec.Zoom))
	g.recoilTicks, g.recoilP, g.recoilY = 0, 0, 0
}

// hitscan casts a shot's pellets from eye against the creatures (single
// player; in multiplayer the server does it against the players).
func (g *game) hitscan(shot mp.Shot, eye [3]float64) {
	spec := g.gun.Gun
	targets := g.creatureTargets()
	w := g.Eng.World
	boxes := func(x, y, z int, id voxel.StateID, buf []collide.AABB) []collide.AABB {
		return w.StateBoxes(id, x, y, z, buf)
	}
	for _, l := range shot.Pellets {
		d := l.Dir()
		h, ok := projectile.Hitscan(w, boxes, eye, d, mp.GunRange, targets)
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
		head := c.isHead(h.Part)
		g.hitMark(head, g.hitCreature(c, dmg, d, float64(spec.Bullet.Knockback)))
	}
}

// hitMark is TACZ's ClientHitMark sound for the shooter's bullet hit:
// kill, else head_hit or flesh_hit. Single player and multiplayer (the
// server's mp.Hit) both play it here.
func (g *game) hitMark(head, killed bool) {
	g.hits.mark(head, killed)
	switch {
	case killed:
		g.gunSound(firearm.SoundKill, actionSoundVolume, 1)
	case head:
		g.gunSound(firearm.SoundHeadHit, actionSoundVolume, 1)
	default:
		g.gunSound(firearm.SoundFleshHit, actionSoundVolume, 1)
	}
}

// applyRecoil turns the camera by the recoil curve's change since the last
// frame, as TACZ does each camera setup.
func (g *game) applyRecoil(alpha float64) {
	p, y := g.gunRecoil.At((float32(g.recoilTicks) + float32(alpha)) / app.TickRate)
	g.Cam.Pitch = math.Max(-1.55, math.Min(1.55, g.Cam.Pitch+float64(p-g.recoilP)*math.Pi/180))
	g.Cam.Yaw += float64(y-g.recoilY) * math.Pi / 180
	g.recoilP, g.recoilY = p, y
}

// hitCreature applies a hit's damage, killing at creatureHealth, and
// pushes the creature knock blocks/tick along dir; it reports a kill.
func (g *game) hitCreature(c *creature, dmg float32, dir [3]float64, knock float64) bool {
	c.damage += dmg
	yaw := math.Atan2(-dir[0], -dir[2])
	if c.damage >= creatureHealth {
		log.Printf("guns: %s killed", c.sp.model.Name)
		g.kill(c, yaw)
		return true
	}
	g.provoke(c)
	pushCreature(c, knock, -dir[0], -dir[2])
	return false
}

// pushCreature is LivingEntity.knockback on a creature: power against
// xd, zd (pointing from the creature to the source).
func pushCreature(c *creature, power, xd, zd float64) {
	if m := c.mob; !m.Static {
		m.VX, m.VY, m.VZ = actors.KnockbackMotion(m.VX, m.VY, m.VZ, m.OnGround, power, xd, zd)
	}
}

// mobEyeFraction is where a creature's eye sits up its box (Minecraft's
// default eye height, 0.85 of the height).
const mobEyeFraction = 0.85

// melee lands the gun's melee hit (ModernKineticGunItem.doMelee): on the
// creatures in single player, by the server on the players in multiplayer.
func (g *game) melee() {
	look := mp.Look{Yaw: g.Cam.Yaw, Pitch: g.Cam.Pitch}
	if g.net != nil {
		g.net.cl.SendMelee(g.gun.Gun.ID, look)
		return
	}
	m := g.gun.Gun.Melee
	dist := float64(m.Distance)
	ex, ey, ez := g.Player.Eye()
	d := look.Dir()
	reach := g.Player.Box().Grow(dist, dist, dist)
	w := g.Eng.World
	boxes := func(x, y, z int, id voxel.StateID, buf []collide.AABB) []collide.AABB {
		return w.StateBoxes(id, x, y, z, buf)
	}
	for _, t := range g.creatureTargets() {
		c := t.(*creature)
		b := c.Box()
		if !reach.Intersects(b) {
			continue
		}
		eye := [3]float64{c.mob.X, b.MinY + (b.MaxY-b.MinY)*mobEyeFraction, c.mob.Z}
		if !m.Reaches([3]float64{ex, ey, ez}, d, eye) || !projectile.InSight(w, boxes, [3]float64{ex, ey, ez}, eye) {
			continue
		}
		// doPerLivingHurt: knockback(melee knockback) against the look,
		// then hurt's own vanilla knockback away from the attacker.
		pushCreature(c, float64(m.Knockback), -d[0], -d[2])
		g.hitCreature(c, m.Damage, [3]float64{c.mob.X - ex, 0, c.mob.Z - ez}, actors.HurtKnockback)
	}
}

// isHead reports whether a hit on part (a bone, -1 for none) struck the head.
func (c *creature) isHead(part int) bool {
	head := c.sp.shape.head
	return part >= 0 && part < len(head) && head[part]
}

// poseGun poses the held gun's tracks between ticks (render hook, before
// the world: the camera bone turns the world view).
func (g *game) poseGun(alpha float64) {
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
}

// drawGunHand draws the held gun as poseGun posed it, the camera moving
// from idle_view toward iron_view while aiming.
func (g *game) drawGunHand(alpha float64) {
	v := g.gunViews[g.gun.Gun.ID]
	zero := mgl32.Mat4{}
	if gunEmpty(&g.gun) {
		for _, b := range v.ammo {
			v.skin[b] = zero
		}
	}
	if !clipAnimates(v.anim.Layers[mainTrack].Clip, v.addMagBone) {
		for _, b := range v.addMag {
			v.skin[b] = zero
		}
	}
	f := g.fov.aim.value()
	view := g.gunHandView(v)
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
	ins = g.shellInstances(v, view, g.fx.dt, sky, blk, ins)
	if v.lens != nil && v.lens.scope {
		for i := gunAt; i < len(ins); i++ { // not over the magnified view
			ins[i].Stencil = render.HandStencil{Test: true, Compare: render.StencilGreater, Ref: lensView127}
		}
	}
	if g.fov.scope > 0 {
		g.DrawScopeView(g.fov.scope, g.scopeClip(v, view), alpha)
	}
	g.Models.DrawHandInstance(float32(g.Cam.Aspect), view, ins...)
	if g.fx.rend != nil {
		g.fx.rend.DrawHand(float32(g.Cam.Aspect), view, g.fx.sprites, sky, blk)
	}
}

// gunHandView is the held gun's hand view this frame, moving from
// idle_view toward iron_view while aiming.
func (g *game) gunHandView(v *gunView) mgl32.Mat4 {
	f := g.fov.aim.value() // FirstPersonRenderGunEvent's AIMING_DYNAMICS
	view := g.cameraAnim(v).Mul4(lerpMat(v.idle, v.iron, f))
	return mgl32.Translate3D(0, g.gun.Gun.IdleRaise*(1-f), 0).Mul4(view) // a scope's idle height adjust
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
	for _, ev := range spec.Sounds {
		evs = append(evs, ev)
	}
	s.preloadEvents(evs)
}

// preloadEvents decodes sound events in the background.
func (s *gunSounds) preloadEvents(evs []string) {
	if s.mixer == nil {
		return
	}
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

// play plays a loaded sound event (nil: none played).
func (s *gunSounds) play(ev string, p audio.Params) *audio.Voice {
	if s.mixer == nil || ev == "" {
		return nil
	}
	s.mu.Lock()
	snd := s.buf[ev]
	s.mu.Unlock()
	if snd == nil {
		return nil
	}
	return s.mixer.Play(snd, p)
}
