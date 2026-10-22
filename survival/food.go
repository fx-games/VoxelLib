package survival

import (
	"math"

	"github.com/memmaker/voxellib/inventory"
)

// Exhaustion per action (EntityPlayer, Block.harvestBlock, FoodStats).
const (
	ExhaustJump       = 0.2
	ExhaustSprintJump = 0.8
	ExhaustAttack     = 0.3
	ExhaustMine       = 0.025
	ExhaustRegen      = 3.0
)

// Food is FoodStats.
type Food struct {
	Level      int
	Saturation float32
	Exhaustion float32
	Timer      int
	Prev       int
}

// NewFood returns a full bar: level 20, saturation 5.
func NewFood() *Food { return &Food{Level: 20, Saturation: 5, Prev: 20} }

// Add is addStats.
func (f *Food) Add(hunger int, saturation float32) {
	f.Level = min(hunger+f.Level, 20)
	f.Saturation = min(f.Saturation+float32(hunger)*saturation*2, float32(f.Level))
}

// AddExhaustion adds exhaustion, capped at 40.
func (f *Food) AddExhaustion(v float32) { f.Exhaustion = min(f.Exhaustion+v, 40) }

// Hungry is needFood.
func (f *Food) Hungry() bool { return f.Level < 20 }

// Tick is FoodStats.onUpdate for body b.
func (f *Food) Tick(b *Body, d Difficulty, naturalRegen bool) {
	f.Prev = f.Level
	if f.Exhaustion > 4 {
		f.Exhaustion -= 4
		if f.Saturation > 0 {
			f.Saturation = max(f.Saturation-1, 0)
		} else if d != Peaceful {
			f.Level = max(f.Level-1, 0)
		}
	}
	switch {
	case naturalRegen && f.Level >= 18 && b.ShouldHeal():
		f.Timer++
		if f.Timer >= 80 {
			b.Heal(1)
			f.AddExhaustion(ExhaustRegen)
			f.Timer = 0
		}
	case f.Level <= 0:
		f.Timer++
		if f.Timer >= 80 {
			if b.Health > 10 || d == Hard || b.Health > 1 && d == Normal {
				b.Hurt(Starve(), 1)
			}
			f.Timer = 0
		}
	default:
		f.Timer = 0
	}
}

// Movement kinds for Moved (EntityPlayer.addMovementStat).
const (
	MoveWalk       = iota // on the ground
	MoveSprint            // on the ground, sprinting
	MoveSwim              // in water, head above
	MoveUnderwater        // head in water
	MoveOther             // ladder, air, riding: no exhaustion
)

// Moved adds the exhaustion for one tick's movement (dx,dy,dz in blocks).
func (f *Food) Moved(dx, dy, dz float64, kind int) {
	cm := func(d2 float64) float32 {
		return float32(math.Round(float64(float32(math.Sqrt(d2)) * 100)))
	}
	switch kind {
	case MoveUnderwater:
		if i := cm(dx*dx + dy*dy + dz*dz); i > 0 {
			f.AddExhaustion(0.015 * i * 0.01)
		}
	case MoveSwim:
		if i := cm(dx*dx + dz*dz); i > 0 {
			f.AddExhaustion(0.015 * i * 0.01)
		}
	case MoveWalk, MoveSprint:
		if i := cm(dx*dx + dz*dz); i > 0 {
			if kind == MoveSprint {
				f.AddExhaustion(0.099999994 * i * 0.01)
			} else {
				f.AddExhaustion(0.01 * i * 0.01)
			}
		}
	}
}

// EatTicks returns how long s takes to eat (32 by default).
func EatTicks(s *inventory.Stack) int {
	if s.Item.Food != nil && s.Item.Food.UseTicks > 0 {
		return s.Item.Food.UseTicks
	}
	return 32
}

// CanEat is EntityPlayer.canEat for the food item s.
func (b *Body) CanEat(s *inventory.Stack) bool {
	fd := s.Item.Food
	return fd != nil && b.Food != nil && (fd.AlwaysEdible || b.Food.Hungry()) && !b.Creative
}

// Eat finishes eating s (ItemFood.onEaten): one item is used up, the food
// values are added and the item's OnEat hook runs with user as Ctx.User.
func (b *Body) Eat(s *inventory.Stack, user any) {
	fd := s.Item.Food
	if fd == nil || b.Food == nil {
		return
	}
	s.Count--
	b.Food.Add(fd.Hunger, fd.Saturation)
	if s.Item.OnEat != nil {
		s.Item.OnEat(&inventory.Ctx{Stack: s, User: user})
	}
}
