package main

// Menus on package ui (through uiapp, drawn over the live world): the main
// menu (single player, multiplayer, quit), the multiplayer screen (name,
// skin, join a found or typed server, create a game) with its create-game
// screen, and the pause menu.

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/memmaker/voxellib/app"
	"github.com/memmaker/voxellib/i18n"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/ui"
	"github.com/memmaker/voxellib/ui/basepack"
	"github.com/memmaker/voxellib/ui/uiapp"
)

// setupUI makes the ui layer: chat lines (the kill feed) over the HUD, and
// the menus.
func (g *game) setupUI() {
	cat := i18n.New()
	if err := cat.LoadFS(basepack.FS); err != nil {
		log.Printf("ui: %v", err)
	}
	cat.SetLanguage("en_US")
	u, err := uiapp.New(g.App, ui.StackFS{basepack.FS}, cat)
	if err != nil {
		log.Printf("ui disabled: %v", err)
		return
	}
	u.HUD.State.Hidden = true // the explorer draws its own HUD; this one shows chat
	g.ui = u
	u.Top = func(ctx *ui.Context) {
		g.drawMatch(ctx)
		g.drawGunHUD(ctx)
	}
	g.Hooks.PauseScreen = func() app.Screen { return g.ui.Screen(g.pauseMenu()) }
}

// hudStack draws several HUDs in order.
type hudStack []app.HUD

func (h hudStack) Draw(fbW, fbH int) {
	for _, d := range h {
		d.Draw(fbW, fbH)
	}
}

// noPause keeps the world running under a screen: a multiplayer game
// can't stop.
type noPause struct{ ui.Screen }

func (noPause) PausesGame() bool { return false }

func (g *game) screen(s ui.Screen) ui.Screen {
	if g.net != nil {
		return noPause{s}
	}
	return s
}

// openMainMenu shows the title menu over the running single-player world.
func (g *game) openMainMenu() {
	if g.ui == nil {
		return
	}
	ctx := g.ui.Ctx
	m := ui.NewMainMenu(ctx, []ui.MenuItem{
		{Label: "Singleplayer", OnClick: func() { g.startSolo(ctx) }},
		{Label: "Multiplayer", OnClick: func() { ctx.Screens.Open(g.multiplayerScreen()) }},
	}, func() { ctx.Screens.Open(g.optionsScreen()) }, g.Quit, nil)
	m.Title, m.TitleY = "§lVOXELLIB", 50
	m.FooterLeft = "VoxelLib engine demo"
	if g.pendingNote != "" {
		m.FooterRight, g.pendingNote = g.pendingNote, ""
	}
	g.ui.Open(noPause{m}) // the world lives on behind the title
}

// startSolo leaves the title for single player: zombie defense, unless
// already in a game (back to it) or exploring the -sandbox world.
func (g *game) startSolo(ctx *ui.Context) {
	ctx.Screens.CloseAll()
	if g.net != nil || g.opt.sandbox {
		return
	}
	if err := g.solo(); err != nil {
		log.Printf("singleplayer: %v", err)
		g.pendingNote = err.Error()
		g.openMainMenu()
	}
}

// pauseMenu is Esc's menu: back to the game, and leaving it.
func (g *game) pauseMenu() ui.Screen {
	ctx := g.ui.Ctx
	quit := func() {
		ctx.Screens.CloseAll()
		g.openMainMenu()
	}
	var extra []ui.MenuItem
	if g.net != nil {
		quit = func() {
			ctx.Screens.CloseAll()
			g.leaveGame("")
		}
		label := "Disconnect"
		if g.net.srv != nil {
			label = "Stop hosting"
		}
		extra = append(extra, ui.MenuItem{Label: label, OnClick: quit})
	} else {
		extra = append(extra, ui.MenuItem{Label: "Multiplayer", OnClick: func() { ctx.Screens.Open(g.multiplayerScreen()) }})
	}
	m := ui.NewPauseMenu(ctx, func() { ctx.Screens.CloseAll() }, func() { ctx.Screens.Open(g.optionsScreen()) }, quit, extra...)
	return g.screen(m)
}

func (g *game) optionsScreen() ui.Screen {
	ctx := g.ui.Ctx
	return g.screen(&ui.SettingsScreen{Title: "menu.options", Settings: []ui.Setting{
		ui.GUIScaleSetting(ctx, nil),
	}})
}

// mpScreen is the multiplayer menu.
type mpScreen struct {
	ui.Base
	g      *game
	name   *ui.TextField
	addr   *ui.TextField
	status string

	mu       sync.Mutex
	found    []mp.ServerInfo
	scanning bool
	lastScan time.Time
	shown    int // servers laid out
}

