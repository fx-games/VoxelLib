package render

import (
	"fmt"
	"math"
	"strings"

	"github.com/memmaker/voxellib/voxel"

	"github.com/go-gl/mathgl/mgl32"
)

// Shader pack mode: a port of an OptiFine (1.7.10 HD U E7) GLSL pack,
// Sildur's Vibrant with the LOTR time fix ("shaders fix lotr"). This file
// holds the CPU side of OptiFine's Shaders class: the uniforms, the sun
// and shadow matrices and the per-frame smoothing. The passes are in
// pack_wgsl.go and pack_gpu.go.

// PackFrame is the world state the shader pack reads each frame
// (Options.Pack; nil turns the pack off).
type PackFrame struct {
	Quality        PackQuality
	WorldTime      int64   // World.getWorldTime, raw ticks
	CelestialAngle float32 // World.getCelestialAngle(partialTicks)
	Rain           float32 // World.getRainStrength(partialTicks)
	MoonPhase      int32
	// EyeLight is Entity.getBrightnessForRender at the eye:
	// sky<<20 | block<<4.
	EyeLight int32
	// EyeInWater is isEyeInWater: 0 none, 1 water, 2 lava (0 in third
	// person or asleep).
	EyeInWater  int32
	NightVision float32 // getNightVisionBrightness, 0 without the effect
	Blindness   float32 // 0..1
	Far         float32 // renderDistanceChunks*16
	// Sneaking etc. are not read by the pack.
}

// PackQuality trades the pack's quality for speed; the zero value is the
// pack's own settings.
type PackQuality struct {
	ShadowRes int // shadow map size (shadowMapResolution), 0: 2048
	GodRays   int // god ray samples, 0: 20
	Clouds    int // volumetric cloud steps (reflections get 2/3), 0: 6
}

// orDefault fills the pack's values in for zero fields.
func (q PackQuality) orDefault() PackQuality {
	if q.ShadowRes <= 0 {
		q.ShadowRes = packShadowRes
	}
	if q.GodRays <= 0 {
		q.GodRays = 20
	}
	if q.Clouds <= 0 {
		q.Clouds = 6
	}
	return q
}

// wgsl sets the knobs' constants in a pack program.
func (q PackQuality) wgsl(src string) string {
	if q == (PackQuality{}).orDefault() {
		return src
	}
	return strings.NewReplacer(
		"const shadowMapResolution = 2048.0;", fmt.Sprintf("const shadowMapResolution = %d.0;", q.ShadowRes),
		"const godRaySamples = 20;", fmt.Sprintf("const godRaySamples = %d;", q.GodRays),
		"const godRayStep = 0.04;", fmt.Sprintf("const godRayStep = %g;", 0.8/float64(q.GodRays)),
		"const cloudSteps = 6;", fmt.Sprintf("const cloudSteps = %d;", q.Clouds),
		"const cloudReflSteps = 4;", fmt.Sprintf("const cloudReflSteps = %d;", max(1, q.Clouds*2/3)),
	).Replace(src)
}

// Pack constants (shaders.settings, shaders.properties, OptiFine
// defaults it doesn't override).
const (
	packSunPathRotation = -40.0
	packShadowDistance  = 80.0 // shadowMapHalfPlane
	packShadowRes       = 2048
	packShadowInterval  = 2.0 // shadowIntervalSize (OptiFine default)
	packWetnessHL       = 200.0
	packDrynessHL       = 75.0
	packEyeHL           = 10.0 // eyeBrightnessHalflife (OptiFine default)
	packNear            = 0.05
	packShadowNear      = 0.05
	packShadowFar       = 256.0
	// calcShadowDistortion: a = exp(Nearshadowplane), b = (exp(Farshadowplane)-a)*shadowDistance/128, k.
	packDistK = 1.8
)

var (
	packDistA = float32(math.Exp(0.05))
	packDistB = float32((math.Exp(1.0) - math.Exp(0.05)) * packShadowDistance / 128)
)

// LotrTime is the pack's "int lotrTime = worldTime/2" with OptiFine's
// worldTime uniform (getWorldTime() % 24000, as int).
func LotrTime(worldTime int64) int32 { return int32(worldTime%24000) / 2 }

