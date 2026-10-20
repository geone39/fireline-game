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
# ---- reference bases (CONTENT-5): content/reference_bases/*.yaml; an independent port of svc/base ValidateLayout ----
PLACE = {}   # id -> kind, w, h, unlock, water, block, limits, cap
for key, kind in (("core", "core"), ("resources", "building"), ("military", "building")):
    for b in bld[key]: PLACE[b["id"]] = dict(kind=kind, w=b["size"][0], h=b["size"][1], unlock=b.get("unlock_hq", 1), water=bool(b.get("requires_water")), block="all", limits=b.get("max_by_hq"), cap=b.get("levels", 1))
for d in defs["passive_obstacles"] + defs["active"]:
    kind = "passive" if d in defs["passive_obstacles"] else "active"
    blk = "all" if kind == "active" else {True: "all", "vehicles": "vehicles"}.get(d.get("blocks"), "none")
    PLACE[d["id"]] = dict(kind=kind, w=d["size"][0], h=d["size"][1], unlock=d.get("unlock_hq", 1), water=bool(d.get("requires_water")), block=blk, limits=d.get("max_count_by_hq"), cap=999)
for i, c in load("building_costs.yaml")["buildings"].items():
    if i in PLACE and "max_level" in c: PLACE[i]["cap"] = c["max_level"]
def hq_limit(t, hq):
    p = PLACE[t]
    if p["kind"] == "core": return 1
    if not p["limits"]: return None
    n = 0
    for at in sorted(p["limits"]):
        if at <= hq: n = p["limits"][at]
    return n
def check_ref_base(rb, label, start=False):   # start=True: the start layout is only checked for placement, not for defences/storage
    e = lambda m: err.append(f"{label}: {m}")
    if rb["map"] not in grid["maps"]: return e(f"unknown map {rb['map']}")
    water = {(x + a, y + b) for x, y, w, h in grid["maps"][rb["map"]]["water"] for b in range(h) for a in range(w)}
    items, hq = rb["layout"]["items"], rb["hq_level"]
    occ, seen, cnt, byuid = {}, set(), {}, {}
    hqs = [it for it in items if it["type"] == "b_hq"]
    if len(hqs) != 1: return e(f"needs exactly one b_hq, has {len(hqs)}")
    if hqs[0]["level"] != hq: e(f"b_hq level {hqs[0]['level']} != hq_level {hq}")
    for it in items:
        u, t = it["uid"], it["type"]
        if u in seen: e(f"duplicate uid {u}")
        seen.add(u); byuid[u] = it
        p = PLACE.get(t)
        if not p: e(f"{u}: type {t} is not placeable"); continue
        cnt[t] = cnt.get(t, 0) + 1
        if it["level"] > p["cap"]: e(f"{u}: level {it['level']} > cap {p['cap']}")
        if it["level"] > hq: e(f"{u}: level {it['level']} > hq_level {hq}")
        if hq < p["unlock"]: e(f"{u}: {t} needs HQ {p['unlock']}")
        w, h = (p["h"], p["w"]) if it["rot"] % 2 else (p["w"], p["h"])
        ts = [(it["x"] + a, it["y"] + b) for b in range(h) for a in range(w)]
        if any(not (bz["x"] <= q[0] < bz["x"] + bz["w"] and bz["y"] <= q[1] < bz["y"] + bz["h"]) for q in ts): e(f"{u}: outside build_zone")
        if any(q in water for q in ts): e(f"{u}: on water")
        for q in ts:
            if q in occ: e(f"{u} overlaps {occ[q]} at {q}"); break
            occ[q] = u
        if p["water"] and not any((qx + dx, qy + dy) in water for qx, qy in ts for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1))): e(f"{u}: {t} must touch the shore")
    for t, n in sorted(cnt.items()):
        lim = hq_limit(t, hq) if t in PLACE else None
        if lim is not None and n > lim: e(f"{t}: {n} placed, limit {lim} at HQ {hq}")
    # HQ reachable on foot from the border. svc/base only needs it reachable by breaching walls; a reference base must also keep a gate.
    def walk(q, breach):
        if q in water: return False
        u = occ.get(q)
        if u is None: return True
        p = PLACE[byuid[u]["type"]]
        return p["block"] != "all" or (breach and p["kind"] == "passive")
    def reach(breach):
        hq_uid = hqs[0]["uid"]
        border = [(x, y) for x in range(gw) for y in (0, gh - 1)] + [(x, y) for y in range(gh) for x in (0, gw - 1)]
        seen_t, qu = set(), []
        for q in border:
            if q not in seen_t and walk(q, breach): seen_t.add(q); qu.append(q)
        for q in qu:
            for n in ((q[0] + 1, q[1]), (q[0] - 1, q[1]), (q[0], q[1] + 1), (q[0], q[1] - 1)):
                if not (0 <= n[0] < gw and 0 <= n[1] < gh): continue
                if occ.get(n) == hq_uid: return True
                if n not in seen_t and walk(n, breach): seen_t.add(n); qu.append(n)
        return False
    if not reach(True): e("HQ is completely enclosed")
    elif not reach(False): e("HQ is not reachable without breaking walls (a reference base keeps a gate)")
    have = lambda pred: any(pred(PLACE[it["type"]], it["type"]) for it in items if it["type"] in PLACE)
    if not start and not have(lambda p, t: p["kind"] == "passive" and p["block"] == "all"): e("no walls/fences")
    if not start and not have(lambda p, t: p["kind"] == "active"): e("no active defence")
    if not start and not have(lambda p, t: t in ("b_vault", "b_fuel_tank")): e("no storage")
    if not start and hq >= 2 and not have(lambda p, t: t in ("d_mine", "d_spring_trap")): e("no traps")
