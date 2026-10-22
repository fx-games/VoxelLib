package crafting

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/inventory"
)

// File is the JSON form of one recipe book plus smelting recipes:
//
//	{
//	  "noRepair": false,
//	  "ores": {"plankWood": ["minecraft:planks#*"]},
//	  "recipes": [
//	    {"type": "shaped", "pattern": ["##", "##"], "key": {"#": "ore:plankWood"},
//	     "result": {"item": "minecraft:crafting_table"}},
//	    {"type": "shaped", "pattern": ["A ", " B"], "key": {...}, "mirrored": false, "keepTag": true, ...},
//	    {"type": "shapeless", "ingredients": ["minecraft:log#*", ["minecraft:dye#1", "ore:dyeRed"]],
//	     "result": {"item": "minecraft:planks#0", "count": 4}}
//	  ],
//	  "smelting": [{"input": "minecraft:iron_ore#*", "result": {"item": "minecraft:iron_ingot"}, "xp": 0.7}]
//	}
//
// An item reference is "<registry name>[#meta]": no meta means 0, "#*" any
// meta (32767). "ore:<name>" refers to an entry of "ores". An ingredient is a
// reference or a list of alternatives. Recipes keep their order (the matcher
// takes the first match; Book.SortVanilla reorders like CraftingManager).
// Shaped recipes are mirrored unless "mirrored" is false.
type File struct {
	NoRepair bool                `json:"noRepair,omitempty"`
	Ores     map[string][]string `json:"ores,omitempty"`
	Recipes  []RecipeJSON        `json:"recipes"`
	Smelting []SmeltJSON         `json:"smelting,omitempty"`
}

// RecipeJSON is one crafting recipe.
type RecipeJSON struct {
	Type        string                     `json:"type"` // "shaped" or "shapeless"
	Pattern     []string                   `json:"pattern,omitempty"`
	Key         map[string]json.RawMessage `json:"key,omitempty"`
	Ingredients []json.RawMessage          `json:"ingredients,omitempty"`
	Result      ResultJSON                 `json:"result"`
	Mirrored    *bool                      `json:"mirrored,omitempty"`
	KeepTag     bool                       `json:"keepTag,omitempty"`
}

// ResultJSON is a recipe output.
type ResultJSON struct {
	Item  string         `json:"item"`
	Count int            `json:"count,omitempty"` // 0 means 1
	Tag   map[string]any `json:"tag,omitempty"`
}

// SmeltJSON is one furnace recipe.
type SmeltJSON struct {
	Input  string     `json:"input"`
	Result ResultJSON `json:"result"`
	XP     float32    `json:"xp"`
}

// Load reads a File and resolves it against reg. Recipes naming unknown items
// are skipped and reported in warnings; a malformed file is an error.
func Load(r io.Reader, reg *inventory.Registry) (*Book, *Smelting, []string, error) {
	var f File
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return nil, nil, nil, fmt.Errorf("crafting: %w", err)
	}
	return f.Resolve(reg)
}

// Resolve turns a decoded File into a book and smelting list.
func (f *File) Resolve(reg *inventory.Registry) (*Book, *Smelting, []string, error) {
	l := loader{reg: reg, ores: f.Ores}
	b := &Book{NoRepair: f.NoRepair}
	for i, rj := range f.Recipes {
		rec, err := l.recipe(rj)
		if err != nil {
			l.warn = append(l.warn, fmt.Sprintf("recipe %d (%s): %v", i, rj.Result.Item, err))
			continue
		}
		b.Recipes = append(b.Recipes, rec)
	}
	sm := &Smelting{}
	for i, sj := range f.Smelting {
		in, err := l.option(sj.Input)
		var out *inventory.Stack
		if err == nil {
			out, err = l.result(sj.Result)
		}
		if err != nil {
			l.warn = append(l.warn, fmt.Sprintf("smelting %d: %v", i, err))
			continue
		}
		sm.List = append(sm.List, Smelt{In: in, Out: out, XP: sj.XP})
	}
	return b, sm, l.warn, nil
}

