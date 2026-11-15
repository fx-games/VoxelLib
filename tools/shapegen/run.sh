#!/bin/sh
# Regenerates voxel/shapes: bake (Go) copies the jar's blockstates and its
# block models flattened, then the game's own block-entity models: Dump.java
# bakes each model layer with the renderer's placement transform (needs a
# JDK and an installed client: $MC, default the macOS launcher folder),
# the Python scripts turn the dumped faces into block-model JSON.
# KEEP=<dir> keeps the dumps there for inspection.
set -e
MC=${MC:-"$HOME/Library/Application Support/minecraft"}
VER=${VER:-26.3}
here=$(cd "$(dirname "$0")" && pwd)
out=${KEEP:-$(mktemp -d)}
mkdir -p "$out"
CP="$MC/versions/$VER/$VER.jar:$(python3 -I - "$MC" "$VER" <<'PY'
import json, os, sys
root, ver = sys.argv[1:]
v = json.load(open(f"{root}/versions/{ver}/{ver}.json"))
print(':'.join(p for p in (f"{root}/libraries/{l['downloads']['artifact']['path']}" for l in v['libraries'] if 'artifact' in l.get('downloads', {})) if os.path.exists(p)))
PY
)"
javac -cp "$CP" -d "$out" "$here/Dump.java"
dump() { (cd "$out" && java -cp "$out:$CP" Dump "$@"); } # the game writes logs/ into its working folder
posed() { (DUMP_ANIM=$1; export DUMP_ANIM; shift; dump "$@"); } # posed <Model class> <setupAnim arg> ...: see Dump
rot16="0 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15"
be=net.minecraft.client.renderer.blockentity
model=net.minecraft.client.model

for t in single:createSingleBodyLayer left:createDoubleBodyLeftLayer right:createDoubleBodyRightLayer; do
	for f in north east south west; do
		dump $model.object.chest.ChestModel "${t#*:}" $be.ChestRenderer modelTransformation dir "$f" > "$out/chest_${t%%:*}_$f.json"
	done
done
for f in north east south west; do
	dump $be.DecoratedPotRenderer createBaseLayer $be.DecoratedPotRenderer modelTransformation dir "$f" > "$out/pot_base_$f.json"
	dump $be.DecoratedPotRenderer createSidesLayer $be.DecoratedPotRenderer modelTransformation dir "$f" > "$out/pot_sides_$f.json"
done

for f in up down north east south west; do
	posed "$be.ShulkerBoxRenderer\$ShulkerBoxModel 0" $model.monster.shulker.ShulkerModel createBoxLayer $be.ShulkerBoxRenderer modelTransform dir "$f" > "$out/shulker_$f.json"
done

dump dyes > "$out/dyes.json"
for r in $rot16; do
	dump $model.object.banner.BannerModel createBodyLayer=true $be.BannerRenderer createGroundTransformation int "$r" > "$out/banner_body_$r.json"
	dump $model.object.banner.BannerFlagModel createFlagLayer=true $be.BannerRenderer createGroundTransformation int "$r" > "$out/banner_flag_$r.json"
done
for f in north east south west; do
	dump $model.object.banner.BannerModel createBodyLayer=false $be.BannerRenderer createWallTransformation dir "$f" > "$out/wall_banner_body_$f.json"
	dump $model.object.banner.BannerFlagModel createFlagLayer=false $be.BannerRenderer createWallTransformation dir "$f" > "$out/wall_banner_flag_$f.json"
done

# skull layers as SkullBlockRenderer.createModel picks them (LayerDefinitions)
for h in mob:$model.object.skull.SkullModel:createMobHeadLayer humanoid:$model.object.skull.SkullModel:createHumanoidHeadLayer \
	dragon:$model.object.skull.DragonHeadModel:createHeadLayer piglin:$model.object.skull.PiglinHeadModel:createHeadModel; do
	k=${h%%:*}; rest=${h#*:}; cls=${rest%%:*}; fac=${rest#*:}
	for r in $rot16; do
		dump "$cls" "$fac" $be.SkullBlockRenderer createGroundTransformation int "$r" > "$out/skull_${k}_$r.json"
	done
	for f in north east south west; do
		dump "$cls" "$fac" $be.SkullBlockRenderer createWallTransformation dir "$f" > "$out/wall_skull_${k}_$f.json"
	done
done

for p in standing:createBodyLayer running:createRunningPoseBodyLayer sitting:createSittingPoseBodyLayer star:createStarPoseBodyLayer; do
	for f in north east south west; do
		posed "$model.object.statue.CopperGolemStatueModel unit" $model.animal.golem.CopperGolemModel "${p#*:}" $be.CopperGolemStatueBlockRenderer modelTransformation dir "$f" > "$out/golem_${p%%:*}_$f.json"
	done
done

dump $be.ConduitRenderer createShellLayer $be.ConduitRenderer DEFAULT_TRANSFORMATION field > "$out/conduit.json"
dump xf $be.TheEndPortalRenderer TRANSFORMATION > "$out/end_portal_xf.json"

dst="$here/../../voxel/shapes/assets/minecraft"
(cd "$here/../.." && go run ./tools/shapegen/bake "$MC/versions/$VER/$VER.jar" voxel/shapes)
for gen in chests pots shulkers banners skulls golems misc; do
	PYTHONDONTWRITEBYTECODE=1 python3 "$here/$gen.py" "$out" "$dst"
done
[ -n "$KEEP" ] || rm -rf "$out"
