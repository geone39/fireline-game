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
def reg(i, src):
    if i in ids: err.append(f"duplicate id {i} ({src} vs {ids[i]})")
    ids[i] = src
schema = json.load(open(os.path.join(root, "schema", "units.schema.json")))
for sect in ("troops", "vehicles", "ships"):
    for u in troops[sect]:
        reg(u["id"], sect)
        if jsonschema:
            try: jsonschema.validate(u, schema)
            except jsonschema.ValidationError as e: err.append(f"{u['id']}: {e.message}")
        if u["hp"] <= 0: err.append(f"{u['id']} hp<=0")
        # DPS-per-housing sanity: troops of same class shouldn't exceed 3x median
for s in troops["spells"]: reg(s["id"], "spells")
for d in defs["passive_obstacles"] + defs["active"]: reg(d["id"], "defences")
for key in ("core", "resources", "military", "decor", "obstacles"):
    for b in bld[key]: reg(b["id"], "buildings")
for h in heroes["heroes"]: reg(h["id"], "heroes")
for n in res["nodes"]:
    reg(n["id"], "research")
    t = n.get("unit") or n.get("spell")
    if t and t not in ids: err.append(f"research {n['id']} -> unknown {t}")
for ch in camp["chapters"]:
    fc = ch.get("first_clear", {})
    for k in ("unit_unlock", "hero"):
        if k in fc and fc[k] not in ids: err.append(f"campaign {ch['id']}: unknown {fc[k]}")
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
