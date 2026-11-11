package particles

// Bedrock particle effects (format 1.10.0, "Snowstorm" files), as TACZ's
// gun muzzle flash and smoke use them through SimpleBedrockModel. An
// Effect is a parsed definition; Effect.Start makes an Emitter, whose
// particles are ordinary Particles: those in emitter space live in the
// emitter and are drawn in its frame (Emitter.Sprites), those sent to the
// world (sbm:fp_emitter_local_space to_world) are added to a System.
//
// Only the components the TACZ definitions use are supported; any other
// component fails Load.

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/importer/bbmodel"
)

// Effect is a loaded particle definition.
type Effect struct {
	ID string
	// Texture is the texture file in the pack ("assets/<ns>/textures/<path>.png");
	// the caller sets it as a renderer sheet and passes that sheet to Start.
	Texture  string
	Additive bool // materials energy_swirl and particles_add
	Lit      bool // particle_appearance_lighting

	local                  localSpace
	count                  bbmodel.Expr
	active                 bbmodel.Expr
	sphere                 bool
	offset, direction      [3]bbmodel.Expr // direction nil: outwards
	radius                 bbmodel.Expr
	life, speed, rot, spin bbmodel.Expr
	drag                   float64
	size                   [2]bbmodel.Expr
	texSize, uvSize        [2]float64
	uv                     [2]bbmodel.Expr // fixed UV; nil with a flipbook
	flipBase, flipStep     [2]float64
	flipFrames             int
	grad                   Gradient
}

// localSpace is sbm:fp_emitter_local_space.
type localSpace struct {
	Position bool `json:"position"`
	Rotation bool `json:"rotation"`
	Velocity bool `json:"velocity"`
	ToWorld  bool `json:"to_world"`
}

// Frame places an emitter: Origin, then local axes x, y, z as Basis
// columns (they may be scaled), and the emitter's velocity in the same
// space (inherited when the definition asks for it).
type Frame struct {
	Origin [3]float64
	Basis  [3][3]float64 // Basis[i] is local axis i
	Vel    [3]float64
}