func (g *game) multiplayerScreen() ui.Screen {
	s := &mpScreen{g: g}
	s.scan()
	return s
}

// scan looks for LAN servers in the background.
func (s *mpScreen) scan() {
	s.mu.Lock()
	if s.scanning {
		s.mu.Unlock()
		return
	}
	s.scanning, s.lastScan = true, time.Now()
	s.mu.Unlock()
	go func() {
		found := mp.Discover(600 * time.Millisecond)
		s.mu.Lock()
		s.found, s.scanning = found, false
		s.mu.Unlock()
	}()
}

func (s *mpScreen) Init(ctx *ui.Context) {
	g := s.g
	s.Widgets = nil
	x := ctx.W/2 - 100
	if s.name == nil {
		s.name = ui.NewTextField(x, 0, 200, 20)
		s.name.MaxLen = 16
		s.name.SetText(g.playerName)
		s.name.OnChange = func(t string) { g.playerName = strings.TrimSpace(t) }
		s.addr = ui.NewTextField(x, 0, 148, 20)
		s.addr.MaxLen = 64
		s.addr.OnEnter = func(t string) { s.join(ctx, strings.TrimSpace(t)) }
	}
	// You: name and skin.
	s.name.X, s.name.Y = x, 42
	s.Add(s.name)
	s.Add(ui.NewButton(x, 66, 200, "Skin: "+g.skins[g.skinIdx].name, func() {
		g.skinIdx = (g.skinIdx + 1) % len(g.skins)
		s.Init(ctx)
	}))
	// Join: the LAN games, filling the room down to the address row, and
	// an address.
	s.mu.Lock()
	found := s.found
	s.mu.Unlock()
	y := mpListY
	for _, f := range found[:min(len(found), max(0, (ctx.H-52-12-24-y)/22))] {
		addr := f.Addr
		label := fmt.Sprintf("%s (%d/%d)", f.Name, f.Players, f.Max)
		s.Add(ui.NewButton(x, y, 200, label, func() { s.join(ctx, addr) }))
		y += 22
	}
	s.addr.X, s.addr.Y = x, y+2
	s.Add(s.addr)
	s.Add(ui.NewButton(x+152, y+2, 48, "Join", func() { s.join(ctx, strings.TrimSpace(s.addr.Text)) }))
	// Create, and leave.
	s.Add(ui.NewButton(x, ctx.H-52, 200, "Create a game...", func() { ctx.Screens.Open(g.createScreen()) }))
	s.Add(ui.NewButton(x, ctx.H-28, 98, "Refresh", func() { s.scan() }))
	s.Add(ui.NewButton(x+102, ctx.H-28, 98, ctx.T("gui.back"), func() { ctx.Screens.Back() }))
}

// mpListY is where the multiplayer screen's server list starts.
const mpListY = 104

func (s *mpScreen) join(ctx *ui.Context, addr string) {
	if addr == "" {
		return
	}
	s.status = "Connecting to " + addr + "..."
	if err := s.g.joinGame(addr); err != nil {
		s.status = "§c" + err.Error()
		return
	}
	ctx.Screens.CloseAll()
}

func (s *mpScreen) Update() {
	s.Base.Update()
	s.mu.Lock()
	scanning, last := s.scanning, s.lastScan
	s.mu.Unlock()
	if !scanning && time.Since(last) > 3*time.Second {
		s.scan()
	}
}

func (s *mpScreen) Draw(ctx *ui.Context, mx, my int, partial float32) {
	// Pick up a finished scan.
	s.mu.Lock()
	n := len(s.found)
	s.mu.Unlock()
	if n != s.shown {
		s.shown = n
		s.Init(ctx)
	}
	ctx.DrawBackground()
	f := ctx.Font
	f.DrawCentered(ctx.Canvas, "Multiplayer", ctx.W/2, 12, 0xFFFFFF)
	x := ctx.W/2 - 100
	f.Draw(ctx.Canvas, "Your name and skin", x, 31, 0xA0A0A0, false)
	// The chosen skin's face, next to its button.
	sk := s.g.skins[s.g.skinIdx]
	t := ctx.Tex("voxellib:skin/" + sk.name)
	ui.DrawTexScaled(ctx.Canvas, t, float32(x-26), 66, 20, 20, 8, 8, 8, 8, 64, 64, 0xFFFFFFFF)
	ui.DrawTexScaled(ctx.Canvas, t, float32(x-26), 66, 20, 20, 40, 8, 8, 8, 64, 64, 0xFFFFFFFF)
	label := "Join a LAN game, or type an address:"
	if n == 0 {
		label = "Join: no LAN games yet (searching...)"
	}
	f.Draw(ctx.Canvas, label, x, mpListY-12, 0xA0A0A0, false)
	if s.status != "" {
		f.DrawCentered(ctx.Canvas, s.status, ctx.W/2, ctx.H-63, 0xFFFFFF)
	}
	s.DrawWidgets(ctx, mx, my)
}

