package render

import (
	"fmt"
	"image"
	"image/draw"
	"math"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/model"
)

// MaxBones is the most bones a model drawn by ModelRenderer may have.
const MaxBones = 96

// ModelRenderer draws animated models (package model) with GPU skinning,
// lit by the world's sky/block light like the terrain and fogged alike.
type ModelRenderer struct {
	r      *Renderer
	prog   *program
	models []*gpuModel
}

type gpuModel struct {
	vao, vbo, ebo uint32
	parts         []gpuPart
	textures      []uint32
}

type gpuPart struct {
	first, count int32 // indices
	tex          int
}

// ModelInstance is one model to draw.
type ModelInstance struct {
	Model      int          // handle from Add
	X, Y, Z    float64      // world position of the model origin
	Yaw        float64      // radians, as Camera.Yaw (0 faces -Z... see Draw)
	Pitch      float64      // radians, tilts the model's +Z up (as Camera.Pitch)
	Scale      float32      // 0 means 1
	Skin       []mgl32.Mat4 // from model.Animator.Skin; nil = bind pose
	Sky, Block uint8        // light levels at the model
	// Tilt tips the whole model over by Tilt radians toward the world
	// direction TiltYaw (yaw convention), pivoting at its origin, e.g. a
	// falling corpse.
	Tilt, TiltYaw float64
	// Fade dissolves the model: 0 is solid, 1 is gone (a dither pattern
	// discards that share of its pixels, so no blending or sorting).
	Fade float32
	// Transform, when set, replaces Yaw/Pitch/Tilt/Scale: it maps model
	// space to the world about X,Y,Z (e.g. a weapon in a creature's hand).
	Transform *mgl32.Mat4
}

// TiltMatrix rotates the vertical toward world direction yaw by angle
// radians, about the horizontal axis across that direction.
func TiltMatrix(angle, yaw float64) mgl32.Mat4 {
	if angle == 0 {
		return mgl32.Ident4()
	}
	fx, fz := math.Sin(yaw), -math.Cos(yaw)
	return mgl32.HomogRotate3D(float32(angle), mgl32.Vec3{float32(fz), 0, float32(-fx)})
}

const modelVS = `#version 330 core
layout(location = 0) in vec3 aPos;
layout(location = 1) in vec3 aNormal;
layout(location = 2) in vec2 aUV;
layout(location = 3) in uvec4 aJoints;
layout(location = 4) in vec4 aWeights;
uniform mat4 uViewProj;
uniform mat4 uModel;
uniform mat4 uBones[96];
out vec2 vUV;
out float vShade;
out float vDist;
void main() {
	mat4 skin = uBones[aJoints.x] * aWeights.x + uBones[aJoints.y] * aWeights.y +
		uBones[aJoints.z] * aWeights.z + uBones[aJoints.w] * aWeights.w;
	vec4 wp = uModel * skin * vec4(aPos, 1.0);
	gl_Position = uViewProj * wp;
	vec3 n = normalize(mat3(uModel) * mat3(skin) * aNormal);
	// Minecraft entity lighting: two fixed lights, ambient 0.4.
	vec3 l0 = normalize(vec3(0.2, 1.0, -0.7)), l1 = normalize(vec3(-0.2, 1.0, 0.7));
	vShade = min(1.0, 0.4 + 0.6 * (max(dot(n, l0), 0.0) + max(dot(n, l1), 0.0)) * 0.6 + 0.25);
	vUV = aUV;
	vDist = length(wp.xz);
}
`

const modelFS = `#version 330 core
in vec2 vUV;
in float vShade;
in float vDist;
uniform sampler2D uTex;
uniform vec2 uLight;
uniform float uDaylight;
uniform vec3 uFogColor;
uniform vec2 uFog;
uniform float uFade;
out vec4 fragColor;
float curve(float l) { return mix(0.04, 1.0, l / (4.0 - 3.0 * l)); }
// blockCurve is curve for block light, raised toward Minecraft's bright
// gamma (1-(1-l)^4) by uBlockBright.
uniform float uBlockBright;
float blockCurve(float l) { return mix(curve(l), mix(0.04, 1.0, 1.0 - pow(1.0 - l, 4.0)), uBlockBright); }
void main() {
	vec4 c = texture(uTex, vUV);
	if (c.a < 0.5) discard;
	// Interleaved gradient noise: a stable screen-space dither for fading.
	if (uFade > 0.0 && fract(52.9829189 * fract(dot(gl_FragCoord.xy, vec2(0.06711056, 0.00583715)))) < uFade) discard;
	float sky = curve(uLight.x) * uDaylight;
	float blk = blockCurve(uLight.y);
	vec3 light = max(max(vec3(sky) * vec3(0.95, 0.97, 1.0), vec3(blk) * vec3(1.0, 0.86, 0.66)), vec3(0.04));
	vec3 col = c.rgb * vShade * light;
	float f = clamp((vDist - uFog.x) / (uFog.y - uFog.x), 0.0, 1.0);
	fragColor = vec4(mix(col, uFogColor, f * f), 1.0);
}
`

