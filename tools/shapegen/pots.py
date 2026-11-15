# Writes the voxel/shapes decorated_pot from the game's DecoratedPotRenderer
# base and sides layers placed by its modelTransformation (dump/*.json).
import json, os, sys
from tojson import elements
dumps, out = sys.argv[1], sys.argv[2]
variants = {}
for facing in ["north", "east", "south", "west"]:
    els = (elements(json.load(open(f"{dumps}/pot_base_{facing}.json")), "#base")
           + elements(json.load(open(f"{dumps}/pot_sides_{facing}.json")), "#side"))
    json.dump({"textures": {"base": "minecraft:entity/decorated_pot/decorated_pot_base",
                            "side": "minecraft:entity/decorated_pot/decorated_pot_side",
                            "particle": "minecraft:block/terracotta"},
               "elements": els},
              open(f"{out}/models/block/decorated_pot_{facing}.json", "w"), indent=1)
    variants[f"facing={facing}"] = {"model": f"minecraft:block/decorated_pot_{facing}"}
json.dump({"variants": variants}, open(f"{out}/blockstates/decorated_pot.json", "w"), indent=1)
print("decorated_pot")
