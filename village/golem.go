package village

// Homing is the villager's and the golem's village check in
// updateAITick: every 70+rand(50) ticks the village within 32 of the
// feet's block (a villager first queues its position for the door
// search).
type Homing struct {
	timer   int // randomTickDivider / homeCheckTimer
	Village *Village
}

// Tick runs the check; true when it ran (the caller then sets the home
// area to the village's centre and 60% of its radius, or detaches it).
func (h *Homing) Tick(c *Collection, x, y, z int, villager bool) bool {
	if h.timer--; h.timer > 0 {
		return false
	}
	if villager {
		c.AddVillagerPosition(x, y, z)
	}
	h.timer = 70 + c.w.IntN(50)
	h.Village = c.Nearest(x, y, z, 32)
	return true
}

// HomeRange is (int)(radius * 0.6F).
func (v *Village) HomeRange() int { return int(float32(v.Radius) * 0.6) }

// Golem is an iron golem's village state: its timers and the rose it
// holds out to a villager (EntityAILookAtVillager).
type Golem struct {
	Homing
	AttackTimer int // attackTimer: the arms' swing after a blow
	RoseTick    int // holdRoseTick
	look        int // EntityAILookAtVillager.lookTime
	Looking     string
}

// LivingTick is onLivingUpdate's timers.
func (g *Golem) LivingTick() {
	if g.AttackTimer > 0 {
		g.AttackTimer--
	}
	if g.RoseTick > 0 {
		g.RoseTick--
	}
}

// Blow is attackEntityAsMob's swing and damage, 7 to 21.
func (g *Golem) Blow(intn func(int) int) float64 {
	g.AttackTimer = 10
	return float64(7 + intn(15))
}

// HoldRose is setHoldingRose.
func (g *Golem) HoldRose(on bool) {
	g.RoseTick = 0
	if on {
		g.RoseTick = 400
	}
}

// LookStart is EntityAILookAtVillager.shouldExecute and startExecuting:
// by day, one tick in 8000, a villager found within 6, 2, 6 is offered a
// rose for 400 ticks.
func (g *Golem) LookStart(day bool, intn func(int) int, nearest func() string) bool {
	if !day || intn(8000) != 0 {
		return false
	}
	if g.Looking = nearest(); g.Looking == "" {
		return false
	}
	g.look = 400
	g.HoldRose(true)
	return true
}

// LookUpdate is updateTask and continueExecuting; false: resetTask ran.
func (g *Golem) LookUpdate() bool {
	if g.look <= 0 {
		g.LookReset()
		return false
	}
	g.look--
	return true
}

// LookReset is resetTask.
func (g *Golem) LookReset() { g.HoldRose(false); g.Looking, g.look = "", 0 }

// DefendTarget is EntityAIDefendVillage.shouldExecute: the nearest
// aggressor if suitable, else one tick in 20 the nearest player of too
// low a reputation if suitable (its name, and true for a player).
func (v *Village) DefendTarget(from Vec, intn func(int) int, suitable func(id string, player bool) bool) (string, bool) {
	if a := v.NearestAggressor(from); a != "" && suitable(a, false) {
		return a, false
	}
	if intn(20) != 0 {
		return "", false
	}
	if p := v.NearestBadPlayer(from); p != "" && suitable(p, true) {
		return p, true
	}
	return "", false
}
