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
	case float64(ammo) < float64(spec.Mag)*0.25 && ammo < 10:
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
	if ammo <= 0 {
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
