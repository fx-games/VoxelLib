#!/bin/sh
# Regenerates voxel/shapes from the game's own block-entity models: Dump.java
# bakes each model layer with the renderer's placement transform (needs a
# JDK and an installed client: $MC, default the macOS launcher folder),
# the Python scripts turn the dumped faces into block-model JSON.
set -e
MC=${MC:-"$HOME/Library/Application Support/minecraft"}
VER=${VER:-26.3}
here=$(cd "$(dirname "$0")" && pwd)
out=$(mktemp -d)
CP="$MC/versions/$VER/$VER.jar:$(python3 -I - "$MC" "$VER" <<'PY'
import json, os, sys
root, ver = sys.argv[1:]
v = json.load(open(f"{root}/versions/{ver}/{ver}.json"))
print(':'.join(p for p in (f"{root}/libraries/{l['downloads']['artifact']['path']}" for l in v['libraries'] if 'artifact' in l.get('downloads', {})) if os.path.exists(p)))
PY
)"
javac -cp "$CP" -d "$out" "$here/Dump.java"
dump() { (cd "$out" && java -cp "$out:$CP" Dump "$@"); } # the game writes logs/ into its working folder
for t in single:createSingleBodyLayer left:createDoubleBodyLeftLayer right:createDoubleBodyRightLayer; do
	for f in north east south west; do
		dump net.minecraft.client.model.object.chest.ChestModel "${t#*:}" net.minecraft.client.renderer.blockentity.ChestRenderer modelTransformation dir "$f" > "$out/chest_${t%%:*}_$f.json"
	done
done
PYTHONDONTWRITEBYTECODE=1 python3 "$here/chests.py" "$out" "$here/../../voxel/shapes/assets/minecraft"
rm -rf "$out"