ref_hqs, ref_keys = {}, set()
for fn in sorted(glob.glob(os.path.join(root, "reference_bases", "*.yaml"))):
    rb = yaml.safe_load(open(fn, encoding="utf-8"))
    label = "reference_bases/" + os.path.basename(fn)
    schema_check(rb, "reference_base.schema.json", label)
    if not jsonschema and not all(k in rb for k in ("id", "name_key", "hq_level", "map", "layout")): err.append(f"{label}: missing keys"); continue
    if rb.get("id") != "ref_" + os.path.splitext(os.path.basename(fn))[0]: err.append(f"{label}: id {rb.get('id')} must be ref_<file name>")
    if rb["id"] in ids: err.append(f"{label}: duplicate id {rb['id']}")
    ids[rb["id"]] = "reference_base"
    if rb["name_key"] in name_keys: err.append(f"{label}: name_key {rb['name_key']} already used by {name_keys[rb['name_key']]}")
    name_keys[rb["name_key"]] = rb["id"]
    if (rb["hq_level"], rb["map"]) in ref_keys: err.append(f"{label}: second reference base for HQ{rb['hq_level']}/{rb['map']}")
    ref_keys.add((rb["hq_level"], rb["map"]))
    ref_hqs.setdefault(rb["hq_level"], []).append(rb["map"])
    check_ref_base(rb, label)
for n in range(1, 11):
    if "plain" not in ref_hqs.get(n, []): err.append(f"reference_bases: HQ{n} has no plain reference base")
# ---- army & laboratory (CONTENT-6, docs/design/army-lab.md §11): army.yaml, capacity_by_level, research nodes, start_layout ----
army = load("army.yaml")
schema_check(army, "army.schema.json", "army.yaml")
econ_f, costs_f = load("economy.yaml"), load("building_costs.yaml")
bmap = {b["id"]: b for key in ("core", "resources", "military") for b in bld[key]}
unit_by_id = {u["id"]: u for u in alluni}
spell_by_id = {s["id"]: s for s in troops["spells"]}
POOLS = ("unit", "garage", "spell")
for cls in troops["classes"]:
    if cls not in army.get("pool_by_class", {}): err.append(f"army.yaml: class {cls} has no pool_by_class")
