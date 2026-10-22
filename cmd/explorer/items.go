package main

import (
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/memmaker/voxellib/items"
	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/render"
)

// itemsState is the explorer's item glue: sprites → render.ItemRenderer,
// drops → items.World, one held item instead of an inventory.
type itemsState struct {
	itemNames []string
	itemImgs  []image.Image // pending upload
	itemRend  *render.ItemRenderer
	drops     items.World
	held      string // item in hand, "" = empty
	itemsLeft int    // items still to scatter around the spawn
	// packFrom is where asset-pack items start in itemNames (0 = none):
	// the scatter alternates between them and the sprite folder, so a small
	// pack isn't drowned out by a big folder.
	packFrom int
	// unscattered items are never scattered (e.g. arrow pickups).
	unscattered []string
	itemRand    *rand.Rand
	instBuf     []render.ItemInstance
}

// maxSprite is the largest item sprite loaded.
const maxSprite = 64

// loadItems reads every sprite PNG in dir (as written by cmd/itemsheet).
func (g *game) loadItems(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.png"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".png")
		if name == "sheet" {
			continue
		}
		r, err := os.Open(f)
		if err != nil {
			return err
		}
		img, err := png.Decode(r)
		r.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		if b := img.Bounds(); b.Dx() > maxSprite || b.Dy() > maxSprite {
			// One huge sprite would size every layer of the item texture
			// array to it (voodoo_03.png is 3712px: the GPU refuses).
			log.Printf("items: skipping %s (%dx%d, max %d)", f, b.Dx(), b.Dy(), maxSprite)
			continue
		}
		g.addItem(name, img)
	}
	if len(files) == 0 {
		return fmt.Errorf("no sprites in %s (run: go run ./cmd/itemsheet)", dir)
	}
	return nil
}

// addItem registers an item sprite; call finishItems once all are added.
func (g *game) addItem(name string, img image.Image) {
	g.itemNames = append(g.itemNames, name)
	g.itemImgs = append(g.itemImgs, img)
}

// finishItems uploads the registered sprites.
func (g *game) finishItems() {
	if len(g.itemImgs) == 0 {
		return
	}
	var err error
	g.itemRend, err = render.NewItemRenderer(g.Eng.Renderer, g.itemNames, g.itemImgs)
	if err != nil {
		log.Printf("items disabled: %v", err)
		return
	}
	if !g.itemRend.Has(g.held) {
		g.held = ""
	}
	g.itemRand = rand.New(rand.NewSource(int64(len(g.itemImgs))))
	g.itemImgs = nil
}

// tickItems scatters items once terrain exists, steps drop physics and
// applies the pickup rule.
func (g *game) tickItems() {
	if g.itemRend == nil || !g.Ready() {
		return
	}
	p := g.Player
	for tries := 0; g.itemsLeft > 0 && tries < 8; tries++ {
		a := g.itemRand.Float64() * 2 * math.Pi
		r := 3 + g.itemRand.Float64()*14
		x, z := p.X+math.Cos(a)*r, p.Z+math.Sin(a)*r
		if !g.Eng.ChunkReady(voxel.ChunkPos{X: int(math.Floor(x)) >> 4, Z: int(math.Floor(z)) >> 4}) {
			continue
		}
		y, ok := g.Eng.SurfaceY(int(math.Floor(x)), int(math.Floor(z)))
		if !ok {
			continue
		}
		lo, hi := 0, len(g.itemNames)
		if g.packFrom > 0 && g.packFrom < hi {
			if g.itemsLeft%2 == 0 {
				lo = g.packFrom
			} else {
				hi = g.packFrom
			}
		}
		name := g.itemNames[lo+g.itemRand.Intn(hi-lo)]
		if slices.Contains(g.unscattered, name) {
			// Pick from everything instead, so a range holding only
			// unscattered items can't stall the scatter.
			name = g.itemNames[g.itemRand.Intn(len(g.itemNames))]
			if slices.Contains(g.unscattered, name) {
				if len(g.unscattered) >= len(g.itemNames) {
					g.itemsLeft = 0
				}
				continue
			}
		}
		g.drops.Spawn(name, x, float64(y)+0.5, z)
		g.itemsLeft--
	}
	g.drops.Tick(g.Terrain)
	if !p.NoClip {
		g.drops.Collect(p.Box(), &g.held, p.X, p.Y, p.Z)
	}
}

// dropHeld throws the held item forward (Q).
func (g *game) dropHeld() {
	if g.held == "" {
		return
	}
	x, y, z := g.Player.Eye()
	g.drops.Toss(g.held, x, y, z, g.Cam.Yaw, g.Cam.Pitch)
	g.held = ""
}

func (g *game) light(x, y, z float64) (uint8, uint8) {
	return g.Eng.World.Light(int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z)))
}

// drawItems draws drops in the world and the held item in first person.
func (g *game) drawItems(alpha float64) {
	if g.itemRend == nil {
		return
	}
	g.instBuf = g.instBuf[:0]
	for _, d := range g.drops.Drops {
		x, y, z, spin := d.Pose(alpha)
		sky, blk := g.Light(x, y, z)
		g.instBuf = append(g.instBuf, render.ItemInstance{Item: d.Item, X: x, Y: y, Z: z, Yaw: spin, Scale: 0.5, Sky: sky, Block: blk})
	}
	g.itemRend.DrawWorld(&g.Cam, g.instBuf)
	if g.crossbowOn() {
		g.drawCrossbowHand(alpha)
	} else if g.held != "" {
		sky, blk := g.Light(g.Cam.X, g.Cam.Y, g.Cam.Z)
		g.itemRend.DrawHand(g.held, float32(g.Cam.Aspect), g.Player.HandPose(alpha), sky, blk)
	}
}