// SunAngle is Shaders.sunAngle for a celestial angle.
func SunAngle(ca float32) float32 {
	if ca < 0.75 {
		return ca + 0.25
	}
	return ca - 0.75
}

// rotGL is glRotatef(deg, x, y, z) as a matrix (axis is unit length).
func rotGL(deg float32, x, y, z float32) mgl32.Mat4 {
	return mgl32.HomogRotate3D(mgl32.DegToRad(deg), mgl32.Vec3{x, y, z})
}

// packCelestial returns sunPosition, moonPosition (view space, length
// 100) for the view matrix mv (gbufferModelView): RenderGlobal.renderSky
// rotates -90° about Y, OptiFine's preCelestialRotate sunPathRotation
// about Z, then celestialAngle·360° about X.
func packCelestial(mv mgl32.Mat4, ca float32) (sun, moon mgl32.Vec3) {
	m := mv.Mul4(rotGL(-90, 0, 1, 0)).Mul4(rotGL(packSunPathRotation, 0, 0, 1)).Mul4(rotGL(ca*360, 1, 0, 0))
	return m.Mul4x1(mgl32.Vec4{0, 100, 0, 0}).Vec3(), m.Mul4x1(mgl32.Vec4{0, -100, 0, 0}).Vec3()
}

// ShadowModelView is Shaders.setCameraShadow's modelview for celestial
// angle ca and camera position (x, y, z); it also returns shadowAngle
// (sunAngle, or sunAngle-0.5 when the moon casts).
func ShadowModelView(ca float32, x, y, z float64) (mgl32.Mat4, float32) {
	sa := SunAngle(ca)
	m := mgl32.Translate3D(0, 0, -100).Mul4(rotGL(90, 1, 0, 0))
	a := ca * -360 // shadowAngleInterval 0
	shadowAngle := sa
	if float64(sa) > 0.5 {
		a += 180
		shadowAngle = sa - 0.5
	}
	m = m.Mul4(rotGL(a, 0, 0, 1)).Mul4(rotGL(packSunPathRotation, 1, 0, 0))
	const iv, h = packShadowInterval, packShadowInterval / 2
	fm := func(v float64) float32 { return float32(math.Mod(float64(float32(v)), iv)) - h } // Java frem
	return m.Mul4(mgl32.Translate3D(fm(x), fm(y), fm(z))), shadowAngle
}

// ShadowProjection is glOrtho(-80, 80, -80, 80, 0.05, 256).
func ShadowProjection() mgl32.Mat4 {
	return mgl32.Ortho(-packShadowDistance, packShadowDistance, -packShadowDistance, packShadowDistance, packShadowNear, packShadowFar)
}

// ShadowDistort is shadow.vsh's distortion of a shadow clip position:
// xy /= log(|xy|·b + a)·k, z /= 6.
func ShadowDistort(p mgl32.Vec4) mgl32.Vec4 {
	d := float32(math.Log(float64(p.Vec2().Len()*packDistB+packDistA))) * packDistK
	return mgl32.Vec4{p[0] / d, p[1] / d, p[2] / 6, p[3]}
}

// packState is the per-session state OptiFine keeps between frames.
type packState struct {
	init             bool
	frameCounter     int32
	frameTimeCounter float32
	wetness          float32
	eyeFade          [2]float32
	prevMV, prevProj mgl32.Mat4
	prevCam          [3]float64
}

// packUniforms is the pack's uniform block (WGSL struct PackU in
// pack_wgsl.go). Matrices are GL convention (depth -1..1).
type packUniforms struct {
	ModelView, ModelViewInv mgl32.Mat4
	Proj, ProjInv           mgl32.Mat4
	PrevModelView, PrevProj mgl32.Mat4
	ShadowMV, ShadowProj    mgl32.Mat4
	SunPos, MoonPos, UpPos  mgl32.Vec4
	ShadowLightPos          mgl32.Vec4
	CamPos, PrevCamPos      mgl32.Vec4 // cameraPosition (float, like OptiFine)
	FogColor                mgl32.Vec4 // the clear colour (colortex0)
	View                    [4]float32 // viewWidth, viewHeight, aspectRatio, frameTimeCounter
	Weather                 [4]float32 // rainStrength, wetness, near, far
	Eye                     [4]float32 // nightVision, blindness, eyeBrightnessSmooth.xy
	Eye2                    [4]float32 // eyeBrightness.xy, frameTime, celestialAngle
	Ints                    [4]int32   // worldTime, isEyeInWater, frameCounter, lotrTime
}