// NewModelRenderer creates an empty model renderer.
func NewModelRenderer(r *Renderer) (*ModelRenderer, error) {
	p, err := newProgram(modelVS, modelFS)
	if err != nil {
		return nil, err
	}
	return &ModelRenderer{r: r, prog: p}, nil
}

// Add uploads a model and returns its handle.
func (mr *ModelRenderer) Add(m *model.Model) (int, error) {
	if len(m.Bones) > MaxBones {
		return 0, fmt.Errorf("render: model %s has %d bones (max %d)", m.Name, len(m.Bones), MaxBones)
	}
	g := &gpuModel{}
	const stride = 3 + 3 + 2 + 1 + 4 // floats; joints packed as 4 bytes
	var vb []float32
	var ib []uint32
	for _, me := range m.Meshes {
		base := uint32(len(vb) / stride)
		first := int32(len(ib))
		for _, v := range me.Verts {
			j := uint32(v.Joints[0]) | uint32(v.Joints[1])<<8 | uint32(v.Joints[2])<<16 | uint32(v.Joints[3])<<24
			w := v.Weights
			if w == ([4]float32{}) {
				w[0] = 1
			}
			vb = append(vb, v.Pos[0], v.Pos[1], v.Pos[2], v.Normal[0], v.Normal[1], v.Normal[2], v.UV[0], v.UV[1],
				mathFloat32frombits(j), w[0], w[1], w[2], w[3])
		}
		for _, i := range me.Indices {
			ib = append(ib, base+i)
		}
		g.parts = append(g.parts, gpuPart{first, int32(len(me.Indices)), me.Texture})
	}
	gl.GenVertexArrays(1, &g.vao)
	gl.BindVertexArray(g.vao)
	gl.GenBuffers(1, &g.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, g.vbo)
	if len(vb) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(vb)*4, gl.Ptr(vb), gl.STATIC_DRAW)
	}
	gl.GenBuffers(1, &g.ebo)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, g.ebo)
	if len(ib) > 0 {
		gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(ib)*4, gl.Ptr(ib), gl.STATIC_DRAW)
	}
	s := int32(stride * 4)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, s, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 3, gl.FLOAT, false, s, 12)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointerWithOffset(2, 2, gl.FLOAT, false, s, 24)
	gl.EnableVertexAttribArray(3)
	gl.VertexAttribIPointerWithOffset(3, 4, gl.UNSIGNED_BYTE, s, 32)
	gl.EnableVertexAttribArray(4)
	gl.VertexAttribPointerWithOffset(4, 4, gl.FLOAT, false, s, 36)
	gl.BindVertexArray(0)

	for _, img := range m.Textures {
		g.textures = append(g.textures, uploadTexture2D(img))
	}
	if len(g.textures) == 0 {
		g.textures = append(g.textures, uploadTexture2D(whiteImage()))
	}
	mr.models = append(mr.models, g)
	return len(mr.models) - 1, nil
}

func whiteImage() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	copy(img.Pix, []byte{255, 255, 255, 255})
	return img
}

func uploadTexture2D(img image.Image) uint32 {
	b := img.Bounds()
	px := toNRGBARect(img)
	var id uint32
	gl.GenTextures(1, &id)
	gl.BindTexture(gl.TEXTURE_2D, id)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(b.Dx()), int32(b.Dy()), 0, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(px.Pix))
	return id
}

