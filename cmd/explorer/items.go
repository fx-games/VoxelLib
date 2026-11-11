package main

import (
	"image"
	"log"
	"math"

	"github.com/memmaker/voxellib/items"
	"github.com/memmaker/voxellib/voxel/render"
)

// itemsState is the explorer's item sprites (render.ItemRenderer): the
// gun pack's gun and ammunition icons, drawn as the server's pickups.
type itemsState struct {
	itemNames []string
	itemImgs  []image.Image // pending upload
	itemRend  *render.ItemRenderer
	instBuf   []render.ItemInstance
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
	g.itemImgs = nil
}

// pickupScale is a pickup sprite's size (a dropped item's).
const pickupScale = 0.5

// drawPickups draws the server's pickups for cam, bobbing and spinning as
// dropped items (items.Drop.Pose) on the server's clock.
func (g *game) drawPickups(cam *render.Camera) {
	if g.itemRend == nil || g.net == nil {
		return
	}
	t := g.net.cl.RenderTime()
	g.instBuf = g.instBuf[:0]
	for _, p := range g.net.cl.Pickups() {
		item := p.Sprite()
		if !g.itemRend.Has(item) {
			continue
		}
		d := items.Drop{X: p.X, Y: p.Y, Z: p.Z, PrevX: p.X, PrevY: p.Y, PrevZ: p.Z, Age: int(t)}
		x, y, z, spin := d.Pose(t - math.Floor(t))
		sky, blk := g.Light(x, y, z)
		g.instBuf = append(g.instBuf, render.ItemInstance{Item: item, X: x, Y: y, Z: z, Yaw: spin, Scale: pickupScale, Sky: sky, Block: blk})
	}
	g.itemRend.DrawWorld(cam, g.instBuf)
}
