# voxellib

A self-contained Minecraft-compatible voxel engine in Go (go-gl/gl 3.3 core, GLFW 3.3, mathgl).

## Packages (`voxel/...`)
| package | purpose |
|---|---|
| `voxel` | Block-state registry, 16³ paletted sections, chunks, world, sky/block light BFS |
| `voxel/anvil` | NBT, region files (.mca) and a chunk codec for 1.18+, 1.13–1.17 and legacy numeric IDs |
| `voxel/mcres` | Resource packs (dirs, zips, jars, stacked), blockstates and models |
| `voxel/gen` | Overworld-like generator: noise splines, caves, ores, trees |
| `voxel/mesh` | Culled/greedy mesher, smooth light and AO, cutout/translucent layers, VisGraph |
| `voxel/render` | Texture-array, region-VBO multidraw renderer with occlusion and frustum culling, fog and sky |
| `voxel/physics` | DDA raycast and Minecraft-style AABB movement |
| `voxel/engine` | Ties it together: async streaming, lighting, meshing, saving |

## Demo
    go run ./cmd/rpg16pack            # builds assets/rpg16pack from ~/Downloads/rpg16
    go run ./cmd/explorer             # world saved to saves/demo
    go run ./cmd/explorer -world ~/Library/Application\ Support/minecraft/saves/X -readonly

Controls: WASD, mouse, Space jump (double-tap or F to fly), Shift sneak/descend, Ctrl sprint,
LMB break, RMB place, 1-9/scroll select block, N day/night, Esc quit.

Debug flags: `-verbose` prints per-phase worst times each 0.5s, including the
worst CPU work per frame (`cpu`, everything before SwapBuffers) and GPU time
(`gpu`, timer query). `-autofly N` flies forward at N blocks/s (vanilla
creative sprint-flying is ~22). `-vsync on|limit|off`.

Frame-time design notes: the render thread never takes the world write lock
(chunk inserts/unloads and light passes run on one worker, light passes only
under the read lock); GPU uploads go to freshly allocated spans with
unsynchronized mapping and freed spans are retired for 4 frames; region
buffers add fixed 2 MiB pages instead of growing; worker threads run at a
lower macOS QoS than the render thread.

Textures: RPG16 pack, CC BY-SA (see assets/rpg16pack/CREDITS.txt).