// Identity is a frame at the origin with unit axes.
var Identity = Frame{Basis: [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}

func (f *Frame) dir(v [3]float64) [3]float64 {
	var o [3]float64
	for i := range 3 {
		for k := range 3 {
			o[k] += f.Basis[i][k] * v[i]
		}
	}
	return o
}

func (f *Frame) point(v [3]float64) [3]float64 {
	o := f.dir(v)
	return [3]float64{o[0] + f.Origin[0], o[1] + f.Origin[1], o[2] + f.Origin[2]}
}

// Bedrock material names.
const (
	materialEnergySwirl  = "energy_swirl"
	materialParticlesAdd = "particles_add"
)

// LoadEffect reads a .particle.json file. Texture ids resolve inside fsys
// like TACZ resource ids ("tacz:particle/x" → assets/tacz/textures/particle/x.png).
func LoadEffect(fsys fs.FS, path string) (*Effect, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, err
	}
	e, err := parseEffect(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return e, nil
}

func parseEffect(data []byte) (*Effect, error) {
	var file struct {
		Format string `json:"format_version"`
		Effect struct {
			Description struct {
				ID     string `json:"identifier"`
				Render struct {
					Material string `json:"material"`
					Texture  string `json:"texture"`
				} `json:"basic_render_parameters"`
			} `json:"description"`
			Components map[string]json.RawMessage `json:"components"`
		} `json:"particle_effect"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	if file.Format != "1.10.0" {
		return nil, fmt.Errorf("format %q, want 1.10.0", file.Format)
	}
	d := file.Effect.Description
	e := &Effect{ID: d.ID, Texture: textureFile(d.Render.Texture)}
	switch d.Render.Material {
	case materialEnergySwirl, materialParticlesAdd:
		e.Additive = true
	default:
		return nil, fmt.Errorf("material %q not supported", d.Render.Material)
	}
	// Defaults of absent components.
	e.count, e.active, e.radius = num(0), num(0), num(0)
	e.life, e.speed, e.rot, e.spin = num(1), num(0), num(0), num(0)
	for k := range e.offset {
		e.offset[k] = num(0)
	}
	e.size = [2]bbmodel.Expr{num(0), num(0)}
	for name, raw := range file.Effect.Components {
		if err := e.component(name, raw); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	if e.grad == nil {
		e.grad = Gradient{{0, [4]float32{1, 1, 1, 1}}}
	}
	return e, e.check()
}

func num(v float64) bbmodel.Expr {
	e, _ := bbmodel.ParseMolang(strconv.FormatFloat(v, 'g', -1, 64))
	return e
}

// textureFile turns "ns:path" into assets/ns/textures/path.png.
func textureFile(id string) string {
	ns, p, ok := strings.Cut(id, ":")
	if !ok {
		ns, p = "minecraft", id
	}
	return "assets/" + ns + "/textures/" + p + ".png"
}

// expr reads a Molang value: a number or an expression string.
func expr(raw json.RawMessage) (bbmodel.Expr, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	switch v := v.(type) {
	case float64:
		return num(v), nil
	case string:
		return bbmodel.ParseMolang(v)
	}
	return nil, fmt.Errorf("molang value %s", raw)
}

func exprs(raw []json.RawMessage, out []bbmodel.Expr) error {
	if len(raw) != len(out) {
		return fmt.Errorf("want %d values, got %d", len(out), len(raw))
	}
	for i, r := range raw {
		var err error
		if out[i], err = expr(r); err != nil {
			return err
		}
	}
	return nil
}

func (e *Effect) component(name string, raw json.RawMessage) error {
	switch name {
	case "sbm:fp_emitter_local_space":
		return json.Unmarshal(raw, &e.local)
	case "minecraft:emitter_rate_instant":
		var c struct {
			N json.RawMessage `json:"num_particles"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return err
		}
		var err error
		e.count, err = expr(c.N)
		return err
	case "minecraft:emitter_lifetime_once":
		var c struct {
			T json.RawMessage `json:"active_time"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return err
		}
		var err error
		e.active, err = expr(c.T)
		return err
	case "minecraft:emitter_shape_point", "minecraft:emitter_shape_sphere":
		var c struct {
			Offset    []json.RawMessage `json:"offset"`
			Radius    json.RawMessage   `json:"radius"`
			Direction []json.RawMessage `json:"direction"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return err
		}
		e.sphere = name == "minecraft:emitter_shape_sphere"
		if c.Offset != nil {
			if err := exprs(c.Offset, e.offset[:]); err != nil {
				return err
			}
		}
		if c.Radius != nil {
			var err error
			if e.radius, err = expr(c.Radius); err != nil {
				return err
			}
		}
		if c.Direction != nil {
			return exprs(c.Direction, e.direction[:])
		}
		return nil
	case "minecraft:particle_lifetime_expression":
		var c struct {
			Max json.RawMessage `json:"max_lifetime"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return err
		}
		var err error
		e.life, err = expr(c.Max)
		return err
	case "minecraft:particle_initial_speed":
		var err error
		e.speed, err = expr(raw)
		return err
	case "minecraft:particle_initial_spin":
		var c struct {
			Rot  json.RawMessage `json:"rotation"`
			Rate json.RawMessage `json:"rotation_rate"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return err
		}
		var err error
		if e.rot, err = expr(c.Rot); err != nil {
			return err
		}
		e.spin, err = expr(c.Rate)
		return err
	case "minecraft:particle_motion_dynamic":
		var c struct {
			Drag float64 `json:"linear_drag_coefficient"`
		}
		err := json.Unmarshal(raw, &c)
		e.drag = c.Drag
		return err
	case "minecraft:particle_appearance_lighting":
		e.Lit = true
		return nil
	case "minecraft:particle_appearance_billboard":
		return e.billboard(raw)
	case "minecraft:particle_appearance_tinting":
		return e.tinting(raw)
	}
	return fmt.Errorf("component not supported")
}

