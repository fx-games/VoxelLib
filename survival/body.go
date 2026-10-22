package survival

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/memmaker/voxellib/inventory"
)

// Body is the health side of a living entity (EntityLivingBase, plus the
// EntityPlayer parts when Player is set).
type Body struct {
	Health, MaxHealth float32
	Absorption        float32
	Player            bool // player rules: difficulty scaling, Forge armour, food, exhaustion
	Creative          bool // capabilities.disableDamage
	Invulnerable      bool

	// Armour worn: for players the inventory's armour array (index 0 boots);
	// it wears down. Mobs may set it too (no wear) and add NaturalArmor.
	Armor        *[4]*inventory.Stack
	NaturalArmor int
	Blocking     bool // sword blocking: halves blockable damage

	// Potion-like modifiers the game keeps up to date (amplifier+1, 0 = none).
	Resistance     int
	FireResistance bool
	JumpBoost      int
	// Protection returns the summed enchantment protection (EPF) of the
	// worn items against d, before capping; nil means none.
	Protection func(d *Damage) int
	// OnHurt may change the damage before armour (ForgeHooks.onLivingHurt).
	OnHurt func(d *Damage, amount float32) float32
	// OnDamaged runs after health was lost; OnDeath when health reached 0.
	OnDamaged func(d *Damage, amount float32)
	OnDeath   func(d *Damage)

	Food *Food // players only

	HurtResist, MaxHurtResist int // invulnerability frames (hurtResistantTime, 20)
	HurtTime                  int // red flash, 10 ticks
	LastDamage                float32
	Fire                      int // ticks burning (>0) or fire countdown (<0)
	FireImmune                bool
	FireCooldown              int // fireResistance: 20 for players, 1 otherwise
	Air                       int // 300 = full
	FallDistance              float32
	Age                       int
	Dead                      bool
	LastSource                *Damage

	Rand *rand.Rand // nil: global source
}

// NewPlayer returns a player body with 20 health, full food and air.
func NewPlayer(armor *[4]*inventory.Stack) *Body {
	return &Body{Health: 20, MaxHealth: 20, Player: true, Armor: armor, Food: NewFood(),
		MaxHurtResist: 20, FireCooldown: 20, Air: 300}
}

// NewMob returns a non-player body with the given max health.
func NewMob(health float32) *Body {
	return &Body{Health: health, MaxHealth: health, MaxHurtResist: 20, FireCooldown: 1, Air: 300}
}

func (b *Body) f32() float32 {
	if b.Rand != nil {
		return b.Rand.Float32()
	}
	return rand.Float32()
}

func (b *Body) intn(n int) int {
	if b.Rand != nil {
		return b.Rand.IntN(n)
	}
	return rand.IntN(n)
}

// Alive reports health > 0.
func (b *Body) Alive() bool { return !b.Dead && b.Health > 0 }

// ShouldHeal reports 0 < health < max.
func (b *Body) ShouldHeal() bool { return b.Health > 0 && b.Health < b.MaxHealth }

func (b *Body) setHealth(h float32) { b.Health = min(max(h, 0), b.MaxHealth) }

// Heal adds health (only while alive).
func (b *Body) Heal(v float32) {
	if v > 0 && b.Health > 0 {
		b.setHealth(b.Health + v)
	}
}

// Exhaust adds exhaustion to a player's food (not in creative).
func (b *Body) Exhaust(v float32) {
	if b.Food != nil && !b.Creative {
		b.Food.AddExhaustion(v)
	}
}

// TotalArmor is getTotalArmorValue.
func (b *Body) TotalArmor() int {
	t := b.NaturalArmor
	if b.Armor != nil {
		for _, s := range b.Armor {
			if s != nil && s.Item.Armor != nil {
				t += s.Item.Armor.Points
			}
		}
	}
	return t
}

// Hurt is attackEntityFrom. diff is the world difficulty (used for players).
// It returns false if the damage was ignored.
func (b *Body) Hurt(d *Damage, amount float32, diff ...Difficulty) bool {
	if b.Invulnerable || b.Health <= 0 {
		return false
	}
	if b.Player {
		if b.Creative && !d.CreativeOK {
			return false
		}
		if d.Scaled && len(diff) > 0 {
			switch diff[0] {
			case Peaceful:
				amount = 0
			case Easy:
				amount = amount/2 + 1
			case Hard:
				amount = amount * 3 / 2
			}
		}
		if amount == 0 {
			return false
		}
	}
	if d.Fire && b.FireResistance {
		return false
	}
	if (d.Type == "anvil" || d.Type == "fallingBlock") && b.Armor != nil && b.Armor[3] != nil {
		if b.wear(3, int(amount*4+b.f32()*amount*2)) {
			b.Armor[3] = nil
		}
		amount *= 0.75
	}
	if float32(b.HurtResist) > float32(b.MaxHurtResist)/2 {
		if amount <= b.LastDamage {
			return false
		}
		b.damage(d, amount-b.LastDamage)
		b.LastDamage = amount
	} else {
		b.LastDamage = amount
		b.HurtResist = b.MaxHurtResist
		b.damage(d, amount)
		b.HurtTime = 10
	}
	b.LastSource = d
	if b.Health <= 0 && !b.Dead {
		b.Dead = true
		if b.OnDeath != nil {
			b.OnDeath(d)
		}
	}
	return true
}

