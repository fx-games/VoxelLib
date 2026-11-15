// Package shapes is VoxelLib's authoritative block-shape collection, in
// resource-pack JSON (blockstates and block models): the shapes resource
// packs lack, i.e. the blocks the game draws as block entities (decorated
// pots, chests, ...), over their entity textures (textures/entity/...) read
// from the pack like any other. respack consults it after the pack.
// empty.json lists the states that draw nothing on purpose.
// respack's TestShapeCoverage names every gap left.
package shapes

import "embed"

// FS holds assets/<ns>/{blockstates,models}/... and empty.json.
//
//go:embed assets empty.json
var FS embed.FS
