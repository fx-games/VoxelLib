package mcmodel

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

// Animation is a Bedrock/GeckoLib/Blockbench animation before it is bound
// to a model: bone names map to channel tracks. Bind it to any model with
// matching bone names (see Bind), which is how a shared animation library
// drives many rigs.
type Animation struct {
	Name   string
	Length float32
	Loop   model.Loop
	Bones  map[string]*BoneAnim
}

// BoneAnim holds the tracks of one bone (nil when not animated).
type BoneAnim struct {
	Rotation, Position, Scale *Track
}

type key struct {
	t         float32
	pre, post [3]Expr
	interp    model.Interp
	easing    string // GeckoLib easing for the segment ending at this key
}

// Track is a keyframe track whose components may be Molang expressions.
// It implements model.Track, converting Bedrock units to model units.
type Track struct {
	keys []key
	// conversion from Bedrock to model space per component
	mul [3]float32
}

func (tr *Track) Sample(t float32) mgl32.Vec4 {
	c := &Ctx{AnimTime: float64(t), LifeTime: float64(t)}
	eval := func(e [3]Expr) mgl32.Vec4 {
		return mgl32.Vec4{float32(e[0].Eval(c)) * tr.mul[0], float32(e[1].Eval(c)) * tr.mul[1], float32(e[2].Eval(c)) * tr.mul[2], 0}
	}
	k := tr.keys
	n := len(k)
	if n == 0 {
		return mgl32.Vec4{}
	}
	if t <= k[0].t {
		return eval(k[0].pre)
	}
	if t >= k[n-1].t {
		return eval(k[n-1].post)
	}
	i := sort.Search(n, func(i int) bool { return k[i].t > t }) - 1
	a, b := k[i], k[i+1]
	f := (t - a.t) / (b.t - a.t)
	if a.interp == model.Step {
		return eval(a.post)
	}
	f = ease(b.easing, f)
	if a.interp == model.CatmullRom || b.interp == model.CatmullRom {
		p0, p3 := a.post, b.pre
		if i > 0 {
			p0 = k[i-1].post
		}
		if i+2 < n {
			p3 = k[i+2].pre
		}
		return model.CatmullRomVec(eval(p0), eval(a.post), eval(b.pre), eval(p3), f)
	}
	va, vb := eval(a.post), eval(b.pre)
	return va.Add(vb.Sub(va).Mul(f))
}

func (tr *Track) sort() {
	sort.SliceStable(tr.keys, func(i, j int) bool { return tr.keys[i].t < tr.keys[j].t })
}

func (tr *Track) end() float32 {
	if len(tr.keys) == 0 {
		return 0
	}
	return tr.keys[len(tr.keys)-1].t
}

// Bedrock → model conversions: rotation X and Y are mirrored, positions
// are in 1/16 block with X mirrored (Blockbench's own Animator does the same).
var (
	rotMul = [3]float32{-1, -1, 1}
	posMul = [3]float32{-1.0 / 16, 1.0 / 16, 1.0 / 16}
	sclMul = [3]float32{1, 1, 1}
)

// Bind turns an animation into a clip for m, matching bones by name (case
// insensitive, with Bone aliases). Unknown bones are skipped. ok is false
// when no bone matched.
func (a *Animation) Bind(m *model.Model) (c *model.Clip, ok bool) {
	c = &model.Clip{Name: a.Name, Length: a.Length, Loop: a.Loop}
	idx := map[string]int{}
	for i, b := range m.Bones {
		idx[canonBone(b.Name)] = i
	}
	names := make([]string, 0, len(a.Bones))
	for n := range a.Bones {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		bi, found := idx[canonBone(n)]
		if !found {
			continue
		}
		ba := a.Bones[n]
		add := func(tr *Track, tg model.Target) {
			if tr != nil && len(tr.keys) > 0 {
				c.Channels = append(c.Channels, model.Channel{Bone: bi, Target: tg, Additive: true, Track: tr})
			}
		}
		add(ba.Rotation, model.Rotation)
		add(ba.Position, model.Translation)
		add(ba.Scale, model.Scale)
	}
	return c, len(c.Channels) > 0
}

