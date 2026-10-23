package voxel

import (
	"math"
	"strconv"
	"strings"

	"github.com/memmaker/voxellib/voxel/collide"
)

// Rot rotates an element like a Minecraft model's element or blockstate
// rotation: Deg degrees about Axis (0=x 1=y 2=z) through Origin (1/16).
// Positive angles follow Minecraft's element convention (right handed).
// Rescale stretches the two other axes by 1/cos(Deg) about Origin, like an
// element's "rescale": a 45° turned full-width plane spans the block again.
// Shear, after the turn, moves a point by Shear·(Origin.y−y)/16 in x and z:
// Minecraft 1.7's leaning torches (RenderBlocks.renderTorchAtAngle), whose
// quads lean by that much at their foot and not at all at Origin.y.
type Rot struct {
	Axis    int
	Deg     float32
	Origin  [3]float32
	Rescale bool
	Shear   [2]float32
}

// Box is one model element in 1/16 units (Minecraft model pixels). Max may
// exceed 16 (signs, fence collision).
type Box struct {
	Min, Max [3]float32
	// UV per face as u0,v0,u1,v1 in 1/16 of the texture; nil maps the
	// texture from the element's position (Minecraft's default/uvlock UVs).
	UV *[6][4]float32
	// Tex overrides the state's texture per face ("" = the state's); TexID
	// is filled in by the registry.
	Tex   [6]string
	TexID [6]uint16
	Rot   []Rot // applied in order; collision uses the rotated bounds
	Skip  uint8 // faces (1<<Face) not drawn
	// UVRot turns a face's texture clockwise by quarter turns (a model
	// face's "rotation"/90); it needs UV.
	UVRot [6]uint8
	// Cull, when set, is per face the side whose opaque neighbour hides it
	// (a model face's "cullface"; FaceCount: never hidden). Nil culls faces
	// on the block boundary by the neighbour they face.
	Cull *[6]Face
	// NoTint marks faces (1<<Face) drawn without the state's tint (model
	// faces without "tintindex").
	NoTint uint8
	// Flat lights the box by one light value per face instead of smoothly
	// (a model's "ambientocclusion": false; Minecraft 1.7's renderers
	// that don't use renderStandardBlock).
	Flat bool
	// PlantTurn turns the box about the block's vertical axis by the
	// plant's position hash (1.7 renderBlockDoublePlant's sunflower head:
	// cos(hash*0.8)*0.1*pi), after Rot (the model's "plant_turn").
	PlantTurn bool
	// NoShade draws the box without the per-face directional shade (a
	// model element's "shade": false; Minecraft 1.7's renderers that set
	// one tessellator colour for every face, as panes, ladders and vines).
	NoShade bool
}

func bx(x0, y0, z0, x1, y1, z1 float32) Box {
	return Box{Min: [3]float32{x0, y0, z0}, Max: [3]float32{x1, y1, z1}}
}

// RotatePoint applies rots in order to p (1/16 units).
func RotatePoint(p [3]float32, rots []Rot) [3]float32 {
	for _, r := range rots {
		s, c := math.Sincos(float64(r.Deg) * math.Pi / 180)
		sn, cs := float32(s), float32(c)
		a, b := (r.Axis+1)%3, (r.Axis+2)%3 // x: y,z  y: z,x  z: x,y
		u, v := p[a]-r.Origin[a], p[b]-r.Origin[b]
		if r.Rescale && cs > 1e-3 {
			k := 1 / max(cs, -cs)
			u, v = u*k, v*k
		}
		p[a], p[b] = r.Origin[a]+u*cs-v*sn, r.Origin[b]+u*sn+v*cs
		if r.Shear != [2]float32{} {
			d := (r.Origin[1] - p[1]) / 16
			p[0], p[2] = p[0]+r.Shear[0]*d, p[2]+r.Shear[1]*d
		}
	}
	return p
}

// rotY turns every box by a blockstate-style y rotation (Minecraft's y=deg).
func rotY(bs []Box, deg float32) []Box { return RotateBoxes(bs, 1, deg) }

