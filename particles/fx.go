package particles

import (
	"math"
	"math/rand"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/fmath"
)

// The FX particles are the reference client's EffectRenderer and EntityFX,
// simulated per tick with its formulas and handed to the renderer as
// Particle sprites: System.Make builds the named vanilla particles
// (RenderGlobal.doSpawnParticle), TickFX runs their onUpdate and Sprites
// draws them. A game adds its own particles with Maker (names) and Kinds
// (behaviour), and block debris with Digging, Destroy and Hit.

// Kind is an FX particle's EntityFX class.
type Kind uint8

const (
	KindNone     Kind = iota // a game kind's Like when it borrows nothing
	KindSmoke                // EntitySmokeFX ("smoke", "largesmoke" scale 2.5)
	KindFlame                // EntityFlameFX
	KindAura                 // EntityAuraFX ("happyVillager": texture 82, white)
	KindLava                 // EntityLavaFX
	KindExplode              // EntityExplodeFX
	KindIcon                 // EntityDiggingFX and EntityBreakingFX: a quarter of a block or item icon
	KindDrop                 // EntityDropParticleFX ("dripWater")
	KindPortal               // EntityPortalFX ("portal")
	KindCrit                 // EntityCritFX ("crit": arrows' trail)
	KindLarge                // EntityLargeExplodeFX ("largeexplode": explosion.png's 4x4 frames)
	KindHuge                 // EntityHugeExplodeFX ("hugeexplosion": spawns largeexplode, not drawn)
	KindHeart                // EntityHeartFX ("heart")
	KindMobSpell             // EntitySpellParticleFX ("mobSpell": potion swirls, tinted by the motion arguments)
	KindFwSpark              // EntityFireworkSparkFX ("fireworksSpark[_rrggbb_fade_tf]": colour, fade colour or -, trail, flicker)
	KindFwFlash              // EntityFireworkOverlayFX ("fireworksOverlay_rrggbb")
	KindNote                 // EntityNoteFX ("note": the colour from the first motion argument)
	KindReddust              // EntityReddustFX ("reddust": the colour from the motion arguments, 0 red meaning 1)
	KindEnchant              // EntityEnchantmentTableParticleFX ("enchantmenttable": glyphs flying to the start)
	KindShovel               // EntitySnowShovelFX ("snowshovel": the snow golem's making)
	KindBubble               // EntityBubbleFX ("bubble")
	KindSplash               // EntitySplashFX ("splash", an EntityRainFX)

	// KindGame is the first of a game's own kinds (System.Kinds).
	KindGame Kind = 64
)

// The renderer sheets the vanilla particles use besides the atlas
// (particles.png by cell); the others are a game's.
const (
	SheetIcons     = 1 // block and item icons (System.Icon's rectangles)
	SheetParticles = 2 // particles.png sampled by rectangle
	SheetExplosion = 5 // textures/entity/explosion.png (EntityLargeExplodeFX)
)

// GlowCell is particles.png's glow cell as a SheetParticles rectangle.
var GlowCell = [4]float32{0.25, 0.125, 0.5, 0.375}

// FX is one EntityFX.
type FX struct {
	Kind                        Kind
	X, Y, Z, PX, PY, PZ         float64
	MX, MY, MZ                  float64
	Age, MaxAge, Tex            int
	Scale, R, G, B              float32
	NoClip, OnGround, Dead      bool
	JX, JY                      float32    // particleTextureJitterX/Y (0..3)
	Layer                       uint8      // getFXLayer: 0 particles.png, 1 terrain, 2 items, 3 and 4 a game's
	Icon                        [4]float32 // KindIcon: the icon's u0, v0, u1, v1 on the icon sheet
	A                           float32    // particleAlpha
	Bob                         int        // KindDrop: bobTimer
	OX, OY, OZ                  float64    // KindPortal, KindEnchant: the start (portalPosX/Y/Z)
	FwFade                      [3]float32 // KindFwSpark: the fade colour (FwHasFade)
	FwHasFade, FwTrail, FwFlick bool

	// W is the box's size when setSize shrank it (0: EntityFX's 0.2); the
	// box keeps its corner and yOffset stays 0.1.
	W float64
	// SpellBase is a spell particle's setBaseSpellTextureIndex (0: 128).
	SpellBase int
	// Sheet, when non-zero, draws Tex as a 16 x 16 cell of that renderer
	// sheet instead of the atlas (a game's own particles.png).
	Sheet uint8
	// Frame0 and Frames are a game kind's texture run; IDs, T and the
	// Roll speed and acceleration its own state (Roll turns the sprite).
	Frame0, Frames             int
	IDs                        [2]string
	T                          float32
	Roll, RollSpeed, RollAccel float32
}

// KindDef is a game's particle kind. Like is the vanilla kind it ticks,
// sizes and draws as (KindNone: none); Tick replaces its onUpdate, After
// runs after Like's, Sprite replaces its size and light boost, and
// Particle its sprite (x, y, z placed, c lit, as Particle passes them).
// A kind Like KindIcon has no gravity (particleGravity 0).
type KindDef struct {
	Like     Kind
	Tick     func(p *FX)
	After    func(p *FX)
	Sprite   func(p *FX, pt float32) (size, boost float32)
	Particle func(p *FX, x, y, z float64, size float32, c [4]float32, pt float64) Particle
}

// kind resolves k to the vanilla kind it acts as and its definition.
func (s *System) kind(k Kind) (Kind, KindDef) {
	if k < KindGame {
		return k, KindDef{}
	}
	d := s.Kinds[k]
	return d.Like, d
}

// Crumb is an EntityBreakingFX of item icon key ("name#meta") at x, y, z
// with motion vx, vy, vz (blocks a tick): an item's crumb or poof. No
// icon (System.Icon), no crumb.
func (s *System) Crumb(key string, x, y, z, vx, vy, vz float64) {
	if p, ok := s.Make("iconcrack_"+key, x, y, z, vx, vy, vz); ok {
		s.AddFX(p)
	}
}

// Burst is NetHandlerPlayClient.handleParticles at speed 0: n particles
// offset by gaussian*spread per axis, each with its own motion.
func (s *System) Burst(name string, n int, x, y, z, spread float64, cam [3]float64) {
	s.rnd()
	for range n {
		ox, oy, oz := s.Rand.NormFloat64()*spread, s.Rand.NormFloat64()*spread, s.Rand.NormFloat64()*spread
		s.Spawn(name, x+ox, y+oy, z+oz, 0, 0, 0, cam)
	}
}