// boneAliases folds the common Minecraft humanoid naming schemes together
// (Java "bipedHead", GeckoLib "head", player "Head"...).
var boneAliases = map[string]string{
	"bipedhead": "head", "bipedbody": "torso", "body": "torso", "bipedrightarm": "rightarm",
	"bipedleftarm": "leftarm", "bipedrightleg": "rightleg", "bipedleftleg": "leftleg",
	"bipedwaist": "waist", "right_arm": "rightarm", "left_arm": "leftarm",
	"right_leg": "rightleg", "left_leg": "leftleg",
}

func canonBone(n string) string {
	n = strings.ToLower(n)
	if a, ok := boneAliases[n]; ok {
		return a
	}
	return n
}

// ParseAnimationFile reads a Bedrock/GeckoLib animation file
// (*.animation.json, format 1.8.0+) and returns its animations.
func ParseAnimationFile(data []byte) ([]*Animation, error) {
	var f struct {
		Animations map[string]struct {
			Loop   json.RawMessage            `json:"loop"`
			Length float32                    `json:"animation_length"`
			Bones  map[string]json.RawMessage `json:"bones"`
		} `json:"animations"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Animations == nil {
		return nil, fmt.Errorf("mcmodel: not a Bedrock animation file")
	}
	var out []*Animation
	for name, a := range f.Animations {
		an := &Animation{Name: name, Length: a.Length, Loop: model.LoopOnce, Bones: map[string]*BoneAnim{}}
		switch strings.Trim(string(a.Loop), `"`) {
		case "true":
			an.Loop = model.LoopRepeat
		case "hold_on_last_frame":
			an.Loop = model.LoopHold
		}
		for bone, raw := range a.Bones {
			var chans map[string]json.RawMessage
			if json.Unmarshal(raw, &chans) != nil {
				continue
			}
			ba := &BoneAnim{}
			for ch, v := range chans {
				var mul [3]float32
				var dst **Track
				switch ch {
				case "rotation":
					mul, dst = rotMul, &ba.Rotation
				case "position":
					mul, dst = posMul, &ba.Position
				case "scale":
					mul, dst = sclMul, &ba.Scale
				default:
					continue
				}
				tr, err := parseChannel(v, mul)
				if err != nil {
					return nil, fmt.Errorf("mcmodel: %s/%s/%s: %w", name, bone, ch, err)
				}
				*dst = tr
				if an.Length == 0 {
					an.Length = max(an.Length, tr.end())
				}
			}
			an.Bones[bone] = ba
		}
		out = append(out, an)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// parseChannel handles a channel value: a constant (array/number/string)
// or a map of time → keyframe.
func parseChannel(raw json.RawMessage, mul [3]float32) (*Track, error) {
	tr := &Track{mul: mul}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) == nil && !isKeyframeObject(m) {
		for ts, kv := range m {
			t, err := strconv.ParseFloat(ts, 32)
			if err != nil {
				return nil, fmt.Errorf("bad time %q", ts)
			}
			k, err := parseKey(kv)
			if err != nil {
				return nil, err
			}
			k.t = float32(t)
			tr.keys = append(tr.keys, k)
		}
		tr.sort()
		return tr, nil
	}
	k, err := parseKey(raw)
	if err != nil {
		return nil, err
	}
	tr.keys = []key{k}
	return tr, nil
}

