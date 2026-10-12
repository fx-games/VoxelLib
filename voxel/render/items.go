package render

import (
	"image"
	"math"
	"unsafe"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/items/extrude"
)

// ItemRenderer draws 2D item sprites extruded into 3D slabs (items/extrude):
// items lying in the world and the item held in first person. All sprites
// share one texture array and one vertex buffer; each item is a range.
type ItemRenderer struct {
	r      *Renderer
	prog   *program
	vao    uint32
	vbo    uint32
	tex    uint32
	index  map[string]int
	ranges [][2]int32 // first, count per item
}

// ItemInstance is one item to draw in the world.
type ItemInstance struct {
	Item       string
	X, Y, Z    float64 // centre
	Yaw        float64 // spin around Y, radians
	Scale      float32 // edge length in blocks (Minecraft drops: 0.5)
	Sky, Block uint8   // light levels 0..15 at the item
}

const itemVS = `#version 330 core
layout(location = 0) in vec3 aPos;
layout(location = 1) in vec3 aUV;
layout(location = 2) in vec3 aNormal;
uniform mat4 uMVP;
uniform mat4 uModel; // model rotation+scale (camera-relative world space)
out vec3 vUV;
out float vShade;
out float vDist;
void main() {
	vec4 wp = uModel * vec4(aPos, 1.0);
	gl_Position = uMVP * vec4(aPos, 1.0);
	vUV = aUV;
	vec3 n = normalize(mat3(uModel) * aNormal);
	// Minecraft-like fixed directional shading: top bright, bottom dark.
	vShade = 0.7 + 0.3 * n.y + 0.1 * abs(n.x);
	vDist = length(wp.xz);
}
`

const itemFS = `#version 330 core
in vec3 vUV;
in float vShade;
in float vDist;
uniform sampler2DArray uTex;
uniform float uLayer;
uniform vec2 uLight; // sky, block in 0..1
uniform float uDaylight;
uniform vec3 uFogColor;
uniform vec2 uFog;
out vec4 fragColor;
float curve(float l) { return mix(0.04, 1.0, l / (4.0 - 3.0 * l)); }
void main() {
	vec4 c = texture(uTex, vec3(vUV.xy, uLayer));
	if (c.a < 0.5) discard;
	float sky = curve(uLight.x) * uDaylight;
	float blk = curve(uLight.y);
	vec3 light = max(max(vec3(sky) * vec3(0.95, 0.97, 1.0), vec3(blk) * vec3(1.0, 0.86, 0.66)), vec3(0.04));
	vec3 col = c.rgb * vShade * light;
	float f = clamp((vDist - uFog.x) / (uFog.y - uFog.x), 0.0, 1.0);
	fragColor = vec4(mix(col, uFogColor, f * f), 1.0);
}
`

// NewItemRenderer extrudes every sprite and uploads them. names[i] is the
// item ID of imgs[i]. Sprites should share one size; others are rescaled.
func NewItemRenderer(r *Renderer, names []string, imgs []image.Image) (*ItemRenderer, error) {
	prog, err := newProgram(itemVS, itemFS)
	if err != nil {
		return nil, err
	}
	ir := &ItemRenderer{r: r, prog: prog, index: map[string]int{}}
	size := 8
	for _, img := range imgs {
		size = max(size, img.Bounds().Dx(), img.Bounds().Dy())
	}
	var verts []float32
	for i, img := range imgs {
		ir.index[names[i]] = i
		m := extrude.Extrude(img)
		ir.ranges = append(ir.ranges, [2]int32{int32(len(verts) / 8), int32(len(m.Verts))})
		for _, v := range m.Verts {
			verts = append(verts, v.Pos[0], v.Pos[1], v.Pos[2], v.UV[0], v.UV[1], v.Normal[0], v.Normal[1], v.Normal[2])
		}
	}
	gl.GenVertexArrays(1, &ir.vao)
	gl.BindVertexArray(ir.vao)
	gl.GenBuffers(1, &ir.vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, ir.vbo)
	if len(verts) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(verts)*4, gl.Ptr(verts), gl.STATIC_DRAW)
	}
	stride := int32(8 * 4)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointerWithOffset(0, 3, gl.FLOAT, false, stride, 0)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointerWithOffset(1, 2, gl.FLOAT, false, stride, 3*4)
	gl.EnableVertexAttribArray(2)
	gl.VertexAttribPointerWithOffset(2, 3, gl.FLOAT, false, stride, 5*4)
	gl.BindVertexArray(0)

	gl.GenTextures(1, &ir.tex)
	gl.BindTexture(gl.TEXTURE_2D_ARRAY, ir.tex)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D_ARRAY, gl.TEXTURE_MAX_LEVEL, 0)
	gl.TexImage3D(gl.TEXTURE_2D_ARRAY, 0, gl.RGBA8, int32(size), int32(size), int32(max(len(imgs), 1)), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	for i, img := range imgs {
		px := toNRGBA(img, size).Pix
		gl.TexSubImage3D(gl.TEXTURE_2D_ARRAY, 0, 0, 0, int32(i), int32(size), int32(size), 1, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&px[0]))
	}
	return ir, nil
}

