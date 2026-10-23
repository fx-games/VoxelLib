package app

import (
	"fmt"
	"math"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/character"
	"github.com/memmaker/voxellib/resloc"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/collide"
	"github.com/memmaker/voxellib/voxel/engine"
	"github.com/memmaker/voxellib/voxel/physics"
	"github.com/memmaker/voxellib/voxel/render"
)

// Terrain adapts the engine's world to character.World and collide.Solids
// (and collide.Boxer). Unloaded chunks are solid so nothing falls out of
// the world while it streams in.
type Terrain struct{ eng *engine.Engine }

func (t *Terrain) Solid(x, y, z int) bool {
	w := t.eng.World
	if y < w.MinY {
		return true
	}
	if w.Chunk(voxel.ChunkPos{X: x >> 4, Z: z >> 4}) == nil {
		return true
	}
	return w.Reg.Info(w.Block(x, y, z)).Collides
}

// Boxes adds stairs, slabs, doors, fences, ... (collide.Boxer).
func (t *Terrain) Boxes(area collide.AABB, out []collide.AABB) []collide.AABB {
	return t.eng.World.Boxes(area, out)
}

func (t *Terrain) Liquid(x, y, z int) character.Liquid {
	name, _, _ := strings.Cut(t.eng.World.Reg.Info(t.eng.World.Block(x, y, z)).Name, "#") // map block keys
	switch name {
	case resloc.Qualify("water"), resloc.Qualify("flowing_water"):
		return character.LiquidWater
	case resloc.Qualify("lava"), resloc.Qualify("flowing_lava"):
		return character.LiquidLava
	}
	return character.LiquidNone
}

var _ character.World = (*Terrain)(nil)

// Player is the first-person player: the character controller plus
// reach, block breaking and placing, the hotbar slot and the hand's swing
// and bob. Its look direction is App.Cam's Yaw and Pitch.
type Player struct {
	*character.Character
	a *App

	Reach      float64 // blocks (default 5)
	CanFly     bool    // double jump toggles flight (default true)
	Selected   int     // hotbar slot
	HotbarSize int     // default 9
	// Spawn point, for Respawn.
	SpawnX, SpawnY, SpawnZ float64
	// Frozen: the chunk under the player isn't ready; the player waits.
	Frozen bool
	// Breaking is the block being broken and its progress 0..1 (for a
	// crack overlay, see BreakStage); Progress 0 = none.
	Breaking voxel.BlockPos
	Progress float64

	findGround, spawnKnown bool
	clicks                 [2]bool // attack/use pressed since the last tick
	breakTicks, breakCD    int
	damage                 float32 // PlayerControllerMP.curBlockDamageMP
	placeCD                int
	useDelay               int // ticks until a held use repeats into the air
	swing                  int
	walkDist, prevWalk     float64 // distanceWalkedModified
	// View bobbing (EntityPlayer.cameraYaw, EntityLivingBase.cameraPitch)
	// and arm lag (EntityPlayerSP.renderArmYaw/Pitch, our radians).
	bobYaw, prevBobYaw     float64
	bobPitch, prevBobPitch float64
	armYaw, prevArmYaw     float64
	armPitch, prevArmPitch float64
	armInit                bool
	stepDist               float64
}

// SwingTicks is the length of the hand swing.
const SwingTicks = 6

func (a *App) newPlayer() *Player {
	x, y, z, fg, known := a.spawn()
	p := &Player{Character: character.New(x, y, z), a: a, Reach: 5, CanFly: true, HotbarSize: 9,
		SpawnX: x, SpawnY: y, SpawnZ: z, Frozen: true, findGround: fg, spawnKnown: known}
	return p
}

// Respawn puts the player back at the spawn point (on the ground).
func (p *Player) Respawn() {
	p.Teleport(p.SpawnX, p.SpawnY, p.SpawnZ)
	p.VX, p.VY, p.VZ = 0, 0, 0
	p.findGround = true
}

// Swing starts the hand's swing animation.
func (p *Player) Swing() { p.swing = SwingTicks }

