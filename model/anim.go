package model

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"
)

// Target is the bone property a channel animates.
type Target uint8

const (
	Translation Target = iota
	Rotation           // quaternion (x,y,z,w), or Euler degrees when Additive
	Scale
)

// Track yields a channel value at time t (seconds). Rotation tracks return
// a quaternion as (x,y,z,w) unless the channel is additive (Euler degrees).
type Track interface {
	Sample(t float32) mgl32.Vec4
}

// Channel animates one property of one bone.
type Channel struct {
	Bone   int
	Target Target
	// Additive channels add to the bind pose (Bedrock style): translation
	// is an offset, rotation adds Euler degrees, scale multiplies.
	// Otherwise the value replaces the bind pose (glTF style).
	Additive bool
	Track    Track
}

// Loop says what happens at the end of a clip.
type Loop uint8

const (
	LoopRepeat Loop = iota
	LoopOnce        // stop at the last frame; Animator.Done reports true
	LoopHold        // like LoopOnce, but stays posed
)

// Clip is a named animation.
type Clip struct {
	Name     string
	Length   float32 // seconds
	Loop     Loop
	Channels []Channel
}

// Interp is a keyframe interpolation mode.
type Interp uint8

const (
	Linear Interp = iota
	Step
	CatmullRom
)

// Keyframes is a plain keyframe track. Rotation tracks with quaternion
// values are interpolated with slerp when Quat is set.
type Keyframes struct {
	Times  []float32
	Values []mgl32.Vec4
	Interp Interp
	Quat   bool
}

func (k *Keyframes) Sample(t float32) mgl32.Vec4 {
	n := len(k.Times)
	if n == 0 {
		return mgl32.Vec4{}
	}
	if t <= k.Times[0] || n == 1 {
		return k.Values[0]
	}
	if t >= k.Times[n-1] {
		return k.Values[n-1]
	}
	i := 0
	for i+1 < n && k.Times[i+1] <= t {
		i++
	}
	a, b := k.Values[i], k.Values[i+1]
	f := (t - k.Times[i]) / (k.Times[i+1] - k.Times[i])
	switch {
	case k.Interp == Step:
		return a
	case k.Quat:
		qa := mgl32.Quat{W: a[3], V: a.Vec3()}
		qb := mgl32.Quat{W: b[3], V: b.Vec3()}
		if qa.Dot(qb) < 0 {
			qb = qb.Scale(-1)
		}
		q := mgl32.QuatNlerp(qa, qb, f)
		return mgl32.Vec4{q.V[0], q.V[1], q.V[2], q.W}
	case k.Interp == CatmullRom:
		p0, p3 := a, b
		if i > 0 {
			p0 = k.Values[i-1]
		}
		if i+2 < n {
			p3 = k.Values[i+2]
		}
		return CatmullRomVec(p0, a, b, p3, f)
	}
	return a.Add(b.Sub(a).Mul(f))
}

// CatmullRomVec interpolates between p1 and p2.
func CatmullRomVec(p0, p1, p2, p3 mgl32.Vec4, t float32) mgl32.Vec4 {
	t2, t3 := t*t, t*t*t
	var out mgl32.Vec4
	for i := 0; i < 4; i++ {
		out[i] = 0.5 * ((2 * p1[i]) + (-p0[i]+p2[i])*t +
			(2*p0[i]-5*p1[i]+4*p2[i]-p3[i])*t2 + (-p0[i]+3*p1[i]-3*p2[i]+p3[i])*t3)
	}
	return out
}

// Animator plays clips on a model and produces skinning matrices.
type Animator struct {
	Model *Model
	Clip  *Clip
	Time  float32
	Speed float32 // playback rate, 1 = normal
	// Fade blends from the previous pose over FadeTime seconds.
	prev     []pose
	fade     float32
	FadeTime float32

	poses   []pose
	world   []mgl32.Mat4
	scratch []pose
}

type pose struct {
	t mgl32.Vec3
	r mgl32.Quat
	s mgl32.Vec3
}

// NewAnimator returns an animator in the bind pose.
func NewAnimator(m *Model) *Animator {
	return &Animator{Model: m, Speed: 1, FadeTime: 0.2}
}

// Play switches to a clip by name (no-op if already playing it, unless
// restart). Unknown names are ignored and return false.
func (a *Animator) Play(name string, restart bool) bool {
	c := a.Model.Clips[name]
	if c == nil {
		return false
	}
	if c == a.Clip && !restart {
		return true
	}
	if a.Clip != nil && a.FadeTime > 0 && len(a.poses) == len(a.Model.Bones) {
		a.prev = append(a.prev[:0], a.poses...)
		a.fade = a.FadeTime
	}
	a.Clip, a.Time = c, 0
	return true
}

// Done reports whether a LoopOnce clip has finished.
func (a *Animator) Done() bool {
	return a.Clip != nil && a.Clip.Loop != LoopRepeat && a.Time >= a.Clip.Length
}

