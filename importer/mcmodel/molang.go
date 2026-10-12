package mcmodel

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"unicode"
)

// Expr is a compiled Molang expression, as found in Bedrock/GeckoLib
// keyframe values ("math.sin(query.anim_time * 180) * 40 + 17").
type Expr interface {
	Eval(c *Ctx) float64
}

// Ctx carries the variables expressions can read.
type Ctx struct {
	AnimTime float64 // query.anim_time, seconds into the clip
	LifeTime float64 // query.life_time
	Vars     map[string]float64
}

type num float64

func (n num) Eval(*Ctx) float64 { return float64(n) }

type fn func(c *Ctx) float64

func (f fn) Eval(c *Ctx) float64 { return f(c) }

// Const reports whether e is a constant and its value.
func Const(e Expr) (float64, bool) {
	n, ok := e.(num)
	return float64(n), ok
}

// ParseMolang compiles an expression. It supports numbers, + - * /, unary
// minus, comparisons, && || !, ?:, parentheses, math.* functions (angles in
// degrees, as Molang), query/variable lookups and "return x;" statements.
// Unknown identifiers evaluate to 0, as in Minecraft.
func ParseMolang(src string) (Expr, error) {
	s := strings.TrimSpace(src)
	s = strings.TrimSuffix(s, ";")
	s = strings.TrimPrefix(s, "return ")
	if s == "" {
		return num(0), nil
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return num(v), nil
	}
	p := &parser{src: s}
	p.next()
	e := p.ternary()
	if p.err == nil && p.tok != "" {
		p.err = fmt.Errorf("unexpected %q", p.tok)
	}
	if p.err != nil {
		return nil, fmt.Errorf("molang %q: %w", src, p.err)
	}
	return fold(e, p.dynamic), nil
}

// fold replaces an expression that reads no variables by its value.
func fold(e Expr, dynamic bool) Expr {
	if dynamic {
		return e
	}
	return num(e.Eval(&Ctx{}))
}

type parser struct {
	src     string
	pos     int
	tok     string
	err     error
	dynamic bool // reads time, variables or random numbers
}

func (p *parser) next() {
	for p.pos < len(p.src) && unicode.IsSpace(rune(p.src[p.pos])) {
		p.pos++
	}
	if p.pos >= len(p.src) {
		p.tok = ""
		return
	}
	c := p.src[p.pos]
	start := p.pos
	switch {
	case c >= '0' && c <= '9' || c == '.':
		for p.pos < len(p.src) && (p.src[p.pos] >= '0' && p.src[p.pos] <= '9' || p.src[p.pos] == '.') {
			p.pos++
		}
		if p.pos < len(p.src) && (p.src[p.pos] == 'f' || p.src[p.pos] == 'F') {
			p.pos++
		}
	case unicode.IsLetter(rune(c)) || c == '_':
		for p.pos < len(p.src) && (unicode.IsLetter(rune(p.src[p.pos])) || unicode.IsDigit(rune(p.src[p.pos])) || p.src[p.pos] == '_' || p.src[p.pos] == '.') {
			p.pos++
		}
	default:
		p.pos++
		if p.pos < len(p.src) {
			two := p.src[start : p.pos+1]
			switch two {
			case "==", "!=", "<=", ">=", "&&", "||", "??":
				p.pos++
			}
		}
	}
	p.tok = p.src[start:p.pos]
}

func (p *parser) expect(t string) {
	if p.tok != t {
		if p.err == nil {
			p.err = fmt.Errorf("expected %q, got %q", t, p.tok)
		}
		return
	}
	p.next()
}

func (p *parser) ternary() Expr {
	c := p.binary(0)
	if p.tok == "?" {
		p.next()
		a := p.ternary()
		var b Expr = num(0)
		if p.tok == ":" {
			p.next()
			b = p.ternary()
		}
		return fn(func(x *Ctx) float64 {
			if c.Eval(x) != 0 {
				return a.Eval(x)
			}
			return b.Eval(x)
		})
	}
	return c
}

var precedence = map[string]int{"??": 1, "||": 2, "&&": 3, "==": 4, "!=": 4, "<": 5, ">": 5, "<=": 5, ">=": 5, "+": 6, "-": 6, "*": 7, "/": 7}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (p *parser) binary(minPrec int) Expr {
	l := p.unary()
	for {
		op := p.tok
		prec, ok := precedence[op]
		if !ok || prec <= minPrec {
			return l
		}
		p.next()
		r := p.binary(prec)
		a, b := l, r
		switch op {
		case "+":
			l = fn(func(c *Ctx) float64 { return a.Eval(c) + b.Eval(c) })
		case "-":
			l = fn(func(c *Ctx) float64 { return a.Eval(c) - b.Eval(c) })
		case "*":
			l = fn(func(c *Ctx) float64 { return a.Eval(c) * b.Eval(c) })
		case "/":
			l = fn(func(c *Ctx) float64 {
				d := b.Eval(c)
				if d == 0 {
					return 0
				}
				return a.Eval(c) / d
			})
		case "<":
			l = fn(func(c *Ctx) float64 { return b2f(a.Eval(c) < b.Eval(c)) })
		case ">":
			l = fn(func(c *Ctx) float64 { return b2f(a.Eval(c) > b.Eval(c)) })
		case "<=":
			l = fn(func(c *Ctx) float64 { return b2f(a.Eval(c) <= b.Eval(c)) })
		case ">=":
			l = fn(func(c *Ctx) float64 { return b2f(a.Eval(c) >= b.Eval(c)) })
		case "==":
			l = fn(func(c *Ctx) float64 { return b2f(a.Eval(c) == b.Eval(c)) })
		case "!=":
			l = fn(func(c *Ctx) float64 { return b2f(a.Eval(c) != b.Eval(c)) })
		case "&&":
			l = fn(func(c *Ctx) float64 { return b2f(a.Eval(c) != 0 && b.Eval(c) != 0) })
		case "||", "??":
			l = fn(func(c *Ctx) float64 { return b2f(a.Eval(c) != 0 || b.Eval(c) != 0) })
		}
	}
}