// F is the particles' rand.nextFloat.
func (s *System) F() float32 { return s.rnd().Float32() }

func (s *System) rnd() *rand.Rand {
	if s.Rand == nil {
		s.Rand = rand.New(rand.NewSource(2))
	}
	return s.Rand
}

// Base is EntityFX(world, x, y, z, mx, my, mz).
func (s *System) Base(k Kind, x, y, z, vx, vy, vz float64) FX {
	s.rnd()
	p := FX{Kind: k, X: x, Y: y, Z: z, PX: x, PY: y, PZ: z, R: 1, G: 1, B: 1, A: 1}
	p.JX, p.JY = s.F()*3, s.F()*3
	p.Scale = (s.F()*0.5 + 0.5) * 2
	p.MaxAge = int(4 / (s.F()*0.9 + 0.1))
	p.MX = vx + float64(float32(s.Rand.Float64()*2-1)*0.4)
	p.MY = vy + float64(float32(s.Rand.Float64()*2-1)*0.4)
	p.MZ = vz + float64(float32(s.Rand.Float64()*2-1)*0.4)
	f := float64(float32(s.Rand.Float64()+s.Rand.Float64()+1) * 0.15)
	l := float64(float32(math.Sqrt(p.MX*p.MX + p.MY*p.MY + p.MZ*p.MZ)))
	p.MX = p.MX / l * f * 0.4000000059604645
	p.MY = p.MY/l*f*0.4000000059604645 + 0.10000000149011612
	p.MZ = p.MZ / l * f * 0.4000000059604645
	return p
}

