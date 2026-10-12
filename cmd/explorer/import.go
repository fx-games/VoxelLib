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
	g.setupCrossbow(p)
	g.packFrom = len(g.itemNames)
	for _, it := range p.Items {
		g.addItem(it.Name, it.Image)
	}
	log.Printf("mcassets: %d mobs, %d props, %d items, %d animations from %s",
		len(walkers), len(props), len(p.Items), len(p.Animations), src)
}

// humanoidClip picks clips from the shared humanoid animation library.
// Most animation.mob.* clips are a combat stance (body turned ~27° with the
// head turned back), slashcombo/leapslam leap 2-3 blocks into the air,
// flourish2 whips the body round twice with an elastic overshoot and
// running leans the torso 25° with the legs only roughly following. On
// wandering mobs all of these read as body parts flying around, so they
// stick to the calm idle and walking cycles.
func humanoidClip(m *model.Model, s mobs.State) string {
	prefer := map[mobs.State][]string{
		mobs.Idle:  {"idle", "animation.mob.idle"},
		mobs.Walk:  {"walking", "animation.mob.walk"},
		mobs.Run:   {"walking", "animation.mob.walk"},
		mobs.Jump:  {"walking", "animation.mob.walk"}, // hops up a block: no leap clip
		mobs.Trick: {"idle", "animation.mob.idle"},
	}[s]
	for _, n := range prefer {
		if _, ok := m.Clips[n]; ok {
			return n
		}
	}
	return clipByName(m, s)
}

// propClip loops a prop's own idle clip. Props without one (their idle is
// often a zero-length "hold the rest pose" clip) stand in the rest pose:
// their other clips are one-shot attacks and openings.
func propClip(m *model.Model, s mobs.State) string {
	for _, n := range m.ClipNames() {
		if strings.Contains(strings.ToLower(n), "idle") {
			return n
		}
	}
	return ""
}
