# Writes voxel/shapes conduit (ConduitRenderer's shell layer, inactive, at
# DEFAULT_TRANSFORMATION), end_portal (TheEndPortalRenderer: a unit cube
# placed by TRANSFORMATION, top and bottom faces) and end_gateway (a unit
# cube, every face). The game draws the portals with its end portal shader;
# here they show the end_portal texture.
import json, sys
from tojson import elements
dumps, out = sys.argv[1], sys.argv[2]

def save(name, m):
    json.dump(m, open(f"{out}/models/block/{name}.json", "w"), indent=1)
    json.dump({"variants": {"": {"model": f"minecraft:block/{name}"}}}, open(f"{out}/blockstates/{name}.json", "w"), indent=1)

save("conduit", {"textures": {"particle": "#shell", "shell": "minecraft:entity/conduit/base"},
                 "elements": elements(json.load(open(f"{dumps}/conduit.json")), "#shell", cull_down_at_zero=False)})
m = json.load(open(f"{dumps}/end_portal_xf.json"))  # row major 4x4
at = lambda p: [round(16 * (sum(m[4 * r + c] * p[c] for c in range(3)) + m[4 * r + 3]), 4) for r in range(3)]
lo, hi = at([0, 0, 0]), at([1, 1, 1])
face = lambda: {"texture": "#portal", "uv": [0, 0, 16, 16]}
sky = {"particle": "minecraft:block/obsidian", "portal": "minecraft:entity/end_portal/end_portal"}
save("end_portal", {"textures": sky, "elements": [{"from": lo, "to": hi, "shade": False,
                                                     "faces": {"up": face(), "down": face()}}]})
save("end_gateway", {"textures": sky, "elements": [{"from": [0, 0, 0], "to": [16, 16, 16], "shade": False,
                                                      "faces": {f: face() | {"cullface": f} for f in ["down", "up", "north", "south", "west", "east"]}}]})
print("conduit, end_portal, end_gateway")
