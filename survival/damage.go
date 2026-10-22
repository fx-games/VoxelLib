// Package survival implements the 1.7.10 (+Forge) survival rules: health,
// damage sources, armour, invulnerability frames, fire, drowning, fall
// damage, hunger (FoodStats), block break speed and harvesting, drops and
// experience. It is pure: the game feeds it what the entity touches each tick
// and acts on the results.
package survival

// Damage is a DamageSource.
type Damage struct {
	Type        string  // "inFire", "fall", "mob", ...
	BypassArmor bool    // isUnblockable
	Absolute    bool    // isDamageAbsolute: ignores resistance and protection
	Fire        bool    // isFireDamage
	Magic       bool    // isMagicDamage
	Projectile  bool    // isProjectile
	Explosion   bool    // isExplosion
	CreativeOK  bool    // canHarmInCreative
	Scaled      bool    // isDifficultyScaled
	Hunger      float32 // exhaustion caused when it hurts (0.3, or 0 when bypassing armour)
	Source      any     // attacking entity (EntityDamageSource.getEntity), the game's type
	Direct      any     // direct entity (arrow), if different
}

func src(t string) *Damage { return &Damage{Type: t, Hunger: 0.3} }

func (d *Damage) bypass() *Damage   { d.BypassArmor, d.Hunger = true, 0; return d }
func (d *Damage) absolute() *Damage { d.Absolute, d.Hunger = true, 0; return d }
func (d *Damage) fire() *Damage     { d.Fire = true; return d }

// The vanilla damage sources. Each call returns a fresh value.
func InFire() *Damage       { return src("inFire").fire() }
func OnFire() *Damage       { return src("onFire").bypass().fire() }
func Lava() *Damage         { return src("lava").fire() }
func InWall() *Damage       { return src("inWall").bypass() }
func Drown() *Damage        { return src("drown").bypass() }
func Starve() *Damage       { return src("starve").bypass().absolute() }
func Cactus() *Damage       { return src("cactus") }
func Fall() *Damage         { return src("fall").bypass() }
func OutOfWorld() *Damage   { d := src("outOfWorld").bypass(); d.CreativeOK = true; return d }
func Generic() *Damage      { return src("generic").bypass() }
func Magic() *Damage        { d := src("magic").bypass(); d.Magic = true; return d }
func Wither() *Damage       { return src("wither").bypass() }
func Anvil() *Damage        { return src("anvil") }
func FallingBlock() *Damage { return src("fallingBlock") }

// MobAttack is causeMobDamage: scaled by difficulty when it hits a player.
func MobAttack(mob any) *Damage { d := src("mob"); d.Source, d.Scaled = mob, true; return d }

// PlayerAttack is causePlayerDamage (not difficulty scaled).
func PlayerAttack(player any) *Damage { d := src("player"); d.Source = player; return d }

// Arrow is causeArrowDamage. Like every indirect source it is not
// difficulty scaled (its direct entity is the arrow, not a mob).
func Arrow(arrow, shooter any) *Damage {
	d := src("arrow")
	d.Direct, d.Source, d.Projectile = arrow, shooter, true
	return d
}

// Explosion is setExplosionSource: scaled by difficulty.
func Explosion(by any) *Damage {
	d := src("explosion")
	d.Source, d.Explosion, d.Scaled = by, true, true
	return d
}

// Difficulty is EnumDifficulty.
type Difficulty int

const (
	Peaceful Difficulty = iota
	Easy
	Normal
	Hard
)