// RotateBoxes appends a blockstate rotation (Minecraft's x=/y=, clockwise seen
// from the positive axis) about the block centre.
func RotateBoxes(bs []Box, axis int, deg float32) []Box {
	if deg == 0 {
		return bs
	}
	out := make([]Box, len(bs))
	rot := Rot{Axis: axis, Deg: -deg, Origin: [3]float32{8, 8, 8}}
	for i, b := range bs {
		b.Rot = append(append([]Rot(nil), b.Rot...), rot)
		if b.Cull != nil { // a cullface turns with the blockstate, not with the element
			c := *b.Cull
			for f, side := range c {
				if side < FaceCount {
					c[f] = RotateFace(side, []Rot{rot})
				}
			}
			b.Cull = &c
		}
		out[i] = b
	}
	return out
}

// LockBoxes is RotateBoxes with uvlock for textures laid out by position
// (Minecraft 1.7 draws every box by RenderBlocks, which projects textures
// in world space): a box without element rotation whose faces all carry
// RenderBlocksUV of their bounds is turned into an axis-aligned box whose
// faces get RenderBlocksUV of the new bounds, each face keeping its
// texture, cullface and tint. Other boxes turn as in RotateBoxes.
func LockBoxes(bs []Box, axis int, deg float32) []Box {
	if deg == 0 {
		return bs
	}
	out := RotateBoxes(bs, axis, deg)
	if int(deg)%90 != 0 {
		return out
	}
	rot := []Rot{{Axis: axis, Deg: -deg, Origin: [3]float32{8, 8, 8}}}
	for i, b := range bs {
		flip, ok := rbDefaults(&b)
		if !ok {
			continue
		}
		p, q := RotatePoint(b.Min, rot), RotatePoint(b.Max, rot)
		n := b
		n.Rot, n.Skip, n.NoTint = nil, 0, 0
		for a := range 3 {
			lo, hi := min(p[a], q[a]), max(p[a], q[a])
			n.Min[a], n.Max[a] = float32(math.Round(float64(lo)*1e4)/1e4), float32(math.Round(float64(hi)*1e4)/1e4)
		}
		var cull [6]Face
		uv := RenderBlocksUV(n.Min, n.Max, flip)
		for f := Face(0); f < FaceCount; f++ {
			g := RotateFace(f, rot)
			n.Tex[g], n.TexID[g] = b.Tex[f], b.TexID[f]
			n.Skip |= (b.Skip >> f & 1) << g
			n.NoTint |= (b.NoTint >> f & 1) << g
			if b.Cull != nil {
				cull[g] = b.Cull[f]
				if cull[g] < FaceCount {
					cull[g] = RotateFace(cull[g], rot)
				}
			}
		}
		if b.Cull != nil {
			n.Cull = &cull
		}
		n.UV = &uv
		out[i] = n
	}
	return out
}

// RenderBlocksUV is the model UV (u0,v0,u1,v1 per face, in the mesher's
// 1.8 JSON mapping) that shows a box's texture as Minecraft 1.7's
// RenderBlocks draws it: the bottom with v along +z, north and east
// mirrored within the box unless flip (RenderBlocks.field_152631_f).
func RenderBlocksUV(lo, hi [3]float32, flip bool) [6][4]float32 {
	x1, y1, z1, x2, y2, z2 := lo[0], lo[1], lo[2], hi[0], hi[1], hi[2]
	uv := [6][4]float32{
		{x1, z2, x2, z1}, {x1, z1, x2, z2},
		{x1, 16 - y2, x2, 16 - y1}, {x1, 16 - y2, x2, 16 - y1},
		{z1, 16 - y2, z2, 16 - y1}, {z1, 16 - y2, z2, 16 - y1},
	}
	if flip {
		uv[FaceNorth][0], uv[FaceNorth][2] = 16-x2, 16-x1
		uv[FaceEast][0], uv[FaceEast][2] = 16-z2, 16-z1
	}
	return uv
}

// rbDefaults reports whether every drawn face of b has RenderBlocksUV of
// its bounds, and with which flip.
func rbDefaults(b *Box) (flip, ok bool) {
	if b.UV == nil || len(b.Rot) > 0 || b.UVRot != [6]uint8{} {
		return false, false
	}
	for _, fl := range [2]bool{false, true} {
		want := RenderBlocksUV(b.Min, b.Max, fl)
		ok = true
		for f := range want {
			for k := range 4 {
				if d := b.UV[f][k] - want[f][k]; b.Skip&(1<<f) == 0 && (d > 1e-3 || d < -1e-3) {
					ok = false
				}
			}
		}
		if ok {
			return fl, true
		}
	}
	return false, false
}

