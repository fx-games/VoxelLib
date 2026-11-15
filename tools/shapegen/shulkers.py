# Writes voxel/shapes shulker boxes from the game's ShulkerModel box layer,
# lid closed (ShulkerBoxModel.setupAnim 0), placed by
# ShulkerBoxRenderer.modelTransform (dump/shulker_*.json).
import json, os, sys
from tojson import elements
dumps, out = sys.argv[1], sys.argv[2]
facings = ["up", "down", "north", "east", "south", "west"]
for facing in facings:
    m = {"textures": {"particle": "#shulker"},
         "elements": elements(json.load(open(f"{dumps}/shulker_{facing}.json")), "#shulker", cull_down_at_zero=False)}
    json.dump(m, open(f"{out}/models/block/template_shulker_box_{facing}.json", "w"), indent=1)
colors = ["white", "orange", "magenta", "light_blue", "yellow", "lime", "pink", "gray",
          "light_gray", "cyan", "purple", "blue", "brown", "green", "red", "black"]
boxes = {"shulker_box": "shulker"} | {f"{c}_shulker_box": f"shulker_{c}" for c in colors}
for name, sheet in boxes.items():
    variants = {}
    for facing in facings:
        json.dump({"parent": f"minecraft:block/template_shulker_box_{facing}",
                   "textures": {"shulker": f"minecraft:entity/shulker/{sheet}", "particle": f"minecraft:block/{name}"}},
                  open(f"{out}/models/block/{name}_{facing}.json", "w"))
        variants[f"facing={facing}"] = {"model": f"minecraft:block/{name}_{facing}"}
    json.dump({"variants": variants}, open(f"{out}/blockstates/{name}.json", "w"), indent=1)
print(len(boxes), "shulker boxes")
