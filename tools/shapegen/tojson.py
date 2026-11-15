"""Turns a Dump.java face list (the game's baked model) into block-model
JSON elements: each cube's bounds, and per face the uv rectangle and
rotation that reproduce the game's per-vertex UVs exactly. A cube the
renderer turns off the block axes (banners, heads at 22.5 degree steps)
keeps its own part-space bounds and gets the turn as the element's
"rotation" "matrix" (a VoxelLib extension: p' = matrix * p)."""
import json, itertools, math

FACES = {(0, -1, 0): "down", (0, 1, 0): "up", (0, 0, -1): "north", (0, 0, 1): "south", (-1, 0, 0): "west", (1, 0, 0): "east"}

AXIS = {"down": 1, "up": 1, "north": 2, "south": 2, "west": 0, "east": 0}

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

def axis_aligned(polys):
    return all(abs(abs(c) - 1) < 1e-3 or abs(c) < 1e-3 for p in polys for c in p["normal"])

def sub(a, b): return [x - y for x, y in zip(a, b)]
def cross(a, b): return [a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]]
def norm(a): return math.sqrt(sum(x * x for x in a))

def affine(polys):
    """The map world = A local + t of a cube's vertices (A as columns)."""
    at = {}
    for p in polys:
        for v in p["verts"]:
            at[tuple(round(c, 3) for c in v[5:8])] = v[:3]
    lo = [min(k[a] for k in at) for a in range(3)]
    hi = [max(k[a] for k in at) for a in range(3)]
    o = at[tuple(lo)]
    cols = [None] * 3
    for a in range(3):
        if hi[a] > lo[a]:
            k = list(lo); k[a] = hi[a]
            cols[a] = [d / (hi[a] - lo[a]) for d in sub(at[tuple(k)], o)]
    for a in range(3):  # a flat cube: its thin axis from the other two
        if cols[a] is None:
            b, c = cols[(a + 1) % 3], cols[(a + 2) % 3]
            n = cross(b, c)
            cols[a] = [x * norm(b) / norm(n) for x in n]
    t = [o[i] - sum(cols[a][i] * lo[a] for a in range(3)) for i in range(3)]
    return cols, t, lo, hi

def solve3(cols, t):
    """x with A x = t (Cramer)."""
    det = lambda c: sum(c[0][i] * cross(c[1], c[2])[i] for i in range(3))
    d = det(cols)
    return [det([t if j == a else cols[j] for j in range(3)]) / d for a in range(3)], d

LOW, HIGH = ["west", "down", "north"], ["east", "up", "south"]  # by axis x, y, z

def turned(polys, texture):
    cols, t, lo, hi = affine(polys)
    shift, det = solve3(cols, t)
    if det < 0:
        raise ValueError("mirroring turn: faces would flip")
    faces = {}
    for p in polys:
        loc = [v[5:8] for v in p["verts"]]
        name = None
        for a in range(3):
            if all(abs(q[a] - lo[a]) < 1e-3 for q in loc):
                name = LOW[a]
            elif all(abs(q[a] - hi[a]) < 1e-3 for q in loc):
                name = HIGH[a]
        if name is None or any(hi[a] == lo[a] for a in range(3) if a != AXIS[name]):
            continue
        f = solve(name, [q + v[3:5] for q, v in zip(loc, p["verts"])], lo, hi)
        f["texture"] = texture
        faces[name] = f
    r = lambda x: round(x, 4)
    return {"from": [r(lo[a] + shift[a]) for a in range(3)], "to": [r(hi[a] + shift[a]) for a in range(3)],
            "rotation": {"origin": [0, 0, 0], "axis": "y", "angle": 0,
                         "matrix": [r(cols[c][row]) for row in range(3) for c in range(3)]},
            "faces": faces}

def elements(dump, texture, cull_down_at_zero=True):
    cubes = {}
    for p in dump:
        cubes.setdefault((p["part"], p["cube"]), []).append(p)
    out = []
    for polys in cubes.values():
        if not axis_aligned(polys):
            out.append(turned(polys, texture))
            continue
        pts = [v[:3] for p in polys for v in p["verts"]]
        lo = [round(min(c[a] for c in pts), 4) for a in range(3)]
        hi = [round(max(c[a] for c in pts), 4) for a in range(3)]
        faces = {}
        for p in polys:
            face = FACES[tuple(round(c) for c in p["normal"])]
            if any(hi[a] == lo[a] for a in range(3) if a != AXIS[face]):
                continue  # zero area (an edge of a flat cube): draws nothing in the game either
            f = solve(face, p["verts"], lo, hi)
            f["texture"] = texture
            if face == "down" and lo[1] == 0 and cull_down_at_zero:
                f["cullface"] = "down"
            faces[face] = f
        out.append({"from": lo, "to": hi, "faces": faces})
    return out
