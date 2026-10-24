package survival

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/memmaker/voxellib/actors"
	"github.com/memmaker/voxellib/inventory"
	"github.com/memmaker/voxellib/particles"
)

// tinyGame is all a game wires once: the player's use, body, inventory
// and its sound and particle output.
type tinyGame struct {
	eat    Eating
	use    actors.ItemUse
	sounds []string
	crumbs int
}

func newTinyGame() *tinyGame {
	g := &tinyGame{}
	g.eat = Eating{Use: &g.use, Actor: &actors.Actor{}, Body: NewPlayer(nil), Inv: inventory.NewPlayer(inventory.PlayerSpec{}),
		Rand:   rand.New(rand.NewPCG(1, 2)),
		Sound:  func(ev string, vol, pitch float64) { g.sounds = append(g.sounds, ev) },
		Crumb:  func(*inventory.Stack, float64, float64, float64, float64, float64, float64) { g.crumbs++ },
		Mouth:  func() (float64, float64, float64, float32, float32) { return 0, 1.62, 0, 0, 0 },
		Effect: func(p string, ticks, amp int) { g.sounds = append(g.sounds, "effect "+p) }}
	return g
}

// eatHeld right-clicks the held stack and ticks the use out.
func (g *tinyGame) eatHeld(t *testing.T) {
	t.Helper()
	if !g.eat.Start(g.eat.Inv.Current()) {
		t.Fatal("not eaten")
	}
	for range 40 {
		g.eat.Tick()
	}
}

// TestEatingDeclared: an item declared as food is eaten with nothing else
// set up: not when full, then 32 ticks with crumbs and the eat sound every
// 4 of the last 25 (6 beats of 5, then 16 at the end), the burp, the food
// added, the effects by their chance, damage and order and the bowl left;
// with no Crumb hook the crumbs are the lib's icon particles; a drink has
// its sound and no crumbs; switching away cancels.
func TestEatingDeclared(t *testing.T) {
	bowl := &inventory.Item{Key: "bowl"}
	stew := &inventory.Item{Key: "stew", Food: &inventory.Food{Hunger: 6, Saturation: 0.6, Leaves: bowl,
		Effects: []inventory.FoodEffect{{Potion: "poison", Ticks: 60, Chance: 1, Meta: -1}, {Potion: "always", Meta: -1},
			{Potion: "never", Chance: 1e-9, Meta: -1}, {Potion: "meta1", Meta: 1}}}}
	g := newTinyGame()
	g.eat.Inv.Main[0] = inventory.New(stew, 1)
	if g.eat.Start(g.eat.Inv.Current()) {
		t.Fatal("eaten when full")
	}
	g.eat.Body.Food.Level = 10
	g.eatHeld(t)
	n := 0
	for _, s := range g.sounds {
		if s == "random.eat" {
			n++
		}
	}
	if n != 7 || g.crumbs != 6*5+16 || !slices.Contains(g.sounds, "random.burp") || !slices.Contains(g.sounds, "effect poison") ||
		!slices.Contains(g.sounds, "effect always") || slices.Contains(g.sounds, "effect never") || slices.Contains(g.sounds, "effect meta1") ||
		g.eat.Body.Food.Level != 16 || g.eat.Inv.Main[0].Item != bowl || g.use.Using != nil {
		t.Fatalf("eat: %d sounds, %d crumbs, %v, level %d, held %v", n, g.crumbs, g.sounds, g.eat.Body.Food.Level, g.eat.Inv.Main[0])
	}

	// No Crumb hook: the crumbs are the lib's particles, a quarter of the
	// item's icon each, with EntityBreakingFX's gravity and half size.
	g = newTinyGame()
	g.eat.Crumb = nil
	var asked []string
	g.eat.Particles = &particles.System{Icon: func(name string) ([4]float32, [3]float32, bool) {
		asked = append(asked, name)
		return [4]float32{0, 0, 0.5, 0.5}, [3]float32{1, 1, 1}, true
	}}
	g.eat.Body.Food.Level = 10
	g.eat.Inv.Main[0] = inventory.New(stew, 1)
	g.eatHeld(t)
	ps := g.eat.Particles
	if len(ps.FX) != 6*5+16 || asked[0] != "item:stew#0" {
		t.Fatalf("default crumbs: %d live, %d asked (%v)", len(ps.FX), len(asked), asked[:1])
	}
	if p := ps.FX[0]; p.Kind != particles.KindIcon || p.Layer != 2 || p.Scale < 0.5 || p.Scale > 1 || p.MaxAge < 4 || p.MaxAge > 40 {
		t.Fatalf("crumb %+v", p)
	}
	my := ps.FX[0].MY
	ps.TickFX()
	if sp := ps.Particle(&ps.FX[0], 0, func(x, y, z float64) [3]float32 { return [3]float32{1, 1, 1} }, nil); sp.Sheet != particles.SheetIcons ||
		sp.UV[2]-sp.UV[0] != 0.125 || ps.FX[0].MY != (my-0.04)*0.9800000190734863 {
		t.Fatalf("crumb sprite %+v, motion %v from %v", sp, ps.FX[0].MY, my)
	}

	milk := &inventory.Item{Key: "milk", Food: &inventory.Food{Drink: true, AlwaysEdible: true}}
	g = newTinyGame()
	g.eat.Inv.Main[0] = inventory.New(milk, 2)
	g.eatHeld(t)
	if g.crumbs != 0 || !slices.Contains(g.sounds, "random.drink") || g.eat.Inv.Main[0].Count != 1 {
		t.Fatalf("drink: %d crumbs, %v, held %v", g.crumbs, g.sounds, g.eat.Inv.Main[0])
	}
	g.eat.Start(g.eat.Inv.Current())
	g.eat.Inv.Held = 1
	g.eat.Tick()
	if g.use.Using != nil || g.eat.Inv.Main[0].Count != 1 {
		t.Fatal("switching away does not cancel")
	}
}

// TestEatingHook: a game's OnEaten composes the lib's pieces with its own
// mechanic: here no burp, a second helping and the stack kept.
func TestEatingHook(t *testing.T) {
	bread := &inventory.Item{Key: "bread", Food: &inventory.Food{Hunger: 5, Saturation: 0.6}}
	g := newTinyGame()
	g.eat.Body.Food.Level = 0
	g.eat.Inv.Main[0] = inventory.New(bread, 1)
	g.eat.OnEaten = func(e *Eating, st *inventory.Stack) {
		e.Feedback(st, 16)
		e.Body.Food.Add(st.Item.Food.Hunger*2, st.Item.Food.Saturation)
	}
	g.eatHeld(t)
	if g.eat.Body.Food.Level != 10 || slices.Contains(g.sounds, "random.burp") || g.eat.Inv.Main[0].Count != 1 || g.use.Using != nil {
		t.Fatalf("hook: level %d, %v, held %v", g.eat.Body.Food.Level, g.sounds, g.eat.Inv.Main[0])
	}
}
