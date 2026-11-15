# Writes voxel/shapes copper golem statues from the game's CopperGolemModel
# pose layers (as CopperGolemStatueModel poses them) placed by
# CopperGolemStatueBlockRenderer.modelTransformation (dump/golem_*.json).
import json, sys
from tojson import elements
dumps, out = sys.argv[1], sys.argv[2]
poses = ["standing", "running", "sitting", "star"]
facings = ["north", "east", "south", "west"]
for p in poses:
    for f in facings:
        json.dump({"textures": {"particle": "#golem"},
                   "elements": elements(json.load(open(f"{dumps}/golem_{p}_{f}.json")), "#golem")},
                  open(f"{out}/models/block/template_copper_golem_statue_{p}_{f}.json", "w"), indent=1)
n = 0
for ox, sheet in [("", ""), ("exposed_", "_exposed"), ("weathered_", "_weathered"), ("oxidized_", "_oxidized")]:
    for wax in ["", "waxed_"]:
        name = f"{wax}{ox}copper_golem_statue"
        variants = {}
        for p in poses:
            for f in facings:
                model = f"{name}_{p}_{f}"
                json.dump({"parent": f"minecraft:block/template_copper_golem_statue_{p}_{f}",
                           "textures": {"golem": f"minecraft:entity/copper_golem/copper_golem{sheet}"}},
                          open(f"{out}/models/block/{model}.json", "w"))
                variants[f"copper_golem_pose={p},facing={f}"] = {"model": f"minecraft:block/{model}"}
        json.dump({"variants": variants}, open(f"{out}/blockstates/{name}.json", "w"), indent=1)
        n += 1
print(n, "copper golem statues")
