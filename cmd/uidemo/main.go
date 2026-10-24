// Command uidemo renders ui screens to PNG on the CPU (ui.SoftCanvas), with
// resource packs stacked like in game, to check the GUI without a window.
//
//	uidemo -jar client.jar -packs a.zip,b.zip -lang de_DE -out /tmp/ui
//
// Packs are listed lowest priority first (options.txt order: the last wins).
package main

import (
	"archive/zip"
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing/fstest"

	"github.com/memmaker/voxellib/i18n"
	"github.com/memmaker/voxellib/inventory"
	"github.com/memmaker/voxellib/resloc"
	"github.com/memmaker/voxellib/ui"
)

func main() {
	home, _ := os.UserHomeDir()
	jar := flag.String("jar", "", "client jar (vanilla assets)")
	packs := flag.String("packs", "", "comma-separated resource packs (zip or dir), lowest priority first")
	lang := flag.String("lang", "en_US", "language")
	out := flag.String("out", ".", "output directory")
	w := flag.Int("w", 854, "framebuffer width")
	h := flag.Int("h", 480, "framebuffer height")
	scale := flag.Int("scale", 0, "GUI scale (0 auto)")
	index := flag.String("index", filepath.Join(home, "Library/Application Support/PrismLauncher/assets/indexes/1.7.10.json"), "launcher asset index (the languages besides en_US live there)")
	flag.Parse()
	if *jar == "" {
		log.Fatal("usage: uidemo -jar <client jar> [-packs a.zip,b.zip] [-lang de_DE] [-out dir]")
	}

	srcs := []fs.FS{open(*jar)} // lowest first
	if l, err := langFromIndex(*index); err == nil {
		srcs = append(srcs, l)
	} else {
		log.Print("no asset index: ", err)
	}
	for _, p := range strings.Split(*packs, ",") {
		if p != "" {
			srcs = append(srcs, open(p))
		}
	}
	var stack ui.StackFS
	for i := len(srcs) - 1; i >= 0; i-- {
		stack = append(stack, srcs[i])
	}
	cat := i18n.New()
	if err := cat.LoadFS(srcs...); err != nil {
		log.Fatal(err)
	}
	cat.SetLanguage(*lang)

	// The canvas size is fixed by the framebuffer; the GUI scale follows.
	gw, gh, s := ui.ScaledResolution(*w, *h, *scale, false)
	_ = gw
	_ = gh
	canvas := ui.NewSoftCanvas(*w, *h, float32(s))
	ctx, err := ui.NewContext(canvas, stack, cat)
	if err != nil {
		log.Fatal(err)
	}
	ctx.GUIScale = *scale
	noop := func() {}
	icon := func(name string) *ui.Sprite {
		return &ui.Sprite{Tex: ctx.Tex("textures/items/" + name + ".png"), W: 16, H: 16, TW: 16, TH: 16}
	}
	apple := &inventory.Item{Key: "apple"}
	sword := &inventory.Item{Key: "iron_sword", MaxStack: 1, MaxDamage: 250}
	item := func(s *inventory.Stack) ui.ItemStack {
		switch {
		case s == nil:
			return ui.ItemStack{}
		case s.Item == sword:
			return ui.ItemStack{Icon: icon("iron_sword"), Count: 1, Damage: float64(s.Damage) / float64(s.Item.MaxDamage), Bar: true, Tooltip: []string{"Iron Sword", "§9+6 Attack Damage"}}
		}
		return ui.ItemStack{Icon: icon("apple"), Count: s.Count, Tooltip: []string{"Apple"}}
	}
	p := inventory.NewPlayer(inventory.PlayerSpec{})
	p.Main[0], p.Main[2], p.Main[20] = &inventory.Stack{Item: sword, Count: 1, Damage: 100}, inventory.New(apple, 5), inventory.New(apple, 5)
	inv, _ := inventory.NewPlayerContainer(p, nil)
	pg := ui.NewPlayerGUI(ctx, p, inv, item)
	chest := inventory.NewChest(3)
	chest.Slots[0], chest.Slots[4] = inventory.New(apple, 5), &inventory.Stack{Item: sword, Count: 1, Damage: 100}

	shots := []struct {
		name   string
		screen func() ui.Screen
		mx, my int
	}{
		{"mainmenu", func() ui.Screen {
			return ui.NewMainMenu(ctx, []ui.MenuItem{{Label: "menu.singleplayer", OnClick: noop}, {Label: "menu.multiplayer", Disabled: true}}, noop, noop, noop)
		}, 210, 128},
		{"pause", func() ui.Screen { return ui.NewPauseMenu(ctx, noop, noop, noop) }, -1, -1},
		{"dialog", func() ui.Screen {
			return &ui.DialogScreen{
				Blocks:  []ui.DialogBlock{{Name: "Gandalf", Text: "Guten Morgen! Was meinst du damit? Wünschst du mir einen guten Morgen, oder meinst du, dass es ein guter Morgen ist, ob ich will oder nicht? Oder dass du dich heute Morgen gut fühlst? Oder dass es ein Morgen ist, an dem man gut sein muss?"}},
				Options: []ui.DialogOption{{Text: "Alles auf einmal, nehme ich an."}, {Text: "§eWer bist du?"}, {Text: "Lebe wohl."}},
			}
		}, -1, -1},
		{"chest", func() ui.Screen {
			return &ui.ContainerScreen{Layout: ui.ChestLayout(3, "container.chest"), Model: ui.NewContainerView(inventory.NewChestContainer(p, chest), item, nil)}
		}, 0, 0},
		{"settings", func() ui.Screen {
			return &ui.SettingsScreen{Title: "options.title", Settings: []ui.Setting{ui.GUIScaleSetting(ctx, func(int) {}), ui.LanguageSetting(ctx)}, OnDone: noop}
		}, -1, -1},
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, sh := range shots {
		clear(canvas.Img.Pix)
		ctx.Screens.CloseAll()
		ctx.Resize(*w, *h)
		ctx.Screens.Open(sh.screen())
		mx, my := sh.mx, sh.my
		if sh.name == "chest" { // hover the sword in the chest
			gw, gh := ctx.W, ctx.H
			mx, my = (gw-176)/2+8+4*18+8, (gh-(114+3*18))/2+18+8
		}
		ctx.Screens.Draw(mx, my, 0)
		save(filepath.Join(*out, sh.name+".png"), canvas)
	}

	// The player inventory, the mouse over the sword in the hotbar.
	clear(canvas.Img.Pix)
	ctx.Screens.CloseAll()
	pg.Open()
	pg.In.X, pg.In.Y = (ctx.W-176)/2+8+8, (ctx.H-166)/2+142+8
	pg.Draw(*w, *h, *w, *h, 0)
	save(filepath.Join(*out, "inventory.png"), canvas)

	// HUD over a plain sky.
	pg.Close()
	ui.DrawRect(canvas, 0, 0, ctx.W, ctx.H, 0xFF7FA8FF)
	hud := pg.HUD
	hud.State = ui.HUDState{Health: 13, MaxHealth: 20, LastHealth: 13, Food: 17, Saturation: 2, Armor: 7, ShowXP: true, XP: 0.6, Level: 12, Crosshair: true}
	p.Hold(2)
	hud.Message("§e<Bilbo>§r Ich bin auf ein Abenteuer gegangen!")
	hud.Title("Beutelsend", "Das Auenland", 10, 70, 20)
	for i := 0; i < 15; i++ {
		pg.Update()
	}
	pg.Draw(*w, *h, *w, *h, 0)
	save(filepath.Join(*out, "hud.png"), canvas)
}

// langFromIndex loads the .lang files of a launcher asset index
// (indexes/<v>.json next to objects/<hh>/<hash>).
func langFromIndex(path string) (fs.FS, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var idx struct {
		Objects map[string]struct{ Hash string }
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, err
	}
	objects := filepath.Join(filepath.Dir(filepath.Dir(path)), "objects")
	m := fstest.MapFS{}
	for name, o := range idx.Objects {
		if !strings.HasPrefix(name, resloc.Default+"/lang/") || len(o.Hash) < 2 {
			continue
		}
		b, err := os.ReadFile(filepath.Join(objects, o.Hash[:2], o.Hash))
		if err != nil {
			continue
		}
		m["assets/"+name] = &fstest.MapFile{Data: b}
	}
	return m, nil
}

func open(p string) fs.FS {
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return os.DirFS(p)
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		log.Fatal(err)
	}
	return zr
}

func save(path string, c *ui.SoftCanvas) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, c.Img); err != nil {
		log.Fatal(err)
	}
	fmt.Println(path)
}
