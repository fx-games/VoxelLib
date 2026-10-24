package survival

import (
	"math"

	"github.com/memmaker/voxellib/actors"
	"github.com/memmaker/voxellib/inventory"
)

// Rand is the random source eating draws from (math/rand/v2's *Rand).
type Rand interface {
	Float32() float32
	Float64() float64
	IntN(n int) int
}

// Eating is a player's eating and drinking (ItemFood with EntityPlayer's
// item use). Declaring an item's inventory.Food is all an item needs:
// wire Start to the use button and Tick to the player's tick once, and
// every food item is eaten with its crumbs, sounds and outcome.
// voxel/render.FirstPersonEat is the first-person motion; the reference
// has no third-person eat pose.
type Eating struct {
	Use   *actors.ItemUse
	Actor *actors.Actor // the user: Keep and Clear
	Body  *Body
	Inv   *inventory.Player
	Rand  Rand
	User  any // Ctx.User of the item's OnEat
	// Sound plays an event at the player.
	Sound func(event string, vol, pitch float64)
	// Crumb spawns one crumb of st (the reference's "iconcrack" item
	// particle, EntityBreakingFX) at a position with a velocity.
	Crumb func(st *inventory.Stack, x, y, z, vx, vy, vz float64)
	// Mouth is where the crumbs come from: the eye and its look (yaw and
	// pitch in the reference's degrees).
	Mouth func() (x, y, z float64, yaw, pitch float32)
	// Effect adds a potion effect for ticks at amp (nil: food effects drop).
	Effect func(potion string, ticks, amp int)
	// Action is the use action of st, "eat" or "drink" (others: no
	// crumbs or sound); nil: FoodAction.
	Action func(st *inventory.Stack) string
	// OnEaten finishes the use when its count runs out
	// (onItemUseFinish); nil: Eaten, the standard outcome. An override
	// composes Eaten's pieces with its own mechanics.
	OnEaten func(e *Eating, st *inventory.Stack)
}

// FoodAction is a food item's EnumAction: "drink", "eat", or "" for an
// item that is not food.
func FoodAction(st *inventory.Stack) string {
	switch {
	case st == nil || st.Item.Food == nil:
		return ""
	case st.Item.Food.Drink:
		return "drink"
	}
	return "eat"
}

// Start is ItemFood.onItemRightClick: a food item starts being eaten when
// the player can eat it. It reports whether it did.
func (e *Eating) Start(st *inventory.Stack) bool {
	if st == nil || !e.Body.CanEat(st) {
		return false
	}
	e.Use.Begin(st, EatTicks(st))
	return true
}

// Tick is the item use's tick (EntityPlayer.onUpdate): switching away or
// dying stops it; every 4th tick of the last 25 has its Feedback, and
// when the count runs out OnEaten finishes it and the use clears.
func (e *Eating) Tick() {
	st := e.Use.Using
	if !e.Use.Keep(e.Actor, e.Inv.Current(), e.Body.Dead) {
		return
	}
	if e.Use.Crumbs() {
		e.Feedback(st, 5)
	}
	if !e.Use.Tick() {
		return
	}
	if e.OnEaten != nil {
		e.OnEaten(e, st)
	} else {
		e.Eaten(st)
	}
	e.Use.Clear(e.Actor)
}

// Eaten is the standard finish: Feedback(16), then ItemFood.onEaten
// (Body.Eat, Burp, FoodEffect) and ConsumeHeld.
func (e *Eating) Eaten(st *inventory.Stack) {
	e.Feedback(st, 16)
	e.Body.Eat(st, e.User)
	e.Burp()
	e.FoodEffect(st)
	e.ConsumeHeld(st)
}

// Feedback is updateItemUse(st, n): the drink sound, or n crumbs from the
// mouth and the eat sound.
func (e *Eating) Feedback(st *inventory.Stack, n int) {
	act := FoodAction(st)
	if e.Action != nil {
		act = e.Action(st)
	}
	switch act {
	case "drink":
		e.Sound(DrinkSound(e.Rand))
	case "eat":
		x, y, z, yaw, pitch := e.Mouth()
		Crumbs(e.Rand, n, x, y, z, yaw, pitch, func(px, py, pz, vx, vy, vz float64) { e.Crumb(st, px, py, pz, vx, vy, vz) })
		e.Sound(EatSound(e.Rand))
	}
}

// Burp is onEaten's burp at the player.
func (e *Eating) Burp() { e.Sound(BurpSound(e.Rand)) }

// FoodEffect is ItemFood.onFoodEaten: the food's potion effect, by its
// chance.
func (e *Eating) FoodEffect(st *inventory.Stack) {
	if st.Item.Food == nil {
		return
	}
	fe := st.Item.Food.Effect
	if fe != nil && e.Rand.Float32() < fe.Chance && e.Effect != nil {
		e.Effect(fe.Potion, fe.Seconds*20, fe.Amp)
	}
}

// ConsumeHeld is onItemUseFinish's hand: the food's Leaves replaces the
// held stack, else a stack used up leaves the slot empty (creative
// players never eat, so nothing is kept for them).
func (e *Eating) ConsumeHeld(st *inventory.Stack) {
	if f := st.Item.Food; f != nil && f.Leaves != nil {
		e.Inv.Set(e.Inv.Held, inventory.New(f.Leaves, 1))
		return
	}
	e.Inv.UseHeld(st, 0, true, true)
}

// EatSound is the eat sound's event, volume and pitch.
func EatSound(r Rand) (string, float64, float64) {
	return "random.eat", float64(0.5 + 0.5*float32(r.IntN(2))), float64((r.Float32()-r.Float32())*0.2 + 1)
}

// DrinkSound is the drink sound's.
func DrinkSound(r Rand) (string, float64, float64) {
	return "random.drink", 0.5, float64(r.Float32()*0.1 + 0.9)
}

// BurpSound is the burp's.
func BurpSound(r Rand) (string, float64, float64) {
	return "random.burp", 0.5, float64(r.Float32()*0.1 + 0.9)
}

// Crumbs emits updateItemUse's n crumbs (also an NPC's eat tasks') from a
// mouth at x, y, z facing yaw and pitch (degrees): each 0.6 ahead, 0.3 to
// 0.9 below and up to 0.15 aside, moving up 0.1 to 0.2 (plus 0.05) and
// up to 0.05 aside, both turned with the look.
func Crumbs(r Rand, n int, x, y, z float64, yaw, pitch float32, emit func(x, y, z, vx, vy, vz float64)) {
	rot := func(x, y, z float64) (float64, float64, float64) {
		a := -pitch * math.Pi / 180 // rotateAroundX
		c, sn := float64(float32(math.Cos(float64(a)))), float64(float32(math.Sin(float64(a))))
		y, z = y*c+z*sn, z*c-y*sn
		b := -yaw * math.Pi / 180 // rotateAroundY
		c, sn = float64(float32(math.Cos(float64(b)))), float64(float32(math.Sin(float64(b))))
		return x*c + z*sn, y, z*c - x*sn
	}
	for range n {
		vx, vy, vz := rot((float64(r.Float32())-0.5)*0.1, r.Float64()*0.1+0.1, 0)
		px, py, pz := rot((float64(r.Float32())-0.5)*0.3, float64(-r.Float32())*0.6-0.3, 0.6)
		emit(px+x, py+y, pz+z, vx, vy+0.05, vz)
	}
}