// Make builds the particle RenderGlobal.doSpawnParticle makes for a name
// (ok false: a name not drawn here).
func (s *System) Make(name string, x, y, z, vx, vy, vz float64) (FX, bool) {
	s.rnd()
	switch name {
	case "smoke", "largesmoke":
		sc := float32(1)
		if name == "largesmoke" {
			sc = 2.5
		}
		return s.Smoke(x, y, z, vx, vy, vz, sc), true
	case "flame":
		p := s.Base(KindFlame, x, y, z, vx, vy, vz)
		p.MX, p.MY, p.MZ = p.MX*0.009999999776482582+vx, p.MY*0.009999999776482582+vy, p.MZ*0.009999999776482582+vz
		for range 6 { // the unused position jitter
			s.F()
		}
		p.MaxAge = int(8/(s.Rand.Float64()*0.8+0.2)) + 4
		p.NoClip, p.Tex = true, 48
		return p, true
	case "heart": // EntityHeartFX, scale factor 2
		p := s.Base(KindHeart, x, y, z, 0, 0, 0)
		p.MX, p.MY, p.MZ = p.MX*0.009999999776482582, p.MY*0.009999999776482582+0.1, p.MZ*0.009999999776482582
		p.Scale *= 0.75 * 2
		p.MaxAge, p.Tex = 16, 80
		return p, true
	case "angryVillager": // EntityHeartFX half a block up, texture 81, white
		p := s.Base(KindHeart, x, y+0.5, z, 0, 0, 0)
		p.MX, p.MY, p.MZ = p.MX*0.009999999776482582, p.MY*0.009999999776482582+0.1, p.MZ*0.009999999776482582
		p.Scale *= 0.75 * 2
		p.MaxAge, p.Tex = 16, 81
		p.R, p.G, p.B = 1, 1, 1
		return p, true
	case "note": // EntityNoteFX, scale factor 2
		p := s.Base(KindNote, x, y, z, 0, 0, 0)
		p.MX, p.MY, p.MZ = p.MX*0.009999999776482582, p.MY*0.009999999776482582+0.2, p.MZ*0.009999999776482582
		c := float32(vx)
		p.R = fmath.Sin((c+0)*math.Pi*2)*0.65 + 0.35
		p.G = fmath.Sin((c+0.33333334)*math.Pi*2)*0.65 + 0.35
		p.B = fmath.Sin((c+0.6666667)*math.Pi*2)*0.65 + 0.35
		p.Scale *= 0.75 * 2
		p.MaxAge, p.Tex = 6, 64
		return p, true
	case "happyVillager":
		p := s.Base(KindAura, x, y, z, vx, vy, vz)
		s.F() // grey, then setRBGColorF(1, 1, 1)
		p.Scale *= s.F()*0.6 + 0.5
		p.MX, p.MY, p.MZ = p.MX*0.019999999552965164, p.MY*0.019999999552965164, p.MZ*0.019999999552965164
		p.MaxAge = int(20 / (s.Rand.Float64()*0.8 + 0.2))
		p.NoClip, p.Tex = true, 82
		return p, true
	case "townaura": // EntityAuraFX, grey, texture 0
		p := s.Base(KindAura, x, y, z, vx, vy, vz)
		g := s.F()*0.1 + 0.2
		p.R, p.G, p.B = g, g, g
		p.Shrink(0.019999999552965164)
		p.Scale *= s.F()*0.6 + 0.5
		p.MX, p.MY, p.MZ = p.MX*0.019999999552965164, p.MY*0.019999999552965164, p.MZ*0.019999999552965164
		p.MaxAge = int(20 / (s.Rand.Float64()*0.8 + 0.2))
		p.NoClip = true
		return p, true
	case "reddust": // EntityReddustFX(x, y, z, 1, r, g, b)
		p := s.Base(KindReddust, x, y, z, 0, 0, 0)
		p.MX, p.MY, p.MZ = p.MX*0.10000000149011612, p.MY*0.10000000149011612, p.MZ*0.10000000149011612
		r := float32(vx)
		if r == 0 {
			r = 1
		}
		k := float32(s.Rand.Float64())*0.4 + 0.6
		p.R = (float32(s.Rand.Float64()*0.20000000298023224) + 0.8) * r * k
		p.G = (float32(s.Rand.Float64()*0.20000000298023224) + 0.8) * float32(vy) * k
		p.B = (float32(s.Rand.Float64()*0.20000000298023224) + 0.8) * float32(vz) * k
		p.Scale *= 0.75
		p.MaxAge = int(8 / (s.Rand.Float64()*0.8 + 0.2))
		return p, true
	case "enchantmenttable":
		p := s.Base(KindEnchant, x, y, z, vx, vy, vz)
		p.MX, p.MY, p.MZ = vx, vy, vz
		p.OX, p.OY, p.OZ = x, y, z
		c := s.F()*0.6 + 0.4
		p.Scale = s.F()*0.5 + 0.2
		p.R, p.G, p.B = c*0.9, c*0.9, c
		p.MaxAge = int(s.Rand.Float64()*10) + 30
		p.NoClip = true
		p.Tex = int(s.Rand.Float64()*26 + 1 + 224)
		return p, true
	case "lava":
		p := s.Base(KindLava, x, y, z, 0, 0, 0)
		p.MX, p.MZ = p.MX*0.800000011920929, p.MZ*0.800000011920929
		p.MY = float64(s.F()*0.4 + 0.05)
		p.Scale *= s.F()*2 + 0.2
		p.MaxAge = int(16 / (s.Rand.Float64()*0.8 + 0.2))
		p.Tex = 49
		return p, true
	case "crit":
		// EntityCritFX: the base motion a tenth plus 0.4 of the given,
		// grey, three quarters size; the constructor runs onUpdate once.
		p := s.Base(KindCrit, x, y, z, 0, 0, 0)
		p.MX = p.MX*0.10000000149011612 + vx*0.4
		p.MY = p.MY*0.10000000149011612 + vy*0.4
		p.MZ = p.MZ*0.10000000149011612 + vz*0.4
		c := float32(s.Rand.Float64()*0.30000001192092896 + 0.6000000238418579)
		p.R, p.G, p.B = c, c, c
		p.Scale *= 0.75
		p.MaxAge = int(6 / (s.Rand.Float64()*0.8 + 0.6))
		p.Tex = 65
		s.critUpdate(&p)
		return p, true
	case "magicCrit":
		// RenderGlobal: an EntityCritFX tinted (r x0.3, g x0.8) on the
		// next texture.
		p, _ := s.Make("crit", x, y, z, vx, vy, vz)
		p.R, p.G = p.R*0.3, p.G*0.8
		p.Tex++
		return p, true
	case "largeexplode":
		// EntityLargeExplodeFX: no motion, 6-9 ticks, grey, half size
		// 2*(1 - arg*0.5), vanilla's layer 3 (here 4: 3 is a game's).
		p := s.Base(KindLarge, x, y, z, 0, 0, 0)
		p.MX, p.MY, p.MZ = 0, 0, 0
		p.MaxAge = 6 + s.Rand.Intn(4)
		c := s.F()*0.6 + 0.4
		p.R, p.G, p.B = c, c, c
		p.Scale = 1 - float32(vx)*0.5
		p.Layer = 4
		return p, true
	case "hugeexplosion":
		p := s.Base(KindHuge, x, y, z, 0, 0, 0)
		p.MX, p.MY, p.MZ = 0, 0, 0
		p.MaxAge, p.Layer = 8, 1
		return p, true
	case "bubble":
		p := s.Base(KindBubble, x, y, z, vx, vy, vz)
		p.Tex = 32
		p.Scale *= s.F()*0.6 + 0.2
		p.MX = vx*0.20000000298023224 + float64(float32(s.Rand.Float64()*2-1)*0.02)
		p.MY = vy*0.20000000298023224 + float64(float32(s.Rand.Float64()*2-1)*0.02)
		p.MZ = vz*0.20000000298023224 + float64(float32(s.Rand.Float64()*2-1)*0.02)
		p.MaxAge = int(8 / (s.Rand.Float64()*0.8 + 0.2))
		p.Shrink(0.019999999552965164)
		return p, true
	case "splash":
		// EntityRainFX(x, y, z): at rest, then its own motion, texture
		// 19-22 and age; EntitySplashFX: gravity 0.04, the next texture,
		// the given motion when it is level.
		p := s.Base(KindSplash, x, y, z, 0, 0, 0)
		p.MX *= 0.30000001192092896
		p.MY = float64(float32(s.Rand.Float64())*0.2 + 0.1)
		p.MZ *= 0.30000001192092896
		p.Tex = 19 + s.Rand.Intn(4) + 1
		p.Shrink(0.009999999776482582)
		p.MaxAge = int(8 / (s.Rand.Float64()*0.8 + 0.2))
		if vy == 0 && (vx != 0 || vz != 0) {
			p.MX, p.MY, p.MZ = vx, vy+0.1, vz
		}
		return p, true
	case "explode":
		p := s.Base(KindExplode, x, y, z, vx, vy, vz)
		p.MX = vx + float64(float32(s.Rand.Float64()*2-1)*0.05)
		p.MY = vy + float64(float32(s.Rand.Float64()*2-1)*0.05)
		p.MZ = vz + float64(float32(s.Rand.Float64()*2-1)*0.05)
		c := s.F()*0.3 + 0.7
		p.R, p.G, p.B = c, c, c
		p.Scale = s.F()*s.F()*6 + 1
		p.MaxAge = int(16/(float64(s.F())*0.8+0.2)) + 2
		p.Tex = 7
		return p, true
	}
	switch {
	case name == "portal":
		// EntityPortalFX: motion is the offset it flies in from, the
		// age curve places it (no collision).
		p := s.Base(KindPortal, x, y, z, vx, vy, vz)
		p.MX, p.MY, p.MZ = vx, vy, vz
		p.OX, p.OY, p.OZ = x, y, z
		c := s.F()*0.6 + 0.4
		p.Scale = s.F()*0.2 + 0.5 // portalParticleScale, scaled in renderParticle
		p.R, p.G, p.B = c*0.9, c*0.3, c
		p.MaxAge = int(s.Rand.Float64()*10) + 40
		p.NoClip, p.Tex = true, int(s.Rand.Float64()*8)
		return p, true
	case name == "dripWater":
		p := s.Base(KindDrop, x, y, z, 0, 0, 0)
		p.MX, p.MY, p.MZ = 0, 0, 0
		p.R, p.G, p.B = 0, 0, 1
		p.Tex, p.Bob = 113, 40
		p.MaxAge = int(64 / (s.Rand.Float64()*0.8 + 0.2))
		return p, true
	case strings.HasPrefix(name, "fireworksSpark"):
		// EntityFireworkSparkFX: EntityFX's 4-argument constructor (no
		// motion of its own), the given motion, 3/4 size, 48-59 ticks.
		p := FX{Kind: KindFwSpark, X: x, Y: y, Z: z, PX: x, PY: y, PZ: z, R: 1, G: 1, B: 1, A: 1}
		p.JX, p.JY = s.F()*3, s.F()*3
		p.Scale = (s.F()*0.5 + 0.5) * 2
		s.F()
		p.MX, p.MY, p.MZ = vx, vy, vz
		p.Scale *= 0.75
		p.MaxAge = 48 + s.Rand.Intn(12)
		p.Tex = 160 + 7
		if f := strings.Split(name, "_"); len(f) == 4 {
			if c, err := strconv.ParseInt(f[1], 16, 32); err == nil {
				p.R, p.G, p.B = float32(c>>16&255)/255, float32(c>>8&255)/255, float32(c&255)/255
			}
			if c, err := strconv.ParseInt(f[2], 16, 32); err == nil {
				p.FwHasFade, p.FwFade = true, [3]float32{float32(c>>16&255) / 255, float32(c>>8&255) / 255, float32(c&255) / 255}
			}
			p.FwTrail, p.FwFlick = f[3][0] == '1', len(f[3]) > 1 && f[3][1] == '1'
		}
		return p, true
	case strings.HasPrefix(name, "fireworksOverlay_"):
		// EntityFireworkOverlayFX: the 4-argument constructor, 4 ticks.
		p := FX{Kind: KindFwFlash, X: x, Y: y, Z: z, PX: x, PY: y, PZ: z, R: 1, G: 1, B: 1, A: 1, MaxAge: 4}
		for range 4 {
			s.F()
		}
		if c, err := strconv.ParseInt(name[len("fireworksOverlay_"):], 16, 32); err == nil {
			p.R, p.G, p.B = float32(c>>16&255)/255, float32(c>>8&255)/255, float32(c&255)/255
		}
		return p, true
	case strings.HasPrefix(name, "iconcrack_"):
		// EntityBreakingFX(world, x, y, z, mx, my, mz, item, meta).
		uv, _, ok := s.IconOf("item:" + name[len("iconcrack_"):])
		if !ok {
			return FX{}, false
		}
		p := s.Base(KindIcon, x, y, z, 0, 0, 0)
		p.Icon, p.Scale, p.Layer = uv, p.Scale/2, 2
		p.MX, p.MY, p.MZ = p.MX*0.10000000149011612+vx, p.MY*0.10000000149011612+vy, p.MZ*0.10000000149011612+vz
		return p, true
	case name == "mobSpell", name == "mobSpellAmbient":
		// RenderGlobal: EntitySpellParticleFX at rest, then
		// setRBGColorF(the motion arguments); the ambient one setAlphaF(0.15).
		p := s.Base(KindMobSpell, x, y, z, 0, 0, 0)
		p.MY *= 0.20000000298023224
		p.MX, p.MZ = p.MX*0.10000000149011612, p.MZ*0.10000000149011612
		p.Scale *= 0.75
		p.MaxAge = int(8 / (s.Rand.Float64()*0.8 + 0.2))
		p.R, p.G, p.B = float32(vx), float32(vy), float32(vz)
		if name == "mobSpellAmbient" {
			p.A = 0.15
		}
		p.Tex = 128 + 7
		return p, true
	case strings.HasPrefix(name, "spell_"), strings.HasPrefix(name, "instantSpell_"):
		// A splash potion's (RenderGlobal aux 2002): "spell" or
		// "instantSpell" (base index 144) with the given motion, the
		// colour (named rrggbb) times 0.75..1, then multiplyVelocity by
		// the horizontal speed asked for.
		_, hex, _ := strings.Cut(name, "_")
		c, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return FX{}, false
		}
		p := s.Base(KindMobSpell, x, y, z, vx, vy, vz)
		p.MY *= 0.20000000298023224
		if vx == 0 && vz == 0 {
			p.MX, p.MZ = p.MX*0.10000000149011612, p.MZ*0.10000000149011612
		}
		p.Scale *= 0.75
		p.MaxAge = int(8 / (s.Rand.Float64()*0.8 + 0.2))
		p.SpellBase = 128
		if strings.HasPrefix(name, "instantSpell_") {
			p.SpellBase = 144
		}
		p.Tex = p.SpellBase + 7
		f4 := 0.75 + s.F()*0.25
		p.R, p.G, p.B = float32(c>>16&255)/255*f4, float32(c>>8&255)/255*f4, float32(c&255)/255*f4
		d4 := float64(float32(math.Sqrt(vx*vx + vz*vz)))
		p.MX, p.MY, p.MZ = p.MX*d4, (p.MY-0.10000000149011612)*d4+0.10000000149011612, p.MZ*d4
		return p, true
	case name == "witchMagic":
		// RenderGlobal: EntitySpellParticleFX, setBaseSpellTextureIndex(144),
		// purple times 0.35..0.85.
		p := s.Base(KindMobSpell, x, y, z, vx, vy, vz)
		p.MY *= 0.20000000298023224
		if vx == 0 && vz == 0 {
			p.MX, p.MZ = p.MX*0.10000000149011612, p.MZ*0.10000000149011612
		}
		p.Scale *= 0.75
		p.MaxAge = int(8 / (s.Rand.Float64()*0.8 + 0.2))
		p.SpellBase = 144
		p.Tex = 144 + 7
		f := 0.5*s.F() + 0.35
		p.R, p.G, p.B = f, 0, f
		return p, true
	case name == "snowshovel":
		// EntitySnowShovelFX: a tenth of the base motion plus the given,
		// white less 0..0.3, 3/4 size.
		p := s.Base(KindShovel, x, y, z, vx, vy, vz)
		p.MX, p.MY, p.MZ = p.MX*0.10000000149011612+vx, p.MY*0.10000000149011612+vy, p.MZ*0.10000000149011612+vz
		c := 1 - float32(s.Rand.Float64()*0.30000001192092896)
		p.R, p.G, p.B = c, c, c
		p.Scale *= 0.75
		p.MaxAge = int(8 / (s.Rand.Float64()*0.8 + 0.2))
		p.Tex = 7
		return p, true
	case name == "snowballpoof":
		// EntityBreakingFX(world, x, y, z, Items.snowball): no motion of
		// its own, snow's particle gravity 1, half size.
		uv, _, ok := s.IconOf("item:snowball#0")
		if !ok {
			return FX{}, false
		}
		p := s.Base(KindIcon, x, y, z, 0, 0, 0)
		p.Icon, p.Scale, p.Layer = uv, p.Scale/2, 2
		return p, true
	case strings.HasPrefix(name, "blockdust_"):
		// EntityBlockDustFX: EntityDiggingFX with the motion as given.
		uv, rgb, ok := s.IconOf("block:" + name[len("blockdust_"):])
		if !ok {
			return FX{}, false
		}
		p := s.Digging(x, y, z, vx, vy, vz, uv)
		p.R, p.G, p.B = p.R*rgb[0], p.G*rgb[1], p.B*rgb[2]
		p.MX, p.MY, p.MZ = vx, vy, vz
		return p, true
	case strings.HasPrefix(name, "blockcrack_"):
		// EntityDiggingFX(...).applyRenderColor(meta).
		uv, rgb, ok := s.IconOf("block:" + name[len("blockcrack_"):])
		if !ok {
			return FX{}, false
		}
		p := s.Digging(x, y, z, vx, vy, vz, uv)
		p.R, p.G, p.B = p.R*rgb[0], p.G*rgb[1], p.B*rgb[2]
		return p, true
	}
	if s.Maker != nil {
		return s.Maker(name, x, y, z, vx, vy, vz)
	}
	return FX{}, false
}

