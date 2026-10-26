package bettercombat

// An exact port of what Better Combat 3.2.2 drives of ZigyTheBird's
// PlayerAnimationLib 1.2.5 core (com.zigythebird.playeranimcore): the
// player-animator (emotecraft) JSON loader, the keyframe animation
// controller and its speed, mirror and adjustment modifiers. All in float32
// like the Java; explicit float32() conversions keep Go from fusing
// multiply-adds the JVM rounds separately. palib_test.go replays
// testdata/golden.json.gz, the library's own output.

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"sort"
	"strconv"
	"strings"
)

const piF = float32(math.Pi) // MochaMath.PI

func d2r(v float32) float32 { return float32(float64(v) * (math.Pi / 180)) } // (float) Math.toRadians

// lerpF is MochaMath.lerp.
func lerpF(start, end, t float32) float32 { return start + float32(t*(end-start)) }

// --- EasingType ---

type easeFn func(t float32) float32

func easeOut(f easeFn) easeFn { return func(t float32) float32 { return 1 - f(1-t) } }
func easeInOut(f easeFn) easeFn {
	return func(t float32) float32 {
		if t < 0.5 {
			return f(t*2) / 2
		}
		return 1 - f((1-t)*2)/2
	}
}
func powF(a, b float32) float32 { return float32(math.Pow(float64(a), float64(b))) } // MochaMath.pow (StrictMath)

var easings = map[string]easeFn{
	"linear":   func(t float32) float32 { return t },
	"constant": func(t float32) float32 { return 0 },
}

func init() {
	base := map[string]easeFn{
		"sine":  func(n float32) float32 { return 1 - float32(math.Cos(float64(n*piF/2))) },
		"quad":  func(n float32) float32 { return n * n },
		"cubic": func(n float32) float32 { return n * n * n },
		"quart": func(n float32) float32 { return powF(n, 4) },
		"quint": func(n float32) float32 { return powF(n, 5) },
		"expo":  func(n float32) float32 { return powF(2, 10*(n-1)) },
		"circ":  func(n float32) float32 { return 1 - float32(math.Sqrt(float64(1-float32(n*n)))) },
		"back":  func(t float32) float32 { const n2 = float32(1.70158); return t * t * float32((n2+1)*t-n2) },
		"elastic": func(t float32) float32 {
			return 1 - float32(powF(float32(math.Cos(float64(t*piF/2))), 3)*float32(math.Cos(float64(t*1*piF))))
		},
		"bounce": func(t float32) float32 {
			const n2 = float32(0.5)
			one := 121.0 / 16 * t * t
			two := 121.0/4*n2*powF(t-6.0/11, 2) + 1 - n2
			three := 121*n2*n2*powF(t-9.0/11, 2) + 1 - n2*n2
			four := 484*n2*n2*n2*powF(t-10.5/11, 2) + 1 - n2*n2*n2
			return min(min(one, two), min(three, four))
		},
	}
	for name, f := range base {
		easings["easein"+name] = f
		easings["easeout"+name] = easeOut(f)
		easings["easeinout"+name] = easeInOut(f)
	}
	// ponytail: step, catmullrom and bezier are not used by Better Combat's animations
}

// easingFromString is EasingType.fromString: LINEAR for unknown names.
func easingFromString(name string) string {
	name = strings.ToLower(name)
	if _, ok := easings[name]; ok {
		return name
	}
	return "linear"
}

// easeApply is EasingTypeTransformer.apply(start, end, lerp): no clamping.
func easeApply(ease string, start, end, t float32) float32 {
	return lerpF(start, end, easings[ease](t))
}

// easeApplyEnv is the keyframe variant: the end past 1, the start on NaN.
func easeApplyEnv(ease string, start, end, t float32) float32 {
	if t >= 1 {
		return end
	}
	if t != t {
		return start
	}
	return easeApply(ease, start, end, t)
}

// --- Keyframes and animations ---

type keyframe struct {
	length, start, end float32
	ease               string
}

// lastKeyframeTime is Keyframe.getLastKeyframeTime: the lengths summed in double.
func lastKeyframeTime(list []keyframe) float32 {
	s := 0.0
	for _, k := range list {
		s += float64(k.length)
	}
	return float32(s)
}

// axes index a bone's keyframe lists and values: position, rotation,
// scale x,y,z, then bend (BoneAnimation and PlayerAnimBone).
const (
	axPX = iota
	axPY
	axPZ
	axRX
	axRY
	axRZ
	axSX
	axSY
	axSZ
	axBend
	axes
)

