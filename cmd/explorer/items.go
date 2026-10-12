package main

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
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
	itemRend  *render.ItemRenderer
	drops     items.World
	held      string // item in hand, "" = empty
	itemsLeft int    // items still to scatter around the spawn
	itemRand  *rand.Rand
	instBuf   []render.ItemInstance
}

// loadItems reads every sprite PNG in dir (as written by cmd/itemsheet).
func (g *game) loadItems(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.png"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	var imgs []image.Image
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
		g.itemNames = append(g.itemNames, name)
		imgs = append(imgs, img)
	}
	if len(imgs) == 0 {
		return fmt.Errorf("no sprites in %s (run: go run ./cmd/itemsheet)", dir)
	}
	g.itemRend, err = render.NewItemRenderer(g.eng.Renderer, g.itemNames, imgs)
	if err != nil {
		return err
	}
	if !g.itemRend.Has(g.held) {
		g.held = ""
	}
	g.itemRand = rand.New(rand.NewSource(int64(len(imgs))))
	return nil
}

// tickItems scatters items once terrain exists, steps drop physics and
// applies the pickup rule.
func (g *game) tickItems() {
	if g.itemRend == nil || g.frozen || g.findGround {
		return
	}
	p := g.player
	for tries := 0; g.itemsLeft > 0 && tries < 8; tries++ {
		a := g.itemRand.Float64() * 2 * math.Pi
		r := 3 + g.itemRand.Float64()*14
		x, z := p.X+math.Cos(a)*r, p.Z+math.Sin(a)*r
		if !g.eng.ChunkReady(voxel.ChunkPos{X: int(math.Floor(x)) >> 4, Z: int(math.Floor(z)) >> 4}) {
			continue
		}
		y, ok := g.eng.SurfaceY(int(math.Floor(x)), int(math.Floor(z)))
		if !ok {
			continue
		}
		g.drops.Spawn(g.itemNames[g.itemRand.Intn(len(g.itemNames))], x, float64(y)+0.5, z)
		g.itemsLeft--
	}
	g.drops.Tick(g.terrain)
	if !p.NoClip {
		g.drops.Collect(p.Box(), &g.held, p.X, p.Y, p.Z)
	}
}

// dropHeld throws the held item forward (Q).
func (g *game) dropHeld() {
	if g.held == "" {
		return
	}
	x, y, z := g.player.Eye()
	g.drops.Toss(g.held, x, y, z, g.cam.Yaw, g.cam.Pitch)
	g.held = ""
}

func (g *game) light(x, y, z float64) (uint8, uint8) {
	return g.eng.World.Light(int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z)))
}

// drawItems draws drops in the world and the held item in first person.
func (g *game) drawItems(alpha float64) {
	if g.itemRend == nil {
		return
	}
	g.instBuf = g.instBuf[:0]
	for _, d := range g.drops.Drops {
		x, y, z, spin := d.Pose(alpha)
		sky, blk := g.light(x, y, z)
		g.instBuf = append(g.instBuf, render.ItemInstance{Item: d.Item, X: x, Y: y, Z: z, Yaw: spin, Scale: 0.5, Sky: sky, Block: blk})
	}
	g.itemRend.DrawWorld(&g.cam, g.instBuf)
	if g.held != "" {
		sky, blk := g.light(g.cam.X, g.cam.Y, g.cam.Z)
		t := g.walkDist * 1.6
		pose := render.HandPose{
			BobX:  float32(math.Sin(t)) * 0.02,
			BobY:  -float32(math.Abs(math.Cos(t))) * 0.03,
			Swing: 1 - (float32(g.swing)-float32(alpha))/6,
		}
		if g.swing == 0 {
			pose.Swing = 0
		}
		g.itemRend.DrawHand(g.held, float32(g.cam.Aspect), pose, sky, blk)
	}
}
