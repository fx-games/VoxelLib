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

Step A merged into lan-multiplayer (6c37edf). Blockers found for step B
(deleting shapeVariants), checked with vanillapack over Pixel Perfection:
- vanillapack generates no blockstates for oak_stairs, oak_fence (and their
  families), rope_ladder-style custom names, iron_spikes, redstone_wire.
- chest (and decorated_pot, signs in modern jars) are block-entity rendered:
  no JSON geometry; VoxelLib's chest/sign boxes exist only in shapes.go.
- States registered without properties (Registry.Default, the picker,
  1.7-style code) match no "k=v" variant (farmland needs moisture=…):
  a JSON-only registry needs each block's default property values.
- Registries without a resolver (voxserver, mpbot, explorer multiplayer,
  engine default) need a geometry source (vanillapack) for stair/slab
  collision.
- The explorer follows the 1.7 reference look; shapes.go carries 1.7
  RenderBlocks UVs that vanillapack's modern models may not reproduce.

## One authoritative shape collection (branch entity-models)

Goal (Felix): one shape collection in VoxelLib everyone uses, no gaps:
every map with every block draws. Format: JSON blockstates/models only.
Sources: the resource pack (jar) for everything it has geometry for;
`voxel/shapes/` (embedded JSON) for what it lacks; `voxel/shapes/empty.json`
for states that draw nothing on purpose. Gaps are found by
`TestShapeCoverage` (VOXELLIB_JAR=<client jar>): every state of every jar
blockstate must resolve to geometry with existing textures.

- [x] C1 Fallback collection embedded in respack, consulted after the pack
- [x] C2 TestShapeCoverage (gap list); 26.3 jar: 148 → 85 gaps
- [x] C3 Bare face texture names ("all", heavy_core) resolve as variables
- [x] C4 Intentionally empty models (pitcher_crop upper 0-2) listed in empty.json
- [x] C5 decorated_pot — copied from DecoratedPotRenderer, checked in a VoxelEdit shot
- [x] C6 chest, trapped_chest, ender_chest, 8 copper chests (single + double halves) — copied from ChestModel; single chests checked in a shot, double halves not yet
- [x] C6a Vertex UVs 16-bit fixed point (1/2048 tile, 5-word vertex): fractional model UVs no longer rounded (the "gaps")
- [ ] C7 17 shulker boxes
- [ ] C8 32 banners (standing rotation 0-15 + wall)
- [ ] C9 12 heads/skulls (standing rotation + wall)
- [ ] C10 end_portal, end_gateway, conduit, 8 copper golem statues
- [ ] C11 Move shapes.go Go shapes (VoxelLib-only blocks: iron_spikes, custom ladders) into the collection; delete shapeVariants (= B1/B2)
- [ ] C12 Runtime: an unresolved state is logged once with its name (no silent placeholder)
- [ ] C13 Packs lacking textures/entity (old VoxelEdit builds): source for them

Shapes are copied from the game, never hand-derived: `tools/shapegen/run.sh`
bakes each renderer's model layer (Dump.java, run against the installed
client) with the renderer's own placement transform and converts the faces
to JSON (tojson.py solves each face's uv/rotation from the game's vertices).
Add a family there (a dump loop + a small generator script), then rerun.
Positions are meshed at 1/128 block: the pot neck's 4.1 px becomes 4.125.

## Texture memory: sprites instead of one layer size (Felix: "Do it")

Goal: entity sheets (64 px) must not inflate every block texture, and fine
model UVs must not cost a 5th vertex word. How: like the game's atlas
sprites. (1) Each texture is a tile of its own size inside the array's
layers; its layer-map entry names layer, tile grid and tile. (2) Faces with
fractional UVs use a region: a registry texture entry {base texture, rect}
whose layer-map entry carries the rect; the vertex keeps the old 4-word
format with 0..16 corner UVs. Index budget: 4096 textures + regions
(26.3: 1311 textures, 923 fractional rects); overflow panics loudly.

- [x] T1 Revert the 5-word vertex (keep the DecodeVertex test migration)
- [ ] T2 Layer map entries RGBA32: code (layer, grid, tile) + region rect
- [ ] T3 Per-texture tile size: layers sized max(largest, 128), textures packed at native size
- [ ] T4 blockTex: always the grad path (per-entry grid is not uniform)
- [ ] T5 Registry: fractional box faces → region entries (typed), corner UVs 0..16
- [ ] T6 Vertex shaders apply the region rect
- [ ] T7 Tests (texarray, mesh, registry), VoxelEdit shot of chests/pot, memory stat before/after
