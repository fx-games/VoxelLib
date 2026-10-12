# Architecture

## Layers
```
cmd/explorer ── demo glue: input, character, items, frame loop
   │
   ├── character        standalone controller (needs only a World interface)
   ├── items            drops + pickup rule (needs only collide.Solids)
   ├── mobs             creature behaviour (needs only collide.Solids)
   ├── projectile       arrows: ballistic flight, sticking in blocks and targets
   ├── importer/roblox, importer/mcmodel → model (+ model/gltf), via importer/packfs
   ├── items/extrude    sprite → mesh (pure)        items/sheet  sheet cutter (pure)
   ├── worldgen/...     terrain generators (implement worldgen.Generator)
   └── voxel/engine     streaming + threads
          ├── voxel/render   GL: terrain, sky, lines, items
          ├── voxel/mesh     section → vertices (pure)
          ├── voxel/anvil    load/save .mca
          ├── voxel/mcres    resource packs
          ├── voxel/physics  raycast       voxel/collide  AABB sweep
          └── voxel          registry, chunks, world, light
```
Rule: lower layers never import higher ones. `character`, `items` and
`worldgen` know nothing about the engine. The explorer adapts the engine to
them, as `terrain` does in `cmd/explorer/player.go`.

## Data model (`voxel`)
- **Block states.** A `StateID` (uint32) indexes the `Registry` table of
  `BlockInfo`: name, shape, layer, opacity, emission, textures. Names follow Minecraft
  (`minecraft:oak_log[axis=y]`). Unknown blocks are resolved from the resource pack.
- **Chunks.** A chunk is a 16×H×16 column of 16³ sections. Sections store a palette
  plus packed indices, and nibble sky and block light.
- **World.** A map of chunks behind an RWMutex. The mesher and light passes take the read
  lock once and use the `*Unlocked` accessors. `SetBlock` takes the write lock and
  updates the heightmap, light and dirty sections.
- **Light.** Minecraft-style BFS for sky light and block light, including removal.

## Engine pipeline (`voxel/engine`)
The main thread calls `Update(x,y,z)` then `Render(cam)` each frame.
1. The **load workers** read a chunk from `WorldDir`, or generate it with the `Generator`.
2. The **light worker** owns every world mutation from streaming (`worldOp` channel)
   and lights a chunk once its neighbours exist.
3. The **mesh workers** mesh dirty sections whose 3×3 neighbourhood is lit.
4. The **main thread** uploads meshes to region pages and renders.

The main thread never takes the world write lock, except for player `SetBlock`. Frame-time
design is covered in README.md and the comments in engine.go and render/region.go.

## Rendering (`voxel/render`)
- **Texture array.** It holds one layer per block texture. Animated textures (`.png.mcmeta`)
  store extra frames as extra layers. A 1-row `R16UI` layer map, updated at 20 Hz, redirects
  each base layer to its current frame in the vertex shader.
- **Vertices.** A vertex is 16 bytes (see mesh/vertex.go). Sections are grouped in 8×4×8
  regions, and each region uses one 2 MiB VBO page and one multidraw per layer.
- **Layers and culling.** Solid, cutout and translucent layers are drawn in that order, with
  translucent quads sorted back to front. Culling is a VisGraph occlusion BFS plus the frustum.
- **Fog.** `Options.Fog` overrides the distance fog, for example under water. `CurrentFog()`
  exposes the active fog to other passes.
- **Items.** `ItemRenderer` extrudes sprites with `items/extrude` into one VBO and one texture
  array. It draws world instances, and the first-person hand with its own projection after a
  depth clear.

## Gameplay pieces
- **`character`.** A Minecraft movement model at 20 Hz: ground and air drag, sprint-jump,
  creative flight, water (sprint-swim follows the look direction), lava, climbing out of
  liquids, and sneak edge protection. The input is a plain struct. Tests check the vanilla
  speeds.
- **`items`.** `World.Tick` runs the drop physics and `Collect` implements the swap rule.
  A swapped-out drop waits until the player leaves it, so it can't bounce back into the hand.
- **`worldgen`.** Implement `Generate(w, chunk)` by filling blocks only; the engine lights the
  chunk. It runs concurrently, so a generator must be deterministic and safe for concurrent use.

## Animated models
- **`model`**: the shared in-memory form. Bones (TRS, or Euler for Bedrock rigs), skinned
  vertices, and clips made of channels. A channel either replaces the bind pose (glTF) or adds
  to it (Bedrock: offsets, Euler degrees, scale factors). `Animator` plays clips with
  crossfades and returns skinning matrices. `Model.Transform` bakes rescaling and turning.
- **Importers** only build `model.Model`s.
  - `roblox`: GLB/glTF packs.
  - `mcmodel`: Blockbench, Bedrock geometry and animations. A keyframe component may be a Molang
    expression (`molang.go`), and animation files form a library. Each animation is bound by bone
    name to every model where at least half its bones match, with humanoid aliases such as
    `bipedHead` = `head`.
- **`render.ModelRenderer`**: GPU skinning (up to 96 bones), terrain light and fog, frustum culling.
- **`mobs`**: behaviour states. The explorer maps states to clip names per species
  (`cmd/explorer/import.go`).

## Assets
- `assets/rpg16pack` is a Minecraft resource pack generated by `cmd/rpg16pack`.
- `assets/items/*.png` holds the item sprites generated by `cmd/itemsheet`.
- `assets/cleannrooster` is a vendored subset (bb, anim, item) of the cleannrooster asset repo.
