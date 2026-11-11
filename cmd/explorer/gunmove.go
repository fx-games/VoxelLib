package main

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/model"
)

// The movement track: default_state_machine.lua's movement_track_states on
// the blending track line's MOVEMENT_TRACK (layer movementTrack), driven
// by the player's movement as TACZ's INPUT_RUN / INPUT_WALK / INPUT_IDLE.

type moveState uint8

const (
	moveIdle moveState = iota
	moveRun
	moveWalk
)

// moveMode is a state's "mode" flag in the script.
type moveMode int8

const (
	modeNone     moveMode = -1 // run: waiting for run_start; walk: nothing chosen
	modeSteady   moveMode = 0  // run: run; walk: idle (just fired, or in the air)
	modeRunHold  moveMode = 1  // run: run_hold in the air
	modeAiming   moveMode = 1  // walk: walk_aiming
	modeForward  moveMode = 2
	modeBackward moveMode = 3
	modeSideway  moveMode = 4
)

// Movement clips (rifle/pistol defaults).
const (
	clipMoveIdle     = "idle"
	clipRunStart     = "run_start"
	clipRun          = "run"
	clipRunHold      = "run_hold"
	clipRunEnd       = "run_end"
	clipWalkAiming   = "walk_aiming"
	clipWalkForward  = "walk_forward"
	clipWalkBackward = "walk_backward"
	clipWalkSideway  = "walk_sideway"
	// walkCycle is the walkDist per walk/run cycle: setAnimationProgress((walkDist % 2) / 2).
	walkCycle = 2.0
	// walkAimFrac is the aiming progress above which walking plays walk_aiming.
	walkAimFrac = 0.5
)

// movement is the movement track's state machine state.
type movement struct {
	state moveState
	mode  moveMode
	// loop: the clip plays LOOP, else PLAY_ONCE_HOLD.
	loop   bool
	anchor float64 // anchorWalkDist
}

// run is context:runAnimation on the movement track (blending).
func (v *gunView) runMove(name string, loop bool, transition float32) {
	c := v.m.Clips[name]
	if c == nil {
		return
	}
	v.anim.Layers[movementTrack].Run(c, transition)
	v.move.loop = loop
}

// tickMovement runs the movement track for a tick (after tickGunTracks).
func (g *game) tickMovement(v *gunView) {
	l, m, p := &v.anim.Layers[movementTrack], &v.move, g.Player
	if c := l.Clip; c != nil && l.Time >= c.Length && m.loop && c.Length > 0 {
		l.Time = float32(math.Mod(float64(l.Time), float64(c.Length)))
	}
	holding := l.Clip != nil && !m.loop && l.Time >= l.Clip.Length
	mainStopped := v.anim.Layers[mainTrack].Clip == nil
	up, down := g.Input.Down(app.ActForward), g.Input.Down(app.ActBack)
	side := g.Input.Down(app.ActLeft) || g.Input.Down(app.ActRight)
	run, walk := p.Sprinting, !p.Sprinting && (up || down || side)
	idle := !run && !walk

	next := m.state
	switch m.state {
	case moveIdle:
		if run && mainStopped {
			next = moveRun
		} else if run || walk {
			next = moveWalk
		}
	case moveRun:
		if idle {
			next = moveIdle
		} else if walk || !mainStopped {
			next = moveWalk
		}
	case moveWalk:
		if idle {
			next = moveIdle
		} else if run && mainStopped {
			next = moveRun
		}
	}
	if next != m.state {
		switch m.state { // exit
		case moveRun:
			v.runMove(clipRunEnd, false, 0.3)
		case moveWalk:
			v.runMove(clipMoveIdle, false, 0.4)
		}
		m.state, m.mode = next, modeNone // entry
		if next == moveRun {
			v.runMove(clipRunStart, false, 0.2)
		}
		holding = false
	}

	dist := p.WalkDist() - m.anchor
	progress := func() {
		if c := l.Clip; c != nil && l.Time >= 0 { // ponytail: not set during a transition
			l.Time = float32(math.Mod(dist, walkCycle)/walkCycle) * c.Length
		}
	}
	play := func(mode moveMode, clip string, transition float32, anchor bool) {
		if m.mode != mode {
			m.mode = mode
			v.runMove(clip, true, transition)
			if anchor {
				m.anchor, dist = p.WalkDist(), 0
			}
		}
	}
	switch m.state {
	case moveIdle:
		if l.Clip == nil || holding {
			v.runMove(clipMoveIdle, true, 0)
		}
	case moveRun:
		if holding {
			v.runMove(clipRun, true, 0.2)
			m.mode, m.anchor, dist = modeSteady, p.WalkDist(), 0
		}
		if m.mode != modeNone {
			if !p.OnGround {
				play(modeRunHold, clipRunHold, 0.6, false)
			} else {
				play(modeSteady, clipRun, 0.2, false)
				progress()
			}
		}
	case moveWalk:
		switch {
		case g.gun.CoolingDown():
			play(modeSteady, clipMoveIdle, 0.3, false)
		case !p.OnGround:
			play(modeSteady, clipMoveIdle, 0.6, false)
		case g.aim > walkAimFrac:
			play(modeAiming, clipWalkAiming, 0.3, false)
		case up:
			play(modeForward, clipWalkForward, 0.4, true)
		case down:
			play(modeBackward, clipWalkBackward, 0.4, true)
		case side:
			play(modeSideway, clipWalkSideway, 0.4, true)
		}
		if m.mode >= modeAiming && m.mode <= modeSideway {
			progress()
		}
	}
}

