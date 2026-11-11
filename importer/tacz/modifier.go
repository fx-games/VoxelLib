package tacz

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/memmaker/voxellib/weapon"
)

// appliedData are the attachment data keys the spec or model uses: the
// modifiers (AttachmentPropertyManager's ids and optional fields) mapped to
// weapon.Gun, and AttachmentData's extended_mag_level, melee and silence.
var appliedData = []string{
	"ads", "ads_addend", "weight", "weight_modifier",
	"inaccuracy", "inaccuracy_addend", "aim_inaccuracy", "sneak_inaccuracy",
	"recoil", "recoil_modifier", "rpm", "ammo_speed", "damage", "effective_range",
	"head_shot", "pierce", "knockback", "movement_speed",
	"extended_mag_level", "melee", "silence",
}

// UnsupportedData are the attachment data keys TACZ reads that the spec has
// no use for, with the reason. An installed attachment's data with a key in
// neither list fails.
var UnsupportedData = map[string]string{
	"lie_inaccuracy":     "no prone stance",
	"armor_ignore":       "no armor",
	"explosion":          "no explosive bullets",
	"ignite":             "no incendiary bullets",
	"recoil_push":        "no recoil push",
	"flash_hider":        "muzzle particle overrides not applied",
	"smoke_hider":        "muzzle particle overrides not applied",
	"smoke_speed_scale":  "muzzle particle overrides not applied",
	"smoke_count_scale":  "muzzle particle overrides not applied",
	"smoke_direction":    "muzzle particle overrides not applied",
	"smoke_directions":   "muzzle particle overrides not applied",
	"smoke_spread":       "muzzle particle overrides not applied",
	"smoke_spread_angle": "muzzle particle overrides not applied",
}

// attMods is an installed attachment's modifiers (identity where absent).
type attMods struct {
	ads, weight, rpm, ammoSpeed, damage, headShot, pierce, knockback modifier
	stand, move, sneak, aim                                          modifier          // inaccuracy by stance
	pitch, yaw                                                       modifier          // recoil
	effectiveRange                                                   *modifier         // nil: none
	moveSpeed                                                        *weapon.MoveSpeed // nil: none
}

