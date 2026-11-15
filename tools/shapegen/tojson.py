"""Turns a Dump.java face list (the game's baked model) into block-model
JSON elements: each cube's bounds, and per face the uv rectangle and
rotation that reproduce the game's per-vertex UVs exactly."""
import json, itertools

FACES = {(0, -1, 0): "down", (0, 1, 0): "up", (0, 0, -1): "north", (0, 0, 1): "south", (-1, 0, 0): "west", (1, 0, 0): "east"}

def st(face, p, lo, hi):
    """Block-model texture coordinates (0..1, origin top-left) of point p on face."""
    n = lambda a: (p[a] - lo[a]) / (hi[a] - lo[a]) if hi[a] != lo[a] else 0.0
    return {"north": (1 - n(0), 1 - n(1)), "south": (n(0), 1 - n(1)),
            "west": (n(2), 1 - n(1)), "east": (1 - n(2), 1 - n(1)),
            "up": (n(0), n(2)), "down": (n(0), 1 - n(2))}[face]

def turn(s, t, rot):
    return {0: (s, t), 90: (t, 1 - s), 180: (1 - s, 1 - t), 270: (1 - t, s)}[rot]

def solve(face, verts, lo, hi):
    us, vs = [v[3] * 16 for v in verts], [v[4] * 16 for v in verts]
    U, V = (min(us), max(us)), (min(vs), max(vs))
    for rot, (u1, u2), (v1, v2) in itertools.product((0, 90, 180, 270), (U, U[::-1]), (V, V[::-1])):
        ok = True
        for v, u, vv in zip(verts, us, vs):
            s, t = turn(*st(face, v[:3], lo, hi), rot)
            if abs(u1 + s * (u2 - u1) - u) > 1e-3 or abs(v1 + t * (v2 - v1) - vv) > 1e-3:
                ok = False
                break
        if ok:
            r = lambda x: round(x, 4)
            f = {"uv": [r(u1), r(v1), r(u2), r(v2)]}
            if rot:
                f["rotation"] = rot
            return f
    raise ValueError(f"{face}: no uv/rotation reproduces {verts}")

def elements(dump, texture, cull_down_at_zero=True):
    cubes = {}
    for p in dump:
        cubes.setdefault((p["part"], p["cube"]), []).append(p)
    out = []
    for polys in cubes.values():
        pts = [v[:3] for p in polys for v in p["verts"]]
        lo = [round(min(c[a] for c in pts), 4) for a in range(3)]
        hi = [round(max(c[a] for c in pts), 4) for a in range(3)]
        faces = {}
        for p in polys:
            face = FACES[tuple(round(c) for c in p["normal"])]
            f = solve(face, p["verts"], lo, hi)
            f["texture"] = texture
            if face == "down" and lo[1] == 0 and cull_down_at_zero:
                f["cullface"] = "down"
            faces[face] = f
        out.append({"from": lo, "to": hi, "faces": faces})
    return out