// Has reports whether an item ID is known.
func (ir *ItemRenderer) Has(item string) bool { _, ok := ir.index[item]; return ok }

func (ir *ItemRenderer) begin(fog Fog) {
	p := ir.prog
	p.use()
	gl.Uniform1i(p.loc("uTex"), 0)
	gl.Uniform1f(p.loc("uDaylight"), ir.r.Opt.Daylight)
	gl.Uniform3fv(p.loc("uFogColor"), 1, &fog.Color[0])
	gl.Uniform2f(p.loc("uFog"), fog.Start, fog.End)
	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D_ARRAY, ir.tex)
	gl.BindVertexArray(ir.vao)
	gl.Enable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE) // slabs are thin; cheap to draw both sides
}

func (ir *ItemRenderer) draw(item string, vp, model mgl32.Mat4, sky, block uint8) {
	i, ok := ir.index[item]
	if !ok {
		return
	}
	p := ir.prog
	mvp := vp.Mul4(model)
	gl.UniformMatrix4fv(p.loc("uMVP"), 1, false, &mvp[0])
	gl.UniformMatrix4fv(p.loc("uModel"), 1, false, &model[0])
	gl.Uniform1f(p.loc("uLayer"), float32(i))
	gl.Uniform2f(p.loc("uLight"), float32(sky)/15, float32(block)/15)
	gl.DrawArrays(gl.TRIANGLES, ir.ranges[i][0], ir.ranges[i][1])
}

func (ir *ItemRenderer) end() {
	gl.BindVertexArray(0)
	gl.Enable(gl.CULL_FACE)
}

// DrawWorld draws items lying in the world. Call after Renderer.Render with
// the same camera.
func (ir *ItemRenderer) DrawWorld(cam *Camera, items []ItemInstance) {
	if len(items) == 0 {
		return
	}
	vp := cam.ViewProj()
	ir.begin(ir.r.CurrentFog())
	for _, it := range items {
		s := it.Scale
		if s == 0 {
			s = 0.5
		}
		model := mgl32.Translate3D(float32(it.X-cam.X), float32(it.Y-cam.Y), float32(it.Z-cam.Z)).
			Mul4(mgl32.HomogRotate3DY(float32(-it.Yaw))).
			Mul4(mgl32.Scale3D(s, s, s))
		ir.draw(it.Item, vp, model, it.Sky, it.Block)
	}
	ir.end()
}

// HandPose describes the first-person held item's motion.
type HandPose struct {
	BobX, BobY float32 // view bobbing offset (walking), in view units
	Swing      float32 // 0..1 swing progress (breaking/placing); 0 = idle
}

// DrawHand draws the held item in the lower right of the view, over the
// world (depth is cleared) with its own fixed 70° projection, like
// Minecraft's first-person item. aspect is the viewport's width/height.
func (ir *ItemRenderer) DrawHand(item string, aspect float32, pose HandPose, sky, block uint8) {
	if _, ok := ir.index[item]; !ok {
		return
	}
	gl.Clear(gl.DEPTH_BUFFER_BIT)
	proj := mgl32.Perspective(mgl32.DegToRad(70), aspect, 0.05, 10)
	sw := float32(math.Sin(float64(pose.Swing) * math.Pi))
	sw2 := float32(math.Sin(math.Sqrt(float64(pose.Swing)) * math.Pi))
	model := mgl32.Translate3D(0.5+pose.BobX-0.25*sw2, -0.38+pose.BobY+0.15*sw2, -0.9-0.15*sw).
		Mul4(mgl32.HomogRotate3DY(mgl32.DegToRad(-70 + 20*sw))).
		Mul4(mgl32.HomogRotate3DZ(mgl32.DegToRad(20))).
		Mul4(mgl32.HomogRotate3DX(mgl32.DegToRad(-80 * sw2))).
		Mul4(mgl32.Scale3D(0.42, 0.42, 0.42))
	ir.begin(Fog{Color: mgl32.Vec3{}, Start: 1e6, End: 2e6}) // no fog on the hand
	ir.draw(item, proj, model, sky, block)
	ir.end()
}

// Delete frees GL resources.
func (ir *ItemRenderer) Delete() {
	gl.DeleteBuffers(1, &ir.vbo)
	gl.DeleteVertexArrays(1, &ir.vao)
	gl.DeleteTextures(1, &ir.tex)
	gl.DeleteProgram(ir.prog.id)
}