// RotateFace is where face f points after rots, or FaceCount when that's
// not along an axis.
func RotateFace(f Face, rots []Rot) Face {
	n := f.Normal()
	d := [3]float32{float32(n.X), float32(n.Y), float32(n.Z)}
	for _, r := range rots {
		d = RotatePoint(d, []Rot{{Axis: r.Axis, Deg: r.Deg}})
	}
	for g := Face(0); g < FaceCount; g++ {
		gn := g.Normal()
		dx, dy, dz := d[0]-float32(gn.X), d[1]-float32(gn.Y), d[2]-float32(gn.Z)
		if dx*dx+dy*dy+dz*dz < 1e-6 {
			return g
		}
	}
	return FaceCount
}

// thin is a d-pixel plate against the side opposite facing (doors, ladders):
// facing=north hugs the south side, as in Minecraft.
func thin(facing string, d float32) Box {
	switch facing {
	case "south":
		return bx(0, 0, 0, 16, 16, d)
	case "west":
		return bx(16-d, 0, 0, 16, 16, 16)
	case "east":
		return bx(0, 0, 0, d, 16, 16)
	}
	return bx(0, 0, 16-d, 16, 16, 16)
}

var (
	clockwise = map[string]string{"north": "east", "east": "south", "south": "west", "west": "north"}
	ccw       = map[string]string{"north": "west", "west": "south", "south": "east", "east": "north"}
	opposite  = map[string]string{"north": "south", "south": "north", "east": "west", "west": "east"}
	// blockstate y rotations turning a south- (chests, signs) or east-facing
	// (wall torch) model to a facing
	yRotFromSouth = map[string]float32{"south": 0, "west": 90, "north": 180, "east": 270}
	yRotFromEast  = map[string]float32{"east": 0, "south": 90, "west": 180, "north": 270}
	faceOf        = map[string]Face{"north": FaceNorth, "south": FaceSouth, "west": FaceWest, "east": FaceEast}
)

// half returns the horizontal half of a block toward dir, y from y0 to y1.
func half(dir string, y0, y1 float32) Box {
	switch dir {
	case "east":
		return bx(8, y0, 0, 16, y1, 16)
	case "west":
		return bx(0, y0, 0, 8, y1, 16)
	case "south":
		return bx(0, y0, 8, 16, y1, 16)
	}
	return bx(0, y0, 0, 16, y1, 8)
}

// quarter is the corner shared by the halves toward a and b.
func quarter(a, b string, y0, y1 float32) Box {
	h, g := half(a, y0, y1), half(b, y0, y1)
	for i := 0; i < 3; i++ {
		h.Min[i], h.Max[i] = max(h.Min[i], g.Min[i]), min(h.Max[i], g.Max[i])
	}
	return h
}

