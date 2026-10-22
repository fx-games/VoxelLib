package survival

import (
	"math/rand/v2"
	"slices"

	"github.com/memmaker/voxellib/inventory"
)

// Block holds the mining rules of one block state.
type Block struct {
	Key          string
	Hardness     float32 // < 0: unbreakable (bedrock -1)
	Material     string  // "rock", "wood", "ground", "plants", "leaves", "web", ...
	NeedsTool    bool    // material.isToolNotRequired() == false (rock, iron, anvil, web, snow, ...)
	HarvestTool  string  // Forge harvest tool class, "" for none
	HarvestLevel int
	Drops        []Drop
	FortuneOre   bool // BlockOre fortune: count × (1 + max(0, rand(fortune+2)-1))
	XPMin, XPMax int  // experience when harvested (ores)
}

// Drop is one possible drop of a block.
type Drop struct {
	Item     *inventory.Item
	Meta     int
	Min, Max int     // count range (0,0 means 1)
	Chance   float32 // 0 means always
}

// Dig is the player's digging situation.
type Dig struct {
	OnGround     bool
	Underwater   bool // head in water
	AquaAffinity bool
	Efficiency   int // enchantment level
	Haste        int // amplifier+1, 0 none
	Fatigue      int // amplifier+1, 0 none
}

func toolSpeed(t *inventory.Tool, b *Block) float32 {
	if t.Class != "" && t.Class == b.HarvestTool {
		return t.Efficiency
	}
	if v, ok := t.BlockSpeed[b.Key]; ok {
		return v
	}
	if v, ok := t.BlockSpeed[b.Material]; ok {
		return v
	}
	if slices.Contains(t.Materials, b.Material) {
		return t.Efficiency
	}
	return 1
}

func harvestLevel(s *inventory.Stack, class string) int {
	if s == nil || s.Item.Tool == nil || s.Item.Tool.Class == "" || s.Item.Tool.Class != class {
		return -1
	}
	return s.Item.Tool.Level
}

// CanHarvest is ForgeHooks.canHarvestBlock: will breaking b with held drop anything?
func CanHarvest(b *Block, held *inventory.Stack) bool {
	if !b.NeedsTool {
		return true
	}
	if held != nil && b.HarvestTool != "" {
		if lv := harvestLevel(held, b.HarvestTool); lv >= 0 {
			return lv >= b.HarvestLevel
		}
	}
	if held == nil || held.Item.Tool == nil {
		return false
	}
	h := held.Item.Tool.Harvests
	return slices.Contains(h, b.Material) || slices.Contains(h, b.Key)
}

// Speed is EntityPlayer.getBreakSpeed.
func Speed(b *Block, held *inventory.Stack, d Dig) float32 {
	f := float32(1)
	if held != nil && held.Item.Tool != nil {
		f = toolSpeed(held.Item.Tool, b)
	}
	if f > 1 && d.Efficiency > 0 && held != nil {
		f += float32(d.Efficiency*d.Efficiency + 1)
	}
	if d.Haste > 0 {
		f *= 1 + float32(d.Haste)*0.2
	}
	if d.Fatigue > 0 {
		f *= 1 - float32(d.Fatigue)*0.2
	}
	if d.Underwater && !d.AquaAffinity {
		f /= 5
	}
	if !d.OnGround {
		f /= 5
	}
	return max(f, 0)
}

// Strength is the break progress per tick (getPlayerRelativeBlockHardness):
// speed / hardness / 30 when harvestable, / 100 otherwise; 0 if unbreakable.
func Strength(b *Block, held *inventory.Stack, d Dig) float32 {
	if b.Hardness < 0 {
		return 0
	}
	if !CanHarvest(b, held) {
		return Speed(b, held, d) / b.Hardness / 100
	}
	return Speed(b, held, d) / b.Hardness / 30
}

// Breaker is PlayerControllerMP's survival block breaking: progress adds up
// per tick, a block breaks at 1, and 5 ticks pass before the next one starts.
type Breaker struct {
	X, Y, Z  int
	Hitting  bool
	Progress float32
	Delay    int
	tool     *inventory.Stack
}