func (e *Effect) billboard(raw json.RawMessage) error {
	var c struct {
		Size   []json.RawMessage `json:"size"`
		Facing string            `json:"facing_camera_mode"`
		UV     struct {
			W      float64           `json:"texture_width"`
			H      float64           `json:"texture_height"`
			UV     []json.RawMessage `json:"uv"`
			UVSize [2]float64        `json:"uv_size"`
			Flip   *struct {
				Base    [2]float64 `json:"base_UV"`
				Size    [2]float64 `json:"size_UV"`
				Step    [2]float64 `json:"step_UV"`
				Max     int        `json:"max_frame"`
				Stretch bool       `json:"stretch_to_lifetime"`
			} `json:"flipbook"`
		} `json:"uv"`
	}
	c.UV.W, c.UV.H = 1, 1
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	if c.Facing != "rotate_xyz" { // the renderer's billboards face the camera fully
		return fmt.Errorf("facing_camera_mode %q not supported", c.Facing)
	}
	if err := exprs(c.Size, e.size[:]); err != nil {
		return err
	}
	e.texSize = [2]float64{c.UV.W, c.UV.H}
	switch f := c.UV.Flip; {
	case f != nil:
		if !f.Stretch {
			return fmt.Errorf("flipbook without stretch_to_lifetime not supported")
		}
		e.flipBase, e.uvSize, e.flipStep, e.flipFrames = f.Base, f.Size, f.Step, f.Max
	case c.UV.UV != nil:
		e.uvSize = c.UV.UVSize
		return exprs(c.UV.UV, e.uv[:])
	default:
		e.uvSize = e.texSize
		e.uv = [2]bbmodel.Expr{num(0), num(0)}
	}
	return nil
}

