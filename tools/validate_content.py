#!/usr/bin/env python3
"""Validate content/*.yaml: schema, unique ids, references, balance sanity. Exit 1 on any error. Run in CI."""
import sys, json, glob, os, yaml
try:
    import jsonschema
except ImportError:
    jsonschema = None
root = os.path.join(os.path.dirname(__file__), "..", "content")
load = lambda n: yaml.safe_load(open(os.path.join(root, n), encoding="utf-8"))
err = []
troops, defs, bld, heroes, res, camp = (load(n) for n in
    ["troops.yaml", "defences.yaml", "buildings.yaml", "heroes.yaml", "research.yaml", "campaign.yaml"])
ids = {}
name_keys = {}   # name_key -> id
def reg(i, src, entry=None):
    if i in ids: err.append(f"duplicate id {i} ({src} vs {ids[i]})")
    ids[i] = src
    nk = entry.get("name_key") if entry is not None else None
    if not nk: err.append(f"{i}: missing name_key")
    elif nk in name_keys and name_keys[nk] != i: err.append(f"{i}: name_key {nk} already used by {name_keys[nk]}")
    else: name_keys[nk] = i
schema = json.load(open(os.path.join(root, "schema", "units.schema.json")))
for sect in ("troops", "vehicles", "ships"):
    for u in troops[sect]:
        reg(u["id"], sect, u)
        if jsonschema:
            try: jsonschema.validate(u, schema)
            except jsonschema.ValidationError as e: err.append(f"{u['id']}: {e.message}")
        if u["hp"] <= 0: err.append(f"{u['id']} hp<=0")
        # DPS-per-housing sanity: troops of same class shouldn't exceed 3x median
for s in troops["spells"]: reg(s["id"], "spells", s)
for d in defs["passive_obstacles"] + defs["active"]: reg(d["id"], "defences", d)
for key in ("core", "resources", "military", "decor", "obstacles"):
    for b in bld[key]: reg(b["id"], "buildings", b)
for h in heroes["heroes"]: reg(h["id"], "heroes", h)
for n in res["nodes"]:
    reg(n["id"], "research", n)
    t = n.get("unit") or n.get("spell")
    if t and t not in ids: err.append(f"research {n['id']} -> unknown {t}")
for ch in camp["chapters"]:
    reg(ch["id"], "campaign", ch)
    fc = ch.get("first_clear", {})
    for k in ("unit_unlock", "hero"):
        if k in fc and fc[k] not in ids: err.append(f"campaign {ch['id']}: unknown {fc[k]}")
# base grid / terrain
grid = load("base_grid.yaml")
if jsonschema:
    try: jsonschema.validate(grid, json.load(open(os.path.join(root, "schema", "base_grid.schema.json"))))
    except jsonschema.ValidationError as e: err.append(f"base_grid.yaml: {e.message}")
gw, gh = grid["grid"]["w"], grid["grid"]["h"]
bz = grid["build_zone"]
if bz["x"] + bz["w"] > gw or bz["y"] + bz["h"] > gh: err.append("base_grid.yaml: build_zone outside grid")
if grid["default_map"] not in grid["maps"]: err.append("base_grid.yaml: default_map not in maps")
for mk, m in grid["maps"].items():
    nk = m.get("name_key")
    if not nk: err.append(f"base_grid.yaml: map {mk}: missing name_key")
    elif nk in name_keys: err.append(f"base_grid.yaml: map {mk}: name_key {nk} already used by {name_keys[nk]}")
    else: name_keys[nk] = f"map:{mk}"
    for r in m.get("water", []):
        if len(r) == 4 and (r[2] < 1 or r[3] < 1 or r[0] + r[2] > gw or r[1] + r[3] > gh): err.append(f"base_grid.yaml: map {mk}: water rect {r} outside grid")
# placement data: every placeable has a size; limits are monotone and start at unlock_hq
for key in ("resources", "military", "decor", "obstacles"):
    for b in bld[key]:
        sz = b.get("size")
        if not (isinstance(sz, list) and len(sz) == 2 and all(isinstance(v, int) and v > 0 for v in sz)): err.append(f"{b['id']}: missing/invalid size")
        lim = b.get("max_by_hq")
        if key in ("resources", "military") and b.get("unlock_hq") and not lim: err.append(f"{b['id']}: missing max_by_hq")
        if lim:
            ks = sorted(lim)
            if ks[0] != b.get("unlock_hq"): err.append(f"{b['id']}: max_by_hq must start at unlock_hq {b.get('unlock_hq')}")
            if [lim[k] for k in ks] != sorted(lim[k] for k in ks): err.append(f"{b['id']}: max_by_hq not non-decreasing")
# strings: every name_key must exist in every language, no orphans, no empty values
langs = sorted(d for d in os.listdir(os.path.join(root, "strings")) if os.path.isdir(os.path.join(root, "strings", d)))
for want in ("ru", "en"):
    if want not in langs: err.append(f"strings/{want} missing")
for lang in langs:
    strs = {}
    for f in sorted(glob.glob(os.path.join(root, "strings", lang, "*.yaml"))):
        for k, v in (yaml.safe_load(open(f, encoding="utf-8")).get("strings") or {}).items():
            if k in strs: err.append(f"strings/{lang}: duplicate key {k}")
            strs[k] = v
    for k in name_keys:
        if not isinstance(strs.get(k), str) or not strs[k].strip(): err.append(f"strings/{lang}: missing/empty {k} ({name_keys[k]})")
    for k in strs:
        if k not in name_keys: err.append(f"strings/{lang}: orphan key {k}")
# balance sanity: damage-per-housing within 3x band inside class
import statistics
for cls in ("INF", "VEH"):
    vals = [u["dps"] / u["housing"] for sect in ("troops", "vehicles") for u in troops[sect] if u["class"] == cls and u.get("dps")]
    med = statistics.median(vals)
    for sect in ("troops", "vehicles"):
        for u in troops[sect]:
            if u["class"] == cls and u.get("dps") and not (med / 3.2 <= u["dps"] / u["housing"] <= med * 3.2):
                err.append(f"{u['id']} dps/housing {u['dps']/u['housing']:.1f} outside 3.2x of median {med:.1f}")
print(f"{len(ids)} ids checked, {len(err)} errors")
for e in err: print(" -", e)
sys.exit(1 if err else 0)
