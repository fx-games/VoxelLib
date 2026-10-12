// Package sheet cuts item sprites out of a sprite sheet laid out on a grid
// of cells, such as Ironchest Games' CC0 "RPG Items" pico-8 sheet.
//
// The cutter needs no hand-made coordinates. It labels the 8-connected
// opaque pixel components, ignores thin divider lines, and accepts a grid
// cell as a sprite when every component touching it lies inside the cell
// and both horizontal neighbour cells are empty. Label text fails the
// second test because its letters sit next to each other.
package sheet

import (
	"fmt"
	"image"
	"image/draw"
)

// Sprite is one cut item.
type Sprite struct {
	Name  string          // "<section>_NN", or "item_NN" outside any section
	Rect  image.Rectangle // cell in the source sheet
	Image *image.NRGBA    // the cell's pixels
}

// Section names a rectangle of the sheet; sprites inside it are named after it.
type Section struct {
	Name string
	Rect image.Rectangle
}

// Options control cutting.
type Options struct {
	Cell     int               // grid cell size in pixels (8 for pico-8 sheets)
	Exclude  []image.Rectangle // areas to skip (e.g. showcase duplicates)
	Sections []Section
}

// Cut finds every sprite in img, in reading order.
func Cut(img image.Image, o Options) []Sprite {
	b := img.Bounds()
	src := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)
	W, H := b.Dx(), b.Dy()
	opaque := func(x, y int) bool { return src.Pix[y*src.Stride+x*4+3] != 0 }

	// Label 8-connected components and record their bounds.
	lab := make([]int32, W*H)
	for i := range lab {
		lab[i] = -1
	}
	var comps []image.Rectangle
	var stack []int
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			if !opaque(x, y) || lab[y*W+x] >= 0 {
				continue
			}
			id := int32(len(comps))
			r := image.Rect(x, y, x+1, y+1)
			lab[y*W+x] = id
			stack = append(stack[:0], y*W+x)
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				cx, cy := p%W, p/W
				r = r.Union(image.Rect(cx, cy, cx+1, cy+1))
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						nx, ny := cx+dx, cy+dy
						if nx >= 0 && ny >= 0 && nx < W && ny < H && opaque(nx, ny) && lab[ny*W+nx] < 0 {
							lab[ny*W+nx] = id
							stack = append(stack, ny*W+nx)
						}
					}
				}
			}
			comps = append(comps, r)
		}
	}
	// Divider lines: one pixel thick and at least a cell long.
	line := make([]bool, len(comps))
	for i, r := range comps {
		line[i] = min(r.Dx(), r.Dy()) == 1 && max(r.Dx(), r.Dy()) >= o.Cell
	}

	C := o.Cell
	ids := func(cx, cy int) []int32 {
		if cx < 0 || cy < 0 || (cx+1)*C > W || (cy+1)*C > H {
			return nil
		}
		var out []int32
		for y := cy * C; y < cy*C+C; y++ {
			for x := cx * C; x < cx*C+C; x++ {
				if id := lab[y*W+x]; id >= 0 && !line[id] {
					out = append(out, id)
				}
			}
		}
		return out
	}

	var out []Sprite
	count := map[string]int{}
	for cy := 0; cy*C+C <= H; cy++ {
		for cx := 0; cx*C+C <= W; cx++ {
			cell := image.Rect(cx*C, cy*C, cx*C+C, cy*C+C)
			s := ids(cx, cy)
			if len(s) == 0 || len(ids(cx-1, cy)) > 0 || len(ids(cx+1, cy)) > 0 {
				continue
			}
			inside := true
			for _, id := range s {
				if !comps[id].In(cell) {
					inside = false
					break
				}
			}
			if !inside || excluded(cell.Min, o.Exclude) {
				continue
			}
			name := "item"
			for _, sec := range o.Sections {
				if cell.Min.In(sec.Rect) {
					name = sec.Name
					break
				}
			}
			count[name]++
			px := image.NewNRGBA(image.Rect(0, 0, C, C))
			draw.Draw(px, px.Bounds(), src, cell.Min, draw.Src)
			out = append(out, Sprite{Name: fmt.Sprintf("%s_%02d", name, count[name]), Rect: cell, Image: px})
		}
	}
	return out
}

func excluded(p image.Point, ex []image.Rectangle) bool {
	for _, r := range ex {
		if p.In(r) {
			return true
		}
	}
	return false
}

func rect(x0, x1, y0, y1 int) image.Rectangle { return image.Rect(x0, y0, x1, y1) }

// Ironchest describes Ironchest Games' "RPG Items" pico-8 sheet
// (Ironchests_RPG_items_pico8_transparentbg_v1.png, 540x800, CC0).
var Ironchest = Options{
	Cell:    8,
	Exclude: []image.Rectangle{rect(0, 135, 112, 245)}, // showcase: duplicates
	Sections: []Section{
		{"armor", rect(150, 300, 100, 180)}, {"boots", rect(310, 410, 100, 170)},
		{"gloves", rect(310, 410, 170, 220)}, {"helmet", rect(420, 540, 100, 345)},
		{"shield", rect(180, 410, 220, 365)}, {"sword", rect(0, 175, 250, 370)},
		{"axe", rect(0, 130, 380, 480)}, {"mace", rect(130, 245, 380, 480)},
		{"spear", rect(245, 340, 380, 480)}, {"bow", rect(340, 465, 380, 480)},
		{"crown", rect(465, 540, 360, 480)},
		{"robe", rect(0, 100, 490, 600)}, {"hood", rect(100, 195, 490, 600)},
		{"staff", rect(195, 290, 490, 600)}, {"orb", rect(290, 370, 490, 570)},
		{"voodoo", rect(290, 370, 570, 600)}, {"scroll", rect(370, 465, 490, 600)},
		{"chest", rect(465, 540, 490, 600)},
		{"potion", rect(0, 145, 610, 800)}, {"amulet", rect(145, 260, 610, 705)},
		{"ring", rect(145, 260, 705, 800)}, {"valuable", rect(260, 385, 610, 705)},
		{"skull", rect(260, 385, 705, 800)}, {"misc", rect(385, 540, 610, 800)},
	},
}