// IconOf is Icon, nothing without one.
func (s *System) IconOf(name string) ([4]float32, [3]float32, bool) {
	if s.Icon == nil {
		return [4]float32{}, [3]float32{}, false
	}
	return s.Icon(name)
}

// Digging is EntityDiggingFX(world, x, y, z, mx, my, mz, block, meta):
// colour 0.6, half size, the block's getIcon(0, meta).
func (s *System) Digging(x, y, z, vx, vy, vz float64, uv [4]float32) FX {
	p := s.Base(KindIcon, x, y, z, vx, vy, vz)
	p.Icon, p.Scale, p.Layer = uv, p.Scale/2, 1
	p.R, p.G, p.B = 0.6, 0.6, 0.6
	return p
}

// Destroy is EffectRenderer.addBlockDestroyEffects: 4x4x4 digging
// particles flying out from the centre, coloured by colorMultiplier
// (rgb: 1 for the grass block, which skips it).
func (s *System) Destroy(x, y, z int, uv [4]float32, rgb [3]float32) {
	for i := range 4 {
		for j := range 4 {
			for k := range 4 {
				px := float64(x) + (float64(i)+0.5)/4
				py := float64(y) + (float64(j)+0.5)/4
				pz := float64(z) + (float64(k)+0.5)/4
				p := s.Digging(px, py, pz, px-float64(x)-0.5, py-float64(y)-0.5, pz-float64(z)-0.5, uv)
				p.R, p.G, p.B = p.R*rgb[0], p.G*rgb[1], p.B*rgb[2]
				s.AddFX(p)
			}
		}
	}
}

