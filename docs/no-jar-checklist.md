# VoxelLib without a jar — handover checklist

Goal (Felix, 2026-11-15): nothing reads a Minecraft client jar at run time.
Jars are only an import source for dev tools whose output is committed.
VoxelLib ships, ready to use:
- every block shape, defined once (voxel/shapes, embedded JSON);
- one default texture pack covering every vanilla block and every shape
  (Pixel Perfection Legacy 26.3, CC BY-SA, assets/pixelperfection);
- the GUI assets its ui package draws (Felix: original Minecraft GUI is
  fine for this hobby project; Dark-Mode-GUI.zip as the baseline, the
  rest from the game);
- `SetTexturePack` to draw with another pack. A pack's own blockstates and
  models override VoxelLib's shapes for the blocks it defines (Felix's
  choice); textures it lacks fall back to the default pack.
Explorer, uidemo and VoxelEdit run without a jar. VoxelEdit's block picker
lists VoxelLib's shapes; its texture-pack picker calls SetTexturePack.

Decisions: shapes are baked from the jar by a dev tool (tools/shapegen)
and committed as flat JSON (no parent chains); vanillapack and the Go
shapes (voxel/shapes.go shapeVariants) are deleted; modern (26.x) shapes
everywhere, the explorer loses its 1.7 shapes and RenderBlocks UVs.

Branch `entity-models` (worktree .claude/worktrees/entity-models), merged
into lan-multiplayer by fast-forward at milestones. Every commit builds.

## Shapes
- [x] S1 shapegen bake: every jar blockstate + its models flattened (textures resolved, elements inline, "shape": "cross" kept) into voxel/shapes
- [ ] S2 Resolver order: the texture pack's own blockstates/models, then voxel/shapes; registries without a pack use voxel/shapes (servers, bots)
- [ ] S3 Delete shapeVariants, Go shape helpers that only fed it, and vanillapack; callers fixed
- [ ] S4 TestShapeCoverage runs on voxel/shapes alone (no jar): every block draws with default-pack textures

## Textures
- [ ] X1 Default pack embedded (Pixel Perfection) and the default TextureSource
- [ ] X2 SetTexturePack(pack): textures from the pack, missing ones from the default; shapes from the pack override (S2); renderer reloads
- [ ] X3 Coverage test: every texture any shape names exists in the default pack

## GUI assets
- [ ] G1 Inventory of what ui, explorer, uidemo read (GUI, font, lang, icons, items, particles, destroy stages)
- [ ] G2 Bundled GUI pack: Dark Mode GUI + the rest from the game (tool + committed files)
- [ ] G3 ui reads the bundled pack by default (basepack folded in or replaced)

## Programs
- [ ] P1 VoxelEdit: no jar; builds remember a texture pack choice, not a copied pack.zip; picker lists VoxelLib's shapes; pack picker via SetTexturePack
- [ ] P2 explorer runs without -jar
- [ ] P3 uidemo runs without -jar
- [ ] P4 Guard test: only tools/ (import) code opens a .jar
