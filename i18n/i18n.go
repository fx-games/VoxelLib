// Package i18n holds translations from vanilla .lang files (key=value,
// UTF-8) and formats them like the 1.7.10 reference's I18n: tables from several
// sources merge with the later one winning, the current language sits on
// top of the fallback (en_US), a missing key translates to itself, and
// %s, %d, %.2f and %1$s style arguments are filled in.
package i18n

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Fallback is the language under every other one.
const Fallback = "en_US"

// Catalog is a set of languages, one of them current.
type Catalog struct {
	langs   map[string]map[string]string
	current string
	// OnChange runs after SetLanguage (reload fonts, rebuild screens).
	OnChange func(code string)
}

// New makes an empty catalog with en_US current.
func New() *Catalog { return &Catalog{langs: map[string]map[string]string{}, current: Fallback} }

// numArg is the reference's rewrite of %d / %.2f / %1$d to %s (Locale.pattern).
var numArg = regexp.MustCompile(`%(\d+\$)?[\d.]*[df]`)

// Load merges a .lang file into language code; its entries override what
// is there (load sources lowest priority first).
func (c *Catalog) Load(code string, r io.Reader) error {
	t := c.langs[code]
	if t == nil {
		t = map[string]string{}
		c.langs[code] = t
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	first := true
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if first {
			line = strings.TrimPrefix(line, "\uFEFF")
			first = false
		}
		if line == "" || line[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		t[k] = numArg.ReplaceAllString(v, "%${1}s")
	}
	return sc.Err()
}

// LoadFS loads every assets/<domain>/lang/<code>.lang in each fsys, the
// sources lowest priority first (a resource pack stack in load order).
// Domains load in name order.
func (c *Catalog) LoadFS(sources ...fs.FS) error {
	for _, fsys := range sources {
		domains, _ := fs.ReadDir(fsys, "assets")
		names := make([]string, 0, len(domains))
		for _, d := range domains {
			names = append(names, d.Name())
		}
		sort.Strings(names)
		for _, d := range names {
			files, _ := fs.ReadDir(fsys, path.Join("assets", d, "lang"))
			for _, f := range files {
				code, ok := strings.CutSuffix(f.Name(), ".lang")
				if !ok || f.IsDir() {
					continue
				}
				r, err := fsys.Open(path.Join("assets", d, "lang", f.Name()))
				if err != nil {
					return err
				}
				err = c.Load(code, r)
				r.Close()
				if err != nil {
					return fmt.Errorf("%s/%s: %w", d, f.Name(), err)
				}
			}
		}
	}
	return nil
}

// Set adds or replaces one entry.
func (c *Catalog) Set(code, key, value string) {
	if c.langs[code] == nil {
		c.langs[code] = map[string]string{}
	}
	c.langs[code][key] = value
}

// Languages lists the loaded language codes, sorted.
func (c *Catalog) Languages() []string {
	var out []string
	for k := range c.langs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Name is the language's display name, "language.name (language.region)"
// from its own file, like the reference's language list.
func (c *Catalog) Name(code string) string {
	t := c.langs[code]
	n, r := t["language.name"], t["language.region"]
	switch {
	case n == "":
		return code
	case r == "":
		return n
	}
	return n + " (" + r + ")"
}

// Language returns the current language code.
func (c *Catalog) Language() string { return c.current }

// SetLanguage switches the current language.
func (c *Catalog) SetLanguage(code string) {
	c.current = code
	if c.OnChange != nil {
		c.OnChange(code)
	}
}

// Has reports whether key is translated in the current language or the
// fallback.
func (c *Catalog) Has(key string) bool {
	_, ok := c.lookup(key)
	return ok
}

func (c *Catalog) lookup(key string) (string, bool) {
	if v, ok := c.langs[c.current][key]; ok {
		return v, true
	}
	v, ok := c.langs[Fallback][key]
	return v, ok
}

// T translates key and fills in args (I18n.format). A missing key comes
// back as itself.
func (c *Catalog) T(key string, args ...any) string {
	v, ok := c.lookup(key)
	if !ok {
		return key
	}
	if len(args) == 0 {
		return v
	}
	return Format(v, args...)
}

// Format fills %s, %1$s and %% in pattern like Java's String.format (after
// the reference turned %d and %f into %s). A malformed pattern returns
// "Format error: " + pattern, as the reference does.
func Format(pattern string, args ...any) string {
	var b strings.Builder
	next := 0
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if ch != '%' {
			b.WriteByte(ch)
			continue
		}
		j := i + 1
		for j < len(pattern) && (pattern[j] >= '0' && pattern[j] <= '9' || pattern[j] == '$' || pattern[j] == '.') {
			j++
		}
		if j >= len(pattern) {
			return "Format error: " + pattern
		}
		spec, conv := pattern[i+1:j], pattern[j]
		i = j
		switch conv {
		case '%':
			b.WriteByte('%')
			continue
		case 'n':
			b.WriteByte('\n')
			continue
		case 's', 'S', 'd', 'f':
		default:
			return "Format error: " + pattern
		}
		idx := -1
		if n, _, ok := strings.Cut(spec, "$"); ok {
			k, err := strconv.Atoi(n)
			if err != nil || k < 1 {
				return "Format error: " + pattern
			}
			idx = k - 1
		} else {
			idx = next
			next++
		}
		if idx >= len(args) {
			return "Format error: " + pattern
		}
		s := javaString(args[idx])
		if conv == 'S' {
			s = strings.ToUpper(s)
		}
		b.WriteString(s)
	}
	return b.String()
}

// javaString is String.valueOf for the common argument types.
func javaString(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case float32:
		return javaFloat(float64(x), 32)
	case float64:
		return javaFloat(x, 64)
	}
	return fmt.Sprint(v)
}

func javaFloat(f float64, bits int) string {
	s := strconv.FormatFloat(f, 'f', -1, bits)
	if !strings.ContainsAny(s, ".eEIN") {
		s += ".0"
	}
	return s
}