func (p *parser) unary() Expr {
	switch p.tok {
	case "-":
		p.next()
		e := p.unary()
		return fn(func(c *Ctx) float64 { return -e.Eval(c) })
	case "+":
		p.next()
		return p.unary()
	case "!":
		p.next()
		e := p.unary()
		return fn(func(c *Ctx) float64 { return b2f(e.Eval(c) == 0) })
	}
	return p.primary()
}

const deg = math.Pi / 180

var funcs = map[string]func(a []float64) float64{
	"math.sin":   func(a []float64) float64 { return math.Sin(a[0] * deg) },
	"math.cos":   func(a []float64) float64 { return math.Cos(a[0] * deg) },
	"math.asin":  func(a []float64) float64 { return math.Asin(a[0]) / deg },
	"math.acos":  func(a []float64) float64 { return math.Acos(a[0]) / deg },
	"math.atan":  func(a []float64) float64 { return math.Atan(a[0]) / deg },
	"math.atan2": func(a []float64) float64 { return math.Atan2(a[0], a[1]) / deg },
	"math.abs":   func(a []float64) float64 { return math.Abs(a[0]) },
	"math.sqrt":  func(a []float64) float64 { return math.Sqrt(a[0]) },
	"math.exp":   func(a []float64) float64 { return math.Exp(a[0]) },
	"math.ln":    func(a []float64) float64 { return math.Log(a[0]) },
	"math.pow":   func(a []float64) float64 { return math.Pow(a[0], a[1]) },
	"math.floor": func(a []float64) float64 { return math.Floor(a[0]) },
	"math.ceil":  func(a []float64) float64 { return math.Ceil(a[0]) },
	"math.round": func(a []float64) float64 { return math.Round(a[0]) },
	"math.trunc": func(a []float64) float64 { return math.Trunc(a[0]) },
	"math.mod":   func(a []float64) float64 { return math.Mod(a[0], a[1]) },
	"math.min":   func(a []float64) float64 { return math.Min(a[0], a[1]) },
	"math.max":   func(a []float64) float64 { return math.Max(a[0], a[1]) },
	"math.clamp": func(a []float64) float64 { return math.Max(a[1], math.Min(a[2], a[0])) },
	"math.lerp":  func(a []float64) float64 { return a[0] + (a[1]-a[0])*a[2] },
	"math.random": func(a []float64) float64 {
		return a[0] + rand.Float64()*(a[1]-a[0])
	},
	"math.random_integer": func(a []float64) float64 {
		return math.Floor(a[0] + rand.Float64()*(a[1]-a[0]+1))
	},
}

var arity = map[string]int{"math.atan2": 2, "math.pow": 2, "math.mod": 2, "math.min": 2, "math.max": 2,
	"math.clamp": 3, "math.lerp": 3, "math.random": 2, "math.random_integer": 2}

func (p *parser) primary() Expr {
	t := p.tok
	switch {
	case t == "(":
		p.next()
		e := p.ternary()
		p.expect(")")
		return e
	case t == "":
		if p.err == nil {
			p.err = fmt.Errorf("unexpected end")
		}
		return num(0)
	case t[0] >= '0' && t[0] <= '9' || t[0] == '.':
		v, err := strconv.ParseFloat(strings.TrimRight(t, "fF"), 64)
		if err != nil && p.err == nil {
			p.err = err
		}
		p.next()
		return num(v)
	}
	p.next()
	name := strings.ToLower(t)
	if p.tok == "(" {
		p.next()
		var args []Expr
		for p.tok != ")" && p.tok != "" && p.err == nil {
			args = append(args, p.ternary())
			if p.tok == "," {
				p.next()
			}
		}
		p.expect(")")
		if strings.HasPrefix(name, "math.random") {
			p.dynamic = true
		}
		f, ok := funcs[name]
		if !ok {
			return num(0)
		}
		n := max(1, arity[name])
		for len(args) < n {
			args = append(args, num(0))
		}
		return fn(func(c *Ctx) float64 {
			v := make([]float64, len(args))
			for i, a := range args {
				v[i] = a.Eval(c)
			}
			return f(v)
		})
	}
	switch name {
	case "math.pi":
		return num(math.Pi)
	case "query.anim_time", "q.anim_time":
		p.dynamic = true
		return fn(func(c *Ctx) float64 { return c.AnimTime })
	case "query.life_time", "q.life_time":
		p.dynamic = true
		return fn(func(c *Ctx) float64 { return c.LifeTime })
	case "true":
		return num(1)
	case "false", "nan", "this":
		return num(0)
	}
	if strings.HasPrefix(name, "variable.") || strings.HasPrefix(name, "v.") {
		p.dynamic = true
		key := name[strings.Index(name, ".")+1:]
		return fn(func(c *Ctx) float64 { return c.Vars[key] })
	}
	return num(0) // other queries: not simulated
}