func (p *Player) click(act Action) { p.clicks[act] = true }

// Headless is an App with input and a player but no window or engine, for
// tests that drive the input path: Input.Hold, then TickInput per tick
// (nothing is targeted, so use goes to Hooks.UseItem).
func Headless() *App {
	a := &App{}
	a.Input = newInput(a)
	a.Player = &Player{Character: character.New(0, 0, 0), a: a}
	return a
}

// TickInput runs the player's held-button step of a tick (attack, use).
func (a *App) TickInput() { a.Player.interact() }

// tick runs one 20 Hz step: terrain readiness, held buttons, then the
// character controller.
func (p *Player) tick() {
	a := p.a
	cx, cz := int(math.Floor(p.X))>>4, int(math.Floor(p.Z))>>4
	if !a.Eng.ChunkReady(voxel.ChunkPos{X: cx, Z: cz}) {
		p.Frozen = true
		p.PrevX, p.PrevY, p.PrevZ = p.X, p.Y, p.Z
		p.clicks = [2]bool{}
		return // wait for terrain under us
	}
	p.Frozen = false
	if p.findGround {
		if y, ok := a.Eng.SurfaceY(int(math.Floor(p.X)), int(math.Floor(p.Z))); ok {
			if !p.spawnKnown || float64(y) > p.Y {
				p.Teleport(p.X, float64(y), p.Z)
			}
			p.SpawnY = p.Y
			p.findGround = false
		}
	}
	if a.Config.Debug.Edits != "" {
		a.applyEdits()
	}
	if p.swing > 0 {
		p.swing--
	}
	p.interact()

	if v := a.Config.Debug.AutoFly; v > 0 {
		p.PrevX, p.PrevY, p.PrevZ = p.X, p.Y, p.Z
		p.Flying = true
		p.X += math.Sin(a.Cam.Yaw) * v / TickRate
		p.Z -= math.Cos(a.Cam.Yaw) * v / TickRate
		return
	}
	in := a.Input
	ci := character.Input{Yaw: a.Cam.Yaw, Pitch: a.Cam.Pitch,
		Forward: axis(in.Down(ActForward), in.Down(ActBack)),
		Strafe:  axis(in.Down(ActRight), in.Down(ActLeft)),
		Jump:    in.Down(ActJump), Sneak: in.Down(ActSneak), Sprint: in.Down(ActSprint)}
	ci.Using = a.Hooks.UsingItem != nil && a.Hooks.UsingItem()
	sneakEdge := p.OnGround && ci.Sneak // moveEntity's flag, from the last tick's onGround
	p.Tick(a.Terrain, ci)
	p.bobTick(a.Cam.Yaw, a.Cam.Pitch, sneakEdge)
	if p.OnGround {
		d := math.Hypot(p.X-p.PrevX, p.Z-p.PrevZ)
		if p.stepDist += d; p.stepDist > 1.6 && !p.Sneaking() {
			p.stepDist = 0
			x, y, z := int(math.Floor(p.X)), int(math.Floor(p.Y-0.2)), int(math.Floor(p.Z))
			p.sound("block.step", voxel.BlockPos{X: x, Y: y, Z: z}, a.Eng.Block(x, y, z))
		}
	}
}

// Sneaking reports whether the sneak action is held.
func (p *Player) Sneaking() bool { return p.a.Input.Down(ActSneak) }

func axis(pos, neg bool) float64 {
	v := 0.0
	if pos {
		v++
	}
	if neg {
		v--
	}
	return v
}

