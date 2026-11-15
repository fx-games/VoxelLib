# JSON block-model shapes — handover checklist

Goal: VoxelLib draws every block from its resource pack's JSON block model
(blockstates + models, variants and multipart), and JSON models are the
only source format for block shapes. `voxel.Box` stays as the baked runtime
form (what JSON compiles to); the hand-coded Go shapes picked by block name
(`voxel/shapes.go` shapeVariants) go away in favour of JSON models
(vanillapack generates them for vanilla names).

Branch `json-shapes` (worktree .claude/worktrees/json-shapes), off lan-multiplayer.

- [ ] A1 One resolver: voxel.ModelResolver returns a Model {Shape, Tex, Boxes, Variants, Cull, Tinted}; FaceResolver folded in
- [ ] A2 respack: non-cube/non-cross models → boxes (ModelBoxes), blockstate x/y rotation + uvlock
- [ ] A3 respack: multipart blockstates evaluated on the state's props
- [ ] A4 Stairs: Variants by shape for neighbour-driven stair shapes
- [ ] A5 Registry uses resolver boxes for ShapeBoxes and for unknown names
- [ ] B1 iron_spikes (VoxelLib's own block) as a JSON model
- [ ] B2 Delete shapeVariants; registries without a resolver get vanillapack geometry
- [ ] B3 Callers (explorer, mpbot, voxserver, VoxelEdit) wired; tests green
- [ ] B4 Merge into lan-multiplayer
