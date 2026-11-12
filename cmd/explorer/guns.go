package main

// Guns: the TACZ default gun pack (importer/tacz, assets/tacz) in first
// person. The held hotbar slot decides the gun in hand (inventory.go), left click
// fires, right click aims down the iron sights, R reloads, H inspects, B
// switches the fire mode. firearm.GunState runs each gun's cycle; its clips
// play on the gun's own model, which holds the camera locators idle_view and
// iron_view. Each pellet is a projectile.Bullet (TACZ EntityKineticBullet)
// flown tick by tick; damage falls off with distance and head hits multiply it.

import (
	"bytes"
	"fmt"
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
	"github.com/memmaker/voxellib/inventory"
	"github.com/memmaker/voxellib/model"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/render"
	firearm "github.com/memmaker/voxellib/weapon"
)

// creatureHealth is the hit points of every creature (a vanilla zombie's).
const creatureHealth = 20

// Bones a gun shows only while it holds rounds (TACZ's GunModelConstant).
var ammoBones = []string{"bullet_in_mag", "bullet_in_barrel", "bullet_chain"}

// additionalMag is the second magazine some reload clips show; TACZ draws
// it only while the playing clip animates it.
const additionalMag = "additional_magazine"

type gunsState struct {
	gunPack *tacz.Pack
	// itemReg is the pack's guns and ammunition as items (inventory.go);
	// gunStack is the gun stack in hand (nil: none), whose state gun runs.
	itemReg   *inventory.Registry
	icons     map[*inventory.Item]*ui.Sprite
	gunStack  *inventory.Stack
	gun       firearm.GunState
	arms      gunArms
	fx        gunFX
	tracerFX  gunTracers   // tracer bullets drawn (guntracers.go)
	ammoFX    ammoEntities // ammo entity models by ammo ID (gunammo.go)
	shells    gunShells
	hudTex    map[string]ui.Texture // HUD sprites by pack/hud path
	heatScale float32               // HeatBarOverlay's eased scale
	lensDisc  int                   // the scopes' view disc model (-1: not uploaded)
	gunViews  map[string]*gunView   // by viewKey, loaded on first use
	// aim is the aiming progress 0..1 (LocalPlayerAim: 1/aim_time per
	// second), aimPrev the last tick's.
	aim, aimPrev firearm.AimProgress
	gunSent      mp.GunFlags // the held gun's state the server has (mp.Client.SendGun)
	fov          gunFOVState
	gunRand      *rand.Rand
	// bullets fly in single player (tickBullets); tracers counts them for
	// the held gun's tracer rounds.
	bullets []*projectile.Bullet
	tracers projectile.Shooter
	// gunRecoil is the last shot's camera curve, recoilTicks ticks old;
	// recoilP/recoilY are its values already applied to the camera.
	gunRecoil        firearm.Recoil
	recoilTicks      int
	recoilP, recoilY float32
	sounds           gunSounds
	// switching: the gun in hand is being put away, then nextStack's gun
	// (nil: none) comes out.
	switching bool
	nextStack *inventory.Stack
	// actionVoice plays the last action sound (actionName), as
	// SoundPlayManager's tmpSoundInstance.
	actionVoice *audio.Voice
	actionName  firearm.GunSound
	// cue is the playing clip's timed sounds.
	cueClip string
	cueT    float32
	cueNext int
	// keys last tick, for presses
	hits   hitMarks        // the HUD hit marker and kill streak
	keyWas map[string]bool // gun keys down last tick (keyPressed)
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

// setupGuns loads the gun pack and the inventory (setupItems); the
// sandbox gets every gun that fits and reloads freely, zombie defense
// nothing (the pickups give them).
func (g *game) setupGuns(sandbox bool) {
	g.heatScale, g.lensDisc = 0.25, -1 // heatScale: HeatBarOverlay.heatScale
	p, err := tacz.LoadDefault()
	if err != nil {
		log.Printf("guns disabled: %v", err)
	} else {
		g.gunPack, g.gunViews = p, map[string]*gunView{}
		g.gunRand = rand.New(rand.NewSource(1))
		g.sounds.files = p.FS
		log.Printf("guns: %d guns, %d ammo types", len(p.Guns), len(p.Ammo))
	}
	g.setupItems(sandbox)
}

// gunOn reports whether a gun is in hand.
func (g *game) gunOn() bool { return g.gunStack != nil }

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

// selectGun switches to gun stack s (nil: none). A gun in hand is put
// away first (LocalPlayerDraw: put_away, then the draw); tickGun takes out
// the next one once it is holstered.
func (g *game) selectGun(s *inventory.Stack) {
	// The draw resets the gun's synced state (ShooterDataHolder.initialData),
	// here and on the server (msgHold with another gun).
	g.aim, g.aimPrev, g.gunSent, g.tracers = 0, 0, 0, projectile.Shooter{}
	g.sendNextHold(s)
	if g.gunOn() && g.gun.Phase != firearm.Holstered {
		g.nextStack, g.switching = s, true
		if g.gun.Phase != firearm.PuttingAway {
			g.gunEvents(g.gun.PutAway())
		}
		return
	}
	g.takeGun(s)
}

// takeGun takes out gun stack s at once (nil: none).
func (g *game) takeGun(s *inventory.Stack) {
	g.switching, g.nextStack, g.gunStack = false, nil, nil
	if s == nil {
		g.sendHold()
		return
	}
	v, err := g.gunView(s)
	if err != nil {
		log.Printf("gun %s: %v", s.Item.Key, err)
		g.sendHold()
		return
	}
	g.gunStack = s
	spec := &v.spec
	ammo, ok := gunAmmo(s)
	if !ok {
		ammo = spec.Mag // a new gun comes loaded
	}
	g.gun = firearm.GunState{Gun: spec, Ammo: ammo, ClipLen: clipLen(v)}
	g.gun.SetFireMode(g.stackMode(s))
	if err := g.resolveMode(); err != nil { // the view may hold another mode's spec
		log.Printf("gun %s: %v", spec.ID, err)
	}
	g.sendHold()
	g.sounds.preload(spec)
	g.gunEvents(g.gun.Draw())
}

// tickGunState ticks the held gun's cycle: reloads take rounds from the
// ammunition stacks (any in the sandbox), unloading gives them back, and
// the rounds in the gun are kept on its stack.
func (g *game) tickGunState(in firearm.GunInput) []firearm.GunEvent {
	id := g.gun.Gun.Ammo
	had := g.ammoStock(id)
	stock := had
	ev := g.gun.Tick(1.0/app.TickRate, in, &stock)
	setGunAmmo(g.gunStack, g.gun.Ammo)
	switch {
	case g.inv().Creative:
	case stock < had:
		g.takeAmmo(id, had-stock)
	case stock > had:
		if left := g.giveAmmo(id, stock-had); left > 0 {
			g.note(fmt.Sprintf("§cInventory full: %d rounds of %s lost", left, g.ammoName(id)))
		}
	}
	return ev
}

// heldView is the view of the gun in hand.
func (g *game) heldView() *gunView {
	return g.gunViews[viewKey(g.gun.Gun.ID, g.stackChoice(g.gunStack))]
}

// resolveMode rebuilds the held gun's spec for its fire mode
// (fire_mode_adjust) and attachments; the view and state stay.
func (g *game) resolveMode() error {
	spec, err := g.gunPack.Gun(g.gun.Gun.ID, g.stackChoice(g.gunStack), g.gun.FireMode())
	if err != nil {
		return err
	}
	v := g.heldView()
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

// viewKey keys gunViews: a gun with its attachments.
func viewKey(id string, c tacz.Choice) string { return id + "|" + c.Key() }

// gunView is gun stack s's view with its attachments, built on first use.
func (g *game) gunView(s *inventory.Stack) (*gunView, error) {
	id, c := s.Item.Key, g.stackChoice(s)
	if v := g.gunViews[viewKey(id, c)]; v != nil {
		return v, nil
	}
	v, err := g.newGunView(id, c, g.gunSpec(id).FireModes()[0])
	if err != nil {
		return nil, err
	}
	g.gunViews[viewKey(id, c)] = v
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
	v.anim.Layers = []model.Layer{boltTrack: {}, mainTrack: {}, movementTrack: {Blend: true}, slideTrack: {Blend: true}}
	v.camBone = m.Bone(cameraBone)
	rest := model.NewAnimator(m)
	rest.Play(firearm.ClipIdle, true)
	v.rest = rest.Skin(nil)
	// the empty gun at rest shows static_bolt_caught over static_idle
	rest.Layers = []model.Layer{{Clip: m.Clips[clipStaticBoltCaught]}}
	v.restEmpty = rest.Skin(nil)
	v.thirdHand = mgl32.Ident4() // TACZ: no positioning node, no positioning
	if b := m.Bone(thirdPersonHandBone); b >= 0 {
		v.thirdHand = v.rest[b].Mul4(m.Bones[b].InvBind.Inv()).Inv()
	}
	world := m.BindWorld()
	v.idle, v.iron = cameraAt(world, m.Bone("idle_view")), cameraAt(world, m.Bone(spec.Sight))
	for _, n := range ammoBones {
		v.ammo = append(v.ammo, m.Descendants(m.Bone(n))...)
	}
	for _, b := range v.ammo {
		v.restEmpty[b] = mgl32.Mat4{}
	}
	v.addMagBone = m.Bone(additionalMag)
	v.addMag = m.Descendants(v.addMagBone)
	v.hands = [2]int{m.Bone("lefthand_pos"), m.Bone("righthand_pos")}
	return v, nil
}

// dropGunView frees the view with this viewKey.
func (g *game) dropGunView(key string) {
	if v := g.gunViews[key]; v != nil {
		g.freeGunView(v)
		delete(g.gunViews, key)
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
	if g.Player.Frozen {
		return // not in the world yet (terrain loading): no input, no shots
	}
	g.Player.MoveSpeed = character.Speed{} // default unless a gun is held
	if !g.gunOn() {
		g.syncGun(false)
		return
	}
	if g.gun.Phase == firearm.Holstered && g.switching {
		g.takeGun(g.nextStack)
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
		Aimed:   g.aim >= firearm.AimProgress(halfAimed),
	}
	// Fire select first (TACZ handles the key before the tick's shot), its
	// spec resolved (postChangeEvent), so a shot this tick uses the new mode.
	if g.keyPressed("firemode") {
		if ev := g.gun.NextMode(); ev != nil {
			setGunMode(g.gunStack, g.gun.FireMode())
			if err := g.resolveMode(); err != nil {
				log.Printf("gun %s: %v", g.gun.Gun.ID, err)
			}
			g.sendHold()
			g.gunEvents(ev)
		}
	}
	g.gunEvents(g.tickGunState(in))

	v := g.heldView()
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
	v := g.heldView()
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
	boltTrack     = 0 // BOLT_CAUGHT_TRACK: static_bolt_caught looping while empty, under the main track
	mainTrack     = 1
	movementTrack = 2 // blended, run by tickMovement
	slideTrack    = 3 // blended, run by tickSlide
	fixedTracks   = 4 // layers past these come and go
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
		if c != nil { // a new blending track per shot, PLAY_ONCE_STOP whatever the clip's loop flag
			v.anim.Layers = append(v.anim.Layers, model.Layer{Clip: c, Blend: true, Play: model.LoopOnce})
		}
		return
	}
	if clip == firearm.ClipDraw {
		v.anim.FadeFrom(0)
	} else {
		v.anim.FadeFrom(mainTransition)
	}
	v.anim.Layers[mainTrack] = model.Layer{Clip: c, Play: mainPlay(clip)}
	g.cueClip, g.cueT, g.cueNext = clip, 0, 0
}

// mainPlay is the play type default_state_machine (spas_12's for the tube
// reload) runs a main track clip with: put_away and the tube reload's intro
// and loop hold their last frame, the rest PLAY_ONCE_STOP.
func mainPlay(clip string) model.Loop {
	switch clip {
	case firearm.ClipPutAway, firearm.ClipReloadIntro, firearm.ClipReloadIntroEmpty, firearm.ClipReloadLoop:
		return model.LoopHold
	}
	return model.LoopOnce
}

// Blending track clips the scripts run by state, not by gun event.
const (
	clipSpin              = "spin"
	clipOverHeat          = "over_heat"
	clipStaticOverHeating = "static_over_heating"
	clipStaticBoltCaught  = "static_bolt_caught"
)

// stateTracks runs the blending tracks the state machines drive from the
// gun's state: over_heat_states (over_heat once on overheating,
// static_over_heating looping while overheated) and the minigun's
// spin_states (spin while the main track is idle, once more while firing).
func (g *game) stateTracks(v *gunView) {
	s := &g.gun
	// bolt_caught_states: static_bolt_caught loops while no round is left
	// (isNoAmmo), e.g. the rpg7's rocket hidden after the shot
	if b := &v.anim.Layers[boltTrack]; !gunEmpty(s) {
		b.Clip = nil
	} else if b.Clip == nil {
		*b = model.Layer{Clip: v.m.Clips[clipStaticBoltCaught], Play: model.LoopRepeat}
	}
	if s.Overheated != v.overheated {
		v.overheated = s.Overheated
		if c := v.m.Clips[clipOverHeat]; c != nil && s.Overheated {
			v.anim.Layers = append(v.anim.Layers, model.Layer{Clip: c, Blend: true, Play: model.LoopOnce})
		}
	}
	setLoops(v, v.m.Clips[clipStaticOverHeating], b2i(s.Overheated))
	if s.Gun.Spin {
		idle := v.anim.Layers[mainTrack].Clip == nil
		setLoops(v, v.m.Clips[clipSpin], b2i(idle)+b2i(idle && s.Firing()))
	}
}

// setLoops keeps n blended LOOP layers of clip c (nil: none to play).
// ponytail: minigun_state_machine reruns spin PLAY_ONCE_HOLD on its own cadence; looping approximates it.
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
		v.anim.Layers = append(v.anim.Layers, model.Layer{Clip: c, Blend: true, Play: model.LoopRepeat})
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
		ls[i].Advance(dt)
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
// While switching, the server already has the next gun (sendNextHold),
// drawn full and not aimed: no flags.
func (g *game) syncGun(aiming bool) {
	var f mp.GunFlags
	if !g.gunOn() {
		g.aim, g.aimPrev = 0, 0
	} else if !g.switching {
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
	look := mp.Look{Yaw: g.Cam.Yaw, Pitch: g.Cam.Pitch}
	if g.net != nil {
		// the server rolls the spread, flies the bullets and deals the damage
		if err := g.net.cl.SendShot(mp.Shot{Gun: spec.ID, Look: look, Heat: g.gun.SpreadFactor()}); err != nil {
			log.Printf("mp: shot: %v", err)
		}
	} else {
		p := g.Player
		prev, now := [3]float64{p.PrevX, p.PrevY, p.PrevZ}, [3]float64{p.X, p.Y, p.Z}
		moving := math.Sqrt(sq(now[0]-prev[0])+sq(now[1]-prev[1])+sq(now[2]-prev[2])) > firearm.MoveThreshold
		sneak := g.Input.Down(app.ActSneak)
		inacc := float64(spec.Spread(g.shotAim() == firearm.Aimed, moving, sneak) * g.gun.SpreadFactor())
		// applyRecoilPush, before the bullets spawn
		push := spec.RecoilPush.Push(look.Dir(), p.OnGround, sneak, spec.Bullet.Pellets)
		p.VX, p.VY, p.VZ = p.VX+push[0], p.VY+push[1], p.VZ+push[2]
		at := projectile.BulletSpawn(prev, now, g.Player.EyeHeight)
		g.bullets = append(g.bullets, projectile.Fire(at, look.Dir(), spec, inacc, &g.tracers, g.gunRand)...)
	}
	g.startMuzzle(spec, nil)
	g.ejectShells(spec, g.heldView())
	g.gunRecoil = spec.NewRecoil(g.gunRand, firearm.AimRecoil(aim, spec.Zoom))
	g.recoilTicks, g.recoilP, g.recoilY = 0, 0, 0
}

func sq(v float64) float64 { return v * v }

// bulletWorld is what the explorer's bullets fly through.
type bulletWorld struct {
	w *voxel.World
	t *app.Terrain
}

func (b bulletWorld) Block(x, y, z int) voxel.StateID     { return b.w.Block(x, y, z) }
func (b bulletWorld) Liquid(x, y, z int) character.Liquid { return b.t.Liquid(x, y, z) }

// tickBullets flies the single player's bullets a tick against the
// creatures (in multiplayer the server flies them against the players).
func (g *game) tickBullets() {
	if len(g.bullets) == 0 {
		return
	}
	w := g.Eng.World
	var targets []projectile.BulletTarget
	var who []*creature
	for _, c := range g.creatures {
		if c.dying != 0 {
			continue
		}
		m := c.mob
		targets = append(targets, projectile.BulletTarget{Box: projectile.HitBox(m.Box(), [3]float64{m.X - m.PrevX, m.Y - m.PrevY, m.Z - m.PrevZ}, false, false),
			Feet: [3]float64{m.X, m.Y, m.Z}, EyeHeight: c.sp.eyeHeight()})
		who = append(who, c)
	}
	col := &projectile.BulletCollider{
		Boxes: func(x, y, z int, id voxel.StateID, buf []collide.AABB) []collide.AABB {
			return w.StateBoxes(id, x, y, z, buf)
		},
		Targets: targets,
		Hit: func(b *projectile.Bullet, h projectile.BulletHit) {
			c := who[h.Target]
			if c.dying != 0 {
				return // killed by an earlier bullet
			}
			// LivingEntity.hurt pushes away from the shooter
			away := [3]float64{c.mob.X - g.Player.X, 0, c.mob.Z - g.Player.Z}
			g.hitMark(h.Head, g.hitCreature(c, b.Damage(h.Pos, h.Head), away, float64(b.Gun.Bullet.Knockback)))
		},
	}
	inSight := func(a, b [3]float64) bool { return projectile.InSight(w, col.Boxes, a, b) }
	col.Explode = func(b *projectile.Bullet, at [3]float64) {
		for i, c := range who {
			dmg, push, ok := projectile.Blast(b.Gun.Bullet.Explosion, at, targets[i].Box, inSight)
			if !ok || c.dying != 0 {
				continue
			}
			if !g.hitCreature(c, dmg, [3]float64{c.mob.X - at[0], 0, c.mob.Z - at[2]}, 0) && !c.mob.Static {
				c.mob.VX, c.mob.VY, c.mob.VZ = c.mob.VX+push[0], c.mob.VY+push[1], c.mob.VZ+push[2]
			}
		}
	}
	bw := bulletWorld{w, g.Terrain}
	live := g.bullets[:0]
	for _, b := range g.bullets {
		if b.Tick(bw, col); !b.Gone {
			live = append(live, b)
		}
	}
	clear(g.bullets[len(live):])
	g.bullets = live
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

// halfAimed is the aiming progress m1866_gun_logic's bolt switches at.
const halfAimed = 0.5

// mobEyeFraction is where a creature's eye sits up its box (Minecraft's
// default eye height, 0.85 of the height).
const mobEyeFraction = 0.85

// eyeHeight is the species' eye above its feet: its own, else the default.
func (sp *species) eyeHeight() float64 {
	if sp.eye != 0 {
		return sp.eye
	}
	return sp.height * mobEyeFraction
}

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
	for _, c := range g.creatures {
		if c.dying > 0 || !reach.Intersects(c.mob.Box()) {
			continue
		}
		eye := [3]float64{c.mob.X, c.mob.Y + c.sp.eyeHeight(), c.mob.Z}
		if !m.Reaches([3]float64{ex, ey, ez}, d, eye) || !projectile.InSight(w, boxes, [3]float64{ex, ey, ez}, eye) {
			continue
		}
		// doPerLivingHurt: knockback(melee knockback) against the look,
		// then hurt's own vanilla knockback away from the attacker.
		pushCreature(c, float64(m.Knockback), -d[0], -d[2])
		g.hitCreature(c, m.Damage, [3]float64{c.mob.X - ex, 0, c.mob.Z - ez}, actors.HurtKnockback)
	}
}

// poseGun poses the held gun's tracks between ticks (render hook, before
// the world: the camera bone turns the world view).
func (g *game) poseGun(alpha float64) {
	v := g.heldView()
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
		l.Advance(da)
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
	v := g.heldView()
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
	return g.cameraAnim(v).Mul4(adsPose(v.idle, v.iron, f, g.gun.Gun.AdsPath, g.gun.Gun.IdleRaise))
}

// adsPose is applyFirstPersonPositioningTransform's move from idle to
// aim at progress f: the rotation (and the move) by the path's rotation
// curve, each translation axis then shifted to its own curve, and a
// scope's idle height adjust fading by the y curve; linear without a path.
func adsPose(idle, aim mgl32.Mat4, f float32, path *firearm.AdsPath, idleRaise float32) mgl32.Mat4 {
	rot, height := f, f
	m := lerpMat(idle, aim, f)
	if path != nil {
		rot, height = path.Rotation.At(f), path.Y.At(f)
		m = lerpMat(idle, aim, rot)
		for i, c := range []firearm.AdsCurve{path.X, path.Y, path.Z} {
			m[12+i] += (aim[12+i] - idle[12+i]) * (c.At(f) - rot)
		}
	}
	return mgl32.Translate3D(0, idleRaise*(1-height), 0).Mul4(m)
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
