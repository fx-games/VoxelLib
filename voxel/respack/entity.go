package respack

import (
	"embed"
	"encoding/json"
	"io/fs"

	"github.com/memmaker/voxellib/voxel"
)

// entityFS holds block models for the blocks the game draws as block
// entities (decorated pots, chests, shulker boxes): their jar models carry no
// elements, so these give them the renderer's geometry over its entity
// textures (textures/entity/...), read from the pack like any other.
//
//go:embed entity
var entityFS embed.FS

// entityPack is entityFS as a pack: resolveState's fallback for states whose
// own model has no geometry.
var entityPack = func() *Pack {
	sub, err := fs.Sub(entityFS, "entity")
	if err != nil {
		panic(err) // the embedded tree is fixed at build time
	}
	return FromFS(sub)
}()

// emptyStates are the states whose jar model draws nothing on purpose (an
// element-less model like the entity ones): entity/empty.json, block state
// strings, each matching states that have all its properties.
var emptyStates = func() map[string][]map[string]string {
	data, err := entityFS.ReadFile("entity/empty.json")
	if err != nil {
		panic(err)
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil {
		panic(err)
	}
	out := map[string][]map[string]string{}
	for _, s := range list {
		name, props := voxel.ParseStateString(s)
		out[voxel.Namespaced(name)] = append(out[voxel.Namespaced(name)], props)
	}
	return out
}()

// drawsNothing reports a state listed in emptyStates.
func drawsNothing(name string, props map[string]string) bool {
	for _, want := range emptyStates[voxel.Namespaced(name)] {
		ok := true
		for k, v := range want {
			ok = ok && props[k] == v
		}
		if ok {
			return true
		}
	}
	return false
}
