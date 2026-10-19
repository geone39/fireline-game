#!/usr/bin/env python3
"""Check art/manifest.json against content ids and the size spec. Exit 1 on error."""
import json, os, struct, sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import gen_placeholders as g  # noqa: E402

ROOT = g.ROOT
m = json.load(open(os.path.join(ROOT, "art", "manifest.json"), encoding="utf-8"))
want = {e[0]: e for e in g.entities()}
got = {s["id"]: s for s in m["sprites"]}
err = [f"missing sprite {i}" for i in want if i not in got] + [f"unknown sprite id {i}" for i in got if i not in want]
for i, s in got.items():
    if i not in want:
        continue
    if set(s) != {"id", "atlas", "frames", "anchors", "fps"}:
        err.append(f"{i}: bad keys")
    a = m["atlases"].get(s["atlas"])
    if not a:
        err.append(f"{i}: atlas {s['atlas']} undefined")
        continue
    W, H = a["size"]
    kind = want[i][2]
    for f in s["frames"]:
        if f["x"] < 0 or f["y"] < 0 or f["x"] + f["w"] > W or f["y"] + f["h"] > H:
            err.append(f"{i}: frame out of atlas")
        if kind in ("unit", "hero", "spell") and (f["w"], f["h"]) != (g.UNIT_FRAME, g.UNIT_FRAME):
            err.append(f"{i}: frame not {g.UNIT_FRAME}px")
        if kind == "building":
            fw, fh = want[i][3]["size"]
            if f["w"] != (fw + fh) * g.TILE_W // 2:
                err.append(f"{i}: width {f['w']} != iso footprint {(fw + fh) * g.TILE_W // 2}")
    if not s["frames"] or not s["anchors"]:
        err.append(f"{i}: empty frames/anchors")
for a in m["atlases"].values():
    p = os.path.join(ROOT, "art", a["image"])
    if not os.path.exists(p):
        err.append(f"missing {a['image']}")
        continue
    d = open(p, "rb").read(32)
    if d[:8] != b"\x89PNG\r\n\x1a\n" or list(struct.unpack(">II", d[16:24])) != a["size"]:
        err.append(f"{a['image']}: bad png/size")
print(f"{len(got)} sprites checked, {len(err)} errors")
for e in err:
    print(" -", e)
sys.exit(1 if err else 0)