func (br *Breaker) same(x, y, z int, held *inventory.Stack) bool {
	ok := br.tool == nil && held == nil
	if br.tool != nil && held != nil {
		ok = held.Item == br.tool.Item && inventory.TagsEqual(held, br.tool) &&
			(held.Damageable() || held.Damage == br.tool.Damage)
	}
	return x == br.X && y == br.Y && z == br.Z && ok
}

// click is clickBlock; it reports an instant break.
func (br *Breaker) click(x, y, z int, held *inventory.Stack, strength float32, air bool) bool {
	if br.Hitting && br.same(x, y, z, held) {
		return false
	}
	if !air && strength >= 1 {
		return true
	}
	br.Hitting, br.X, br.Y, br.Z, br.tool, br.Progress = true, x, y, z, held.Copy(), 0
	return false
}

// Tick advances breaking the block at x,y,z for one tick while the attack
// button is held; pressed is true on the tick the button went down. strength
// is Strength for that block and air reports an empty target. It returns true
// when the block breaks this tick; the game then calls BreakBlock.
func (br *Breaker) Tick(x, y, z int, held *inventory.Stack, strength float32, air, pressed bool) bool {
	if pressed && br.click(x, y, z, held, strength, air) {
		return true
	}
	if br.Delay > 0 {
		br.Delay--
		return false
	}
	if !br.same(x, y, z, held) {
		return br.click(x, y, z, held, strength, air)
	}
	if air {
		br.Hitting = false
		return false
	}
	br.Progress += strength
	if br.Progress >= 1 {
		br.Hitting, br.Progress, br.Delay = false, 0, 5
		return true
	}
	return false
}

// Stage returns the crack overlay stage 0–9, or -1 for none.
func (br *Breaker) Stage() int {
	if !br.Hitting {
		return -1
	}
	return int(br.Progress*10) - 1
}

// Reset is resetBlockRemoving (the button was released or the target lost).
func (br *Breaker) Reset() { br.Hitting, br.Progress = false, 0 }

// Broken is the outcome of breaking a block in survival.
type Broken struct {
	Drops     []*inventory.Stack
	XP        int
	ToolBroke bool
}

// BreakBlock is ItemInWorldManager.tryHarvestBlock for survival: the held
// tool wears (BreakDamage when hardness != 0) and is removed from the hotbar
// when it breaks; if the block is harvestable the player is exhausted and
// gets drops and XP. body may be nil (no exhaustion).
func BreakBlock(b *Block, p *inventory.Player, body *Body, fortune int, r *rand.Rand) Broken {
	var out Broken
	held := p.Current()
	harvest := CanHarvest(b, held)
	if held != nil && held.Item.Tool != nil && held.Item.Tool.BreakDamage > 0 && b.Hardness != 0 && !p.Creative {
		if held.AddDamage(held.Item.Tool.BreakDamage) && held.Count <= 0 {
			p.Main[p.Held] = nil
			out.ToolBroke = true
		}
	}
	if !harvest || p.Creative {
		return out
	}
	if body != nil {
		body.Exhaust(ExhaustMine)
	}
	out.Drops = b.Harvest(fortune, r)
	if b.XPMax > 0 {
		out.XP = b.XPMin + intn(r, b.XPMax-b.XPMin+1)
	}
	return out
}

func intn(r *rand.Rand, n int) int {
	if r != nil {
		return r.IntN(n)
	}
	return rand.IntN(n)
}

func f32(r *rand.Rand) float32 {
	if r != nil {
		return r.Float32()
	}
	return rand.Float32()
}

// Harvest rolls the block's drops (Block.getDrops) with a fortune level.
func (b *Block) Harvest(fortune int, r *rand.Rand) []*inventory.Stack {
	var out []*inventory.Stack
	for _, d := range b.Drops {
		if d.Chance > 0 && f32(r) > d.Chance {
			continue
		}
		n := 1
		if d.Max > 0 {
			n = d.Min + intn(r, d.Max-d.Min+1)
		}
		if fortune > 0 && b.FortuneOre {
			n *= max(intn(r, fortune+2)-1, 0) + 1
		}
		if n > 0 {
			out = append(out, &inventory.Stack{Item: d.Item, Count: n, Damage: d.Meta})
		}
	}
	return out
}