func isKeyframeObject(m map[string]json.RawMessage) bool {
	for _, k := range []string{"vector", "pre", "post", "lerp_mode", "easing"} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func parseKey(raw json.RawMessage) (key, error) {
	var obj map[string]json.RawMessage
	k := key{}
	if json.Unmarshal(raw, &obj) == nil {
		if lm, ok := obj["lerp_mode"]; ok && strings.Contains(string(lm), "catmullrom") {
			k.interp = model.CatmullRom
		}
		if e, ok := obj["easing"]; ok {
			json.Unmarshal(e, &k.easing)
		}
		var err error
		if v, ok := obj["vector"]; ok {
			if k.pre, err = parseVec(v); err != nil {
				return k, err
			}
			k.post = k.pre
		}
		if v, ok := obj["pre"]; ok {
			if k.pre, err = parseVec(v); err != nil {
				return k, err
			}
			if _, ok := obj["post"]; !ok {
				k.post = k.pre
			}
		}
		if v, ok := obj["post"]; ok {
			if k.post, err = parseVec(v); err != nil {
				return k, err
			}
			if _, ok := obj["pre"]; !ok {
				k.pre = k.post
			}
		}
		if k.pre[0] == nil {
			return k, fmt.Errorf("keyframe without value")
		}
		return k, nil
	}
	v, err := parseVec(raw)
	k.pre, k.post = v, v
	return k, err
}

// parseVec reads [x,y,z] (numbers or Molang strings), a single number or
// string (all components), or {"vector": ...}.
func parseVec(raw json.RawMessage) ([3]Expr, error) {
	var out [3]Expr
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		if v, ok := obj["vector"]; ok {
			return parseVec(v)
		}
		return out, fmt.Errorf("bad vector %s", raw)
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		if len(arr) == 0 {
			return out, fmt.Errorf("empty vector")
		}
		for i := 0; i < 3; i++ {
			e, err := parseScalar(arr[min(i, len(arr)-1)])
			if err != nil {
				return out, err
			}
			out[i] = e
		}
		return out, nil
	}
	e, err := parseScalar(raw)
	out = [3]Expr{e, e, e}
	return out, err
}

func parseScalar(raw json.RawMessage) (Expr, error) {
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return num(f), nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("bad value %s", raw)
	}
	return ParseMolang(s)
}

// ease applies a GeckoLib easing curve (easeInSine, easeOutBack, ...).
func ease(name string, t float32) float32 {
	if name == "" || name == "linear" {
		return t
	}
	x := float64(t)
	var in func(float64) float64
	switch {
	case strings.HasSuffix(name, "Sine"):
		in = func(x float64) float64 { return 1 - math.Cos(x*math.Pi/2) }
	case strings.HasSuffix(name, "Quad"):
		in = func(x float64) float64 { return x * x }
	case strings.HasSuffix(name, "Cubic"):
		in = func(x float64) float64 { return x * x * x }
	case strings.HasSuffix(name, "Quart"):
		in = func(x float64) float64 { return x * x * x * x }
	case strings.HasSuffix(name, "Quint"):
		in = func(x float64) float64 { return x * x * x * x * x }
	case strings.HasSuffix(name, "Expo"):
		in = func(x float64) float64 {
			if x == 0 {
				return 0
			}
			return math.Pow(2, 10*x-10)
		}
	case strings.HasSuffix(name, "Circ"):
		in = func(x float64) float64 { return 1 - math.Sqrt(1-x*x) }
	case strings.HasSuffix(name, "Back"):
		in = func(x float64) float64 { return 2.70158*x*x*x - 1.70158*x*x }
	case strings.HasSuffix(name, "Elastic"):
		in = func(x float64) float64 {
			if x == 0 || x == 1 {
				return x
			}
			return -math.Pow(2, 10*x-10) * math.Sin((x*10-10.75)*2*math.Pi/3)
		}
	case strings.HasSuffix(name, "Bounce"):
		out := func(x float64) float64 {
			const n, d = 7.5625, 2.75
			switch {
			case x < 1/d:
				return n * x * x
			case x < 2/d:
				x -= 1.5 / d
				return n*x*x + 0.75
			case x < 2.5/d:
				x -= 2.25 / d
				return n*x*x + 0.9375
			}
			x -= 2.625 / d
			return n*x*x + 0.984375
		}
		in = func(x float64) float64 { return 1 - out(1-x) }
	case name == "step":
		return 0
	default:
		return t
	}
	switch {
	case strings.HasPrefix(name, "easeInOut"):
		if x < 0.5 {
			return float32(in(2*x) / 2)
		}
		return float32(1 - in(2-2*x)/2)
	case strings.HasPrefix(name, "easeOut"):
		return float32(1 - in(1-x))
	}
	return float32(in(x))
}
