#!/usr/bin/env python3
"""Validate content/*.yaml: schema, unique ids, references, balance sanity. Exit 1 on any error. Run in CI."""
import sys, json, glob, os, re, yaml
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
for dc in heroes.get("decoys", []): reg(dc["id"], "decoys", dc)
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

# ---- battle keys (CONTENT-4): everything the sim reads must exist and be consistent (docs/design/battle-sim.md §3.2, §14) ----
def schema_check(obj, fname, label):
    if not jsonschema: return
    try: jsonschema.validate(obj, json.load(open(os.path.join(root, "schema", fname))))
    except jsonschema.ValidationError as e: err.append(f"{label}: {e.message}")
battle_f = load("battle.yaml")
schema_check(battle_f, "battle.schema.json", "battle.yaml")
B = battle_f.get("battle", {})
BATTLE_KEYS = ("time_limit_s", "turn_s", "max_turns", "splash_falloff_pct", "spells_per_turn", "paradrop_edge_cells")
for k in BATTLE_KEYS:
    if k not in B: err.append(f"battle.yaml: missing battle.{k}")
# every battle.<key> that sim (non-test) names in its comments must exist in battle.yaml
simdir = os.path.join(root, "..", "sim")
for fn in sorted(glob.glob(os.path.join(simdir, "*.go"))):
    if fn.endswith("_test.go"): continue
    for k in sorted(set(re.findall(r"battle\.([a-z_]+)", open(fn, encoding="utf-8").read()))):
        if k not in B and k != "yaml": err.append(f"sim/{os.path.basename(fn)} reads battle.{k} but battle.yaml has no such key")