for cls in army.get("pool_by_class", {}):
    if cls not in troops["classes"]: err.append(f"army.yaml: pool_by_class.{cls} is not a class in troops.yaml")
for where, b in list(army.get("pool_building", {}).items()) + list(army.get("producers", {}).items()) + [("lab_building", army.get("lab_building"))]:
    if b not in bmap: err.append(f"army.yaml: {where} -> unknown building {b}")
if army.get("lab_building") != "b_laboratory": err.append("army.yaml: lab_building must be b_laboratory")
for u in alluni:   # the producer of the unit must exist and match its unlock key (barracks/factory/shipyard)
    prod = army.get("producers", {}).get(u["class"])
    if prod and prod.removeprefix("b_") not in u["unlock"]: err.append(f"{u['id']}: unlock has no level of its producer {prod}")
    if not isinstance(u.get("train_s"), int) or not isinstance(u.get("housing"), int) or not u.get("cost"): err.append(f"{u['id']}: train_s/housing/cost required")
    if u["class"] in ("INF", "AIR") and set(u["cost"]) != {"fuel"}: err.append(f"{u['id']}: infantry/air cost fuel only")
    if u["class"] in ("VEH", "SEA") and "cash" not in u["cost"]: err.append(f"{u['id']}: vehicles/ships cost cash")
for sp in troops["spells"]:
    if not isinstance(sp.get("train_s"), int) or sp["train_s"] < 1: err.append(f"{sp['id']}: missing train_s")
    if set(sp["cost"]) != {"fuel"}: err.append(f"{sp['id']}: spells cost fuel only")
    if "level_growth" in sp: err.append(f"{sp['id']}: level_growth for spells waits for SIM-9 (OQ-ARMY-2)")
if army.get("housing_perk", {}).get("key") not in {n["perk"]["key"] for n in res["nodes"] if "perk" in n}: err.append("army.yaml: housing_perk.key is not a perk of any research node")
if army.get("cancel_refund_started_pct") != costs_f.get("cancel_refund_pct"): err.append("army.yaml: cancel_refund_started_pct != building_costs.yaml cancel_refund_pct")
for u in army.get("start_army", []):
    if u["id"] not in unit_by_id: err.append(f"army.yaml: start_army unknown unit {u['id']}")
    elif u["count"] * unit_by_id[u["id"]]["cost"].get("fuel", 0) > econ_f["start_grant"]["fuel"]: err.append("army.yaml: start_army costs more fuel than start_grant.fuel")
# capacity_by_level: exactly the pool buildings, one positive integer per building level, non-decreasing
cap_bld = set(army.get("pool_building", {}).values())
for i, b in bmap.items():
    c = b.get("capacity_by_level")
    if i in cap_bld:
        if not (isinstance(c, list) and len(c) == b["levels"] and all(isinstance(v, int) and v > 0 for v in c)): err.append(f"{i}: capacity_by_level must be {b['levels']} positive integers (one per level)")
        elif c != sorted(c): err.append(f"{i}: capacity_by_level must be non-decreasing")
    elif c is not None: err.append(f"{i}: capacity_by_level is only for pool buildings")