// shapeVariants returns a ShapeBoxes state's elements. Key "" is the shape on
// its own; other keys are the neighbour-dependent shapes Connected picks
// (stair corners, double chest halves).
func shapeVariants(name string, p map[string]string) map[string][]Box {
	n := name[strings.IndexByte(name, ':')+1:]
	one := func(b ...Box) map[string][]Box { return map[string][]Box{"": b} }
	switch {
	case strings.HasSuffix(n, "_stairs"):
		// StairBlock's shapes: a slab plus the step.
		lo, hi, s0, s1 := float32(0), float32(8), float32(8), float32(16)
		if p["half"] == "top" {
			lo, hi, s0, s1 = 8, 16, 0, 8
		}
		f := p["facing"]
		if _, ok := faceOf[f]; !ok {
			f = "north"
		}
		slab := bx(0, lo, 0, 16, hi, 16)
		return map[string][]Box{
			"":            {slab, half(f, s0, s1)},
			"outer_left":  {slab, quarter(f, ccw[f], s0, s1)},
			"outer_right": {slab, quarter(f, clockwise[f], s0, s1)},
			"inner_left":  {slab, half(f, s0, s1), quarter(opposite[f], ccw[f], s0, s1)},
			"inner_right": {slab, half(f, s0, s1), quarter(opposite[f], clockwise[f], s0, s1)},
		}
	case strings.HasSuffix(n, "_slab"):
		switch p["type"] {
		case "top":
			return one(bx(0, 8, 0, 16, 16, 16))
		case "double":
			return one(bx(0, 0, 0, 16, 16, 16))
		}
		return one(bx(0, 0, 0, 16, 8, 16))
	case strings.HasSuffix(n, "_door"):
		f := p["facing"]
		if p["open"] == "true" {
			if p["hinge"] == "right" {
				f = ccw[f]
			} else {
				f = clockwise[f]
			}
		}
		return one(thin(f, 3))
	case strings.HasSuffix(n, "_fence"):
		return one(bx(6, 0, 6, 10, 16, 10))
	case n == "torch":
		return one(torch(bx(7, 0, 7, 9, 10, 9)))
	case n == "wall_torch":
		// block/template_torch_wall: leans 22.5° away from the wall.
		b := torch(bx(-1, 3.5, 7, 1, 13.5, 9))
		b.Rot = []Rot{{Axis: 2, Deg: -22.5, Origin: [3]float32{0, 3.5, 8}}}
		return one(rotY([]Box{b}, yRotFromEast[p["facing"]])...)
	case n == "ladder":
		return one(thin(p["facing"], 1))
	case n == "snow":
		h := float32(2)
		if l, err := strconv.Atoi(p["layers"]); err == nil && l > 1 && l <= 8 {
			h = float32(2 * l)
		}
		return one(bx(0, 0, 0, 16, h, 16))
	case n == "farmland":
		return one(bx(0, 0, 0, 16, 15, 16))
	case strings.HasSuffix(n, "_pressure_plate"):
		if p["powered"] == "true" {
			return one(bx(1, 0, 1, 15, 0.5, 15))
		}
		return one(bx(1, 0, 1, 15, 1, 15))
	case n == "redstone_wire":
		b := bx(0, 0.25, 0, 16, 0.25, 16)
		b.Skip = ^uint8(1<<FaceUp | 1<<FaceDown)
		return one(b)
	case n == "lever":
		return one(lever(p)...)
	case n == "chest":
		return chest(p["facing"])
	case strings.HasSuffix(n, "_wall_sign"):
		return one(wallSign(p["facing"])...)
	case strings.HasSuffix(n, "_sign"):
		return one(sign(p["rotation"])...)
	}
	return one(bx(0, 0, 0, 16, 16, 16))
}

// torch gives an element block/template_torch's UVs.
func torch(b Box) Box {
	side := [4]float32{7, 6, 9, 16}
	b.UV = &[6][4]float32{{7, 13, 9, 15}, {7, 6, 9, 8}, side, side, side, side}
	return b
}

// lever is block/lever(_on) turned by Minecraft's lever blockstate.
func lever(p map[string]string) []Box {
	base := bx(5, 0, 4, 11, 3, 12)
	base.UV = &[6][4]float32{{5, 4, 11, 12}, {5, 4, 11, 12}, {5, 0, 11, 3}, {5, 0, 11, 3}, {4, 0, 12, 3}, {4, 0, 12, 3}}
	for f := range base.Tex {
		base.Tex[f] = "block/cobblestone"
	}
	h := bx(7, 1, 7, 9, 11, 9)
	side := [4]float32{7, 6, 9, 16}
	h.UV = &[6][4]float32{{}, {7, 6, 9, 8}, side, side, side, side}
	h.Skip = 1 << FaceDown
	angle := float32(-45)
	if p["powered"] == "true" {
		angle = 45
	}
	h.Rot = []Rot{{Axis: 0, Deg: angle, Origin: [3]float32{8, 1, 8}}}
	bs := []Box{base, h}
	f := p["facing"]
	y := map[string]float32{"north": 0, "east": 90, "south": 180, "west": 270}[f]
	switch p["face"] {
	case "wall":
		bs = RotateBoxes(bs, 0, 90)
	case "ceiling":
		bs = RotateBoxes(bs, 0, 180)
		y = map[string]float32{"south": 0, "west": 90, "north": 180, "east": 270}[f]
	}
	return rotY(bs, y)
}

// sheet names a w×h pixel region of a texture sheet sheetW pixels wide as a
// texture of its own (mcres.Pack cuts it out).
func sheet(tex string, sheetW, x, y, w, h int) string {
	return tex + "#" + strconv.Itoa(sheetW) + ":" + strconv.Itoa(x) + "," + strconv.Itoa(y) + "," + strconv.Itoa(w) + "," + strconv.Itoa(h)
}