// Hit is EffectRenderer.addBlockHitEffects(x, y, z, side): one digging
// particle just off the hit side within the block bounds b (relative),
// multiplyVelocity(0.2), multipleParticleScaleBy(0.6).
func (s *System) Hit(x, y, z, side int, b [6]float64, uv [4]float32, rgb [3]float32) {
	s.rnd()
	const f = 0.10000000149011612
	px := float64(x) + s.Rand.Float64()*(b[3]-b[0]-f*2) + f + b[0]
	py := float64(y) + s.Rand.Float64()*(b[4]-b[1]-f*2) + f + b[1]
	pz := float64(z) + s.Rand.Float64()*(b[5]-b[2]-f*2) + f + b[2]
	switch side {
	case 0:
		py = float64(y) + b[1] - f
	case 1:
		py = float64(y) + b[4] + f
	case 2:
		pz = float64(z) + b[2] - f
	case 3:
		pz = float64(z) + b[5] + f
	case 4:
		px = float64(x) + b[0] - f
	case 5:
		px = float64(x) + b[3] + f
	}
	p := s.Digging(px, py, pz, 0, 0, 0, uv)
	p.R, p.G, p.B = p.R*rgb[0], p.G*rgb[1], p.B*rgb[2]
	p.MX, p.MY, p.MZ = p.MX*0.2, (p.MY-0.10000000149011612)*0.2+0.10000000149011612, p.MZ*0.2
	p.Scale *= 0.6
	s.AddFX(p)
}

// Smoke is EntitySmokeFX(world, x, y, z, mx, my, mz, scale).
func (s *System) Smoke(x, y, z, vx, vy, vz float64, sc float32) FX {
	p := s.Base(KindSmoke, x, y, z, 0, 0, 0)
	p.MX, p.MY, p.MZ = p.MX*0.10000000149011612+vx, p.MY*0.10000000149011612+vy, p.MZ*0.10000000149011612+vz
	c := float32(s.Rand.Float64() * 0.30000001192092896)
	p.R, p.G, p.B = c, c, c
	p.Scale *= 0.75 * sc
	p.MaxAge = int(8 / (s.Rand.Float64()*0.8 + 0.2))
	p.MaxAge = int(float32(p.MaxAge) * sc)
	p.Tex = 7
	return p
}

// AddFX is EffectRenderer.addEffect: 4000 per layer, the layer's oldest go
// first.
func (s *System) AddFX(p FX) {
	n, first := 0, -1
	for i := range s.FX {
		if s.FX[i].Layer == p.Layer {
			if first < 0 {
				first = i
			}
			n++
		}
	}
	if n >= 4000 {
		s.FX = append(s.FX[:first], s.FX[first+1:]...)
	}
	s.FX = append(s.FX, p)
}

// Spawn is World.spawnParticle on the client: RenderGlobal.doSpawnParticle
// drops particles more than 16 blocks from the camera (particles "all").
func (s *System) Spawn(name string, x, y, z, vx, vy, vz float64, cam [3]float64) {
	dx, dy, dz := cam[0]-x, cam[1]-y, cam[2]-z
	if dx*dx+dy*dy+dz*dz > 16*16 {
		return
	}
	if p, ok := s.Make(name, x, y, z, vx, vy, vz); ok {
		s.AddFX(p)
	}
}

// WorldFX is World.spawnParticle from inside a particle: spawn's 16-block
// camera check, the particle joining after this tick's updates.
func (s *System) WorldFX(q FX) {
	dx, dy, dz := s.Cam[0]-q.X, s.Cam[1]-q.Y, s.Cam[2]-q.Z
	if dx*dx+dy*dy+dz*dz <= 16*16 {
		s.spawned = append(s.spawned, q)
	}
}