# research nodes
lab_levels = bmap["b_laboratory"]["levels"]
rnode = {n["id"]: n for n in res["nodes"]}
chains = {}
for n in res["nodes"]:
    i = n["id"]
    schema_check(n, "research.schema.json", i)
    if n["tier"] not in res["tiers"]: err.append(f"{i}: unknown tier {n['tier']}")
    if not 1 <= n["lab"] <= lab_levels: err.append(f"{i}: lab {n['lab']} outside 1..{lab_levels} (b_laboratory levels)")
    item = unit_by_id.get(n.get("unit")) or spell_by_id.get(n.get("spell"))
    if "perk" in n:
        pk, pv = n["perk"]["key"], n["perk"]["value"]
        if (pk == "scan_reveals_traps") != isinstance(pv, bool): err.append(f"{i}: perk {pk} value must be {'bool' if pk == 'scan_reveals_traps' else 'int'}")
        if isinstance(pv, int) and not isinstance(pv, bool) and pv < 1: err.append(f"{i}: perk value must be >= 1")
    elif item is None: err.append(f"{i}: unknown unit/spell {n.get('unit') or n.get('spell')}")
    else:
        key = n.get("unit") or n.get("spell")
        chains.setdefault(key, []).append(n)
        if n["lab"] + 1 < item["unlock"]["hq"]: err.append(f"{i}: lab {n['lab']} is reached only after HQ {n['lab'] + 1}, but {key} unlocks at HQ {item['unlock']['hq']} (need lab >= unlock.hq - 1)")
        if "spell" in n and not n.get("soon"): err.append(f"{i}: spell nodes must be soon: true until level_growth for spells exists (SIM-9, OQ-ARMY-2)")
        if "unit" in n and n.get("soon"): err.append(f"{i}: soon is only for spell nodes")
    for r in n.get("requires", []):
        q = rnode.get(r)
        if q is None: err.append(f"{i}: requires unknown node {r}")
        elif "perk" in n or (q.get("unit"), q.get("spell")) != (n.get("unit"), n.get("spell")) or q["to_level"] >= n["to_level"]: err.append(f"{i}: requires {r} must be a node of the same item with a lower to_level")
for key, ns in chains.items():
    ns.sort(key=lambda n: n["to_level"])
    if len({n["to_level"] for n in ns}) != len(ns): err.append(f"research: {key} has two nodes with the same to_level")
    for a, b in zip(ns, ns[1:]):
        if b["lab"] < a["lab"]: err.append(f"{b['id']}: lab lower than the previous node {a['id']} of the chain")
        if "requires" in b and a["id"] not in b["requires"]: err.append(f"{b['id']}: requires must include the previous node {a['id']}")
# start layout: HQ L1 + a ready camp, valid against the placement rules
sl = load("start_layout.yaml")
if sorted(it["type"] for it in sl["layout"]["items"]) != ["b_camp", "b_hq"]: err.append("start_layout.yaml: needs exactly one b_hq and one b_camp")
check_ref_base({"map": grid["default_map"], "hq_level": 1, "layout": sl["layout"]}, "start_layout.yaml", start=True)
# UI strings (strings/*/army.yaml): res.*, army.*, lab.*, perk.*, reason.* — not name_keys of ids, so the required ones are listed here
UI_PREFIXES = ("res.", "army.", "lab.", "perk.", "reason.")
UI_REASONS = ("housing_full", "locked", "queue_full", "not_multiple_of_group", "no_stock", "housing_exceeded", "ship_no_water", "hero_unavailable",
              "lab_level", "lab_busy", "prerequisite", "housing_in_use", "queue_not_empty", "research_in_progress")
UI_REQUIRED = ({v["name_key"] for v in econ_f["resources"].values()} | {"army.pool." + p for p in POOLS} | {"reason." + r for r in UI_REASONS}
               | {"army.title", "army.capacity", "army.used", "army.free", "army.queue", "army.train", "army.cancel", "army.speedup", "army.coming_soon", "lab.title", "lab.research", "lab.coming_soon"}
               | {f"perk.{n['perk']['key']}{sfx}" for n in res["nodes"] if "perk" in n for sfx in ("", ".desc")})
ui_keys = {}
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
        if k not in name_keys and not k.startswith(UI_PREFIXES): err.append(f"strings/{lang}: orphan key {k}")
    for k in sorted(UI_REQUIRED):
        if k not in strs: err.append(f"strings/{lang}: missing UI key {k}")
    ui_keys[lang] = {k for k in strs if k.startswith(UI_PREFIXES)}
if len({frozenset(v) for v in ui_keys.values()}) > 1: err.append("strings: UI keys (res/army/lab/perk/reason) differ between languages")
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
