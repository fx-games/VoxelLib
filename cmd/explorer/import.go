package main

import (
	"log"
	"os"
	"strings"

	"github.com/memmaker/voxellib/importer/mcmodel"
	"github.com/memmaker/voxellib/importer/roblox"
	"github.com/memmaker/voxellib/mobs"
	"github.com/memmaker/voxellib/model"
)

// importPets loads a Roblox-style creature pack; a missing pack is fine.
// Winged models fly, the rest walk; their Idle/Walk/Run/Jump/Trick clips
// map one to one onto the behaviour states.
func (g *game) importPets(src string) {
	if _, err := os.Stat(src); err != nil {
		log.Printf("pets: %s not found, skipping", src)
		return
	}
	ms, err := roblox.Import(src)
	if err != nil {
		log.Printf("pets: %v", err)
		return
	}
	g.addSpecies(ms, clipByName)
	log.Printf("pets: imported %d models from %s", len(ms), src)
}

// importMCAssets loads a Minecraft-style pack: humanoid mobs wander with
// the shared humanoid animations, other models stand around as animated
// props, and item textures join the pickup items.
func (g *game) importMCAssets(src string) {
	if _, err := os.Stat(src); err != nil {
		log.Printf("mcassets: %s not found, skipping", src)
		return
	}
	p, err := mcmodel.Import(src)
	if err != nil {
		log.Printf("mcassets: %v", err)
		return
	}
	for _, s := range p.Skipped {
		log.Printf("mcassets: skipped %s", s)
	}
	var walkers, props []*model.Model
	for _, m := range p.Models {
		lo, hi := m.Bounds()
		switch {
		case m.Tags["armor"]: // hollow wearable sets
		case m.Tags["humanoid"] && lo[1] < 0.2: // full bodies, not floating pieces
			walkers = append(walkers, m)
		case hi[1]-lo[1] > 0.6 && len(m.Clips) > 0:
			props = append(props, m)
		}
	}
	g.addSpecies(walkers, humanoidClip)
	n := len(g.species)
	g.addSpecies(props, propClip)
	for _, sp := range g.species[n:] {
		sp.static = true
	}
	g.packFrom = len(g.itemNames)
	for _, it := range p.Items {
		g.addItem(it.Name, it.Image)
	}
	log.Printf("mcassets: %d mobs, %d props, %d items, %d animations from %s",
		len(walkers), len(props), len(p.Items), len(p.Animations), src)
}

// humanoidClip picks clips from the shared humanoid animation library.
func humanoidClip(m *model.Model, s mobs.State) string {
	prefer := map[mobs.State][]string{
		mobs.Idle:  {"animation.mob.idle", "idle", "animation.unknown.idle"},
		mobs.Walk:  {"animation.mob.walk", "walking", "animation.unknown.walk"},
		mobs.Run:   {"running", "animation.merc.heavy_run", "animation.mob.walk"},
		mobs.Jump:  {"animation.mob.jugg.leapslam"},
		mobs.Trick: {"animation.mob.slashcombo", "animation.mob.wave", "animation.mob.trickster.flourish2"},
	}[s]
	for _, n := range prefer {
		if _, ok := m.Clips[n]; ok {
			return n
		}
	}
	return clipByName(m, s)
}

// propClip loops a prop's idle-ish clip (or its first one) as its "trick".
func propClip(m *model.Model, s mobs.State) string {
	names := m.ClipNames()
	for _, n := range names {
		if strings.Contains(strings.ToLower(n), "idle") {
			return n
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return ""
}
