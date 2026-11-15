package respack

import (
	"encoding/json"

	"github.com/memmaker/voxellib/voxel"
	"github.com/memmaker/voxellib/voxel/shapes"
)

// entityPack is VoxelLib's shape collection (voxel/shapes) as a pack: resolveState's fallback for states whose
// own model has no geometry.
var entityPack = func() *Pack {
	return FromFS(shapes.FS)
}()

// emptyStates are the states whose jar model draws nothing on purpose (an
// element-less model like the entity ones): shapes' empty.json, block state
// strings, each matching states that have all its properties.
var emptyStates = func() map[string][]map[string]string {
	data, err := shapes.FS.ReadFile("empty.json")
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