// Update advances time by dt seconds.
func (a *Animator) Update(dt float32) {
	a.Time += dt * a.Speed
	if a.fade > 0 {
		a.fade -= dt
	}
	if c := a.Clip; c != nil && c.Length > 0 {
		if c.Loop == LoopRepeat {
			a.Time = float32(math.Mod(float64(a.Time), float64(c.Length)))
		} else if a.Time > c.Length {
			a.Time = c.Length
		}
	}
}

// Skin computes one skinning matrix per bone (model root * bone world *
// inverse bind) into dst, which is grown as needed and returned.
func (a *Animator) Skin(dst []mgl32.Mat4) []mgl32.Mat4 {
	m := a.Model
	n := len(m.Bones)
	if cap(a.poses) < n {
		a.poses, a.world = make([]pose, n), make([]mgl32.Mat4, n)
		a.scratch = make([]pose, n)
	}
	a.poses, a.world = a.poses[:n], a.world[:n]
	euler := a.scratch[:n] // .t holds the Euler accumulator
	for i, b := range m.Bones {
		a.poses[i] = pose{b.T, b.R, b.S}
		euler[i].t = b.Euler
	}
	if c := a.Clip; c != nil {
		for _, ch := range c.Channels {
			if ch.Bone < 0 || ch.Bone >= n {
				continue
			}
			v := ch.Track.Sample(a.Time)
			p := &a.poses[ch.Bone]
			switch {
			case ch.Target == Translation && ch.Additive:
				p.t = p.t.Add(v.Vec3())
			case ch.Target == Translation:
				p.t = v.Vec3()
			case ch.Target == Rotation && ch.Additive:
				euler[ch.Bone].t = euler[ch.Bone].t.Add(v.Vec3())
			case ch.Target == Rotation:
				p.r = mgl32.Quat{W: v[3], V: v.Vec3()}.Normalize()
			case ch.Target == Scale && ch.Additive:
				p.s = mgl32.Vec3{p.s[0] * v[0], p.s[1] * v[1], p.s[2] * v[2]}
			case ch.Target == Scale:
				p.s = v.Vec3()
			}
		}
	}
	for i, b := range m.Bones {
		if b.UseEuler {
			a.poses[i].r = EulerZYX(euler[i].t)
		}
	}
	if a.fade > 0 && len(a.prev) == n {
		f := 1 - a.fade/a.FadeTime
		for i := range a.poses {
			p, q := &a.poses[i], a.prev[i]
			p.t = q.t.Add(p.t.Sub(q.t).Mul(f))
			p.s = q.s.Add(p.s.Sub(q.s).Mul(f))
			p.r = mgl32.QuatNlerp(q.r, p.r, f)
		}
	}
	root := m.root0()
	if cap(dst) < n {
		dst = make([]mgl32.Mat4, n)
	}
	dst = dst[:n]
	for i, b := range m.Bones {
		p := a.poses[i]
		local := mgl32.Translate3D(p.t[0], p.t[1], p.t[2]).Mul4(p.r.Mat4()).Mul4(mgl32.Scale3D(p.s[0], p.s[1], p.s[2]))
		if b.Parent >= 0 {
			a.world[i] = a.world[b.Parent].Mul4(local)
		} else {
			a.world[i] = local
		}
		dst[i] = root.Mul4(a.world[i]).Mul4(b.InvBind)
	}
	return dst
}

// BindWorld returns each bone's bind-pose world matrix (no animation);
// importers use it to compute inverse bind matrices.
func (m *Model) BindWorld() []mgl32.Mat4 {
	w := make([]mgl32.Mat4, len(m.Bones))
	for i, b := range m.Bones {
		r := b.R
		if b.UseEuler {
			r = EulerZYX(b.Euler)
		}
		local := mgl32.Translate3D(b.T[0], b.T[1], b.T[2]).Mul4(r.Mat4()).Mul4(mgl32.Scale3D(b.S[0], b.S[1], b.S[2]))
		if b.Parent >= 0 {
			w[i] = w[b.Parent].Mul4(local)
		} else {
			w[i] = local
		}
	}
	return w
}

// SkinWorld computes skinning matrices from caller-posed bone world
// matrices (model space, one per bone) instead of clips: procedural
// animation that sets every bone's transform in code each frame. dst is
// grown as needed and returned.
func (m *Model) SkinWorld(world []mgl32.Mat4, dst []mgl32.Mat4) []mgl32.Mat4 {
	n := len(m.Bones)
	if cap(dst) < n {
		dst = make([]mgl32.Mat4, n)
	}
	dst = dst[:n]
	root := m.root0()
	for i, b := range m.Bones {
		dst[i] = root.Mul4(world[i]).Mul4(b.InvBind)
	}
	return dst
}