// Move is Entity.moveEntity for a 0.2 particle, reduced to its centre: an
// axis that runs into a solid block stops and loses its motion.
func (s *System) Move(p *FX) {
	if p.NoClip || s.Solid == nil {
		p.X, p.Y, p.Z = p.X+p.MX, p.Y+p.MY, p.Z+p.MZ
		return
	}
	// Entity.moveEntity for EntityFX's 0.2 box (yOffset 0.1: the box is
	// centred on the position) against full cubes: Y, then X, then Z; a
	// clipped axis loses its motion.
	const h = 0.1
	w := p.W
	if w == 0 {
		w = 0.2
	}
	box := [6]float64{p.X - w/2, p.Y - h, p.Z - w/2, p.X + w/2, p.Y - h + w, p.Z + w/2}
	fl := func(v float64) int { return int(math.Floor(v)) }
	clip := func(a int, d float64) float64 {
		lo, hi := box, box
		if d < 0 {
			lo[a] += d
		} else {
			hi[a+3] += d
		}
		for x := fl(lo[0]); x <= fl(hi[3]); x++ {
			for y := fl(lo[1]); y <= fl(hi[4]); y++ {
				for z := fl(lo[2]); z <= fl(hi[5]); z++ {
					if !s.Solid(x, y, z) {
						continue
					}
					c := [3]float64{float64(x), float64(y), float64(z)}
					// AxisAlignedBB.calculate?Offset: the other two axes must overlap.
					ok := true
					for b := range 3 {
						if b != a && (box[b+3] <= c[b] || box[b] >= c[b]+1) {
							ok = false
						}
					}
					if !ok {
						continue
					}
					if d > 0 && box[a+3] <= c[a] {
						d = min(d, c[a]-box[a+3])
					} else if d < 0 && box[a] >= c[a]+1 {
						d = max(d, c[a]+1-box[a])
					}
				}
			}
		}
		box[a] += d
		box[a+3] += d
		return d
	}
	dy := clip(1, p.MY)
	p.OnGround = p.MY != dy && p.MY < 0
	dx := clip(0, p.MX)
	dz := clip(2, p.MZ)
	if dx != p.MX {
		p.MX = 0
	}
	if dy != p.MY {
		p.MY = 0
	}
	if dz != p.MZ {
		p.MZ = 0
	}
	p.X, p.Y, p.Z = box[0]+w/2, box[1]+h, box[2]+w/2
}

// Shrink is setSize(w, w) after the 0.2 box was placed: the box keeps its
// low corner, so the particle sits w/2 into it across from there on.
func (p *FX) Shrink(w float64) {
	p.W = w
	d := 0.1 - w/2
	p.X, p.Z, p.PX, p.PZ = p.X-d, p.Z-d, p.PX-d, p.PZ-d
}

func (p *FX) Drag(k float64) {
	p.MX, p.MY, p.MZ = p.MX*k, p.MY*k, p.MZ*k
	if p.OnGround {
		p.MX, p.MZ = p.MX*0.699999988079071, p.MZ*0.699999988079071
	}
}

