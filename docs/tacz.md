# TACZ guns → VoxelLib (zombie apocalypse demo)

Goal: the explorer demo becomes a zombie apocalypse with every weapon of the TACZ
default gun pack (github.com/Sh1roCu/TACZ-Refabricated, branch 1.20.1, commit c27bbbd),
next to the crossbow.

How: the pack is committed verbatim in its original formats (approved exception to the
glTF-only rule, so animations play exactly as authored and other TACZ packs drop in):
- `assets/tacz/tacz_default_gun` = `src/main/resources/assets/tacz/custom/tacz_default_gun`
- `assets/tacz/defaults` = the mod's `src/main/resources/assets/tacz/animations/*_default.animation.json`

`importer/tacz.Load` reads the gun/ammo specs at startup (typed `weapon.Gun`), `Pack.Model(id)`
parses a gun's Bedrock model + animations on demand (bbmodel), with the bones TACZ hides
on a gun without attachments removed. Lua state machines are not run; `weapon.GunState`
is the gun cycle (draw, idle, shoot, bolt, reload magazine/tube/inventory, inspect, put away).

## Checklist
- [x] assets/tacz: pack copied verbatim
- [x] importer/tacz: loader (JSON with comments), lazy models, sound cues per clip
- [x] weapon: Gun spec + GunState cycle, DamageAt, Spread, Kick (tests)
- [ ] explorer: guns as hotbar items (slot icons), first-person model with clips, arms at lefthand_pos/righthand_pos
- [ ] explorer: hitscan pellets with spread vs creatures (headshot multiplier, falloff), recoil kick, ADS zoom
- [ ] audio: play pack sounds (cues per clip, shoot, dry) — audio.Sounds needs the pack's tacz_sounds layout
- [ ] runtime bone visibility: bullet_in_mag/bullet_in_barrel/bullet_chain with ammo, additional_magazine while keyed
- [ ] explorer: zombies (hostile, waves), ammo loot
- [ ] later: muzzle flash, shells, attachments (builtin stocks on AK etc. are currently stripped), minigun heat
