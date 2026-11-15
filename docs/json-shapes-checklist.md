# JSON block-model shapes — handover checklist

Goal: VoxelLib draws every block from its resource pack's JSON block model
(blockstates + models, variants and multipart), and JSON models are the
only source format for block shapes. `voxel.Box` stays as the baked runtime
form (what JSON compiles to); the hand-coded Go shapes picked by block name
(`voxel/shapes.go` shapeVariants) go away in favour of JSON models
(vanillapack generates them for vanilla names).

Branch `json-shapes` (worktree .claude/worktrees/json-shapes), off lan-multiplayer.

- [x] A1 One resolver: voxel.ModelResolver returns a Model {Shape, Tex, Boxes, Variants, Cull, Tinted}; FaceResolver folded in
- [x] A2 respack: non-cube/non-cross models → boxes (ModelBoxes), blockstate x/y rotation + uvlock
- [x] A3 respack: multipart blockstates evaluated on the state's props
- [x] A4 Stairs: Variants by shape for neighbour-driven stair shapes
- [x] A5 Registry uses resolver boxes for ShapeBoxes and for unknown names
- [ ] B1 iron_spikes (VoxelLib's own block) as a JSON model
- [ ] B2 Delete shapeVariants; registries without a resolver get vanillapack geometry
- [ ] B3 Callers (explorer, mpbot, voxserver, VoxelEdit) wired; tests green
- [ ] B4 Merge into lan-multiplayer

Notes:
- Blocks drawn by block-entity renderers (decorated_pot, chests, signs in 1.13+ jars: model builtin/entity, no elements) have no JSON geometry; resolver returns ok=false for them.
- Zero-volume boxes (plant planes) never collide.
- uvlock on non-RenderBlocks boxes turns textures with the box (LockBoxes only re-maps RenderBlocks-default UVs): unverified against the reference.
- app.Input.Cursor added (bird's-eye style screens need the cursor before it moves).