// Draw renders model instances. Call after Renderer.Render with the same
// camera. A model faces +Z in model space; Yaw turns it like the camera, so
// an instance with the camera's yaw faces the same way as the camera.
func (mr *ModelRenderer) Draw(cam *Camera, insts []ModelInstance) {
	if len(insts) == 0 {
		return
	}
	p := mr.prog
	p.use()
	vp := cam.ViewProj()
	fog := mr.r.CurrentFog()
	gl.UniformMatrix4fv(p.loc("uViewProj"), 1, false, &vp[0])
	gl.Uniform1i(p.loc("uTex"), 0)
	gl.Uniform1f(p.loc("uDaylight"), mr.r.Opt.Daylight)
	gl.Uniform1f(p.loc("uBlockBright"), mr.r.Opt.BlockBrightness)
	gl.Uniform3fv(p.loc("uFogColor"), 1, &fog.Color[0])
	gl.Uniform2f(p.loc("uFog"), fog.Start, fog.End)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.Enable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE) // mirrored Blockbench cubes flip winding
	ident := [MaxBones]mgl32.Mat4{}
	for i := range ident {
		ident[i] = mgl32.Ident4()
	}
	fr := makeFrustum(vp)
	for _, in := range insts {
		if in.Model < 0 || in.Model >= len(mr.models) {
			continue
		}
		dx, dy, dz := float32(in.X-cam.X), float32(in.Y-cam.Y), float32(in.Z-cam.Z)
		if !fr.box(dx-3, dy-1, dz-3, dx+3, dy+4, dz+3) {
			continue
		}
		g := mr.models[in.Model]
		s := in.Scale
		if s == 0 {
			s = 1
		}
		// Model +Z must turn to the camera's forward at yaw: forward is
		// (sin yaw, 0, -cos yaw), which is +Z rotated by pi - yaw about Y.
		// Pitch then tilts +Z up about the model's X axis.
		if in.Transform != nil {
			gl.Uniform1f(mr.prog.loc("uFade"), in.Fade)
			mr.drawModel(g, mgl32.Translate3D(dx, dy, dz).Mul4(*in.Transform), in.Skin, in.Sky, in.Block, &ident)
			continue
		}
		model := mgl32.Translate3D(dx, dy, dz).Mul4(TiltMatrix(in.Tilt, in.TiltYaw)).Mul4(mgl32.HomogRotate3DY(float32(3.141592653589793 - in.Yaw))).
			Mul4(mgl32.HomogRotate3DX(float32(-in.Pitch))).Mul4(mgl32.Scale3D(s, s, s))
		gl.Uniform1f(mr.prog.loc("uFade"), in.Fade)
		mr.drawModel(g, model, in.Skin, in.Sky, in.Block, &ident)
	}
	gl.BindVertexArray(0)
	gl.Enable(gl.CULL_FACE)
}

func (mr *ModelRenderer) drawModel(g *gpuModel, model mgl32.Mat4, skin []mgl32.Mat4, sky, block uint8, ident *[MaxBones]mgl32.Mat4) {
	p := mr.prog
	gl.UniformMatrix4fv(p.loc("uModel"), 1, false, &model[0])
	if len(skin) > 0 {
		gl.UniformMatrix4fv(p.loc("uBones"), int32(min(len(skin), MaxBones)), false, &skin[0][0])
	} else {
		gl.UniformMatrix4fv(p.loc("uBones"), MaxBones, false, &ident[0][0])
	}
	gl.Uniform2f(p.loc("uLight"), float32(sky)/15, float32(block)/15)
	gl.BindVertexArray(g.vao)
	for _, pt := range g.parts {
		t := 0
		if pt.tex >= 0 && pt.tex < len(g.textures) {
			t = pt.tex
		}
		gl.BindTexture(gl.TEXTURE_2D, g.textures[t])
		gl.DrawElementsWithOffset(gl.TRIANGLES, pt.count, gl.UNSIGNED_INT, uintptr(pt.first)*4)
	}
}

// DrawHand draws a model in first person, over the world (depth is cleared)
// with a fixed 70° projection like ItemRenderer.DrawHand. view places the
// model in view space (camera at the origin looking down -Z, +Y up).
func (mr *ModelRenderer) DrawHand(handle int, aspect float32, view mgl32.Mat4, sky, block uint8) {
	if handle < 0 || handle >= len(mr.models) {
		return
	}
	gl.Clear(gl.DEPTH_BUFFER_BIT)
	p := mr.prog
	p.use()
	proj := mgl32.Perspective(mgl32.DegToRad(70), aspect, 0.05, 10)
	gl.UniformMatrix4fv(p.loc("uViewProj"), 1, false, &proj[0])
	gl.Uniform1i(p.loc("uTex"), 0)
	gl.Uniform1f(p.loc("uDaylight"), mr.r.Opt.Daylight)
	gl.Uniform1f(p.loc("uBlockBright"), mr.r.Opt.BlockBrightness)
	gl.Uniform3f(p.loc("uFogColor"), 0, 0, 0)
	gl.Uniform2f(p.loc("uFog"), 1e6, 2e6) // no fog on the hand
	gl.Uniform1f(p.loc("uFade"), 0)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.Enable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)
	var ident [MaxBones]mgl32.Mat4
	for i := range ident {
		ident[i] = mgl32.Ident4()
	}
	mr.drawModel(mr.models[handle], view, nil, sky, block, &ident)
	gl.BindVertexArray(0)
	gl.Enable(gl.CULL_FACE)
}

func mathFloat32frombits(b uint32) float32 { return math.Float32frombits(b) }

func toNRGBARect(img image.Image) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}
