# Architecture

## Layers
```
cmd/explorer ── demo: items, creatures, combat, crossbow, torches, signs (hooks on app)
   │
app ─────────── game shell: window, input mapping, 20 Hz tick + render loop,
   │            player (controller, camera, hand, reach, break/place), entities,
   │            save/load glue, screens/HUD hooks, sound hooks
   │
   ├── character        standalone controller (needs only a World interface)
   ├── items            drops + pickup rule (needs only collide.Solids)
   ├── mobs             creature behaviour (needs only collide.Solids)
   ├── actors           NPCs/monsters/mounts on character physics, nav, projectile
   ├── quest            quests, dialogs, triggers, Go hooks (stdlib only)
   ├── projectile       arrows: ballistic flight, sticking in blocks and targets
   ├── audio            Minecraft-1.7.10-style sound: mixer on oto, Ogg Vorbis, sounds.json events, music ticker
   ├── importer/roblox, importer/mcmodel → model (+ model/gltf), via importer/packfs
   ├── items/extrude    sprite → mesh (pure)        items/sheet  sheet cutter (pure)
   ├── worldgen/...     terrain generators (implement worldgen.Generator)
   └── voxel/engine     streaming + threads
          ├── voxel/render   WebGPU: terrain, sky, lines, items
          ├── voxel/voxelmap VoxelMap reader (ids.json, blocks.json, models, regions)
          ├── voxel/mesh     section → vertices (pure)
          ├── voxel/anvil    load/save .mca
          ├── voxel/mcres    resource packs
          ├── voxel/physics  raycast       voxel/collide  AABB sweep
          ├── voxel/nav      A* pathfinding (stdlib only)
          └── voxel          registry, chunks, world, light
```
Rule: lower layers never import higher ones. `character`, `items` and
`worldgen` know nothing about the engine. `app` adapts the engine to
them, as `app.Terrain` does in `app/player.go`.

## Game shell (`app`)
A game fills `app.Config` (window, `engine.Config`, debug switches) and `app.Hooks`
(nil hooks are skipped), then calls `Run`. Each session (`Run`, and again after
`NewGame`) creates the engine, `App.Models` (a shared `render.ModelRenderer`) and the
player, then calls `Hooks.Start` and `Hooks.Load` with the slot's blob. The loop: poll
input → ticks at 20 Hz (screens' `Update`; unless paused: player, entities,
`Hooks.Tick`) → `Engine.Update` → `Hooks.Render(alpha, &opt)` → terrain, block outline,
entities, `Hooks.Draw` (hand last) → crosshair, `HUD`, screens → present.
Input goes to the top `Screen` while one is open (the game is paused, the mouse
free); otherwise bound actions go to `Hooks.Action` first, then the app (move, jump,
attack/use with `Hooks.BlockInteract`/`EntityInteract`, `BreakTicks`, `HeldBlock`,
pause, hotbar, F2/F3). The save blob is JSON: the player plus the game's own bytes.

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
   With a VoxelMap (`Config.Map`) a chunk is the map's region chunk, else the generator's,
   with the save slot's edited sections laid over it (`save.go`).
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
- **`inventory`, `crafting`, `survival`.** Pure 1.7.10 (+Forge) rules, no rendering: item
  registry and stacks, containers with exact `slotClick`; shaped/shapeless/repair recipes and
  the furnace, loaded from JSON (`crafting.File`); health, damage, armour, food, fire, air,
  falls, break speed and drops. The game wires world drops (`items`) to `Player.Add` and
  `Container.Drop`. Tests check vanilla numbers.
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
  (`cmd/explorer/import.go`). `Hostile` mobs chase a goal and run an attack cycle
  (`Attack` state, `Strike` on the hit tick); weapons, clips and damage live in
  `cmd/explorer/combat.go`.
- **`actors`**: a `Manager` ticks `Actor`s (bodies are `character.Character`s; AI walkers
  push with speed², as 1.7.10's move helper). Brains (stand, wander, follow in formation on the
  leader's track, routes with A* legs and door opening/closing, guard, flee), 1.7.10 combat (hurt
  window, knockback, armour, melee cooldown/reach, EntityAIArrowAttack via `projectile`), factions
  with player standing, riding with the horse jump bar, a `Spawner` for game spawn tables, and
  `Save`/`Load`. Only `Travel` (the story's group skip) moves actors without walking.
- **`quest`**: an `Engine` runs a `Book` (quests, dialogs, conditions, triggers, areas) for one
  player against the game's `Env`; dialogs go to a `Presenter` (the ui), named Go hooks do what
  map scripts did, and `State` is the save blob part.

## Audio (`audio`)
- **Mixer** (oto, 44.1 kHz stereo float): voices with volume, pitch (linear-interpolated resampling,
  clamped 0.5..2), linear attenuation over 16 × max(1, volume) blocks, equal-power pan for mono
  sources (stereo is not spatialized, as in OpenAL), live category volumes (`master` scales all).
  28 buffered + 4 streamed voices; the oldest is dropped. `Mixer.Read` is testable without a device.
- **Registry**: sounds.json semantics of 1.7.10's SoundHandler (merge in pack order, `replace`,
  categories, weighted variants, `type: event` references, whole-file rejection on errors).
- **Sounds**: `Open(dir, mixer)` reads a merged `sounds.json` (+ `blocksounds.json`);
  `Play(event, audio.At(x, y, z, vol, pitch))` / `audio.UI(vol, pitch)`; files decode in the
  background and are cached, `stream` entries decode while playing. `Tick()` releases delayed sounds.
- **MusicTicker**: 1.7.10 MusicTicker timing (first track after 100 ticks, random delay after each
  track, type change stops the track) with a `Pick` hook per situation; `MusicPool` is a weighted,
  tagged track list for region music.

## Assets
- `assets/rpg16pack` is a Minecraft resource pack generated by `cmd/rpg16pack`.
- `assets/items/*.png` holds the item sprites generated by `cmd/itemsheet`.
- `assets/cleannrooster` is a vendored subset (bb, anim, item) of the cleannrooster asset repo.
