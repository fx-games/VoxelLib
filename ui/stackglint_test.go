package ui_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"

	"github.com/memmaker/voxellib/inventory"
	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxellib/voxel/render"
)

// TestStackGlint: one enchanted item glints as a slot icon, in the hand
// and on the ground with no game code; Item.Glint always glints and the
// GlintOf hook decides over both.
func TestStackGlint(t *testing.T) {
	enc := func(img image.Image) []byte {
		var b bytes.Buffer
		png.Encode(&b, img)
		return b.Bytes()
	}
	glint := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := range glint.Pix {
		glint.Pix[i] = 255
	}
	fs := fstest.MapFS{
		ui.FontASCII: {Data: enc(image.NewNRGBA(image.Rect(0, 0, 128, 128)))},
		ui.AssetPath(ui.GlintTexture): {Data: enc(glint)},
	}
	ctx, err := ui.NewContext(ui.NewSoftCanvas(32, 32, 1), fs, nil)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for i := range img.Pix {
		img.Pix[i] = []uint8{40, 40, 40, 255}[i%4]
	}
	tex := ctx.SetTexture("test:item/sword", img)
	icon := &ui.Sprite{Tex: tex, W: 16, H: 16}

	sword := &inventory.Item{Key: "test:sword"}
	st := &inventory.Stack{Item: sword, Count: 1, Tag: map[string]any{"ench": []any{map[string]any{"id": int16(16), "lvl": int16(1)}}}}

	// the slot: a game's item func that knows nothing of glints
	chest := inventory.NewChest(1)
	chest.Slots[0] = st
	view := ui.NewContainerView(inventory.NewChestContainer(inventory.NewPlayer(inventory.PlayerSpec{}), chest),
		func(s *inventory.Stack) ui.ItemStack { return ui.ItemStack{Icon: icon, Count: s.Count} }, nil)
	slot := view.Slot(0)
	if !slot.Glint {
		t.Fatal("enchanted slot icon: no glint")
	}
	ui.DrawItem(ctx, slot, 0, 0)
	if c := ctx.Canvas.(*ui.SoftCanvas).Img.NRGBAAt(8, 8); c == (color.NRGBA{40, 40, 40, 255}) {
		t.Fatal("slot icon drawn bare:", c)
	}
	// the first-person hand and the ground drop
	hand := render.ItemPose{Item: "test:sword", Stack: st}
	drop := render.ItemPose{Item: "test:sword", Stack: st, Y: render.DropYOffset}
	if hand.GlintPasses() != 1 || drop.GlintPasses() != 1 {
		t.Fatal("enchanted hand / drop glint passes", hand.GlintPasses(), drop.GlintPasses())
	}

	star := &inventory.Stack{Item: &inventory.Item{Key: "test:star", Glint: true}, Count: 1}
	plain := &inventory.Stack{Item: sword, Count: 1}
	if !star.Glints() || plain.Glints() || ui.ItemOf(plain, icon).Glint {
		t.Fatal("Item.Glint always glints, a plain stack never")
	}
	inventory.GlintOf = func(s *inventory.Stack) bool { return false }
	defer func() { inventory.GlintOf = nil }()
	if st.Glints() || star.Glints() || view.Slot(0).Glint {
		t.Fatal("GlintOf vetoes")
	}
}