// entityBox textures b like an entity model cube with texOffs u,v and size
// dx,dy,dz pixels. Chest sheets store the side strips upside down (flipV).
func entityBox(b Box, tex string, sheetW, u, v, dx, dy, dz int, flipV bool) Box {
	r := func(x, y, w, h int) string { return sheet(tex, sheetW, x, y, w, h) }
	b.Tex[FaceDown] = r(u+dz, v, dx, dz)
	b.Tex[FaceUp] = r(u+dz+dx, v, dx, dz)
	b.Tex[FaceWest] = r(u, v+dz, dz, dy)
	b.Tex[FaceNorth] = r(u+dz, v+dz, dx, dy)
	b.Tex[FaceEast] = r(u+dz+dx, v+dz, dz, dy)
	b.Tex[FaceSouth] = r(u+2*dz+dx, v+dz, dx, dy)
	full, side := [4]float32{0, 0, 16, 16}, [4]float32{0, 0, 16, 16}
	if flipV {
		side = [4]float32{0, 16, 16, 0}
	}
	b.UV = &[6][4]float32{full, full, side, side, side, side}
	return b
}

// chest is Minecraft's chest entity model (ChestRenderer) as elements,
// modelled facing south and turned by the facing. "left"/"right" are the
// halves of a double chest whose partner is clockwise/counter-clockwise
// of the facing (ChestType).
func chest(facing string) map[string][]Box {
	if _, ok := faceOf[facing]; !ok {
		facing = "south"
	}
	y := yRotFromSouth[facing]
	const t = "entity/chest/normal"
	single := []Box{
		entityBox(bx(1, 0, 1, 15, 10, 15), t, 64, 0, 19, 14, 10, 14, true),
		entityBox(bx(1, 9, 1, 15, 14, 15), t, 64, 0, 0, 14, 5, 14, true),
		entityBox(bx(7, 7, 15, 9, 11, 16), t, 64, 0, 0, 2, 4, 1, false),
	}
	left := []Box{ // partner at -x (west of a south-facing chest)
		entityBox(bx(0, 0, 1, 15, 10, 15), t+"_left", 64, 0, 19, 15, 10, 14, true),
		entityBox(bx(0, 9, 1, 15, 14, 15), t+"_left", 64, 0, 0, 15, 5, 14, true),
		entityBox(bx(0, 7, 15, 1, 11, 16), t+"_left", 64, 0, 0, 1, 4, 1, false),
	}
	right := []Box{
		entityBox(bx(1, 0, 1, 16, 10, 15), t+"_right", 64, 0, 19, 15, 10, 14, true),
		entityBox(bx(1, 9, 1, 16, 14, 15), t+"_right", 64, 0, 0, 15, 5, 14, true),
		entityBox(bx(15, 7, 15, 16, 11, 16), t+"_right", 64, 0, 0, 1, 4, 1, false),
	}
	return map[string][]Box{"": rotY(single, y), "left": rotY(left, y), "right": rotY(right, y)}
}

// sign is a standing sign (SignRenderer: the 24×12 board and post scaled by
// 2/3), turned by rotation in 22.5° steps.
func sign(rotation string) []Box {
	const t, s = "entity/signs/oak", 64
	board := entityBox(bx(0, 28.0/3, 22.0/3, 16, 52.0/3, 26.0/3), t, s, 0, 0, 24, 12, 2, false)
	post := entityBox(bx(22.0/3, 0, 22.0/3, 26.0/3, 28.0/3, 26.0/3), t, s, 0, 14, 2, 14, 2, false)
	// the sheet's "north" strip is the front, facing south at rotation 0
	board.Tex[FaceNorth], board.Tex[FaceSouth] = board.Tex[FaceSouth], board.Tex[FaceNorth]
	r, _ := strconv.Atoi(rotation)
	return rotY([]Box{board, post}, float32(r)*22.5)
}

// wallSign is the sign board alone, 5px lower and pushed 7px back against
// the wall it hangs on (SignRenderer's wall offset), front facing facing.
func wallSign(facing string) []Box {
	board := entityBox(bx(0, 13.0/3, 1.0/3, 16, 37.0/3, 5.0/3), "entity/signs/oak", 64, 0, 0, 24, 12, 2, false)
	board.Tex[FaceNorth], board.Tex[FaceSouth] = board.Tex[FaceSouth], board.Tex[FaceNorth]
	return rotY([]Box{board}, SignYaw(facing, ""))
}