// interact handles clicks and held buttons: attack breaks, use places,
// both go to the interaction hooks first.
func (p *Player) interact() {
	a, in := p.a, p.a.Input
	clicks := p.clicks
	p.clicks = [2]bool{}
	if p.useDelay > 0 {
		p.useDelay--
	}
	attack := (clicks[Attack] || in.Down(ActAttack)) && !in.suppress[ActAttack]
	use := (clicks[Use] || in.Down(ActUse)) && !in.suppress[ActUse]
	if !attack {
		p.Progress, p.breakTicks = 0, 0
		p.breakCD = 0
	}
	if !use {
		p.placeCD = 0
	}
	if !attack && !use {
		return
	}
	hit, ent, ok := a.Target()
	if ent != nil {
		p.Progress = 0
		for act, on := range [2]bool{attack, use} {
			if on && clicks[act] { // entities: once per click
				p.Swing()
				if a.Hooks.EntityInteract != nil {
					a.Hooks.EntityInteract(ent, Action(act))
				}
			}
		}
		return
	}
	if !ok {
		p.Progress = 0
		if use && a.Hooks.UseItem != nil && (clicks[Use] || p.useDelay == 0 && (a.Hooks.UsingItem == nil || !a.Hooks.UsingItem())) {
			p.useDelay = 4
			a.Hooks.UseItem()
		}
		return
	}
	if clicks[Attack] && attack && a.Hooks.BlockInteract != nil && a.Hooks.BlockInteract(hit, Attack) {
		p.Swing()
		in.suppress[ActAttack] = true // holding the button doesn't repeat it
		return
	}
	if attack {
		p.breakStep(hit)
	}
	if use {
		if p.placeCD--; p.placeCD <= 0 {
			p.placeCD = 4
			if a.Hooks.BlockInteract != nil && a.Hooks.BlockInteract(hit, Use) {
				p.Swing()
				p.placeCD = math.MaxInt32 // once per click
				return
			}
			p.place(hit)
		}
	}
}

// breakStep advances breaking the targeted block by one tick.
func (p *Player) breakStep(hit physics.Hit) {
	a := p.a
	if a.Hooks.BlockHit != nil {
		a.Hooks.BlockHit(hit)
	}
	if p.breakCD > 0 {
		p.breakCD--
		return
	}
	str := float32(math.Inf(1))
	if a.Hooks.BreakStrength != nil {
		str = a.Hooks.BreakStrength(hit.State)
	}
	if str <= 0 {
		p.Progress = 0
		return
	}
	if hit.Block != p.Breaking || p.breakTicks == 0 {
		p.Breaking, p.breakTicks, p.damage = hit.Block, 0, 0
	}
	p.breakTicks++
	p.damage += str // curBlockDamageMP
	if p.swing == 0 {
		p.Swing() // swing on while breaking
	}
	if p.damage < 1 {
		p.Progress = float64(p.damage)
		if p.breakTicks%4 == 1 {
			p.sound("block.hit", hit.Block, hit.State)
		}
		return
	}
	p.Progress, p.breakTicks, p.damage = 0, 0, 0
	p.breakCD = 5 // the reference's delay before the next block
	if math.IsInf(float64(str), 1) {
		p.breakCD = 4 // instant breaking repeats every 5 ticks
	}
	p.BreakBlock(hit.Block)
}

// BreakBlock removes the block at pos as if the player broke it: sound,
// Hooks.BlockBroken, and a plant standing on it pops too.
func (p *Player) BreakBlock(pos voxel.BlockPos) {
	a := p.a
	st := a.Eng.Block(pos.X, pos.Y, pos.Z)
	a.Eng.SetBlock(pos.X, pos.Y, pos.Z, voxel.Air)
	p.sound("block.break", pos, st)
	if a.Hooks.BlockBroken != nil {
		a.Hooks.BlockBroken(pos, st)
	}
	// plants can't float
	up := voxel.BlockPos{X: pos.X, Y: pos.Y + 1, Z: pos.Z}
	if above := a.Eng.Block(up.X, up.Y, up.Z); a.Eng.World.Reg.Info(above).Shape == voxel.ShapeCross {
		p.BreakBlock(up)
	}
}