func (b *Body) wear(i, n int) bool {
	s := b.Armor[i]
	if !b.Player || b.Creative {
		return false
	}
	return s.AddDamage(n) && s.Count <= 0
}

// damage is damageEntity.
func (b *Body) damage(d *Damage, amount float32) {
	if b.OnHurt != nil {
		if amount = b.OnHurt(d, amount); amount <= 0 {
			return
		}
	}
	if b.Player {
		if !d.BypassArmor && b.Blocking && amount > 0 {
			amount = (1 + amount) * 0.5
		}
		amount = b.forgeArmor(d, amount)
		if amount <= 0 {
			return
		}
	} else if !d.BypassArmor {
		amount = amount * float32(25-b.TotalArmor()) / 25
	}
	amount = b.potionReduce(d, amount)
	before := amount
	amount = max(amount-b.Absorption, 0)
	b.Absorption -= before - amount
	if amount != 0 {
		b.Exhaust(d.Hunger)
		b.setHealth(b.Health - amount)
		if b.OnDamaged != nil {
			b.OnDamaged(d, amount)
		}
	}
}

// potionReduce is applyPotionDamageCalculations.
func (b *Body) potionReduce(d *Damage, amount float32) float32 {
	if d.Absolute {
		return amount
	}
	if b.Resistance > 0 && d.Type != "outOfWorld" {
		amount = amount * float32(25-b.Resistance*5) / 25
	}
	if amount <= 0 {
		return 0
	}
	if b.Protection != nil {
		m := min(b.Protection(d), 25)
		i := (m+1)>>1 + b.intn(m>>1+1)
		i = min(i, 20)
		if i > 0 {
			amount = amount * float32(25-i) / 25
		}
	}
	return amount
}

type armorProp struct {
	slot  int
	ratio float64
	max   int
}

// forgeArmor is ISpecialArmor.ArmorProperties.ApplyArmor for plain ItemArmor
// pieces (priority 0): each absorbs points/25 of the damage, capped by its
// remaining durability, and wears by max(1, int(absorbed)).
func (b *Body) forgeArmor(d *Damage, amount float32) float32 {
	dmg := float64(amount) * 25
	if b.Armor == nil || d.BypassArmor {
		return float32(dmg / 25)
	}
	var props []armorProp
	for i, s := range b.Armor {
		if s != nil && s.Item.Armor != nil {
			props = append(props, armorProp{i, float64(s.Item.Armor.Points) / 25, s.Item.MaxDamage + 1 - s.Damage})
		}
	}
	if len(props) == 0 {
		return float32(dmg / 25)
	}
	standardize(props, dmg)
	ratio := 0.0
	for _, p := range props {
		ratio += p.ratio
		if absorb := dmg * p.ratio; absorb > 0 {
			n := int(max(absorb/25, 1))
			if b.wear(p.slot, n) {
				b.Armor[p.slot] = nil
			}
		}
	}
	dmg -= dmg * ratio
	return float32(dmg / 25)
}

// standardize is StandardizeList for a single priority.
func standardize(a []armorProp, dmg float64) {
	key := func(p armorProp) float64 {
		if p.ratio == 0 {
			return 0
		}
		return float64(p.max) * 100 / p.ratio
	}
	sort.SliceStable(a, func(i, j int) bool { return int(key(a[i])-key(a[j])) < 0 })
	total := 0.0
	start := 0
	for x := 0; x < len(a); x++ {
		total += a[x].ratio
		if x != len(a)-1 {
			continue
		}
		if total > 1 {
			for y := start; y <= x; y++ {
				nr := a[y].ratio / total
				if nr*dmg > float64(a[y].max) {
					a[y].ratio = float64(a[y].max) / dmg
					total = 0
					for z := 0; z <= y; z++ {
						total += a[z].ratio
					}
					start = y + 1
					x = y
					break
				}
				a[y].ratio = nr
			}
		} else {
			for y := start; y <= x; y++ {
				if dmg*a[y].ratio > float64(a[y].max) {
					a[y].ratio = float64(a[y].max) / dmg
				}
			}
		}
	}
}