// SignYaw is a sign's turn in degrees from facing south: from a wall sign's
// facing, else from a standing sign's 16-step rotation.
func SignYaw(facing, rotation string) float32 {
	switch facing {
	case "west":
		return 90
	case "north":
		return 180
	case "east":
		return 270
	case "south":
		return 0
	}
	r, _ := strconv.Atoi(rotation)
	return float32(r) * 22.5
}

// prop reads one property from a canonical "k=v,k=v" string.
func prop(props, key string) string {
	for _, kv := range strings.Split(props, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}

// Nb gives the state at offset d from a block.
type Nb func(d [3]int) *StateInfo

// At is the neighbour on face f.
func (nb Nb) At(f Face) *StateInfo { return nb(f.Off()) }

// Connected returns a state's elements in context, as Minecraft works them
// out from the neighbours (nb.At(f) is the state beside it on face f): fence
// arms, stair corners, double chests.
func Connected(info *StateInfo, nb Nb) []Box {
	switch {
	case len(info.Parts) > 0:
		return info.partBoxes(nb, false)
	case info.Connects:
		out := append([]Box(nil), info.Boxes...)
		for f := FaceNorth; f <= FaceEast; f++ {
			o := nb.At(f)
			if !o.Opaque && !o.Connects {
				continue
			}
			for _, y := range [2][2]float32{{6, 9}, {12, 15}} {
				b := bx(7, y[0], 7, 9, y[1], 9)
				switch f {
				case FaceNorth:
					b.Min[2] = 0
				case FaceSouth:
					b.Max[2] = 16
				case FaceWest:
					b.Min[0] = 0
				case FaceEast:
					b.Max[0] = 16
				}
				out = append(out, b)
			}
		}
		return out
	case len(info.Variants) < 2:
		return info.Boxes
	case info.Name == "minecraft:chest":
		f := chestFacing(info)
		same := func(dir string) bool {
			o := nb.At(faceOf[dir])
			return o.Def == info.Def && chestFacing(o) == f
		}
		if same(clockwise[f]) {
			return info.Variants["left"]
		}
		if same(ccw[f]) {
			return info.Variants["right"]
		}
		return info.Boxes
	}
	return info.Variants[stairShape(info, nb)]
}

func chestFacing(info *StateInfo) string {
	if f := prop(info.Props, "facing"); faceOf[f] != 0 {
		return f
	}
	return "south"
}

// stairShape is StairBlock.getStairsShape.
func stairShape(info *StateInfo, nb Nb) string {
	f, h := prop(info.Props, "facing"), prop(info.Props, "half")
	if _, ok := faceOf[f]; !ok {
		return ""
	}
	isStairs := func(o *StateInfo) bool { return strings.HasSuffix(o.Name, "_stairs") }
	ns := func(dir string) bool { return dir == "north" || dir == "south" }
	canTake := func(dir string) bool {
		o := nb.At(faceOf[dir])
		return !isStairs(o) || prop(o.Props, "facing") != f || prop(o.Props, "half") != h
	}
	if o := nb.At(faceOf[f]); isStairs(o) && prop(o.Props, "half") == h {
		of := prop(o.Props, "facing")
		if _, ok := faceOf[of]; ok && ns(of) != ns(f) && canTake(opposite[of]) {
			if of == ccw[f] {
				return "outer_left"
			}
			return "outer_right"
		}
	}
	if o := nb.At(faceOf[opposite[f]]); isStairs(o) && prop(o.Props, "half") == h {
		of := prop(o.Props, "facing")
		if _, ok := faceOf[of]; ok && ns(of) != ns(f) && canTake(of) {
			if of == ccw[f] {
				return "inner_left"
			}
			return "inner_right"
		}
	}
	return ""
}

// partBoxes gathers the elements (or, with collision, the collision boxes)
// of a block with Parts in context.
func (info *StateInfo) partBoxes(nb Nb, collision bool) []Box {
	out := info.Boxes
	if collision && info.Collision != nil {
		out = info.Collision
	}
	out = append([]Box(nil), out...)
	var done []string
	var flags uint32
	for i := range info.Parts {
		p := &info.Parts[i]
		if p.Group != "" && slicesContains(done, p.Group) || !holds(p.When, nb, flags) {
			continue
		}
		if p.set != 0 {
			flags |= p.set
			continue
		}
		if p.Group != "" {
			done = append(done, p.Group)
		}
		bs := p.Boxes
		if collision && p.Collision != nil {
			bs = p.Collision
		}
		if p.Replace {
			out = append(out[:0:0], bs...)
		} else {
			out = append(out, bs...)
		}
	}
	return out
}

func slicesContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// FaceTex is the texture of cube face f with the neighbours nb: Tex[f]
// unless a Retex entry holds.
func (info *StateInfo) FaceTex(f Face, nb Nb) uint16 {
	for i := range info.Retex {
		x := &info.Retex[i]
		if x.Tex[f] != "" && holds(x.When, nb, 0) {
			return x.ids[f]
		}
	}
	return info.Tex[f]
}

func holds(cs []Cond, nb Nb, flags uint32) bool {
	for _, c := range cs {
		ok := false
		if c.Flag {
			for _, t := range c.Any {
				if (flags>>t.id&1 != 0) != t.Not {
					ok = true
					break
				}
			}
			if !ok {
				return false
			}
			continue
		}
		d := c.Face.Off()
		d[1] += int(c.Dy)
		o := nb(d)
		for _, t := range c.Any {
			if o.hasTag(t.id) != t.Not {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func (info *StateInfo) hasTag(id uint16) bool {
	switch id {
	case tagAir:
		return info.Shape == ShapeNone && info.Name == "minecraft:air"
	case tagOpaque:
		return info.Opaque
	}
	for _, t := range info.Tags {
		if t == id {
			return true
		}
	}
	return false
}

// Boxes implements collide.Boxer for ShapeBoxes blocks overlapping area
// (fences collide 1.5 blocks tall, as in Minecraft).
func (w *World) Boxes(area collide.AABB, out []collide.AABB) []collide.AABB {
	area.Grow(0, 0.5, 0).Blocks(func(x, y, z int) {
		info := w.Reg.Info(w.Block(x, y, z))
		boxes := info.Collision
		if len(info.Parts) > 0 {
			boxes = info.partBoxes(func(d [3]int) *StateInfo {
				return w.Reg.Info(w.Block(x+d[0], y+d[1], z+d[2]))
			}, true)
		} else if boxes == nil {
			if info.Shape != ShapeBoxes || !collidable(info.Name) {
				return
			}
			nb := func(d [3]int) *StateInfo {
				return w.Reg.Info(w.Block(x+d[0], y+d[1], z+d[2]))
			}
			boxes = Connected(info, nb)
		}
		for _, b := range boxes {
			if len(b.Rot) > 0 {
				b = axisAligned(b)
			}
			top := float64(b.Max[1])
			if info.Connects {
				top = 24
			}
			bb := collide.AABB{
				MinX: float64(x) + float64(b.Min[0])/16, MinY: float64(y) + float64(b.Min[1])/16, MinZ: float64(z) + float64(b.Min[2])/16,
				MaxX: float64(x) + float64(b.Max[0])/16, MaxY: float64(y) + top/16, MaxZ: float64(z) + float64(b.Max[2])/16,
			}
			if bb.Intersects(area) {
				out = append(out, bb)
			}
		}
	})
	return out
}

// axisAligned is the bounding box of a rotated element.
func axisAligned(b Box) Box {
	lo := [3]float32{1e9, 1e9, 1e9}
	hi := [3]float32{-1e9, -1e9, -1e9}
	for c := 0; c < 8; c++ {
		p := b.Min
		for a := 0; a < 3; a++ {
			if c>>a&1 == 1 {
				p[a] = b.Max[a]
			}
		}
		p = RotatePoint(p, b.Rot)
		for a := 0; a < 3; a++ {
			lo[a], hi[a] = min(lo[a], p[a]), max(hi[a], p[a])
		}
	}
	return Box{Min: lo, Max: hi}
}

// collidable: torches, plates, wire, signs, levers and thin snow are walked
// through, as in Minecraft.
func collidable(name string) bool {
	n := name[strings.IndexByte(name, ':')+1:]
	switch {
	case strings.Contains(n, "torch"), strings.HasSuffix(n, "_pressure_plate"), strings.HasSuffix(n, "_sign"),
		n == "redstone_wire", n == "snow", n == "lever":
		return false
	}
	return true
}