type boneAnim struct{ keys [axes][]keyframe }

type animation struct {
	name       string
	length     float32
	begin, end *float32 // beginTick, endTick extra data, when the file has them
	loop       bool
	bones      map[string]*boneAnim
}

func (a *animation) hasEndTick() bool { return !a.loop && a.end != nil }

// --- PlayerAnimatorLoader ---

var boneDefaults = map[string][3]float32{
	"right_arm": {-5, 2, 0}, "left_arm": {5, 2, 0}, "left_leg": {1.9, 12, 0.1}, "right_leg": {-1.9, 12, 0.1},
}

// playerBoneName is UniversalAnimLoader.getCorrectPlayerBoneName.
func playerBoneName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// jsonFloat is JsonPrimitive.getAsFloat (Float.parseFloat of the text).
func jsonFloat(v any) float32 {
	switch v := v.(type) {
	case json.Number:
		f, _ := strconv.ParseFloat(string(v), 32)
		return float32(f)
	case string:
		f, _ := strconv.ParseFloat(v, 32)
		return float32(f)
	case bool:
		if v {
			return 1
		}
	}
	return 0
}

func jsonInt(v any) int { return int(jsonFloat(v)) }

// jsonBool is getAsBoolean: a string is Boolean.parseBoolean.
func jsonBool(v any) bool {
	switch v := v.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	}
	return false
}