// Env is what the entity touches during one tick.
type Env struct {
	HeadInWater  bool // isInsideOfMaterial(water)
	Wet          bool // in water or in rain
	InLava       bool
	InFire       bool // touching a fire block
	InWall       bool // head inside an opaque block
	CanBreathe   bool // water breathing
	Difficulty   Difficulty
	NaturalRegen bool // game rule, normally true
}

// Tick runs one 20 Hz tick of the body's rules: fire, lava, suffocation,
// air, invulnerability timers, peaceful regeneration and food.
func (b *Body) Tick(e Env) {
	b.Age++
	hurt := func(d *Damage, v float32) { b.Hurt(d, v, e.Difficulty) }
	if e.Wet {
		b.FallDistance = 0 // handleWaterMovement
	}
	// Entity.onEntityUpdate
	if b.Fire > 0 {
		if b.FireImmune {
			b.Fire = max(b.Fire-4, 0)
		} else {
			if b.Fire%20 == 0 {
				hurt(OnFire(), 1)
			}
			b.Fire--
		}
	}
	if e.InLava {
		if !b.FireImmune {
			hurt(Lava(), 4)
			b.SetFire(15)
		}
		b.FallDistance *= 0.5
	}
	// EntityLivingBase.onEntityUpdate
	if b.Alive() && e.InWall {
		hurt(InWall(), 1)
	}
	if b.FireImmune {
		b.Fire = 0
	}
	if b.Alive() && e.HeadInWater {
		if !e.CanBreathe && !(b.Player && b.Creative) {
			b.Air--
			if b.Air == -20 {
				b.Air = 0
				hurt(Drown(), 2)
			}
		}
	} else {
		b.Air = 300
	}
	if b.Alive() && e.Wet {
		b.Fire = 0
	}
	if b.HurtTime > 0 {
		b.HurtTime--
	}
	if b.HurtResist > 0 {
		b.HurtResist--
	}
	// EntityPlayer.onLivingUpdate: the vanilla expression ticks%20*12==0
	// heals every 20 ticks.
	if b.Player && e.Difficulty == Peaceful && b.Health < b.MaxHealth && e.NaturalRegen && b.Age%20*12 == 0 {
		b.Heal(1)
	}
	// Entity.moveEntity: fire blocks
	if e.InFire {
		if !b.FireImmune {
			hurt(InFire(), 1)
		}
		if !e.Wet {
			b.Fire++
			if b.Fire == 0 {
				b.SetFire(8)
			}
		}
	} else if b.Fire <= 0 {
		b.Fire = -b.FireCooldown
	}
	if e.Wet && b.Fire > 0 {
		b.Fire = -b.FireCooldown
	}
	if b.Food != nil {
		b.Food.Tick(b, e.Difficulty, e.NaturalRegen)
	}
}

// SetFire sets the entity burning for at least sec seconds.
func (b *Body) SetFire(sec int) { b.Fire = max(b.Fire, sec*20) }

// Burning reports isBurning.
func (b *Body) Burning() bool { return !b.FireImmune && b.Fire > 0 }

// Move updates the fall distance for one tick of movement (updateFallState):
// dy is the vertical movement, onGround the state after it. Landing applies
// fall damage.
func (b *Body) Move(dy float64, onGround bool, diff Difficulty) {
	if onGround {
		if b.FallDistance > 0 {
			b.Land(b.FallDistance, diff)
			b.FallDistance = 0
		}
	} else if dy < 0 {
		b.FallDistance = float32(float64(b.FallDistance) - dy)
	}
}

// Land applies fall damage for a fall of dist blocks:
// ceil(dist - 3 - jump boost) points.
func (b *Body) Land(dist float32, diff Difficulty) {
	if n := int(math.Ceil(float64(dist - 3 - float32(b.JumpBoost)))); n > 0 {
		b.Hurt(Fall(), float32(n), diff)
	}
}

// Respawn resets a dead player: full health, a new food bar, air, no fire.
func (b *Body) Respawn() {
	b.Health, b.Absorption, b.Dead, b.LastSource = b.MaxHealth, 0, false, nil
	b.HurtResist, b.HurtTime, b.LastDamage = 0, 0, 0
	b.Fire, b.Air, b.FallDistance = 0, 300, 0
	if b.Food != nil {
		b.Food = NewFood()
	}
}