func (p *Player) place(hit physics.Hit) {
	a := p.a
	if a.Hooks.HeldBlock == nil {
		return
	}
	id := a.Hooks.HeldBlock()
	if id == voxel.Air {
		return
	}
	reg := a.Eng.World.Reg
	pos := hit.Block
	if !reg.Info(hit.State).Def.Replaceable {
		pos = pos.Offset(hit.Face)
	}
	if !reg.Info(a.Eng.Block(pos.X, pos.Y, pos.Z)).Def.Replaceable {
		return
	}
	if reg.Info(id).Collides && !p.NoClip && p.Box().Intersects(collide.Block(pos.X, pos.Y, pos.Z)) {
		return
	}
	p.Swing()
	a.Eng.SetBlock(pos.X, pos.Y, pos.Z, id)
	p.sound("block.place", pos, id)
	if a.Hooks.BlockPlaced != nil {
		a.Hooks.BlockPlaced(pos, id)
	}
}

func (p *Player) sound(ev string, pos voxel.BlockPos, st voxel.StateID) {
	if h := p.a.Hooks.Sound; h != nil {
		h(ev, st, float64(pos.X)+0.5, float64(pos.Y)+0.5, float64(pos.Z)+0.5)
	}
}

// Target is what the player looks at within reach: an entity (ent != nil)
// if it is nearer than the block, else the block (ok).
func (a *App) Target() (hit physics.Hit, ent *Entity, ok bool) {
	p := a.Player
	if a.Eng == nil { // headless
		return
	}
	ex, ey, ez := p.Eye()
	f := a.Cam.Forward()
	dx, dy, dz := float64(f[0]), float64(f[1]), float64(f[2])
	if a.Hooks.RayBoxes != nil {
		hit, ok = physics.RaycastBox(a.Eng.World, ex, ey, ez, dx, dy, dz, p.Reach, a.Hooks.RayBoxes)
	} else {
		hit, ok = a.Eng.Raycast(ex, ey, ez, dx, dy, dz, p.Reach)
	}
	best := p.Reach
	if ok {
		best = hit.Dist
	}
	for _, e := range a.Entities.List {
		if e.Removed || e.Width == 0 {
			continue
		}
		if t, in := rayBox(ex, ey, ez, dx, dy, dz, e.Box()); in && t < best {
			best, ent = t, e
		}
	}
	return hit, ent, ok || ent != nil
}

// rayBox is the slab test: the distance along the unit ray to box b.
func rayBox(ox, oy, oz, dx, dy, dz float64, b collide.AABB) (float64, bool) {
	tmin, tmax := 0.0, math.Inf(1)
	for _, s := range [3][4]float64{{ox, dx, b.MinX, b.MaxX}, {oy, dy, b.MinY, b.MaxY}, {oz, dz, b.MinZ, b.MaxZ}} {
		if s[1] == 0 {
			if s[0] < s[2] || s[0] > s[3] {
				return 0, false
			}
			continue
		}
		t0, t1 := (s[2]-s[0])/s[1], (s[3]-s[0])/s[1]
		if t0 > t1 {
			t0, t1 = t1, t0
		}
		tmin, tmax = math.Max(tmin, t0), math.Min(tmax, t1)
	}
	return tmin, tmin <= tmax
}

// bobTick is one tick of the bobbing and arm-lag state: Entity.moveEntity's
// distanceWalkedModified (+0.6 x the horizontal move, not while sneaking
// on the ground), EntityPlayer.onLivingUpdate's cameraYaw (horizontal
// speed up to 0.1, 0 in the air) and cameraPitch (atan(-motionY*0.2)*15,
// 0 on the ground), and EntityPlayerSP's renderArmYaw/Pitch, which follow
// the look halfway each tick.
// ponytail: riding and death are not seen here (both stop the bob in MC).
func (p *Player) bobTick(yaw, pitch float64, sneakEdge bool) {
	p.prevWalk = p.walkDist
	if !sneakEdge {
		p.walkDist += math.Hypot(p.X-p.PrevX, p.Z-p.PrevZ) * 0.6
	}
	f := min(math.Hypot(p.VX, p.VZ), 0.1)
	f1 := math.Atan(-p.VY*0.20000000298023224) * 15
	if !p.OnGround {
		f = 0
	} else {
		f1 = 0
	}
	p.prevBobYaw, p.prevBobPitch = p.bobYaw, p.bobPitch
	p.bobYaw += (f - p.bobYaw) * 0.4
	p.bobPitch += (f1 - p.bobPitch) * 0.8
	if !p.armInit {
		p.armYaw, p.armPitch, p.armInit = yaw, pitch, true
	}
	p.prevArmYaw, p.prevArmPitch = p.armYaw, p.armPitch
	p.armYaw += (yaw - p.armYaw) * 0.5
	p.armPitch += (pitch - p.armPitch) * 0.5
}