// TickFX is each particle's onUpdate; the dead are dropped.
func (s *System) TickFX() {
	s.rnd()
	live := s.FX[:0]
	for i := range s.FX {
		p := &s.FX[i]
		p.PX, p.PY, p.PZ = p.X, p.Y, p.Z
		k, def := s.kind(p.Kind)
		if def.Tick != nil {
			def.Tick(p)
			if !p.Dead {
				live = append(live, *p)
			}
			continue
		}
		switch k {
		case KindSmoke, KindExplode:
			if p.Age >= p.MaxAge {
				p.Dead = true
			}
			p.Age++
			p.Tex = 7 - p.Age*8/p.MaxAge
			p.MY += 0.004
			y0 := p.Y
			s.Move(p)
			if k == KindExplode {
				p.Drag(0.8999999761581421)
				break
			}
			if p.Y == y0 {
				p.MX, p.MZ = p.MX*1.1, p.MZ*1.1
			}
			p.Drag(0.9599999785423279)
		case KindReddust: // EntityReddustFX.onUpdate
			if p.Age >= p.MaxAge {
				p.Dead = true
			}
			p.Age++
			p.Tex = 7 - p.Age*8/p.MaxAge
			y0 := p.Y
			s.Move(p)
			if p.Y == y0 {
				p.MX, p.MZ = p.MX*1.1, p.MZ*1.1
			}
			p.Drag(0.9599999785423279)
		case KindEnchant: // EntityEnchantmentTableParticleFX.onUpdate
			f := 1 - float32(p.Age)/float32(p.MaxAge)
			f2 := 1 - f
			f2 *= f2
			f2 *= f2
			p.X = p.OX + p.MX*float64(f)
			p.Y = p.OY + p.MY*float64(f) - float64(f2*1.2)
			p.Z = p.OZ + p.MZ*float64(f)
			if p.Age >= p.MaxAge {
				p.Dead = true
			}
			p.Age++
		case KindHeart: // EntityHeartFX.onUpdate
			if p.Age >= p.MaxAge {
				p.Dead = true
			}
			p.Age++
			y0 := p.Y
			s.Move(p)
			if p.Y == y0 {
				p.MX, p.MZ = p.MX*1.1, p.MZ*1.1
			}
			p.Drag(0.8600000143051147)
		case KindNote: // EntityNoteFX.onUpdate
			if p.Age >= p.MaxAge {
				p.Dead = true
			}
			p.Age++
			y0 := p.Y
			s.Move(p)
			if p.Y == y0 {
				p.MX, p.MZ = p.MX*1.1, p.MZ*1.1
			}
			p.Drag(0.6600000262260437)
		case KindCrit:
			s.critUpdate(p)
		case KindLarge:
			if p.Age++; p.Age == p.MaxAge {
				p.Dead = true
			}
		case KindHuge:
			Scatter(s.Rand.Float64, 6, p.X, p.Y, p.Z, 4, func(x, y, z float64) {
				q, _ := s.Make("largeexplode", x, y, z, float64(float32(p.Age)/8), 0, 0)
				s.WorldFX(q)
			})
			if p.Age++; p.Age == p.MaxAge {
				p.Dead = true
			}
		case KindFwSpark: // EntityFireworkSparkFX.onUpdate
			if p.Age >= p.MaxAge {
				p.Dead = true
			}
			p.Age++
			if p.Age > p.MaxAge/2 {
				p.A = 1 - (float32(p.Age)-float32(p.MaxAge/2))/float32(p.MaxAge)
				if p.FwHasFade {
					p.R += (p.FwFade[0] - p.R) * 0.2
					p.G += (p.FwFade[1] - p.G) * 0.2
					p.B += (p.FwFade[2] - p.B) * 0.2
				}
			}
			p.Tex = 160 + 7 - p.Age*8/p.MaxAge
			p.MY -= 0.004
			s.Move(p)
			p.Drag(0.9100000262260437)
			if p.FwTrail && p.Age < p.MaxAge/2 && (p.Age+p.MaxAge)%2 == 0 {
				q, _ := s.Make("fireworksSpark", p.X, p.Y, p.Z, 0, 0, 0)
				q.R, q.G, q.B = p.R, p.G, p.B
				q.Age = q.MaxAge / 2
				q.FwHasFade, q.FwFade, q.FwFlick = p.FwHasFade, p.FwFade, p.FwFlick
				s.spawned = append(s.spawned, q)
			}
		case KindFwFlash: // EntityFX.onUpdate without motion
			if p.Age++; p.Age > p.MaxAge {
				p.Dead = true
			}
		case KindFlame:
			if p.Age >= p.MaxAge {
				p.Dead = true
			}
			p.Age++
			s.Move(p)
			p.Drag(0.9599999785423279)
		case KindShovel: // EntitySnowShovelFX.onUpdate
			if p.Age >= p.MaxAge {
				p.Dead = true
			}
			p.Age++
			p.Tex = 7 - p.Age*8/p.MaxAge
			p.MY -= 0.03
			s.Move(p)
			p.Drag(0.9900000095367432)
		case KindIcon: // EntityFX.onUpdate, particleGravity 1 (a kind like it: 0)
			if p.Age >= p.MaxAge {
				p.Dead = true
			}
			p.Age++
			if p.Kind == KindIcon {
				p.MY -= 0.04
			}
			s.Move(p)
			p.Drag(0.9800000190734863)
		case KindMobSpell: // EntitySpellParticleFX.onUpdate
			if p.Age >= p.MaxAge {
				p.Dead = true
			}
			p.Age++
			p.Tex = max(p.SpellBase, 128) + 7 - p.Age*8/p.MaxAge
			p.MY += 0.004
			y0 := p.Y
			s.Move(p)
			if p.Y == y0 {
				p.MX, p.MZ = p.MX*1.1, p.MZ*1.1
			}
			p.Drag(0.9599999785423279)
		case KindPortal:
			f := float32(p.Age) / float32(p.MaxAge)
			f2 := f
			f = 1 - (-f + f*f*2)
			p.X = p.OX + p.MX*float64(f)
			p.Y = p.OY + p.MY*float64(f) + float64(1-f2)
			p.Z = p.OZ + p.MZ*float64(f)
			if p.Age >= p.MaxAge {
				p.Dead = true
			}
			p.Age++
		case KindDrop: // water
			p.R, p.G, p.B = 0.2, 0.3, 1
			p.MY -= 0.06
			if p.Bob--; p.Bob >= 0 {
				p.MX, p.MY, p.MZ = p.MX*0.02, p.MY*0.02, p.MZ*0.02
				p.Tex = 113
			} else {
				p.Tex = 112
			}
			s.Move(p)
			p.Drag(0.9800000190734863)
			if p.MaxAge--; p.MaxAge < 0 {
				p.Dead = true
			}
			if p.OnGround {
				p.Dead = true
				q, _ := s.Make("splash", p.X, p.Y, p.Z, 0, 0, 0)
				s.WorldFX(q)
			}
			if s.Wet != nil {
				fl := func(v float64) int { return int(math.Floor(v)) }
				if ok, meta := s.Wet(fl(p.X), fl(p.Y), fl(p.Z)); ok {
					// BlockLiquid.getLiquidHeightPercent(meta)
					h := float32(meta + 1)
					if meta >= 8 {
						h = 0
					}
					if p.Y < float64(float32(fl(p.Y)+1)-h/9) {
						p.Dead = true
					}
				}
			}
		case KindBubble: // EntityBubbleFX.onUpdate
			p.MY += 0.002
			s.Move(p)
			p.MX, p.MY, p.MZ = p.MX*0.8500000238418579, p.MY*0.8500000238418579, p.MZ*0.8500000238418579
			if s.Water != nil && !s.Water(int(math.Floor(p.X)), int(math.Floor(p.Y)), int(math.Floor(p.Z))) {
				p.Dead = true
			}
			if p.MaxAge--; p.MaxAge < 0 {
				p.Dead = true
			}
		case KindSplash: // EntityRainFX.onUpdate, gravity 0.04
			p.MY -= 0.03999999910593033
			s.Move(p)
			p.MX, p.MY, p.MZ = p.MX*0.9800000190734863, p.MY*0.9800000190734863, p.MZ*0.9800000190734863
			if p.MaxAge--; p.MaxAge < 0 {
				p.Dead = true
			}
			if p.OnGround {
				if s.Rand.Float64() < 0.5 {
					p.Dead = true
				}
				p.MX, p.MZ = p.MX*0.699999988079071, p.MZ*0.699999988079071
			}
			if s.Wet != nil {
				fl := func(v float64) int { return int(math.Floor(v)) }
				if ok, meta := s.Wet(fl(p.X), fl(p.Y), fl(p.Z)); ok {
					h := float32(meta + 1) // BlockLiquid.getLiquidHeightPercent
					if meta >= 8 {
						h = 0
					}
					if p.Y < float64(float32(fl(p.Y)+1)-h/9) {
						p.Dead = true
					}
				}
			}
		case KindAura:
			s.Move(p)
			p.MX, p.MY, p.MZ = p.MX*0.99, p.MY*0.99, p.MZ*0.99
			if p.MaxAge <= 0 {
				p.Dead = true
			}
			p.MaxAge--
		case KindLava:
			if p.Age >= p.MaxAge {
				p.Dead = true
			}
			p.Age++
			if s.F() > float32(p.Age)/float32(p.MaxAge) {
				s.WorldFX(s.Smoke(p.X, p.Y, p.Z, p.MX, p.MY, p.MZ, 1))
			}
			p.MY -= 0.03
			s.Move(p)
			p.Drag(0.9990000128746033)
		}
		if def.After != nil {
			def.After(p)
		}
		if !p.Dead {
			live = append(live, *p)
		}
	}
	clear(s.FX[len(live):])
	s.FX = live
	for _, q := range s.spawned {
		s.AddFX(q)
	}
	s.spawned = s.spawned[:0]
}

// critUpdate is EntityCritFX.onUpdate.
func (s *System) critUpdate(p *FX) {
	p.PX, p.PY, p.PZ = p.X, p.Y, p.Z
	if p.Age >= p.MaxAge {
		p.Dead = true
	}
	p.Age++
	s.Move(p)
	p.G = float32(float64(p.G) * 0.96)
	p.B = float32(float64(p.B) * 0.9)
	p.Drag(0.699999988079071)
	p.MY -= 0.019999999552965164
	if p.OnGround {
		p.MX, p.MZ = p.MX*0.699999988079071, p.MZ*0.699999988079071
	}
}