// update advances the smoothing state by dtMs (OptiFine diffSystemTime)
// and fills the uniforms for this frame. view is gbufferModelView (the
// camera rotation incl. bobbing), proj gbufferProjection.
func (s *packState) update(f *PackFrame, cam [3]float64, view, proj mgl32.Mat4, w, h int, dtMs int64) packUniforms {
	if !s.init {
		s.init = true
		s.prevMV, s.prevProj, s.prevCam = view, proj, cam
		s.wetness = f.Rain
	}
	s.frameCounter++
	if s.frameCounter >= 720720 {
		s.frameCounter = 0
	}
	ft := float32(dtMs) / 1000
	s.frameTimeCounter += ft
	if s.frameTimeCounter >= 3600 {
		s.frameTimeCounter -= 3600 // OptiFine: %= 3600
	}
	fd := float32(dtMs) * 0.01
	hl := float32(packWetnessHL)
	if s.wetness < f.Rain {
		hl = packDrynessHL // sic: OptiFine swaps the half-lives
	}
	k := float32(math.Exp(math.Log(0.5) * float64(fd) / float64(hl)))
	s.wetness = s.wetness*k + f.Rain*(1-k)
	ke := float32(math.Exp(math.Log(0.5) * float64(fd) / packEyeHL))
	eb := [2]float32{float32(f.EyeLight & 0xffff), float32(f.EyeLight >> 16)}
	for i := range eb {
		s.eyeFade[i] = s.eyeFade[i]*ke + eb[i]*(1-ke)
	}

	smv, shadowAngle := ShadowModelView(f.CelestialAngle, cam[0], cam[1], cam[2])
	sun, moon := packCelestial(view, f.CelestialAngle)
	light := sun
	if shadowAngle != SunAngle(f.CelestialAngle) {
		light = moon
	}
	up := view.Mul4x1(mgl32.Vec4{0, 100, 0, 0})
	v4 := func(v mgl32.Vec3) mgl32.Vec4 { return v.Vec4(0) }
	u := packUniforms{
		ModelView: view, ModelViewInv: view.Inv(), Proj: proj, ProjInv: proj.Inv(),
		PrevModelView: s.prevMV, PrevProj: s.prevProj,
		ShadowMV: smv, ShadowProj: ShadowProjection(),
		SunPos: v4(sun), MoonPos: v4(moon), UpPos: up, ShadowLightPos: v4(light),
		CamPos:     mgl32.Vec4{float32(cam[0]), float32(cam[1]), float32(cam[2]), 0},
		PrevCamPos: mgl32.Vec4{float32(s.prevCam[0]), float32(s.prevCam[1]), float32(s.prevCam[2]), 0},
		View:       [4]float32{float32(w), float32(h), float32(w) / float32(h), s.frameTimeCounter},
		Weather:    [4]float32{f.Rain, s.wetness, packNear, f.Far},
		Eye:        [4]float32{f.NightVision, f.Blindness, float32(math.Round(float64(s.eyeFade[0]))), float32(math.Round(float64(s.eyeFade[1])))},
		Eye2:       [4]float32{eb[0], eb[1], ft, f.CelestialAngle},
		Ints:       [4]int32{int32(f.WorldTime % 24000), f.EyeInWater, s.frameCounter, LotrTime(f.WorldTime)},
	}
	s.prevMV, s.prevProj, s.prevCam = view, proj, cam
	return u
}

// PackMaterials is PackMaterial per state of a registry table, for
// mesh.Config.Materials (engine.SetMaterials) while the pack is on.
func PackMaterials(table []voxel.StateInfo) []uint8 {
	m := make([]uint8, len(table))
	for i := range table {
		if d := table[i].Def; d != nil {
			m[i] = PackMaterial(d.Name)
		}
	}
	return m
}

// lightCode packs light levels for the pack programs (model light.w,
// block fogColor.w): sky + 16*block. The plain programs ignore it.
func lightCode(sky, block uint8) float32 { return float32(min(sky, 15)) + 16*float32(min(block, 15)) }