// createScreen sets up a game to host: its name in the LAN list, the
// world seed and type (a generator or one of the maps in mp.MapsDir).
type createScreen struct {
	ui.Base
	g          *game
	name, seed *ui.TextField
	worlds     []mp.MapInfo
	world      int // index into worlds
	mode       mp.Mode
	status     string
}

func (g *game) createScreen() ui.Screen {
	worlds := []mp.MapInfo{{Gen: "overworld", Title: "World type: Overworld"}, {Gen: "flat", Title: "World type: Flat"}}
	for _, m := range mp.Maps() {
		worlds = append(worlds, mp.MapInfo{Gen: m.Gen, Title: "Map: " + m.Title})
	}
	return &createScreen{g: g, worlds: worlds, mode: mp.ZombieDefense}
}

func (s *createScreen) Init(ctx *ui.Context) {
	s.Widgets = nil
	x := ctx.W/2 - 100
	if s.name == nil {
		s.name = ui.NewTextField(x, 0, 200, 20)
		s.name.MaxLen = 32
		s.name.SetText(s.g.playerName + "'s game")
		s.seed = ui.NewTextField(x, 0, 200, 20)
	}
	s.name.X, s.name.Y = x, 52
	s.seed.X, s.seed.Y = x, 92
	s.Add(s.name)
	s.Add(s.seed)
	s.Add(ui.NewButton(x, 120, 200, s.worlds[s.world].Title, func() { s.world = (s.world + 1) % len(s.worlds); s.Init(ctx) }))
	s.Add(ui.NewButton(x, 144, 200, "Mode: "+s.mode.String(), func() {
		s.mode = (s.mode + 1) % mp.NumModes
		s.Init(ctx)
	}))
	s.Add(ui.NewButton(x, ctx.H-28, 98, "Create game", func() { s.create(ctx) }))
	s.Add(ui.NewButton(x+102, ctx.H-28, 98, ctx.T("gui.cancel"), func() { ctx.Screens.Back() }))
}

func (s *createScreen) create(ctx *ui.Context) {
	if err := s.g.hostGame(strings.TrimSpace(s.name.Text), seedOf(s.seed.Text), s.worlds[s.world].Gen, s.mode); err != nil {
		s.status = "§c" + err.Error()
		return
	}
	ctx.Screens.CloseAll()
}

// modeHelp says what each mode is.
var modeHelp = map[mp.Mode]string{
	mp.Deathmatch:     "Everyone against everyone; the most kills lead",
	mp.TeamDeathmatch: "Red against Blue; the team's kills add up",
	mp.BattleRoyale:   "One life a round; the energy wall closes in every 5 minutes",
	mp.ZombieDefense:  "Together against zombies; find guns and ammo lying about",
}

// seedOf is the reference's seed field: a number as is, other text by its
// (Java) hash, nothing random (0).
func seedOf(t string) int64 {
	t = strings.TrimSpace(t)
	if t == "" {
		return 0
	}
	if n, err := strconv.ParseInt(t, 10, 64); err == nil && n != 0 {
		return n
	}
	var h int32
	for _, r := range utf16.Encode([]rune(t)) {
		h = 31*h + int32(r)
	}
	return int64(h)
}

func (s *createScreen) Draw(ctx *ui.Context, mx, my int, partial float32) {
	ctx.DrawBackground()
	f := ctx.Font
	x := ctx.W/2 - 100
	f.DrawCentered(ctx.Canvas, "Create a LAN game", ctx.W/2, 12, 0xFFFFFF)
	f.Draw(ctx.Canvas, "Game name", x, 41, 0xA0A0A0, false)
	f.Draw(ctx.Canvas, "Seed (blank for random)", x, 81, 0xA0A0A0, false)
	f.DrawCentered(ctx.Canvas, modeHelp[s.mode], ctx.W/2, 170, 0xA0A0A0)
	f.DrawCentered(ctx.Canvas, fmt.Sprintf("Up to 4 players on UDP port %d", mp.DefaultPort), ctx.W/2, 182, 0xA0A0A0)
	if s.status != "" {
		f.DrawCentered(ctx.Canvas, s.status, ctx.W/2, ctx.H-39, 0xFFFFFF)
	}
	s.DrawWidgets(ctx, mx, my)
}

// registerSkins makes the skins' textures known to the ui (for the faces).
func (g *game) registerSkins() {
	if g.ui == nil {
		return
	}
	for _, sk := range g.skins {
		g.ui.Ctx.SetTexture("voxellib:skin/"+sk.name, sk.img)
	}
}