// Sprite is the particle at partial tick pt: its size (EntityFX quads are
// 0.2*particleScale wide) and block-light boost.
func (s *System) Sprite(p *FX, pt float32) (size float32, boost float32) {
	k, def := s.kind(p.Kind)
	if def.Sprite != nil {
		return def.Sprite(p, pt)
	}
	f := (float32(p.Age) + pt) / float32(p.MaxAge)
	sc := p.Scale
	switch k {
	case KindFwSpark:
		return 0.2 * sc, 16 // getBrightnessForRender 15728880
	case KindPortal:
		f = 1 - f
		return 0.2 * sc * (1 - f*f), 0
	case KindSmoke, KindCrit, KindHeart, KindNote, KindReddust, KindShovel:
		sc *= min(1, max(0, f*32))
	case KindFlame:
		sc *= 1 - f*f*0.5
		boost = min(1, max(0, f)) * 15
	case KindLava:
		sc *= 1 - f*f
		boost = 15
	}
	return 0.2 * sc, boost
}

// Light is the light colour at a position; BoostLight the same raised by
// a block-light boost and a sky boost (getBrightnessForRender), a boost
// above 15 a fixed brightness (16: 15728880, 17: 15728864).
type (
	Light      func(x, y, z float64) [3]float32
	BoostLight func(x, y, z float64, boost, skyBoost float32) [3]float32
)

// Particle is the renderer's sprite for p at partial tick pt, already
// placed and lit.
func (s *System) Particle(p *FX, pt float64, light Light, boostLight BoostLight) Particle {
	k, def := s.kind(p.Kind)
	x, y, z := p.PX+(p.X-p.PX)*pt, p.PY+(p.Y-p.PY)*pt, p.PZ+(p.Z-p.PZ)*pt
	size, boost := s.Sprite(p, float32(pt))
	// Entity.getBrightnessForRender: the current (not interpolated)
	// position, y - yOffset 0.1 + 0.66 * the box height 0.2.
	bh := 0.20000000298023224
	if p.W != 0 {
		bh = p.W
	}
	ly := p.Y - 0.10000000149011612 + bh*0.66
	var lc [3]float32
	if k == KindPortal || k == KindEnchant { // their getBrightnessForRender: sky + (age/maxAge)^4 * 15
		f := float32(p.Age) / float32(p.MaxAge)
		f *= f
		lc = boostLight(p.X, ly, p.Z, 0, float32(int(f*f*15*16))/16)
	} else if boost > 0 {
		lc = boostLight(p.X, ly, p.Z, boost, 0)
	} else {
		lc = light(p.X, ly, p.Z)
	}
	c := [4]float32{p.R * lc[0], p.G * lc[1], p.B * lc[2], p.A}
	if def.Particle != nil {
		return def.Particle(p, x, y, z, size, c, pt)
	}
	switch k {
	case KindLarge: // full bright (240), its own grey, explosion.png's frame
		u, v, e, _ := Flipbook(p.Age, float32(pt), p.MaxAge)
		h := 4 * p.Scale
		c := [4]float32{p.R, p.G, p.B, 1}
		return Particle{X: x, Y: y, Z: z, PrevX: x, PrevY: y, PrevZ: z, Life: 1, Size0: h, Size1: h,
			Col0: c, Col1: c, Sheet: SheetExplosion, UV: [4]float32{u, v, u + e, v + e}}
	case KindFwFlash: // its renderParticle: half size 7.1*sin((age+pt-1)*pi/4), alpha 0.6 - (age+pt-1)/8
		t := float32(p.Age) + float32(pt) - 1
		h := 2 * float32(math.Abs(float64(7.1*float32(math.Sin(float64(t*0.25*math.Pi))))))
		c[3] = 0.6 - t*0.25*0.5
		return Particle{X: x, Y: y, Z: z, PrevX: x, PrevY: y, PrevZ: z, Life: 1, Size0: h, Size1: h,
			Col0: c, Col1: c, Sheet: SheetParticles, UV: GlowCell}
	case KindFwSpark: // renderParticle: a flickering spark shows on every other third of a tick
		if p.FwFlick && p.Age >= p.MaxAge/3 && (p.Age+p.MaxAge)/3%2 != 0 {
			size = 0
		}
	case KindIcon:
		// EntityDiggingFX/EntityBreakingFX.renderParticle: the icon's
		// interpolated U and V over a quarter, from the jitter.
		du, dv := p.Icon[2]-p.Icon[0], p.Icon[3]-p.Icon[1]
		uv := [4]float32{p.Icon[0] + du*p.JX/4, p.Icon[1] + dv*p.JY/4, p.Icon[0] + du*(p.JX+1)/4, p.Icon[1] + dv*(p.JY+1)/4}
		return Particle{X: x, Y: y, Z: z, PrevX: x, PrevY: y, PrevZ: z, Life: 1, Size0: size, Size1: size,
			Col0: c, Col1: c, Sheet: SheetIcons, UV: uv}
	}
	if p.Sheet != 0 { // another particles.png-like sheet: EntityFX's 1/16 cells
		u, v := float32(p.Tex%16)/16, float32(p.Tex/16)/16
		return Particle{X: x, Y: y, Z: z, PrevX: x, PrevY: y, PrevZ: z, Life: 1, Size0: size, Size1: size,
			Col0: c, Col1: c, Sheet: p.Sheet, UV: [4]float32{u, v, u + 0.0624375, v + 0.0624375}}
	}
	tex := p.Tex
	if tex < 0 { // -1 on the last tick: GL_REPEAT wraps to row 0's last cell
		tex += 16
	}
	return Particle{X: x, Y: y, Z: z, PrevX: x, PrevY: y, PrevZ: z, Life: 1, Size0: size, Size1: size,
		Col0: c, Col1: c, Sprite: uint16(tex) + 1}
}

// Sprites appends the renderer's sprites at partial tick pt to buf: the
// free particles lit at their place, then the FX (the huge explosion
// draws nothing of its own).
func (s *System) Sprites(buf []Particle, pt float64, light Light, boostLight BoostLight) []Particle {
	for i := range s.P {
		q := &s.P[i]
		x, y, z, size, col := q.Lerp(pt)
		lc := light(x, y, z)
		c := [4]float32{col[0] * lc[0], col[1] * lc[1], col[2] * lc[2], col[3]}
		buf = append(buf, Particle{X: x, Y: y, Z: z, PrevX: x, PrevY: y, PrevZ: z, Life: 1,
			Size0: size, Size1: size, Col0: c, Col1: c, Sprite: q.Sprite, Additive: q.Additive})
	}
	for i := range s.FX {
		if s.FX[i].Kind == KindHuge {
			continue
		}
		buf = append(buf, s.Particle(&s.FX[i], pt, light, boostLight))
	}
	return buf
}