func (e *Effect) tinting(raw json.RawMessage) error {
	var c struct {
		Color struct {
			Interpolant string            `json:"interpolant"`
			Gradient    map[string]string `json:"gradient"`
		} `json:"color"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return err
	}
	// The gradient runs over life (Particle.Grad); check the interpolant is
	// age / lifetime by evaluating it.
	in, err := bbmodel.ParseMolang(c.Color.Interpolant)
	if err != nil {
		return err
	}
	for _, s := range [][2]float64{{0.3, 1.2}, {0.05, 0.1}} {
		ctx := bbmodel.Ctx{Vars: map[string]float64{"particle_age": s[0], "particle_lifetime": s[1]}}
		if got := in.Eval(&ctx); math.Abs(got-s[0]/s[1]) > 1e-9 {
			return fmt.Errorf("interpolant %q is not v.particle_age / v.particle_lifetime", c.Color.Interpolant)
		}
	}
	for k, hex := range c.Color.Gradient {
		t, err := strconv.ParseFloat(k, 64)
		if err != nil {
			return err
		}
		col, err := argb(hex)
		if err != nil {
			return err
		}
		e.grad = append(e.grad, GradStop{T: float32(t), Col: col})
	}
	if len(e.grad) == 0 {
		return fmt.Errorf("empty gradient")
	}
	sort.Slice(e.grad, func(i, j int) bool { return e.grad[i].T < e.grad[j].T })
	return nil
}

// argb parses Bedrock's "#AARRGGBB" into RGBA 0..1.
func argb(s string) ([4]float32, error) {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if err != nil || len(s) != 9 {
		return [4]float32{}, fmt.Errorf("colour %q, want #AARRGGBB", s)
	}
	c := func(shift uint) float32 { return float32(v>>shift&0xff) / 255 }
	return [4]float32{c(16), c(8), c(0), c(24)}, nil
}

// check rejects sizes Particle cannot carry: Size0..Size1 is linear over
// life, so the size must be affine in v.particle_age.
func (e *Effect) check() error {
	ctx := bbmodel.Ctx{Vars: map[string]float64{"particle_lifetime": 1, "particle_random_1": 0.5,
		"particle_random_2": 0.5, "particle_random_3": 0.5, "particle_random_4": 0.5}, Rand: rand.New(rand.NewSource(1))}
	at := func(t float64) float64 { ctx.Vars["particle_age"] = t; return e.size[0].Eval(&ctx) }
	if a, b, m := at(0), at(1), at(0.5); math.Abs((a+b)/2-m) > 1e-9 {
		return fmt.Errorf("billboard size is not linear in v.particle_age")
	}
	return nil
}

// Emitter is a running effect.
type Emitter struct {
	fx  *Effect
	ctx bbmodel.Ctx
	age float64
	// started: the instant burst is out.
	started bool
	active  float64
	// Local places the emitter for its own particles (the hand's view
	// space in first person); World places it for those sent to the world.
	Local, World Frame
	// WorldScale scales the size of particles sent to the world (TACZ's
	// getWorldParticleScale: tan(worldFov/2) / tan(modelFov/2) inverted).
	WorldScale float64
	sheet      uint8
	own        System // particles in emitter space
}

// Start makes an emitter with the Molang variables vars (keys without the
// "v." prefix, lower case), drawing randomness from rng and its texture
// from renderer sheet. Set its frames before the first Tick.
func (e *Effect) Start(vars map[string]float64, rng *rand.Rand, sheet uint8) *Emitter {
	v := make(map[string]float64, len(vars)+8)
	for k, x := range vars {
		v[k] = x
	}
	em := &Emitter{fx: e, ctx: bbmodel.Ctx{Vars: v, Rand: rng}, Local: Identity, World: Identity, WorldScale: 1, sheet: sheet}
	em.active = e.active.Eval(&em.ctx)
	return em
}

// Done reports whether the emitter is past its active time with no own
// particles left.
func (em *Emitter) Done() bool { return em.started && em.age >= em.active && len(em.own.P) == 0 }

// Tick emits the instant burst on the first call (Tick(0) shows it this
// frame), then ages the emitter and moves its own particles by dt seconds.
// World particles go to world, which ticks them itself.
func (em *Emitter) Tick(dt float64, world *System) {
	if !em.started {
		em.started = true
		n := int(em.fx.count.Eval(&em.ctx))
		for range n {
			em.spawn(world)
		}
	}
	em.age += dt
	em.own.Tick(dt, nil)
}

// Sprites appends the emitter's own particles in its Local frame.
func (em *Emitter) Sprites(buf []Particle) []Particle {
	ls := em.fx.local
	for _, p := range em.own.P {
		at := func(x, y, z float64) (float64, float64, float64) {
			v := [3]float64{x, y, z}
			if ls.Rotation {
				v = em.Local.dir(v)
			}
			if ls.Position {
				v = [3]float64{v[0] + em.Local.Origin[0], v[1] + em.Local.Origin[1], v[2] + em.Local.Origin[2]}
			}
			return v[0], v[1], v[2]
		}
		p.X, p.Y, p.Z = at(p.X, p.Y, p.Z)
		p.PrevX, p.PrevY, p.PrevZ = at(p.PrevX, p.PrevY, p.PrevZ)
		buf = append(buf, p)
	}
	return buf
}

func (em *Emitter) spawn(world *System) {
	e, c, r := em.fx, &em.ctx, em.ctx.Rand
	for i := 1; i <= 4; i++ {
		c.Vars["particle_random_"+strconv.Itoa(i)] = r.Float64()
	}
	c.Vars["particle_age"] = 0
	life := e.life.Eval(c)
	c.Vars["particle_lifetime"] = life

	var off, pos, dir [3]float64
	for k := range 3 {
		off[k] = e.offset[k].Eval(c)
	}
	pos = off
	if e.sphere {
		// Uniform in the ball: a random direction at radius r·∛u.
		rad := e.radius.Eval(c) * math.Cbrt(r.Float64())
		d := randomUnit(r)
		for k := range 3 {
			pos[k] += d[k] * rad
		}
	}
	if e.direction[0] != nil {
		for k := range 3 {
			dir[k] = e.direction[k].Eval(c)
		}
	}
	if dir == ([3]float64{}) { // no or zero direction: outwards from the shape's centre
		dir = [3]float64{pos[0] - off[0], pos[1] - off[1], pos[2] - off[2]}
	}
	if l := math.Sqrt(dir[0]*dir[0] + dir[1]*dir[1] + dir[2]*dir[2]); l > 0 {
		sp := e.speed.Eval(c) / l
		dir = [3]float64{dir[0] * sp, dir[1] * sp, dir[2] * sp}
	}

	size := func(age float64) float32 { c.Vars["particle_age"] = age; return float32(2 * e.size[0].Eval(c)) } // Bedrock size is the half extent
	p := Particle{Life: life, Size0: size(0), Size1: size(life), Drag: e.drag, Ghost: true,
		Additive: e.Additive, Lit: e.Lit, Grad: e.grad, Sheet: em.sheet,
		Roll: float32(e.rot.Eval(c) * math.Pi / 180), Spin: float32(e.spin.Eval(c) * math.Pi / 180)}
	c.Vars["particle_age"] = 0
	var u, v float64
	if e.flipFrames > 0 {
		u, v = e.flipBase[0], e.flipBase[1]
		p.Frames = uint8(e.flipFrames)
		p.Step = [2]float32{float32(e.flipStep[0] / e.texSize[0]), float32(e.flipStep[1] / e.texSize[1])}
	} else {
		u, v = e.uv[0].Eval(c), e.uv[1].Eval(c)
	}
	p.UV = [4]float32{float32(u / e.texSize[0]), float32(v / e.texSize[1]),
		float32((u + e.uvSize[0]) / e.texSize[0]), float32((v + e.uvSize[1]) / e.texSize[1])}

	ls := e.local
	if ls.ToWorld {
		f := &em.World
		pos, dir = f.point(pos), f.dir(dir)
		if ls.Velocity {
			dir = [3]float64{dir[0] + f.Vel[0], dir[1] + f.Vel[1], dir[2] + f.Vel[2]}
		}
		p.Size0, p.Size1 = p.Size0*float32(em.WorldScale), p.Size1*float32(em.WorldScale)
	} else {
		// What does not follow the emitter is fixed by its frame now.
		f := &em.Local
		if !ls.Rotation {
			pos, dir = f.dir(pos), f.dir(dir)
		}
		if !ls.Position {
			pos = [3]float64{pos[0] + f.Origin[0], pos[1] + f.Origin[1], pos[2] + f.Origin[2]}
		}
		if ls.Velocity {
			dir = [3]float64{dir[0] + f.Vel[0], dir[1] + f.Vel[1], dir[2] + f.Vel[2]}
		}
	}
	p.X, p.Y, p.Z = pos[0], pos[1], pos[2]
	p.PrevX, p.PrevY, p.PrevZ = p.X, p.Y, p.Z
	p.VX, p.VY, p.VZ = dir[0], dir[1], dir[2]
	if ls.ToWorld {
		world.Add(p)
	} else {
		em.own.Add(p)
	}
}

func randomUnit(r *rand.Rand) [3]float64 {
	for {
		d := [3]float64{r.Float64()*2 - 1, r.Float64()*2 - 1, r.Float64()*2 - 1}
		if l := d[0]*d[0] + d[1]*d[1] + d[2]*d[2]; l > 1e-6 && l <= 1 {
			l = math.Sqrt(l)
			return [3]float64{d[0] / l, d[1] / l, d[2] / l}
		}
	}
}
