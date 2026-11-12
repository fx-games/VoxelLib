package main

import (
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"log"
	"math/rand/v2"
	"slices"

	"github.com/memmaker/voxellib/audio"
	"github.com/memmaker/voxellib/explosion"
	"github.com/memmaker/voxellib/particles"
	"github.com/memmaker/voxellib/resloc"
	"github.com/memmaker/voxellib/voxel/render"
)

// blastFX draws explosions with the vanilla FX particles (particles.System's
// EntityFX path): Explosion.doExplosionB's hugeexplosion, or largeexplode
// for a small blast, ticked with the world.
type blastFX struct {
	sys  particles.System
	rend *render.ParticleRenderer
	off  bool // no renderer or no explosion.png: explosions are not drawn
	// sounds are the vanilla sounds (-mcassets), nil without them.
	sounds *audio.Sounds
	rnd    *rand.Rand // the blast sound's pitch
}

// explodeSound is the blast's sound (Explosion.doExplosionB, TACZ
// ProjectileExplosion's GENERIC_EXPLODE), at volume explodeVolume.
const (
	explodeSound  = "random.explode"
	explodeVolume = 4
)

// vanillaSounds opens the vanilla sounds of the launcher asset index
// index, played through m; nil (logged) without it.
func vanillaSounds(index string, m *audio.Mixer) *audio.Sounds {
	files, err := resloc.AssetIndex(index)
	var s *audio.Sounds
	if err == nil {
		exists := func(f string) bool { _, err := fs.Stat(files, audio.SoundPath(f)); return err == nil }
		s, err = audio.OpenFS(files, resloc.Default+"/sounds.json", exists, m)
	}
	if err != nil {
		log.Printf("vanilla sounds (-mcassets): %v", err)
		return nil
	}
	return s
}

// explosionTexture is EntityLargeExplodeFX's texture in the client jar.
const explosionTexture = "assets/minecraft/textures/entity/explosion.png"

// hugeBlastRadius is the radius from which doExplosionB spawns
// hugeexplosion rather than largeexplode.
const hugeBlastRadius = 2

// explosionFX shows an explosion of radius at at (ProjectileExplosion's
// finalizeExplosion particles and sound).
func (g *game) explosionFX(at [3]float64, radius float32) {
	b := &g.blasts
	if b.sounds != nil {
		if b.rnd == nil {
			b.rnd = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
		}
		b.sounds.Play(explodeSound, audio.At(at[0], at[1], at[2], explodeVolume, explosion.Pitch(b.rnd)))
	}
	if b.rend == nil && !b.off {
		b.off = true
		img, err := g.jarImage(explosionTexture)
		if err == nil {
			b.rend, err = render.NewParticleRenderer(g.Eng.Renderer)
		}
		if err != nil {
			log.Printf("explosions: %v", err)
			return
		}
		b.rend.SetSheet(particles.SheetExplosion, img)
		b.off = false
	}
	if b.off {
		return
	}
	name := "largeexplode"
	if radius >= hugeBlastRadius {
		name = "hugeexplosion"
	}
	b.sys.Cam = [3]float64{g.Cam.X, g.Cam.Y, g.Cam.Z}
	b.sys.Spawn(name, at[0], at[1], at[2], 1, 0, 0, b.sys.Cam)
}

// tickBlasts runs the explosions' particles a tick.
func (g *game) tickBlasts() {
	g.blasts.sys.Cam = [3]float64{g.Cam.X, g.Cam.Y, g.Cam.Z}
	g.blasts.sys.TickFX()
}

// drawBlasts draws them full bright (EntityLargeExplodeFX's 240).
func (g *game) drawBlasts(cam *render.Camera, alpha float64) {
	b := &g.blasts
	if b.rend == nil || len(b.sys.FX) == 0 {
		return
	}
	white := func(x, y, z float64) [3]float32 { return [3]float32{1, 1, 1} }
	boost := func(x, y, z float64, _, _ float32) [3]float32 { return [3]float32{1, 1, 1} }
	b.rend.Draw(cam, b.sys.Sprites(nil, alpha, white, boost), alpha, nil)
}

// vanillaCrack is the client jar's destroy_stage texture %d.
const vanillaCrack = "assets/minecraft/textures/blocks/destroy_stage_%d.png"

// cracks are the breaking cracks: the client jar's (RenderGlobal's
// destroyBlockIcons), else the pack's (respack draws its own where it has
// none).
func (g *game) cracks() []image.Image {
	imgs := slices.Clone(g.crackTex)
	for i := range imgs {
		img, err := g.jarImage(fmt.Sprintf(vanillaCrack, i))
		if err != nil {
			log.Printf("cracks: %v (the pack's instead)", err)
			return g.crackTex
		}
		imgs[i] = img
	}
	return imgs
}

// jarImage reads a PNG from the client jar (-jar).
func (g *game) jarImage(name string) (image.Image, error) {
	if g.jar == nil {
		return nil, fs.ErrNotExist
	}
	f, err := g.jar.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}