// ViewBob is EntityRenderer.setupViewBobbing at this frame's alpha.
func (p *Player) ViewBob(alpha float64) mgl32.Mat4 {
	f1 := p.walkDist - p.prevWalk
	f2 := float32(-(p.walkDist + f1*alpha))
	f3 := float32(p.prevBobYaw + (p.bobYaw-p.prevBobYaw)*alpha)
	f4 := float32(p.prevBobPitch + (p.bobPitch-p.prevBobPitch)*alpha)
	sin := func(x float32) float32 { return float32(math.Sin(float64(x))) }
	cos := func(x float32) float32 { return float32(math.Cos(float64(x))) }
	abs := func(x float32) float32 { return float32(math.Abs(float64(x))) }
	deg := mgl32.DegToRad
	return mgl32.Translate3D(sin(f2*math.Pi)*f3*0.5, -abs(cos(f2*math.Pi)*f3), 0).
		Mul4(mgl32.HomogRotate3DZ(deg(sin(f2*math.Pi) * f3 * 3))).
		Mul4(mgl32.HomogRotate3DX(deg(abs(cos(f2*math.Pi-0.2)*f3) * 5))).
		Mul4(mgl32.HomogRotate3DX(deg(f4)))
}

// HandPose is the first-person hand's view (the view bob, then
// ItemRenderer.renderItemInFirstPerson's arm lag: the look minus the
// interpolated render arm angles, x0.1, about X then Y) and swing.
func (p *Player) HandPose(alpha float64) render.HandPose {
	yaw, pitch := p.a.Cam.Yaw, p.a.Cam.Pitch
	ay := p.prevArmYaw + (p.armYaw-p.prevArmYaw)*alpha
	ap := p.prevArmPitch + (p.armPitch-p.prevArmPitch)*alpha
	// MC pitch grows looking down (ours up), its yaw turns the same way.
	lag := mgl32.HomogRotate3DX(float32(-(pitch - ap) * 0.1)).Mul4(mgl32.HomogRotate3DY(float32((yaw - ay) * 0.1)))
	pose := render.HandPose{View: p.ViewBob(alpha).Mul4(lag)}
	if p.swing > 0 {
		pose.Swing = 1 - (float32(p.swing)-float32(alpha))/SwingTicks
	}
	return pose
}

// applyEdits applies Debug.Edits relative to the player, once.
func (a *App) applyEdits() {
	p := a.Player
	bx, by, bz := int(math.Floor(p.X)), int(math.Floor(p.Y)), int(math.Floor(p.Z))
	for _, e := range strings.Split(a.Config.Debug.Edits, ";") {
		parts := strings.SplitN(e, ",", 4)
		if len(parts) != 4 {
			continue
		}
		var dx, dy, dz int
		fmt.Sscan(parts[0], &dx)
		fmt.Sscan(parts[1], &dy)
		fmt.Sscan(parts[2], &dz)
		id := voxel.Air
		if parts[3] != "air" {
			if m := a.Config.Engine.Map; m != nil {
				id = m.State(parts[3])
			} else {
				id = a.Eng.World.Reg.Parse(parts[3])
			}
		}
		a.Eng.SetBlock(bx+dx, by+dy, bz+dz, id)
	}
	a.Config.Debug.Edits = ""
}

// BreakStage is the crack overlay stage 0..9 of Breaking, -1 for none
// (PlayerControllerMP: (int)(curBlockDamageMP*10)-1).
func (p *Player) BreakStage() int {
	if p.Progress <= 0 {
		return -1
	}
	return int(p.damage*10) - 1
}
