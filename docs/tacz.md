# TACZ guns → VoxelLib (zombie apocalypse demo)

Goal: the explorer demo becomes a zombie apocalypse with every weapon of the TACZ
default gun pack (github.com/Sh1roCu/TACZ-Refabricated, branch 1.20.1,
`src/main/resources/assets/tacz/custom/tacz_default_gun`), next to the crossbow.

How: `cmd/importtacz` converts the pack once into committed assets in `assets/tacz`
(private repo, private use). Models + animations become glTF (Bedrock additive
clips baked to absolute keyframes), gun stats become our own typed JSON, textures
and sounds are copied. The runtime never reads TACZ formats.
Lua state machines are not run; `weapon` implements the default gun cycle
(draw, idle, shoot, reload tactical/empty, inspect, put away) natively.

## Checklist
- [x] model: bake additive/Molang clips to absolute keyframes (`model.Bake`)
- [x] importer/tacz: read pack (JSON with comments): gun index, data, display, ammo
- [x] cmd/importtacz: write assets/tacz (gltf, guns.json, slot icons, sounds, en_us names)
- [ ] weapon: Gun spec + gun state (fire modes, rpm, mag, reload timing, damage falloff, recoil, spread)
- [ ] explorer: guns as hotbar items, first-person model with clips, sounds, hitscan on creatures, muzzle flash
- [ ] explorer: zombies (hostile, waves at night/around player), ammo loot
- [ ] attachments (scopes, mags, grips) — later