type loader struct {
	reg  *inventory.Registry
	ores map[string][]string
	warn []string
}

// option resolves "name[#meta]".
func (l *loader) option(ref string) (Option, error) {
	name, meta, found := strings.Cut(ref, "#")
	m := 0
	if found {
		if meta == "*" {
			m = inventory.Wildcard
		} else {
			v, err := strconv.Atoi(meta)
			if err != nil {
				return Option{}, fmt.Errorf("bad meta in %q", ref)
			}
			m = v
		}
	}
	it := l.reg.Get(name)
	if it == nil {
		return Option{}, fmt.Errorf("unknown item %q", name)
	}
	return Option{it, m}, nil
}

func (l *loader) ref(ref string) (Ingredient, error) {
	if ore, ok := strings.CutPrefix(ref, "ore:"); ok {
		var in Ingredient
		for _, r := range l.ores[ore] {
			if o, err := l.option(r); err == nil {
				in = append(in, o)
			}
		}
		if len(in) == 0 {
			return nil, fmt.Errorf("ore %q has no known items", ore)
		}
		return in, nil
	}
	o, err := l.option(ref)
	if err != nil {
		return nil, err
	}
	return Ingredient{o}, nil
}

// ingredient resolves a reference or a list of them; unknown alternatives in
// a list are dropped as long as one remains.
func (l *loader) ingredient(raw json.RawMessage) (Ingredient, error) {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return l.ref(one)
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("ingredient %s: want a string or a list of strings", raw)
	}
	var in Ingredient
	for _, r := range list {
		if o, err := l.ref(r); err == nil {
			in = append(in, o...)
		}
	}
	if len(in) == 0 {
		return nil, fmt.Errorf("no known item in %s", raw)
	}
	return in, nil
}

func (l *loader) result(rj ResultJSON) (*inventory.Stack, error) {
	o, err := l.option(rj.Item)
	if err != nil {
		return nil, err
	}
	n := rj.Count
	if n <= 0 {
		n = 1
	}
	return (&inventory.Stack{Item: o.Item, Count: n, Damage: o.Meta, Tag: rj.Tag}).Copy(), nil
}

func (l *loader) recipe(rj RecipeJSON) (Recipe, error) {
	out, err := l.result(rj.Result)
	if err != nil {
		return nil, err
	}
	switch rj.Type {
	case "shaped":
		h := len(rj.Pattern)
		if h == 0 {
			return nil, fmt.Errorf("empty pattern")
		}
		w := len([]rune(rj.Pattern[0]))
		keys := map[rune]Ingredient{}
		for k, raw := range rj.Key {
			if len([]rune(k)) != 1 {
				return nil, fmt.Errorf("key %q is not one character", k)
			}
			in, err := l.ingredient(raw)
			if err != nil {
				return nil, err
			}
			keys[[]rune(k)[0]] = in
		}
		r := &Shaped{W: w, H: h, Out: out, Mirrored: rj.Mirrored == nil || *rj.Mirrored, KeepTag: rj.KeepTag}
		for _, row := range rj.Pattern {
			if len([]rune(row)) != w {
				return nil, fmt.Errorf("pattern rows differ in width")
			}
			for _, c := range row {
				if c == ' ' {
					r.Cells = append(r.Cells, nil)
					continue
				}
				in, ok := keys[c]
				if !ok {
					return nil, fmt.Errorf("pattern symbol %q has no key", c)
				}
				r.Cells = append(r.Cells, in)
			}
		}
		return r, nil
	case "shapeless":
		r := &Shapeless{Out: out}
		for _, raw := range rj.Ingredients {
			in, err := l.ingredient(raw)
			if err != nil {
				return nil, err
			}
			r.In = append(r.In, in)
		}
		if len(r.In) == 0 {
			return nil, fmt.Errorf("no ingredients")
		}
		return r, nil
	}
	return nil, fmt.Errorf("unknown recipe type %q", rj.Type)
}
