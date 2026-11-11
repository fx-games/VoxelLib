package main

// Refit (J with a gun in hand): TACZ's GunRefitScreen reduced to a list of
// the gun's attachment slots; clicking a slot installs the next attachment
// the gun allows there (after the last: none). The gun's view and spec are
// rebuilt for the new choice (tacz.Pack.Model/Gun), and the attachment's
// install or uninstall sound plays as SoundPlayManager.playerRefitSound.

import (
	"fmt"
	"log"
	"maps"
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

// gunChoice is the attachments installed on a gun (its default until refitted).
func (g *game) gunChoice(id string) tacz.Choice {
	if c, ok := g.choices[id]; ok {
		return c
	}
	return g.gunPack.DefaultChoice(id)
}

// refit installs attachment att ("": none) in a slot of the held gun and
// rebuilds its view, keeping the gun's state.
func (g *game) refit(slot tacz.Slot, att string) error {
	id := g.gun.Gun.ID
	c := maps.Clone(g.gunChoice(id))
	old := c[slot]
	c[slot] = att
	if _, err := g.gunPack.Gun(id, c); err != nil {
		return err
	}
	g.choices[id] = c
	g.dropGunView(id)
	v, err := g.gunView(id)
	if err != nil {
		return err
	}
	st := g.gun
	st.Gun, st.ClipLen = &v.spec, clipLen(v)
	st.Ammo = min(st.Ammo, v.spec.Mag)
	g.gun = st
	g.sounds.preload(&v.spec)
	if att != "" {
		g.refitSound(att, tacz.SoundInstall)
	} else {
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
		for _, a := range g.gunPack.Allowed(id, s) {
			evs = append(evs, g.gunPack.AttachmentSound(a, tacz.SoundInstall), g.gunPack.AttachmentSound(a, tacz.SoundUninstall))
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
	c := g.gunChoice(id)
	x := ctx.W/2 - refitButtonW/2
	for i, slot := range g.gunPack.AttachmentSlots(id) {
		allowed := g.gunPack.Allowed(id, slot)
		cur := c[slot]
		label := "None"
		if cur != "" {
			label = g.gunPack.AttachmentName(cur)
		}
		s.Add(ui.NewButton(x, refitTop+i*refitRow, refitButtonW, fmt.Sprintf("%s: %s", slotTitle(slot), label), func() {
			// The next allowed attachment after the current one; after the last, none.
			next := ""
			if k := slices.Index(allowed, cur); k+1 < len(allowed) {
				next = allowed[k+1]
			}
			if err := g.refit(slot, next); err != nil {
				log.Printf("refit: %v", err)
				s.status = "§c" + err.Error()
			}
			s.Init(ctx)
		}))
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
