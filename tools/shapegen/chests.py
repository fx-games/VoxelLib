# Writes voxel/shapes chest blockstates and models from the game's own
# ChestModel layers placed by ChestRenderer.modelTransformation (dump/*.json).
import json, os, sys
from tojson import elements
dumps, out = sys.argv[1], sys.argv[2]
os.makedirs(f"{out}/models/block", exist_ok=True)
os.makedirs(f"{out}/blockstates", exist_ok=True)
halves = {"single": "", "left": "_left", "right": "_right"}
facings = ["north", "east", "south", "west"]
for t, half in halves.items():
    for facing in facings:
        m = {"textures": {"particle": "#chest"},
             "elements": elements(json.load(open(f"{dumps}/chest_{t}_{facing}.json")), "#chest")}
        json.dump(m, open(f"{out}/models/block/template_chest{half}_{facing}.json", "w"), indent=1)
blocks = {"chest": "normal", "trapped_chest": "trapped", "ender_chest": "ender"}
for ox in ["", "exposed_", "weathered_", "oxidized_"]:
    sheet = "copper" + ("_" + ox[:-1] if ox else "")
    blocks[ox + "copper_chest"] = blocks["waxed_" + ox + "copper_chest"] = sheet
for name, sheet in blocks.items():
    variants = {}
    types = {"single": ""} if name == "ender_chest" else halves
    for t, half in types.items():
        for facing in facings:
            model = f"minecraft:block/{name}{half}_{facing}"
            json.dump({"parent": f"minecraft:block/template_chest{half}_{facing}",
                       "textures": {"chest": f"minecraft:entity/chest/{sheet}{half}"}},
                      open(f"{out}/models/block/{name}{half}_{facing}.json", "w"))
            variants[f"facing={facing}" + ("" if name == "ender_chest" else f",type={t}")] = {"model": model}
    json.dump({"variants": variants}, open(f"{out}/blockstates/{name}.json", "w"), indent=1)
print(len(blocks), "chests")