// cameraBone is the gun bone whose animation turns the player's view
// (TACZ CameraSetupEvent).
const cameraBone = "camera"

// cameraDelta is the camera bone's animated rotation relative to its bind
// pose, in its own frame: the view frame (it sits at idle_view).
func cameraDelta(v *gunView) mgl32.Quat {
	if v.camBone < 0 {
		return mgl32.QuatIdent()
	}
	b := v.m.Bones[v.camBone]
	bind := b.R
	if b.UseEuler {
		bind = model.EulerZYX(b.Euler)
	}
	return bind.Inverse().Mul(v.anim.LocalRotation(v.camBone))
}

// cameraAnim is the hand view's turn by the camera bone (applyItemInHandCameraAnimation).
func (g *game) cameraAnim(v *gunView) mgl32.Mat4 {
	return cameraDelta(v).Inverse().Mat4()
}

// cameraView turns the world view by the camera bone as TACZ's
// applyLevelCameraAnimation does: the rotation's Euler angles (its
// formulas) are added to the camera's yaw, pitch and roll, so yaw turns
// about the world's up axis.
func (g *game) cameraView() {
	q := cameraDelta(g.heldView())
	x, y, z, w := float64(q.V[0]), float64(q.V[1]), float64(q.V[2]), float64(q.W)
	yaw := math.Asin(max(-1, min(1, 2*(w*y-x*z))))
	pitch := math.Atan2(2*(w*x+y*z), 1-2*(x*x+y*y))
	roll := math.Atan2(2*(w*z+x*y), 1-2*(y*y+z*z))
	f := g.Cam.Forward()
	up := mgl32.LookAtV(mgl32.Vec3{}, f, mgl32.Vec3{0, 1, 0}).Mul4x1(mgl32.Vec4{0, 1, 0, 0}).Vec3()
	turn := mgl32.HomogRotate3DZ(float32(-roll)).
		Mul4(mgl32.HomogRotate3DX(float32(-pitch))).
		Mul4(mgl32.HomogRotate3D(float32(-yaw), up))
	if g.Cam.Bob != (mgl32.Mat4{}) {
		turn = g.Cam.Bob.Mul4(turn)
	}
	g.Cam.Bob = turn
}
