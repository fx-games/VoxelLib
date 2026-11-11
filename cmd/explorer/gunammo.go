package main

import (
	"log"
	"math"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/memmaker/voxellib/projectile"
	"github.com/memmaker/voxellib/voxel/render"
	"github.com/memmaker/voxellib/weapon"
)

// ammoEntityLight is EntityKineticBullet.getBlockLightLevel: bullets are
// lit as full block light.
const ammoEntityLight = 15

// ammoEntities caches the ammo displays' entity models by ammo ID
// (-1: none or failed to load).
type ammoEntities map[string]int

// drawAmmoEntities draws each bullet whose ammo has an entity model
// (EntityBulletRenderer.render: the model at the bullet, turned by its
// lerped rotation), single player and the server's bullets alike.
func (g *game) drawAmmoEntities(cam *render.Camera, alpha float64) {
	if g.gunPack == nil {
		return
	}
	if g.ammoFX == nil {
		g.ammoFX = ammoEntities{}
	}
	var insts []render.ModelInstance
	var mats []mgl32.Mat4
	add := func(b *projectile.Bullet, spec *weapon.Gun) {
		if spec == nil {
			return
		}
		h, ok := g.ammoFX[spec.Ammo]
		if !ok {
			h = -1
			m, err := g.gunPack.AmmoEntityModel(spec.Ammo)
			if err == nil && m != nil {
				h, err = g.Models.Add(m)
			}
			if err != nil {
				log.Printf("guns: ammo %s entity: %v", spec.Ammo, err)
				h = -1
			}
			g.ammoFX[spec.Ammo] = h
		}
		if h < 0 {
			return
		}
		pos := b.Lerp(alpha)
		yaw, pitch := b.LerpRotation(alpha)
		mats = append(mats, mgl32.HomogRotate3DY(float32(yaw-math.Pi)).Mul4(mgl32.HomogRotate3DX(float32(pitch))))
		sky, _ := g.Light(pos[0], pos[1], pos[2])
		insts = append(insts, render.ModelInstance{Model: h, X: pos[0], Y: pos[1], Z: pos[2], Sky: sky, Block: ammoEntityLight})
	}
	if g.net == nil {
		for _, b := range g.bullets {
			add(b, b.Gun)
		}
	} else {
		for _, b := range g.net.cl.Bullets {
			add(b.Bullet, g.gunSpec(b.Gun))
		}
	}
	for i := range insts {
		insts[i].Transform = &mats[i]
	}
	g.Models.Draw(cam, insts)
}