T10 = B.get("turn_s", 1) * 10
t10 = lambda v: int(round(v * 10))
cells = lambda r: max(1, t10(r) // 10)           # range / radius: floor, at least 1 for a reach
mcells = lambda sp: max(1, t10(sp) * T10 // 100)  # speed -> cells per turn
LG = {n: f.get("level_growth", {}) for n, f in (("troops", troops), ("defences", defs), ("heroes", heroes))}
for n, g in LG.items():
    for k in ("hp_permille", "dps_permille"):
        if not isinstance(g.get(k), int) or g[k] < 1000: err.append(f"{n}.yaml: level_growth.{k} missing or < 1000")
def check_cells(u, key, want):
    if u.get(key) != want: err.append(f"{u['id']}: {key} {u.get(key)} != {want} (floor of the per-second value at battle.turn_s={B.get('turn_s')}; update both)")
alluni = [u for sect in ("troops", "vehicles", "ships") for u in troops[sect]]
for u in alluni + heroes["heroes"] + heroes.get("decoys", []):
    i = u["id"]
    if u.get("ap") not in (1, 2): err.append(f"{i}: ap must be 1 or 2")
    # owner decision OQ-7.1: ap 2 only for light infantry / recon (INF, housing 1)
    if u.get("ap") == 2 and not (u["class"] == "INF" and u.get("housing") == 1): err.append(f"{i}: ap 2 is only for light infantry/recon (INF, housing 1)")
    if u.get("speed"): check_cells(u, "move_cells", mcells(u["speed"]))
    elif u.get("move_cells") != 0: err.append(f"{i}: stationary unit needs move_cells: 0")
    if not u.get("id", "").startswith("u_decoy"):
        check_cells(u, "range_cells", cells(u.get("range", 0)))
    if u.get("splash_r") is not None and u.get("splash_cells", 0) != t10(u["splash_r"]) // 10 and "splash_cells" in u:
        err.append(f"{i}: splash_cells != floor(splash_r)")
    if (u.get("dps") or u.get("burst")) and u.get("range_cells", 0) < 1: err.append(f"{i}: attack needs range_cells >= 1")
    if u.get("heal_ps") and not u.get("heal_vs"): err.append(f"{i}: heal_ps needs heal_vs")
    if u.get("burst") and not u.get("wall_mult"): err.append(f"{i}: burst needs wall_mult")
    if u.get("burst") and "splash_cells" not in u: err.append(f"{i}: burst needs splash_cells")
paras = [u for u in alluni if u.get("paradrop")]
if not any(u["id"] == "u_paratrooper" and u.get("group", 0) >= 2 for u in paras): err.append("u_paratrooper: needs group (>=2) and paradrop: true")
for s in troops["spells"]:
    schema_check(s, "spell.schema.json", s["id"])
    check_cells(s, "radius_cells", t10(s["radius"]) // 10)
for d in defs["passive_obstacles"] + defs["active"]:
    i = d["id"]
    schema_check(d, "defence.schema.json", i)
    if d.get("dps") and d.get("range_cells", 0) < 1: err.append(f"{i}: attack needs range_cells")
    if d.get("range") is not None and d.get("dps"): check_cells(d, "range_cells", t10(d["range"]) // 10)
    if d.get("min_range"): check_cells(d, "min_range_cells", t10(d["min_range"]) // 10)
    if d.get("splash_r") is not None and "splash_cells" in d and d["splash_cells"] != t10(d["splash_r"]) // 10: err.append(f"{i}: splash_cells != floor(splash_r)")
    if d in defs["active"] and "destruction_weight" not in d: err.append(f"{i}: missing destruction_weight")
    if d in defs["passive_obstacles"] and "destruction_weight" in d: err.append(f"{i}: passive obstacles are not counted, no destruction_weight")
    if d.get("trap") == "mine" and not (d.get("burst") and d.get("splash_cells") and d.get("vs")): err.append(f"{i}: mine needs burst, splash_cells, vs")
    if d.get("trap") == "spring" and not (d.get("max_hp_flung") and d.get("fling_cells")): err.append(f"{i}: spring trap needs max_hp_flung, fling_cells")
need = {"d_flame_trap": ("overheat_fire_turns", "cool_turns"), "d_tesla": ("chain", "chain_cells"), "d_jammer": ("aura_r", "jam_pct"),
        "d_spring_trap": ("trap", "fling_cells"), "d_mine": ("trap", "burst"), "d_bunker": ("garrison",), "d_wire": ("slow_pct", "dot_dps")}
dmap = {d["id"]: d for d in defs["passive_obstacles"] + defs["active"]}
for i, ks in need.items():
    for k in ks:
        if i not in dmap or k not in dmap[i]: err.append(f"{i}: missing {k}")
decoy_ids = {dc["id"] for dc in heroes.get("decoys", [])}
for h in heroes["heroes"]:
    i = h["id"]
    schema_check(h, "hero.schema.json", i)
    if (h.get("aura_cells", 0) > 0) != (h.get("aura_dmg_permille", 0) + h.get("aura_move_permille", 0) > 0): err.append(f"{i}: aura needs aura_cells and a bonus (and vice versa)")
    ef = h["ability"]["effect"]
    if ef["kind"] == "spawn_decoy" and ef.get("unit") not in decoy_ids: err.append(f"{i}: decoy unit {ef.get('unit')} not in heroes.yaml decoys")
    if ef["kind"] == "stealth" and ef.get("bonus_vs") not in ("wire", "wall", "building", "defence", "INF", "VEH", "AIR", "SEA", "HQ"): err.append(f"{i}: bad bonus_vs")
    if ef["kind"] == "buff_aura" and h.get("in_r1", True): err.append(f"{i}: buff_aura is out of R1 (OQ-7.6): set in_r1: false")
bkeys = ("core", "resources", "military")
for key in bkeys:
    for b in bld[key]:
        i = b["id"]
        hp = b.get("hp_l1")
        if not isinstance(hp, int) or hp <= 0: err.append(f"{i}: missing hp_l1")
        if i != "b_hq" and not isinstance(b.get("hp_growth"), (int, float)): err.append(f"{i}: missing hp_growth")
        if not isinstance(b.get("destruction_weight"), int) or b["destruction_weight"] < 1: err.append(f"{i}: missing destruction_weight")
for key in ("decor", "obstacles"):
    for b in bld[key]:
        if not (b.get("untargetable") and b.get("hp_l1", 0) > 0): err.append(f"{b['id']}: decor/obstacle needs untargetable: true and hp_l1 (sim requires hp > 0)")
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
