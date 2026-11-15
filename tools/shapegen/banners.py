# Writes voxel/shapes banners from the game's BannerModel (pole, bar) and
# BannerFlagModel layers placed by BannerRenderer's ground (16 rotations)
# and wall transformations (dump/*banner_*.json). The game draws the flag
# with banner_base and then the dyed "base" pattern over it; the flag here
# is the dyed base pattern alone (tint_color, the dye's texture diffuse
# colour from dump/dyes.json). Patterns (block entity data) are not drawn.
import json, sys
from tojson import elements
dumps, out = sys.argv[1], sys.argv[2]
dyes = json.load(open(f"{dumps}/dyes.json"))

def template(name, body, flag):
    els = elements(body, "#base", cull_down_at_zero=False) + elements(flag, "#flag", cull_down_at_zero=False)
    for e in els[len(elements(body, "#base", False)):]:
        for f in e["faces"].values():
            f["tintindex"] = 0
    json.dump({"textures": {"particle": "minecraft:block/oak_planks", "base": "minecraft:entity/banner/banner_base",
                            "flag": "minecraft:entity/banner/base"}, "elements": els},
              open(f"{out}/models/block/{name}.json", "w"), indent=1)

for r in range(16):
    template(f"template_banner_{r}", json.load(open(f"{dumps}/banner_body_{r}.json")), json.load(open(f"{dumps}/banner_flag_{r}.json")))
facings = ["north", "east", "south", "west"]
for f in facings:
    template(f"template_wall_banner_{f}", json.load(open(f"{dumps}/wall_banner_body_{f}.json")), json.load(open(f"{dumps}/wall_banner_flag_{f}.json")))
for color, rgb in dyes.items():
    for name, keys in ((f"{color}_banner", [(f"rotation={r}", f"template_banner_{r}") for r in range(16)]),
                       (f"{color}_wall_banner", [(f"facing={f}", f"template_wall_banner_{f}") for f in facings])):
        variants = {}
        for key, parent in keys:
            model = f"{name}_{parent.removeprefix('template_').removeprefix('wall_').removeprefix('banner_')}"
            json.dump({"parent": f"minecraft:block/{parent}", "tint_color": rgb},
                      open(f"{out}/models/block/{model}.json", "w"))
            variants[key] = {"model": f"minecraft:block/{model}"}
        json.dump({"variants": variants}, open(f"{out}/blockstates/{name}.json", "w"), indent=1)
print(len(dyes) * 2, "banners")