func loadPlayerAnim(fsys fs.FS, path string) (*animation, error) {
	b, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	var node map[string]any
	if err := dec.Decode(&node); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	emote, ok := node["emote"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: not an emotecraft animation", path)
	}
	version := 1
	if v, ok := node["version"]; ok {
		version = jsonInt(v)
	}
	if version > 3 {
		return nil, fmt.Errorf("%s: version %d", path, version)
	}
	a := &animation{bones: map[string]*boneAnim{}}
	a.name = strings.TrimSuffix(path[strings.LastIndexAny(path, `/\`)+1:], ".json")
	easeBefore := jsonBool(emote["easeBeforeKeyframe"])
	beginTick := float32(0)
	if v, ok := emote["beginTick"]; ok {
		beginTick = jsonFloat(v)
		a.begin = &beginTick
	}
	endTick := beginTick + 1
	if v, ok := emote["endTick"]; ok {
		endTick = max(jsonFloat(v), endTick)
		e := endTick
		a.end = &e
	}
	if endTick <= 0 {
		return nil, fmt.Errorf("%s: endTick must be bigger than 0", path)
	}
	if _, ok := emote["isLoop"]; ok {
		if rt, ok := emote["returnTick"]; ok && jsonBool(emote["isLoop"]) {
			returnTick := max(jsonInt(rt)-1, 0)
			if returnTick != 0 {
				return nil, fmt.Errorf("%s: returnTick loops are not supported", path) // ponytail: no pose uses one
			}
			a.loop = true
		}
	}
	stopTick := jsonFloat(emote["stopTick"])
	if !a.loop {
		if stopTick <= endTick {
			endTick += 3
		} else {
			endTick = stopTick
		}
	}
	a.length = endTick
	degrees := true
	if v, ok := emote["degrees"]; ok {
		degrees = jsonBool(v)
	}
	moves, _ := emote["moves"].([]any)
	sort.SliceStable(moves, func(i, j int) bool {
		return jsonInt(moves[i].(map[string]any)["tick"]) < jsonInt(moves[j].(map[string]any)["tick"])
	})
	for _, mv := range moves {
		obj := mv.(map[string]any)
		tick := jsonFloat(obj["tick"])
		if tick > endTick {
			continue
		}
		easeName := "linear"
		if v, ok := obj["easing"]; ok {
			easeName = v.(string)
		}
		ease := easingFromString(easeName) // easingTypeFromString
		if ease == "linear" {
			ease = easingFromString("ease" + easeName)
		}
		turn := 0
		if v, ok := obj["turn"]; ok {
			turn = jsonInt(v)
		}
		for key, val := range obj {
			if key == "tick" || key == "comment" || key == "easing" || key == "turn" {
				continue
			}
			boneKey := playerBoneName(key)
			if version < 3 && boneKey == "torso" {
				boneKey = "body"
			}
			bone := a.bones[boneKey]
			if bone == nil {
				bone = &boneAnim{}
				a.bones[boneKey] = bone
			}
			part, _ := val.(map[string]any)
			addBodyPart(boneKey, bone, part, degrees, tick, ease, turn)
		}
	}
	if body := a.bones["body"]; body != nil && len(body.keys[axBend]) > 0 {
		torso := a.bones["torso"]
		if torso == nil {
			torso = &boneAnim{}
			a.bones["torso"] = torso
		}
		torso.keys[axBend] = append(torso.keys[axBend], body.keys[axBend]...)
		body.keys[axBend] = nil
		empty := true
		for _, k := range body.keys {
			empty = empty && len(k) == 0
		}
		if empty {
			delete(a.bones, "body")
		}
	}
	for name, bone := range a.bones {
		if !easeBefore {
			for i := range bone.keys {
				bone.keys[i] = correctEasings(bone.keys[i])
			}
		}
		if name == "right_item" || name == "left_item" {
			bone.keys[axPY], bone.keys[axPZ] = bone.keys[axPZ], bone.keys[axPY]
			bone.keys[axRY], bone.keys[axRZ] = bone.keys[axRZ], bone.keys[axRY]
		}
	}
	return a, nil
}

const (
	ttNone = iota // TransformType null
	ttPosition
	ttRotation
	ttScale
	ttBend
)

func addBodyPart(boneName string, bone *boneAnim, node map[string]any, degrees bool, tick float32, ease string, turn int) {
	isItem := boneName == "right_item" || boneName == "left_item"
	isCape := boneName == "cape"
	isBody := boneName == "body"
	def := boneDefaults[boneName]
	posType := ttNone
	if isBody {
		posType = ttPosition
	}
	negY := isItem || posType == ttNone || (isBody && posType == ttRotation)
	addPart(bone, axPX, def[0], posType, "x", node, degrees, tick, ease, turn, isItem || isCape || isBody)
	addPart(bone, axPY, def[1], posType, "y", node, degrees, tick, ease, turn, negY)
	addPart(bone, axPZ, def[2], posType, "z", node, degrees, tick, ease, turn, (isItem && posType == ttRotation) || isCape)
	addPart(bone, axRX, 0, ttRotation, "pitch", node, degrees, tick, ease, turn, isItem || isCape || isBody)
	addPart(bone, axRY, 0, ttRotation, "yaw", node, degrees, tick, ease, turn, isItem || isBody)
	addPart(bone, axRZ, 0, ttRotation, "roll", node, degrees, tick, ease, turn, isItem || isCape)
	addPart(bone, axSX, 0, ttScale, "scaleX", node, degrees, tick, ease, turn, false)
	addPart(bone, axSY, 0, ttScale, "scaleY", node, degrees, tick, ease, turn, false)
	addPart(bone, axSZ, 0, ttScale, "scaleZ", node, degrees, tick, ease, turn, false)
	addPart(bone, axBend, 0, ttBend, "bend", node, degrees, tick, ease, turn, false)
}

// addPart is addPartIfExists: lastTick is the axis list's summed length
// before adding.
func addPart(bone *boneAnim, axis int, def float32, tt int, name string, node map[string]any, degrees bool, tick float32, ease string, turn int, negate bool) {
	v, ok := node[name]
	if !ok {
		return
	}
	part := bone.keys[axis]
	prevTime := float32(0)
	if len(part) > 0 {
		prevTime = lastKeyframeTime(part)
	}
	delta := tick - prevTime
	value := jsonFloat(v)
	if tt == ttNone {
		value -= def
	}
	if negate {
		value *= -1
	}
	if tt == ttRotation {
		if degrees {
			value = d2r(value)
		}
		value += float32(piF * 2 * float32(turn))
	}
	if tt == ttPosition {
		value *= 16
	}
	start := float32(0)
	if len(part) > 0 {
		start = part[len(part)-1].end
	} else if tt == ttScale {
		start = 1
	}
	bone.keys[axis] = append(part, keyframe{delta, start, value, ease})
}

// correctEasings gives each keyframe the easing of the one before it (the
// first ease-in-out sine) and holds the last value for a thousandth of a
// tick.
func correctEasings(list []keyframe) []keyframe {
	if len(list) == 0 {
		return list
	}
	prev := "easeinoutsine"
	for i := range list {
		list[i].ease, prev = prev, list[i].ease
	}
	last := list[len(list)-1]
	return append(list, keyframe{0.001, last.end, last.end, prev})
}

// --- Bones ---

// animBone is a PlayerAnimBone: position, rotation, scale, bend.
type animBone struct {
	name string
	v    [axes]float32
}

func newAnimBone(name string) *animBone {
	b := &animBone{name: name}
	b.setToInitialPose()
	return b
}

func (b *animBone) setToInitialPose() { b.v = [axes]float32{axSX: 1, axSY: 1, axSZ: 1} }

// advBone is an AdvancedPlayerAnimBone: a bone with per-axis enabled flags
// and transition lengths.
type advBone struct {
	animBone
	enabled [axes]bool
	trans   [axes]*float32
}

func (b *advBone) setEnabled(on bool) {
	for i := range b.enabled {
		b.enabled[i] = on
	}
}

func (b *animBone) copyIfEnabled(from *advBone) {
	for i := range b.v {
		if from.enabled[i] {
			b.v[i] = from.v[i]
		}
	}
}

// beginOrEndTickLerp eases this bone (the vanilla pose) into the
// animation's bone before beginTick (anim nil) or out of it after endTick.
func (b *animBone) beginOrEndTickLerp(from *advBone, animTime float32, anim *animation) {
	for i := range b.v {
		if !from.enabled[i] {
			continue
		}
		start, end, trans := b.v[i], from.v[i], from.trans[i]
		ease := "easeinoutsine"
		if anim != nil {
			start, end = end, start
			if trans == nil {
				t := anim.length - *anim.end
				trans = &t
			}
			if ba := anim.bones[b.name]; ba != nil && len(ba.keys[i]) > 0 { // easeBeforeKeyframe is false
				ease = ba.keys[i][len(ba.keys[i])-1].ease
			}
		}
		if trans == nil {
			b.v[i] = end
			continue
		}
		b.v[i] = easeApply(ease, start, end, animTime / *trans)
	}
}

// --- AnimationController (HumanoidAnimationController) ---

type animData struct {
	velocity, pt float32
	firstPerson  bool
}

// rawAnim is a RawAnimation of one stage; a new one per trigger, compared
// by identity.
type rawAnim struct{ anim *animation }

type playState int

const (
	stateStopped playState = iota
	stateRunning
	statePaused
)

var humanoidBones = []string{"body", "right_arm", "left_arm", "right_leg", "left_leg", "head", "torso", "right_item", "left_item", "cape", "elytra"}

type controller struct {
	bones       map[string]*advBone
	active      map[string]*advBone
	triggered   *rawAnim
	currentRaw  *rawAnim
	current     *animation // the queued animation playing, or nil
	queued      bool       // the queue holds it still (never, once polled)
	tick        int
	startFrom   float32
	state       playState
	needsReload bool
	data        *animData
	postSetup   func(bones map[string]*advBone)
	chain       layer // the first modifier, ending in the accessor
}

// layer is a modifier in the chain (AbstractModifier) or the controller
// itself at its end (InternalAnimationAccessor).
type layer interface {
	tick(state *animData)
	setupAnim(state *animData)
	transform(bone *animBone)
}

func newController(modifiers ...layer) *controller {
	c := &controller{bones: map[string]*advBone{}, active: map[string]*advBone{}}
	for _, n := range humanoidBones {
		b := &advBone{animBone: *newAnimBone(n)}
		c.bones[n] = b
	}
	var next layer = accessor{c}
	for i := len(modifiers) - 1; i >= 0; i-- {
		switch m := modifiers[i].(type) {
		case *speedMod:
			m.next = next
		case *mirrorMod:
			m.next = next
		case *adjustMod:
			m.next, m.c = next, c
		}
		next = modifiers[i]
	}
	c.chain = next
	return c
}

type accessor struct{ c *controller }

func (a accessor) tick(*animData) {
	a.c.handleAnimation()
	if a.c.state == stateRunning {
		a.c.tick++
	}
}
func (a accessor) setupAnim(state *animData) { a.c.process(state) }
func (a accessor) transform(b *animBone)     { a.c.transformRaw(b) }

func (c *controller) isActive() bool { return c.state == stateRunning || c.state == statePaused }

func (c *controller) tickLayers(state *animData) { c.chain.tick(state) }
func (c *controller) setupAnim(state *animData) {
	c.data = state
	c.chain.setupAnim(state)
}
func (c *controller) transform(b *animBone) { c.chain.transform(b) }

func (c *controller) stop() { c.state = stateStopped }

func (c *controller) hasFinished() bool { return c.currentRaw != nil && c.state == stateStopped }

func (c *controller) trigger(a *animation) {
	c.stop()
	c.triggered = &rawAnim{a}
	c.needsReload = true
	c.state = stateRunning
	c.tick = 0
	c.startFrom = 0
}

func (c *controller) setAnimation(raw *rawAnim, startFrom float32) {
	if raw == nil || raw.anim == nil {
		c.stop()
		return
	}
	if c.needsReload || raw != c.currentRaw {
		c.currentRaw = raw
		c.startFrom = startFrom
		c.tick = 0
		c.state = stateRunning
		c.current, c.queued = raw.anim, false
		c.setupNewAnimation()
		c.needsReload = false
	}
}

// handleAnimation reports whether to continue: Better Combat's state
// handler stops.
func (c *controller) handleAnimation() bool {
	if c.triggered != nil {
		if c.currentRaw != c.triggered {
			c.current = nil
		}
		c.setAnimation(c.triggered, c.startFrom)
		if !c.hasFinished() {
			return true
		}
		c.triggered = nil
		c.needsReload = true
	}
	return false
}

// animTicks is getAnimationTicks.
func (c *controller) animTicks() float32 {
	if c.data == nil {
		return 0
	}
	return float32(c.tick) + c.startFrom + c.data.pt
}

func (c *controller) process(state *animData) {
	adjusted := max(0, state.pt+c.startFrom+float32(c.tick))
	cont := c.handleAnimation()
	if !cont || (c.current == nil && !c.queued) {
		c.state = stateStopped
		return
	}
	if c.state == stateRunning {
		c.processCurrent(adjusted)
	}
}

func (c *controller) processCurrent(adjusted float32) {
	a := c.current
	if a == nil {
		c.state = stateStopped
		return
	}
	if adjusted >= a.length {
		// ponytail: LOOP (the pose_* animations) is not played: Better Combat disables poses in first person
		c.state = stateStopped
		c.current = nil
		for _, b := range c.bones {
			b.setToInitialPose()
		}
		return
	}
	for _, b := range c.bones {
		b.setToInitialPose()
	}
	for name, ba := range a.bones {
		b := c.bones[name]
		if b == nil {
			continue
		}
		for i := range b.v {
			b.v[i] = c.computeAnimValue(a, ba.keys[i], adjusted, i >= axSX && i <= axSZ, &b.trans[i])
		}
	}
}

func (c *controller) computeAnimValue(a *animation, frames []keyframe, tick float32, scale bool, trans **float32) float32 {
	endTick := a.length - 1
	if a.end != nil {
		endTick = *a.end
	}
	var frame keyframe
	idx, startTick := -1, float32(0)
	if len(frames) == 0 {
		if scale {
			frame = keyframe{0, 1, 1, "linear"}
		} else {
			frame = keyframe{0, 0, 0, "linear"}
		}
	} else {
		total := float32(0)
		for i, f := range frames {
			total += f.length
			if total > tick {
				frame, idx, startTick = f, i, tick-(total-f.length)
				break
			}
		}
		if idx < 0 {
			idx = len(frames) - 1
			frame, startTick = frames[idx], tick
		}
	}
	start, end := frame.start, frame.end
	switch {
	case a.begin != nil && len(frames) > 0 && idx == 0 && *a.begin > tick:
		start = end
		l := frame.length
		*trans = &l
	case a.hasEndTick() && len(frames) > 0 && idx == len(frames)-1 && endTick <= tick:
		l := a.length - endTick
		*trans = &l
	default:
		*trans = nil
	}
	lerp := float32(0)
	if frame.length > 0 {
		lerp = startTick / frame.length
	}
	return easeApplyEnv(frame.ease, start, end, lerp)
}

func (c *controller) setupNewAnimation() {
	a := c.current
	if a == nil {
		return
	}
	clear(c.active)
	for name, b := range c.bones {
		b.setEnabled(a.bones[name] != nil)
	}
	for name, ba := range a.bones {
		b := c.bones[name]
		if b == nil {
			continue
		}
		c.active[name] = b
		for i := range b.enabled { // disableAxisIfNotModified
			b.enabled[i] = len(ba.keys[i]) > 0
		}
	}
	if c.postSetup != nil {
		c.postSetup(c.bones)
	}
}

// transformRaw is get3DTransformRaw: the animation's bone written over
// the vanilla one in b.
func (c *controller) transformRaw(b *animBone) {
	from := c.active[b.name]
	if from == nil {
		return
	}
	a := c.current
	switch {
	case a == nil:
		b.copyIfEnabled(from)
	case a.begin != nil && *a.begin > c.animTicks():
		b.beginOrEndTickLerp(from, c.animTicks(), nil)
	case a.hasEndTick() && *a.end <= c.animTicks():
		b.beginOrEndTickLerp(from, c.animTicks()-*a.end, a)
	default:
		b.copyIfEnabled(from)
	}
}

// --- Modifiers ---

// speedMod is Better Combat's TransmissionSpeedModifier over PlayerAnimationLib's SpeedModifier.
type speedMod struct {
	speed          float32
	delta, shifted float32
	elapsed        float32
	gears          []gear
	next           layer
}

type gear struct{ time, speed float32 }

func (m *speedMod) set(speed float32, gears []gear) { m.speed, m.gears, m.elapsed = speed, gears, 0 }

func (m *speedMod) tick(state *animData) {
	delta := 1 - m.delta
	m.delta = 0
	m.step(delta, state)
	m.elapsed += 1
}

func (m *speedMod) setupAnim(state *animData) {
	time := m.elapsed + state.pt
	for _, g := range m.gears {
		if time > g.time {
			m.speed = g.speed
		}
	}
	delta := state.pt - m.delta
	m.delta = state.pt
	m.step(delta, state)
}

func (m *speedMod) step(delta float32, state *animData) {
	delta = float32(delta * m.speed)
	delta += m.shifted
	for delta > 1 {
		delta -= 1
		m.next.tick(state)
	}
	state.pt = delta
	m.next.setupAnim(state)
	m.shifted = delta
}

func (m *speedMod) transform(b *animBone) { m.next.transform(b) }

type mirrorMod struct {
	enabled bool
	next    layer
}

var mirrorMap = map[string]string{"left_arm": "right_arm", "left_leg": "right_leg", "left_item": "right_item",
	"right_arm": "left_arm", "right_leg": "left_leg", "right_item": "left_item"}

func (m *mirrorMod) tick(state *animData)      { m.next.tick(state) }
func (m *mirrorMod) setupAnim(state *animData) { m.next.setupAnim(state) }
func (m *mirrorMod) transform(b *animBone) {
	if !m.enabled {
		m.next.transform(b)
		return
	}
	name := b.name
	if n, ok := mirrorMap[name]; ok {
		name = n
	}
	mirrorBone(b)
	nb := &animBone{name: name, v: b.v}
	m.next.transform(nb)
	mirrorBone(nb)
	b.v = nb.v
}

func mirrorBone(b *animBone) {
	b.v[axPX] *= -1
	b.v[axRY] *= -1
	b.v[axRZ] *= -1
}

// partMod is an AdjustmentModifier.PartModifier: rotation and offset to
// add (scale zero).
type partMod struct{ rot, off [3]float32 }

type adjustMod struct {
	c      *controller
	data   *animData
	source func(bone string, data *animData) (partMod, bool)
	next   layer
}

func (m *adjustMod) tick(state *animData) {
	m.next.tick(state)
	m.data = state
}

func (m *adjustMod) setupAnim(state *animData) {
	m.next.setupAnim(state)
	m.data = state
}

func (m *adjustMod) fadeIn() float32 {
	fade := float32(1)
	if a := m.c.current; a != nil {
		if a.begin != nil && *a.begin > 0 {
			fade = m.c.animTicks() / *a.begin
		}
		fade = min(fade, 1)
	}
	return fade
}

func (m *adjustMod) fadeOut() float32 {
	fade := float32(1)
	if a := m.c.current; a != nil {
		stop := a.length
		end := stop
		if a.end != nil {
			end = *a.end
		}
		position := -(m.c.animTicks() - stop)
		length := stop - end
		if length > 0 {
			fade = min(position/length, 1)
		}
	}
	return fade
}

func (m *adjustMod) transform(b *animBone) {
	pm, ok := m.source(b.name, m.data)
	fade := m.fadeIn() * m.fadeOut()
	m.next.transform(b)
	if ok {
		for i := range 3 {
			b.v[axPX+i] += float32(pm.off[i] * fade)
			b.v[axRX+i] += float32(pm.rot[i] * fade)
		}
	}
}
