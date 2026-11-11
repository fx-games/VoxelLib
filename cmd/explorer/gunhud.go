package main

import (
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"

	"github.com/memmaker/voxellib/ui"
	firearm "github.com/memmaker/voxellib/weapon"
)

// gunHUDDir holds TACZ's own HUD textures (fire mode icons).
const gunHUDDir = "assets/tacz/hud"

// TACZ GunHudOverlay colours (ARGB-less font colours, ARGB fills).
const (
	hudAmmoLow       = 0xFF5555
	hudAmmoInventory = 0xFFFF55 // a gun fed straight from the inventory
	hudAmmo          = 0xFFFFFF
	hudStock         = 0xAAAAAA
	hudEmptyTint     = 0xFFFF4D4D // setShaderColor(1, 0.3, 0.3, 1)
	hudWhite         = 0xFFFFFFFF
	// HeatBarOverlay: the bar's fills (white → yellow → red), the locked
	// blink (red/yellow every heatBlink ticks).
	heatWhite  = 0x9FFFFFFF
	heatYellow = 0x9FFFFF00
	heatRed    = 0x9FFF0000
	heatBlink  = 10
	heatText   = 0xFFFFFFFF
	heatTextR  = 0xFFFF0000
	heatTextY  = 0xFFFFFF00
)

// drawGunHUD is TACZ's GunHudOverlay: the gun's HUD sprite, a divider, the
// rounds in the gun (×1.5), the carried stock (×0.8) and the fire mode,
// bottom right.
func (g *game) drawGunHUD(ctx *ui.Context) {
	if !g.gunOn() {
		return
	}
	s := &g.gun
	spec := s.Gun
	w, h := ctx.W, ctx.H
	c, f := ctx.Canvas, ctx.Font
	inventory := spec.Reload.Kind == firearm.Inventory
	ammo, stock := s.Ammo, g.stock[spec.Ammo]
	if inventory {
		ammo = stock
	}
	ammo = min(ammo, 9999)
	color := uint32(hudAmmo)
	switch {
	case float64(ammo) < float64(spec.Mag)*0.25 && ammo < 10 || s.Overheated:
		color = hudAmmoLow
	case inventory:
		color = hudAmmoInventory
	}
	cur := fmt.Sprintf("%03d", ammo)
	stockText := ""
	if !inventory {
		stockText = fmt.Sprintf("%04d", stock)
	}
	ui.DrawRect(c, w-75, h-43, w-74, h-25, hudWhite)
	f.DrawScaled(c, cur, 0, 0, 1.5, float32(int(float32(w-70)/1.5)), float32(int(float32(h-43)/1.5)), color, false)
	curW := float32(f.Width(cur)) * 1.5
	f.DrawScaled(c, stockText, 0, 0, 0.8, float32(int((float32(w-68)+curW)/0.8)), float32(int(float32(h-43)/0.8)), hudStock, false)

	icon, tint := spec.Hud, uint32(hudWhite)
	if ammo <= 0 || s.Overheated {
		if spec.HudEmpty != "" {
			icon = spec.HudEmpty
		} else {
			tint = hudEmptyTint
		}
	}
	if t := g.gunHUDTex(ctx, "pack:"+icon, func() (image.Image, error) { return g.gunPack.Icon(icon) }); t != nil {
		ui.DrawTexScaled(c, t, float32(w-117), float32(h-44), 39, 13, 0, 0, 1, 1, 1, 1, tint)
	}
	mode := map[firearm.FireMode]string{firearm.Semi: "fire_mode_semi", firearm.Auto: "fire_mode_auto", firearm.Burst: "fire_mode_burst"}[s.FireMode()]
	if t := g.gunHUDTex(ctx, "hud:"+mode, func() (image.Image, error) { return readHUDPNG(mode) }); t != nil {
		ui.DrawTexScaled(c, t, float32(int(float32(w)-68.5+curW)), float32(h-38), 10, 10, 0, 0, 1, 1, 1, 1, hudWhite)
	}
	g.drawGunHeat(ctx)
}

// drawGunHeat is TACZ's HeatBarOverlay: around the crosshair, the heat
// ring (heat_base) and a bar filling with the heat, the whole scaled by
// 0.75 + heat/8 (eased toward it per frame).
func (g *game) drawGunHeat(ctx *ui.Context) {
	s := &g.gun
	h := s.Gun.Heat
	if h == nil || h.Max <= 0 {
		return
	}
	pct := s.Heat / h.Max
	want := pct/8 + 0.75
	switch {
	case g.heatScale < want:
		g.heatScale += 0.05
	case g.heatScale > want:
		g.heatScale -= 0.025
	}
	if g.heatScale > want-0.03 && g.heatScale < want+0.055 {
		g.heatScale = want
	}
	k := g.heatScale
	w, hh := float32(int(float32(ctx.W)/k)), float32(int(float32(ctx.H)/k))
	at := func(v float32) int { return int(v * k) }
	red := g.Tick%(2*heatBlink) < heatBlink
	bar, tint, text, label := heatColor(pct), uint32(hudWhite), uint32(heatText), fmt.Sprintf("%.1f%%", pct*100)
	if s.Overheated {
		bar, tint, text, label = heatYellow, 0xFFFFFF1A, heatTextY, "!OVERHEAT!"
		if red {
			bar, tint, text = heatRed, 0xFFFF1A1A, heatTextR
		}
	}
	c, f := ctx.Canvas, ctx.Font
	ui.DrawRect(c, at(w/2-30), at(hh/2+30), at(w/2-30+float32(int(pct*60))), at(hh/2+34), bar)
	if t := g.gunHUDTex(ctx, "hud:heat_base", func() (image.Image, error) { return readHUDPNG("heat_base") }); t != nil {
		ui.DrawTexScaled(c, t, (w/2-64)*k, (hh/2-44)*k, 128*k, 128*k, 0, 0, 1, 1, 1, 1, tint)
	}
	f.DrawScaled(c, label, 0, 0, k, w/2-float32(f.Width(label)/2), hh/2+38, text, true)
}

// heatColor is HeatBarOverlay.getHeatColor unlocked.
func heatColor(p float32) uint32 {
	switch {
	case p < 0.4:
		return heatWhite
	case p <= 0.65:
		return lerpARGB(p*4-1.6, heatWhite, heatYellow)
	}
	return lerpARGB((p-0.65)/0.35, heatYellow, heatRed)
}

// lerpARGB is FastColor.ARGB32.lerp.
func lerpARGB(t float32, a, b uint32) uint32 {
	var out uint32
	for sh := 0; sh < 32; sh += 8 {
		x, y := float32(a>>sh&0xFF), float32(b>>sh&0xFF)
		out |= uint32(x+(y-x)*t) & 0xFF << sh
	}
	return out
}

// gunHUDTex uploads a HUD sprite once (nil if it can't be read).
func (g *game) gunHUDTex(ctx *ui.Context, key string, load func() (image.Image, error)) ui.Texture {
	if t, ok := g.hudTex[key]; ok {
		return t
	}
	var t ui.Texture
	if img, err := load(); err != nil {
		log.Printf("guns: hud %s: %v", key, err)
	} else {
		t = ctx.SetTexture("explorer:gunhud/"+key, img)
	}
	if g.hudTex == nil {
		g.hudTex = map[string]ui.Texture{}
	}
	g.hudTex[key] = t
	return t
}

func readHUDPNG(name string) (image.Image, error) {
	r, err := os.Open(filepath.Join(gunHUDDir, name+".png"))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	img, _, err := image.Decode(r)
	return img, err
}
