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
- [x] audio: pack sounds (cues per clip, shoot, display dry_fire else tacz:dry_fire); an event "ns:path" is assets/<ns>/tacz_sounds/<path>.ogg, as TACZ registers them
- [x] missing sounds: 172 cue ids have no file at their TACZ location, so TACZ is silent for them too (listed in importer/tacz/testdata/absent_sounds.txt; TestSoundsResolve fails on any other unresolved id). 163 have no namespace (minecraft:<id>; the pack ships no assets/minecraft and the base names exist nowhere in it). 9 are pack typos whose file exists elsewhere, left as is to match TACZ: classicr:mrad/mrad_raise_first_rattle, tacz:ak74m:ak74m_inspect_tilt, tacz:hk_mp7a1/tacz:hk_mp7a1/hk_mp7a1_reload_maghit, tacz:m1014/cloth_move_3, tacz:m95/mag_drup_large_drum_dirt, tacz:qbz_191/qbz_95_reload_empty_fast_rotate, m1866_reload_loop_arm, m249_reload_empty_load_belt, spr15hb_reload_empty_maghit
- [ ] display sounds besides shoot/dry_fire (shoot_3p, draw, put_away, reload_*, inspect*, melee_*, silence, head_hit, kill ...) are not played; absent among them: <gun>_inspect/_inspect_empty/_reload_empty/_reload_tactical of ai_awp, ak47, aug, hk_g3, hk_mp5a5, m249, p90, timeless50, type_81 (fn_fal reload_*), awp_bolt, m1866/minigun silence(_3p)
- [x] runtime bone visibility: bullet_in_mag/bullet_in_barrel/bullet_chain with ammo, additional_magazine while keyed
- [x] explorer: zombie waves (vanilla biped in the zombie skin posed as ZombieModel, always hostile, bare-handed melee), ammo loot (a magazine of the held gun's ammo per kill, into the stock); crossbow and guns exclusive (no arrows from guns)
- [x] muzzle flash and smoke: TACZ "renew" particle definitions on the particles package (hand space + world)
- [x] animation tracks: static_idle base, main track override, blended shoot per shot (model.Animator layers)
- [x] shells: ShellRender per shot at shell/shell_N bones (display's shell model, else the ammo's), camera space from the bone's first-frame pose
- [x] attachments: builtin_attachments shown as TACZ shows them (adapters, <type>_pos grafts via model.Graft, bone visibility), scope aiming (scope_view_N, zoom, idle_scope_height_adjust)
- [x] zoom as CameraSetupEvent: world FOV magnificationToFov(1+(zoom−1)·easeInExpo(aim)), hand FOV model_fov→scope views_fov/fov (else zoom_model_fov), both via SecondOrderDynamics(0.5,1.2,0.5); hand pose via AIMING_DYNAMICS(1.2,1.2,0.5). Verified: SVD 8.25×, AUG 4.25× reticle; P90 sight reticle faint (unverified against the mod)
- [x] state tracks: over_heat once / static_over_heating looping while locked; minigun spin (minigun_state_machine: spin while main track idle, stacked while Firing). Unverified: barrel spin seen on screen; spin_low absent from the pack
- [x] deliberate deviation: picture-in-picture scopes (explorer -full-zoom: TACZ's whole-screen zoom, no second render, for slower machines). TACZ magnifies the whole screen; here the screen keeps the base FOV and only a scope lens's view disc shows the magnified world (App.DrawScopeView, called from the gun's hand draw: a second world + entity render into render.BeginScopeView, drawn under the lens stencil with HandStencil.ScopeView). Iron sights, sights without a view disc (P90, renderSight) and zoom without a lens are not magnified. Shader-pack frames render no scope view (their hand pass has no stencil); unverified there
- [x] scope view cost: rasterised only inside the scope oculars' screen bounds (ModelRenderer.HandBounds, a scissor on the full-size scope target, so texels stay screen pixels); no shadow/pack/light/animation work (Renderer.RenderView); its narrow frustum culls to ~45 visible sections vs ~310 in the normal view. SVD aimed, M5, uncapped (VOXELLIB_VSYNC=off) 15 s: 4.25 → 3.44 ms mean frame (-full-zoom 3.75–3.90 ms); CPU +0.7 ms over -full-zoom either way. The scope view lacks particles (torch flames, muzzle smoke, shells), arrows, signs, torches, held weapons on mobs, block outline and weather: they are game draws with the main camera (todo)
- [x] scope lens parts split out of the gun (tacz.Lens: ring, body, oculars, divisions on the gun skeleton); ring + body drawn plainly
- [x] scope stencil passes: renderScope/renderBoth/renderSight order on render.HandStencil (cmd/explorer/gunlens.go); view disc at the ocular depth with the reference angular size (80/90·aim)
- [x] heat (GunHeatData + sp_heat.lua: lock, cooling·t², rpm/inaccuracy factors; HeatBarOverlay)
- [x] movement track: movement_track_states (idle/walk/run, run_start/run/run_hold/run_end, walk_forward/backward/sideway/aiming, walkDist progress) on a blended layer with per-track transitions (model.Layer.Run); camera bone turns world and hand views (cmd/explorer/gunmove.go). Verified: transitions traced against the Lua, sprint pose screenshot, draw camera angles match the clip keys. Unverified: TACZ's Java input triggers (INPUT_RUN = sprinting, WALK = any move key) and camera axis signs/multiplier (no Java source here; derived as the viewer riding the camera bone); progress not set during a transition; slide track not done
- [x] guns in LAN multiplayer (-host/-connect, voxserver): the held gun is PlayerInfo.Gun (client msgHold, server rebroadcasts the info); a round is a reliable mp.Shot (gun ID + each pellet's yaw/pitch, spread rolled by the shooter) that the server casts as hitscan from the shooter's eye against the lag-compensated players (Server.targets, shared with arrows), dealing DamageAt × headshot through Server.damage (shared with arrows: knockback, kill feed, scores); everyone else gets msgFired and plays the shoot sound at the shooter (64 blocks) and a world-space muzzle flash. Other players' guns are the first-person model at static_idle, turned to their look, scaled 0.4. ServerConfig.Guns: the explorer host passes its pack, voxserver loads tacz.LoadDefault. Verified: mp TestGunShot/TestShotEncoding; host+client run (client's AK-47 killed the host server-side, host got 30 Fired). Unverified: remote gun pose seen from the side, remote flash/sound on screen/speakers. Trusted from the client (LAN): ammo, fire rate, spread. Multiplayer has no creatures, zombie waves or ammo loot (single player only), so nothing there to sync
- Fixed with it: single-player pellets flew mirrored in X (−sin yaw; now mp.Look.Dir = render.Camera.Forward). Check for the class: `grep -rn "\-math.Sin(.*[yY]aw" --include=*.go` — a direction (not a "back" offset) with −sin yaw is wrong
- [x] README/ARCHITECTURE
- [ ] pre-existing: the plain-skin first-person arm shows as a long blue band in front of the gun (also in single player)