// parseMods reads an attachment data's modifiers as the modifiers'
// readJson do (deprecated forms included).
func parseMods(raw []byte) (attMods, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return attMods{}, err
	}
	for k := range keys {
		if _, ok := UnsupportedData[k]; !ok && !slices.Contains(appliedData, k) {
			return attMods{}, fmt.Errorf("unknown attachment data %q", k)
		}
	}
	var d struct {
		Ads              *modifier `json:"ads"`
		AdsAddend        float64   `json:"ads_addend"`
		Weight           float64   `json:"weight"`
		WeightModifier   *modifier `json:"weight_modifier"`
		Inaccuracy       *modifier `json:"inaccuracy"`
		InaccuracyAddend float64   `json:"inaccuracy_addend"`
		AimInaccuracy    *modifier `json:"aim_inaccuracy"`
		SneakInaccuracy  *modifier `json:"sneak_inaccuracy"`
		Recoil           *struct{ Pitch, Yaw *modifier }
		RecoilModifier   *struct{ Pitch, Yaw float64 } `json:"recoil_modifier"`
		RPM              *modifier                     `json:"rpm"`
		AmmoSpeed        *modifier                     `json:"ammo_speed"`
		Damage           *modifier                     `json:"damage"`
		EffectiveRange   *modifier                     `json:"effective_range"`
		HeadShot         *modifier                     `json:"head_shot"`
		Pierce           *modifier                     `json:"pierce"`
		Knockback        *modifier                     `json:"knockback"`
		MoveSpeed        *weapon.MoveSpeed             `json:"movement_speed"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return attMods{}, err
	}
	or := func(m *modifier, def modifier) modifier {
		if m == nil {
			return def
		}
		return *m
	}
	addend := func(v float64) modifier { return modifier{addend: v, multiplier: 1} }
	m := attMods{
		ads:    or(d.Ads, addend(d.AdsAddend)),
		weight: or(d.WeightModifier, addend(d.Weight)),
		rpm:    or(d.RPM, identity), ammoSpeed: or(d.AmmoSpeed, identity), damage: or(d.Damage, identity),
		headShot: or(d.HeadShot, identity), pierce: or(d.Pierce, identity), knockback: or(d.Knockback, identity),
		// inaccuracy: every stance but aim, sneak and lie
		stand: or(d.Inaccuracy, addend(d.InaccuracyAddend)), move: or(d.Inaccuracy, addend(d.InaccuracyAddend)),
		sneak: or(d.SneakInaccuracy, identity), aim: or(d.AimInaccuracy, identity),
		pitch: identity, yaw: identity,
		effectiveRange: d.EffectiveRange, moveSpeed: d.MoveSpeed,
	}
	switch {
	case d.Recoil != nil:
		m.pitch, m.yaw = or(d.Recoil.Pitch, identity), or(d.Recoil.Yaw, identity)
	case d.RecoilModifier != nil: // the deprecated form: percents
		m.pitch = modifier{percent: d.RecoilModifier.Pitch, multiplier: 1}
		m.yaw = modifier{percent: d.RecoilModifier.Yaw, multiplier: 1}
	}
	return m, nil
}

// RPM bounds of GunData.getShootInterval.
const (
	minRPM = 1
	maxRPM = 1200
)

// noAdjustRange is the effective range of a gun without damage_adjust
// (EffectiveRangeModifier: Integer.MAX_VALUE).
const noAdjustRange = math.MaxInt32

// applyMods applies the installed attachments' modifiers (in slot order)
// to g as the modifiers' eval and their readers do. It runs without
// attachments too: TACZ always reads the cache (the RPM clamp of
// GunData.getShootInterval, Math.round of rpm and pierce).
func applyMods(g *weapon.Gun, atts []attMods) error {
	all := func(f func(*attMods) modifier) []modifier {
		out := make([]modifier, len(atts))
		for i := range atts {
			out[i] = f(&atts[i])
		}
		return out
	}
	ev := func(f func(*attMods) modifier, base float32) float32 { return float32(eval(all(f), float64(base))) }
	g.Aim = ev(func(a *attMods) modifier { return a.ads }, g.Aim)
	g.Weight = ev(func(a *attMods) modifier { return a.weight }, g.Weight)
	// ModernKineticGunScriptAPI clamps the spread at 0
	in := &g.Inaccuracy
	in.Stand = max(0, ev(func(a *attMods) modifier { return a.stand }, in.Stand))
	in.Move = max(0, ev(func(a *attMods) modifier { return a.move }, in.Move))
	in.Sneak = max(0, ev(func(a *attMods) modifier { return a.sneak }, in.Sneak))
	in.Aim = max(0, ev(func(a *attMods) modifier { return a.aim }, in.Aim))
	// CameraSetupEvent scales the recoil curves by the ParameterizedCache of the aim factor
	for _, c := range []struct {
		keys *[]weapon.RecoilKey
		mod  func(*attMods) modifier
		name string
	}{{&g.Recoil.Pitch, func(a *attMods) modifier { return a.pitch }, "pitch"},
		{&g.Recoil.Yaw, func(a *attMods) modifier { return a.yaw }, "yaw"}} {
		k, err := evalScale(all(c.mod))
		if err != nil {
			return fmt.Errorf("recoil %s: %w", c.name, err)
		}
		keys := slices.Clone(*c.keys)
		for i := range keys {
			keys[i].Min, keys[i].Max = keys[i].Min*float32(k), keys[i].Max*float32(k)
		}
		*c.keys = keys
	}
	g.RPM = float32(min(max(javaRound(eval(all(func(a *attMods) modifier { return a.rpm }), float64(g.RPM))), minRPM), maxRPM))
	b := &g.Bullet
	b.Speed = ev(func(a *attMods) modifier { return a.ammoSpeed }, b.Speed)
	b.Headshot = ev(func(a *attMods) modifier { return a.headShot }, b.Headshot)
	// EntityKineticBullet clamps the knockback at 0
	b.Knockback = max(0, ev(func(a *attMods) modifier { return a.knockback }, b.Knockback))
	b.Pierce = javaRound(eval(all(func(a *attMods) modifier { return a.pierce }), float64(b.Pierce)))
	damage := func(a *attMods) modifier { return a.damage }
	b.Damage = ev(damage, b.Damage)
	b.Falloff = slices.Clone(b.Falloff)
	for i := range b.Falloff {
		b.Falloff[i].Damage = ev(damage, b.Falloff[i].Damage)
	}
	// EffectiveRangeModifier: the first damage_adjust distance (the bullet
	// compares each pair's distance with it); without damage_adjust, the one
	// pair (Integer.MAX_VALUE, damage).
	var ranges []modifier
	for _, a := range atts {
		if a.effectiveRange != nil {
			ranges = append(ranges, *a.effectiveRange)
		}
	}
	if len(ranges) > 0 {
		first := b.Falloff[0].Dist
		r := float32(eval(ranges, float64(first)))
		for i := range b.Falloff {
			if b.Falloff[i].Dist == first {
				b.Falloff[i].Dist = r
			}
		}
	}
	// ExtraMovementModifier: MoveSpeed.of sums and clamps at 0
	for _, a := range atts {
		if s := a.moveSpeed; s != nil {
			g.MoveSpeed.Base, g.MoveSpeed.Aim, g.MoveSpeed.Reload = g.MoveSpeed.Base+s.Base, g.MoveSpeed.Aim+s.Aim, g.MoveSpeed.Reload+s.Reload
		}
	}
	if slices.ContainsFunc(atts, func(a attMods) bool { return a.moveSpeed != nil }) {
		ms := &g.MoveSpeed
		ms.Base, ms.Aim, ms.Reload = max(0, ms.Base), max(0, ms.Aim), max(0, ms.Reload)
	}
	return nil
}

// Attachment modifiers as TACZ evaluates them (AttachmentPropertyManager.eval,
// ParameterizedCache): every installed attachment's modifier for a property
// (its data, or the gun's exclusive_attachments entry) is gathered, then
// value = (base + Σaddend) · max(1 + Σpercent, 0) · Π max(multiplier, 0),
// and each modifier's Lua function in slot order maps (x = value, r = base)
// to y.

// modifier is a TACZ Modifier.
type modifier struct {
	addend, percent, multiplier float64
	function                    *luaFunc // nil: none
}

// identity is a modifier that changes nothing (the absent one).
var identity = modifier{multiplier: 1}

func (m *modifier) UnmarshalJSON(b []byte) error {
	var raw struct {
		Addend, Percent float64
		Multiplier      *float64
		Function        string
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*m = modifier{addend: raw.Addend, percent: raw.Percent, multiplier: 1}
	if raw.Multiplier != nil {
		m.multiplier = *raw.Multiplier
	}
	if raw.Function != "" { // AttachmentScriptManager.compile: "" is none
		f, err := compileLua(raw.Function)
		if err != nil {
			return fmt.Errorf("function %q: %w", raw.Function, err)
		}
		m.function = f
	}
	return nil
}

// eval is AttachmentPropertyManager.eval(modifiers, base).
func eval(mods []modifier, base float64) float64 {
	add, pct, mul := base, 1.0, 1.0
	for _, m := range mods {
		add += m.addend
		pct += m.percent
		mul *= max(m.multiplier, 0)
	}
	v := add * max(pct, 0) * mul
	for _, m := range mods {
		if m.function != nil {
			v = m.function.eval(v, base)
		}
	}
	return v
}

// evalScale is the factor a ParameterizedCache scales its input by (the
// recoil modifiers): exact only without addends or functions, which make
// the result depend on the input.
func evalScale(mods []modifier) (float64, error) {
	pct, mul := 1.0, 1.0
	for _, m := range mods {
		if m.addend != 0 || m.function != nil {
			return 0, fmt.Errorf("only percent and multiplier are supported")
		}
		pct += m.percent
		mul *= max(m.multiplier, 0)
	}
	return max(pct, 0) * mul, nil
}

// javaRound is Java's Math.round (half up).
func javaRound(v float64) int { return int(math.Floor(v + 0.5)) }

// A Lua modifier function, in the subset the packs use: statements
// "y = expr" and "if cond then … [elseif cond then …] [else …] end";
// expressions of numbers, x, r, + - * /, unary -, parentheses and the
// comparisons < > <= >= == ~=. Anything else fails to compile (TACZ runs
// full Lua). The source is lower-cased first, as TACZ does.
type luaFunc struct{ run func(*luaEnv) }

type luaEnv struct {
	x, r, y float64
	ySet    bool
}

// eval is CompiledScript.eval: y if the function set it, else value.
func (f *luaFunc) eval(value, base float64) float64 {
	env := luaEnv{x: value, r: base}
	f.run(&env)
	if !env.ySet {
		return value
	}
	return env.y
}

func compileLua(src string) (*luaFunc, error) {
	toks, err := luaTokens(strings.ToLower(src))
	if err != nil {
		return nil, err
	}
	p := &luaParser{toks: toks}
	run, err := p.block()
	if err != nil {
		return nil, err
	}
	if p.i < len(toks) {
		return nil, fmt.Errorf("unexpected %q", toks[p.i])
	}
	return &luaFunc{run: run}, nil
}

var luaOps = []string{"==", "~=", "<=", ">=", "<", ">", "+", "-", "*", "/", "(", ")", "=", ";"}

func luaTokens(s string) ([]string, error) {
	var out []string
	for i := 0; i < len(s); {
		c := rune(s[i])
		switch {
		case unicode.IsSpace(c):
			i++
		case unicode.IsLetter(c) || c == '_':
			j := i
			for j < len(s) && (unicode.IsLetter(rune(s[j])) || unicode.IsDigit(rune(s[j])) || s[j] == '_') {
				j++
			}
			out, i = append(out, s[i:j]), j
		case unicode.IsDigit(c) || c == '.':
			j := i
			for j < len(s) && (unicode.IsDigit(rune(s[j])) || s[j] == '.' || s[j] == 'e' ||
				(s[j] == '-' || s[j] == '+') && s[j-1] == 'e') {
				j++
			}
			out, i = append(out, s[i:j]), j
		default:
			op := ""
			for _, o := range luaOps {
				if strings.HasPrefix(s[i:], o) {
					op = o
					break
				}
			}
			if op == "" {
				return nil, fmt.Errorf("unsupported character %q", c)
			}
			out, i = append(out, op), i+len(op)
		}
	}
	return out, nil
}

type luaParser struct {
	toks []string
	i    int
}

func (p *luaParser) peek() string {
	if p.i < len(p.toks) {
		return p.toks[p.i]
	}
	return ""
}

func (p *luaParser) expect(t string) error {
	if p.peek() != t {
		return fmt.Errorf("want %q, have %q", t, p.peek())
	}
	p.i++
	return nil
}

// block parses statements up to end, else, elseif or the end of input.
func (p *luaParser) block() (func(*luaEnv), error) {
	var stmts []func(*luaEnv)
	for {
		switch t := p.peek(); t {
		case "", "end", "else", "elseif":
			return func(e *luaEnv) {
				for _, s := range stmts {
					s(e)
				}
			}, nil
		case ";":
			p.i++
		case "y":
			p.i++
			if err := p.expect("="); err != nil {
				return nil, err
			}
			v, err := p.number()
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, func(e *luaEnv) { e.y, e.ySet = v(e), true })
		case "if":
			s, err := p.ifStmt()
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, s)
		default:
			return nil, fmt.Errorf("unsupported statement at %q", t)
		}
	}
}

func (p *luaParser) ifStmt() (func(*luaEnv), error) {
	type branch struct {
		cond func(*luaEnv) bool
		body func(*luaEnv)
	}
	var branches []branch
	for kw := "if"; kw == "if" || kw == "elseif"; kw = p.peek() {
		p.i++
		c, err := p.cond()
		if err != nil {
			return nil, err
		}
		if err := p.expect("then"); err != nil {
			return nil, err
		}
		b, err := p.block()
		if err != nil {
			return nil, err
		}
		branches = append(branches, branch{c, b})
	}
	els := func(*luaEnv) {}
	if p.peek() == "else" {
		p.i++
		b, err := p.block()
		if err != nil {
			return nil, err
		}
		els = b
	}
	if err := p.expect("end"); err != nil {
		return nil, err
	}
	return func(e *luaEnv) {
		for _, b := range branches {
			if b.cond(e) {
				b.body(e)
				return
			}
		}
		els(e)
	}, nil
}

// luaExpr is a parsed expression: a number or a comparison.
type luaExpr struct {
	num  func(*luaEnv) float64
	bool func(*luaEnv) bool
}

func (p *luaParser) number() (func(*luaEnv) float64, error) {
	x, err := p.expr()
	if err != nil {
		return nil, err
	}
	return x.number()
}

func (x luaExpr) number() (func(*luaEnv) float64, error) {
	if x.num == nil {
		return nil, fmt.Errorf("a comparison where a number is wanted")
	}
	return x.num, nil
}

// cond is an if condition: a comparison, or a number (always true in Lua).
func (p *luaParser) cond() (func(*luaEnv) bool, error) {
	x, err := p.expr()
	if err != nil {
		return nil, err
	}
	if x.bool == nil {
		return func(*luaEnv) bool { return true }, nil
	}
	return x.bool, nil
}

var luaCompare = map[string]func(a, b float64) bool{
	"<": func(a, b float64) bool { return a < b }, ">": func(a, b float64) bool { return a > b },
	"<=": func(a, b float64) bool { return a <= b }, ">=": func(a, b float64) bool { return a >= b },
	"==": func(a, b float64) bool { return a == b }, "~=": func(a, b float64) bool { return a != b },
}

// expr is arith [comparison arith].
func (p *luaParser) expr() (luaExpr, error) {
	a, err := p.arith()
	if err != nil {
		return luaExpr{}, err
	}
	cmp, ok := luaCompare[p.peek()]
	if !ok {
		return a, nil
	}
	p.i++
	b, err := p.arith()
	if err != nil {
		return luaExpr{}, err
	}
	l, err := a.number()
	if err != nil {
		return luaExpr{}, err
	}
	r, err := b.number()
	if err != nil {
		return luaExpr{}, err
	}
	return luaExpr{bool: func(e *luaEnv) bool { return cmp(l(e), r(e)) }}, nil
}

func (p *luaParser) arith() (luaExpr, error) { return p.binary(p.term, "+", "-") }
func (p *luaParser) term() (luaExpr, error)  { return p.binary(p.unary, "*", "/") }

var luaArith = map[string]func(a, b float64) float64{
	"+": func(a, b float64) float64 { return a + b }, "-": func(a, b float64) float64 { return a - b },
	"*": func(a, b float64) float64 { return a * b }, "/": func(a, b float64) float64 { return a / b },
}

// binary parses operand (op operand)* for two operators, left-associative.
func (p *luaParser) binary(operand func() (luaExpr, error), op1, op2 string) (luaExpr, error) {
	a, err := operand()
	if err != nil {
		return luaExpr{}, err
	}
	for op := p.peek(); op == op1 || op == op2; op = p.peek() {
		p.i++
		b, err := operand()
		if err != nil {
			return luaExpr{}, err
		}
		l, err := a.number()
		if err != nil {
			return luaExpr{}, err
		}
		r, err := b.number()
		if err != nil {
			return luaExpr{}, err
		}
		f := luaArith[op]
		a = luaExpr{num: func(e *luaEnv) float64 { return f(l(e), r(e)) }}
	}
	return a, nil
}

// unary is -unary, x, r, a number or a parenthesised expression.
func (p *luaParser) unary() (luaExpr, error) {
	t := p.peek()
	p.i++
	switch t {
	case "-":
		x, err := p.unary()
		if err != nil {
			return luaExpr{}, err
		}
		v, err := x.number()
		if err != nil {
			return luaExpr{}, err
		}
		return luaExpr{num: func(e *luaEnv) float64 { return -v(e) }}, nil
	case "x":
		return luaExpr{num: func(e *luaEnv) float64 { return e.x }}, nil
	case "r":
		return luaExpr{num: func(e *luaEnv) float64 { return e.r }}, nil
	case "(":
		x, err := p.expr()
		if err != nil {
			return luaExpr{}, err
		}
		return x, p.expect(")")
	}
	n, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return luaExpr{}, fmt.Errorf("unsupported operand %q", t)
	}
	return luaExpr{num: func(*luaEnv) float64 { return n }}, nil
}
