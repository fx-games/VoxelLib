package basepack_test

import (
	"image/png"
	"os"
	"testing"

	"github.com/memmaker/voxellib/i18n"
	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxellib/ui/basepack"
)

// The pack alone carries a menu: the font measures, the strings translate
// and a main menu and the options draw (BASEPACK_PNG=dir writes them).
func TestMenuFromPackAlone(t *testing.T) {
	cat := i18n.New()
	if err := cat.LoadFS(basepack.FS); err != nil {
		t.Fatal(err)
	}
	cat.SetLanguage("en_US")
	c := ui.NewSoftCanvas(854, 480, 2)
	ctx, err := ui.NewContext(c, basepack.FS, cat)
	if err != nil {
		t.Fatal(err)
	}
	if got := ctx.T("menu.options"); got != "Options..." {
		t.Errorf("menu.options is %q", got)
	}
	if w := ctx.Font.Width("Hello"); w < 20 || w > 40 {
		t.Errorf("Hello is %d px wide", w)
	}
	noop := func() {}
	for name, s := range map[string]ui.Screen{
		"main":    ui.NewMainMenu(ctx, []ui.MenuItem{{Label: "menu.singleplayer", OnClick: noop}, {Label: "Load Game", Disabled: true}}, noop, noop, nil),
		"options": &ui.SettingsScreen{Title: "options.title", Settings: []ui.Setting{ui.GUIScaleSetting(ctx, nil), {Label: "options.fov", Slider: true}}},
	} {
		ctx.Screens.CloseAll()
		clear(c.Img.Pix)
		ctx.Resize(854, 480)
		ctx.Screens.Open(s)
		ctx.Screens.Draw(210, 128, 0)
		if dir := os.Getenv("BASEPACK_PNG"); dir != "" {
			f, _ := os.Create(dir + "/" + name + ".png")
			png.Encode(f, c.Img)
			f.Close()
		}
	}
}
