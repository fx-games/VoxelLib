package main

// Refit (J with a gun in hand): TACZ's GunRefitScreen reduced to a list of
// the gun's attachment slots; clicking a slot installs the next attachment
// the gun allows there (after the last: none). The gun's view and spec are
// rebuilt for the new choice (tacz.Pack.Model/Gun), and the attachment's
// install or uninstall sound plays as SoundPlayManager.playerRefitSound.

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/memmaker/voxellib/audio"
	"github.com/memmaker/voxellib/importer/tacz"
	"github.com/memmaker/voxellib/ui"
)

// refitSoundVolume is playerRefitSound's volume (pitch 1).
const refitSoundVolume = 1

// Refit screen layout (GUI pixels).
const (
	refitTop     = 40
	refitRow     = 24
	refitButtonW = 260
)

// refit installs attachment att ("": none) in a slot of the held gun and
// rebuilds its view, keeping the gun's state.
func (g *game) refit(slot tacz.Slot, att string) error {
	c := g.stackChoice(g.gunStack).Clone()
	c.Attach(slot, att)
	return g.applyChoice(slot, c)
}

// refitAdapter selects slot adapter ad ("": none) for a slot of the held
// gun; an installed attachment the adapter doesn't take comes off.
func (g *game) refitAdapter(slot tacz.Slot, ad string) error {
	id := g.gun.Gun.ID
	c := g.stackChoice(g.gunStack).Clone()
	c.Adapt(slot, ad)
	if !slices.Contains(g.gunPack.Allowed(id, slot, ad), c.Attachments[slot]) {
		c.Attach(slot, "")
	}
	return g.applyChoice(slot, c)
}

// applyChoice refits the held gun with c, which changed slot, and plays the
// slot's install or uninstall sound.
func (g *game) applyChoice(slot tacz.Slot, c tacz.Choice) error {
	id, was := g.gun.Gun.ID, g.stackChoice(g.gunStack)
	old, att := was.Attachments[slot], c.Attachments[slot]
	if _, err := g.gunPack.Gun(id, c, g.gun.FireMode()); err != nil {
		return err
	}
	g.dropGunView(viewKey(id, was))
	setGunChoice(g.gunStack, c)
	v, err := g.gunView(g.gunStack)
	if err != nil {
		return err
	}
	st := g.gun
	st.Gun, st.ClipLen = &v.spec, clipLen(v)
	st.Ammo = min(st.Ammo, v.spec.Mag)
	g.gun = st
	if err := g.resolveMode(); err != nil { // the view is built in the first mode
		return err
	}
	g.sounds.preload(&v.spec)
	g.sendHold()
	switch {
	case att == old:
	case att != "":
		g.refitSound(att, tacz.SoundInstall)
	default:
		g.refitSound(old, tacz.SoundUninstall)
	}
	return nil
}

// refitSound is SoundPlayManager.playerRefitSound.
func (g *game) refitSound(att string, s tacz.RefitSound) {
	g.sounds.play(g.gunPack.AttachmentSound(att, s), audio.UI(refitSoundVolume, 1))
}

// openRefit opens the refit screen for the held gun.
func (g *game) openRefit() {
	if !g.gunOn() || g.ui == nil {
		return
	}
	id := g.gun.Gun.ID
	var evs []string
	for _, s := range g.gunPack.AttachmentSlots(id) {
		for _, ad := range append([]string{""}, g.gunPack.SlotAdapters(id, s)...) {
			for _, a := range g.gunPack.Allowed(id, s, ad) {
				evs = append(evs, g.gunPack.AttachmentSound(a, tacz.SoundInstall), g.gunPack.AttachmentSound(a, tacz.SoundUninstall))
			}
		}
	}
	g.sounds.preloadEvents(evs)
	g.ui.Open(g.screen(&refitScreen{g: g}))
}

// refitScreen lists the held gun's slots.
type refitScreen struct {
	ui.Base
	g      *game
	status string
}

func (s *refitScreen) Init(ctx *ui.Context) {
	s.Widgets = nil
	g := s.g
	id := g.gun.Gun.ID
	c := g.stackChoice(g.gunStack)
	x := ctx.W/2 - refitButtonW/2
	row := 0
	// button cycles cur through options: the next after it; after the last, none.
	button := func(title, cur string, options []string, name func(string) string, set func(string) error) {
		label := "None"
		if cur != "" {
			label = name(cur)
		}
		s.Add(ui.NewButton(x, refitTop+row*refitRow, refitButtonW, fmt.Sprintf("%s: %s", title, label), func() {
			next := ""
			if k := slices.Index(options, cur); k+1 < len(options) {
				next = options[k+1]
			}
			if err := set(next); err != nil {
				log.Printf("refit: %v", err)
				s.status = "§c" + err.Error()
			}
			s.Init(ctx)
		}))
		row++
	}
	for _, slot := range g.gunPack.AttachmentSlots(id) {
		if ads := g.gunPack.SlotAdapters(id, slot); len(ads) > 0 {
			button(slotTitle(slot)+" adapter", c.Adapters[slot], ads, g.gunPack.SlotAdapterName,
				func(ad string) error { return g.refitAdapter(slot, ad) })
		}
		button(slotTitle(slot), c.Attachments[slot], g.gunPack.Allowed(id, slot, c.Adapters[slot]), g.gunPack.AttachmentName,
			func(att string) error { return g.refit(slot, att) })
	}
	s.Add(ui.NewButton(x, ctx.H-28, refitButtonW, ctx.T("gui.done"), func() { ctx.Screens.Back() }))
}

// slotTitle is a slot's name for the screen ("extended_mag" → "Extended mag").
func slotTitle(s tacz.Slot) string {
	t := strings.ReplaceAll(string(s), "_", " ")
	return strings.ToUpper(t[:1]) + t[1:]
}

func (s *refitScreen) Draw(ctx *ui.Context, mx, my int, partial float32) {
	ctx.Font.DrawCentered(ctx.Canvas, "Refit: "+s.g.gun.Gun.Name, ctx.W/2, 16, 0xFFFFFF)
	if s.status != "" {
		ctx.Font.DrawCentered(ctx.Canvas, s.status, ctx.W/2, ctx.H-39, 0xFFFFFF)
	}
	s.DrawWidgets(ctx, mx, my)
}
