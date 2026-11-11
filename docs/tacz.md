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
- [x] weapon: Gun spec + GunState cycle, DamageAt, Spread, Recoil (TACZ spline-delta camera recoil; tests)
- [x] explorer: first-person model with clips, keys V (next gun), R, H, B; -gun, -press
- [x] arms at lefthand_pos/righthand_pos as TACZ draws them (renderHand pose, player skin, mp.Body.FirstPersonArms)
- [x] explorer: TACZ GunHudOverlay (hud sprite, ammo, stock, fire mode icon; assets/tacz/hud from the mod); no hotbar in the explorer, so slot icons stay unused
- [x] explorer: hitscan pellets with spread vs creatures (headshot multiplier, falloff), recoil kick, ADS zoom
- [x] audio: pack sounds (cues per clip, shoot, dry); 172 cues name sounds missing from the pack
- [x] runtime bone visibility: bullet_in_mag/bullet_in_barrel/bullet_chain with ammo, additional_magazine while keyed
- [x] explorer: zombie waves (vanilla biped in the zombie skin posed as ZombieModel, always hostile, bare-handed melee), ammo loot (a magazine of the held gun's ammo per kill, into the stock); crossbow and guns exclusive (no arrows from guns)
- [x] muzzle flash and smoke: TACZ "renew" particle definitions on the particles package (hand space + world)
- [x] animation tracks: static_idle base, main track override, blended shoot per shot (model.Animator layers)
- [x] shells: ShellRender per shot at shell/shell_N bones (display's shell model, else the ammo's), camera space from the bone's first-frame pose
- [x] attachments: builtin_attachments shown as TACZ shows them (adapters, <type>_pos grafts via model.Graft, bone visibility), scope aiming (scope_view_N, zoom, idle_scope_height_adjust)
- [x] zoom as CameraSetupEvent: world FOV magnificationToFov(1+(zoom−1)·easeInExpo(aim)), hand FOV model_fov→scope views_fov/fov (else zoom_model_fov), both via SecondOrderDynamics(0.5,1.2,0.5); hand pose via AIMING_DYNAMICS(1.2,1.2,0.5). Verified: SVD 8.25×, AUG 4.25× reticle; P90 sight reticle faint (unverified against the mod)
- [x] scope lens parts split out of the gun (tacz.Lens: ring, body, oculars, divisions on the gun skeleton); ring + body drawn plainly
- [x] scope stencil passes: renderScope/renderBoth/renderSight order on render.HandStencil (cmd/explorer/gunlens.go); view disc at the ocular depth with the reference angular size (80/90·aim)
- [x] heat (GunHeatData + sp_heat.lua: lock, cooling·t², rpm/inaccuracy factors; HeatBarOverlay)
- [ ] TACZ movement/camera tracks, missing sounds, guns in multiplayer, README/ARCHITECTURE
