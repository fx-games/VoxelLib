# Writes voxel/shapes heads and skulls from the layers
# SkullBlockRenderer.createModel picks, placed by its ground (16
# rotations) and wall transformations (dump/*skull_*.json). A player head
# without a profile shows the default skin.
import json, sys
from tojson import elements
dumps, out = sys.argv[1], sys.argv[2]
heads = {  # ground name, wall name: layer, texture
    ("skeleton_skull", "skeleton_wall_skull"): ("mob", "skeleton/skeleton"),
    ("wither_skeleton_skull", "wither_skeleton_wall_skull"): ("mob", "skeleton/wither_skeleton"),
    ("zombie_head", "zombie_wall_head"): ("humanoid", "zombie/zombie"),
    ("creeper_head", "creeper_wall_head"): ("mob", "creeper/creeper"),
    ("player_head", "player_wall_head"): ("humanoid", "player/wide/steve"),
    ("dragon_head", "dragon_wall_head"): ("dragon", "enderdragon/dragon"),
    ("piglin_head", "piglin_wall_head"): ("piglin", "piglin/piglin"),
}
facings = ["north", "east", "south", "west"]
for layer in {l for l, _ in heads.values()}:
    for key, dump in [(str(r), f"skull_{layer}_{r}") for r in range(16)] + [(f"wall_{f}", f"wall_skull_{layer}_{f}") for f in facings]:
        json.dump({"textures": {"particle": "minecraft:block/soul_sand"},
                   "elements": elements(json.load(open(f"{dumps}/{dump}.json")), "#skin", cull_down_at_zero=False)},
                  open(f"{out}/models/block/template_skull_{layer}_{key}.json", "w"), indent=1)
for (ground, wall), (layer, skin) in heads.items():
    for name, keys in ((ground, [(f"rotation={r}", str(r)) for r in range(16)]),
                       (wall, [(f"facing={f}", f"wall_{f}") for f in facings])):
        variants = {}
        for prop, key in keys:
            model = f"{name}_{key.removeprefix('wall_')}"
            json.dump({"parent": f"minecraft:block/template_skull_{layer}_{key}",
                       "textures": {"skin": f"minecraft:entity/{skin}"}},
                      open(f"{out}/models/block/{model}.json", "w"))
            variants[prop] = {"model": f"minecraft:block/{model}"}
        json.dump({"variants": variants}, open(f"{out}/blockstates/{name}.json", "w"), indent=1)
print(len(heads) * 2, "heads")
