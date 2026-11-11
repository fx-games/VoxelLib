package main

// The animals: Roblox-style creature packs (the Cube Pets), imported at
// runtime.

import (
	"log"
	"os"

	"github.com/memmaker/voxellib/importer/roblox"
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
