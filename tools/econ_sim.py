#!/usr/bin/env python3
"""Economy calibration + player simulation for Fireline (codename).
Usage: python3 tools/econ_sim.py [--json out.json]
Two stages:
 1. calibrate(): from target days-per-HQ-level and total upgrade cost, derive required income and split it
    between passive collectors / PvP raids / campaign.
 2. simulate(): an actual day-by-day agent (casual/regular/active) buys upgrades with a builder queue,
    collects capped passive income, loots raids, and we check days-to-HQ vs targets (+-20%).
All knobs live in ECON dict -> mirrored in content/economy.yaml (single source: this file generates it).
"""
import json, sys, math

HQ_MAX = 10
# --- design targets (days spent AT each HQ level before moving up), casual 2 sessions/day
TARGET_DAYS = {1:1, 2:2, 3:4, 4:7, 5:11, 6:16, 7:22, 8:30, 9:40, 10:52}  # cumulative ~185 d casual
PROFILES = {  # sessions/day, raids/session, campaign stages/day, builder-time use efficiency
    "casual":  dict(sessions=2, raids=2, campaign=0.5, eff=0.75),
    "regular": dict(sessions=4, raids=3, campaign=1.0, eff=0.9),
    "active":  dict(sessions=7, raids=4, campaign=1.5, eff=1.0),
}
# total upgrade bill to "finish" an HQ level (cash, fuel) -- buildings+defences+walls / army+research
def cost_cash(L): return round(2000 * 2.05 ** (L - 1), -2)
def cost_fuel(L): return round(1200 * 2.10 ** (L - 1), -2)
# builders unlocked by HQ level
BUILDERS = {1:2, 2:2, 3:3, 4:3, 5:4, 6:4, 7:5, 8:5, 9:5, 10:5}
# passive production: cash/hour per collector, count, storage cap (all at that HQ level)
def collectors(L):  # (count, rate_per_hour_each)
    return (min(2 + L // 2, 6), round(40 * 1.62 ** (L - 1), -1) if L > 1 else 40)
def storage(L): return round(1500 * 2.0 ** (L - 1), -2)  # per resource per vault set
# share of total income by source (design target at casual; raids take more as player activity rises)
SPLIT = dict(passive=0.50, raids=0.38, campaign=0.12)
# raid loot: fraction of defender's *stored* (lootable) resource, hard cap per raid, scales by league gap
LOOT_PCT = 0.075           # % of victim's available per full 3-star win; scales with stars: 1*=0.40,2*=0.70,3*=1.0
LOOT_CAP_PER_HQ = lambda L: round(storage(L) * 0.35, -2)
STAR_MULT = {0: 0.0, 1: 0.40, 2: 0.70, 3: 1.00}
CAMPAIGN_MULT = 1.6        # campaign pays this multiple per "attack-equivalent" but with an energy gate
# one-off wallet grant to a new guest (idem key start_grant:<playerID>); mirrored in content/economy.yaml start_grant
# onboarding spend (docs/design/ux-onboarding.md steps 1-7): MG nest 600 cash; 5 riflemen 125 fuel; HQ1->2 step + 2nd collector/barracks margin
ONBOARD_CASH, ONBOARD_FUEL = 1200, 500
START_GRANT = dict(cash=1500, fuel=600, gems=0, alloy=0, trophies=0)
CAMPAIGN_ENERGY_PER_DAY = 3  # stages per day cap (slows player on purpose)
SHIELD_HOURS = {1: 16, 2: 12, 3: 8}   # shield after being raided with 1/2/3 stars lost
RAID_COOLDOWN_SAME_TARGET_H = 24      # same attacker->target cooldown
DAILY_RAID_LIMIT_PER_TARGET = 3       # a base can be raided max N times per day
MAX_LOOT_LOSS_PER_DAY_PCT = 0.35      # victim can lose max 35% of stored stock per day (anti-grief)

def calibrate():
    rows = []
    for L in range(1, HQ_MAX + 1):
        d = TARGET_DAYS[L]
        need_c, need_f = cost_cash(L), cost_fuel(L)
        daily_c, daily_f = need_c / d, need_f / d
        cnt, rate = collectors(L)
        passive_day_cap = cnt * rate * 24                          # if never capped & collected
        # effective passive limited by storage (casual collects ~2x/day): min(production, 2 * storage)
        passive_eff = min(passive_day_cap, 2 * storage(L) * 0.6)
        rows.append(dict(hq=L, days=d, cash_total=need_c, fuel_total=need_f,
                         need_cash_day=round(daily_c), need_fuel_day=round(daily_f),
                         collectors=cnt, rate_h=rate, passive_cap_day=round(passive_eff),
                         storage=storage(L), loot_cap=LOOT_CAP_PER_HQ(L), builders=BUILDERS[L]))
    return rows

def simulate(profile, seed=1):
    import random
    rnd = random.Random(seed)
    p = PROFILES[profile]
    day = 0; L = 1; progress_c = 0.0; progress_f = 0.0
    bank_c = float(START_GRANT['cash']); bank_f = float(START_GRANT['fuel'])   # new guest starts with the grant
    builder_days_needed = None
    hist = {}
    spent_c = spent_f = 0.0
    day_at = {}
    while L <= HQ_MAX and day < 600:
        day += 1
        cnt, rate = collectors(L)
        st = storage(L)
        # passive: accumulates between sessions, capped by storage between collections
        gap_h = 24 / p["sessions"]
        per_collect = min(cnt * rate * gap_h, st * 0.5)   # half of storage per resource pool is collector share
        got_c = got_f = 0.0
        for _ in range(p["sessions"]):
            got_c += per_collect; got_f += per_collect * 0.55
        # raids: each raid wins avg ~1.9 stars, loot = pct of victim stock (peer-level) capped
        for _ in range(int(p["sessions"] * p["raids"])):
            stars = rnd.choices([0,1,2,3], weights=[.15,.30,.35,.20])[0]
            victim_stock = st * 0.55
            loot = min(victim_stock * LOOT_PCT * 3 * STAR_MULT[stars], LOOT_CAP_PER_HQ(L))
            got_c += loot; got_f += loot * 0.6
        # campaign (energy-gated)
        stages = min(p["campaign"] * p["sessions"] / 2, CAMPAIGN_ENERGY_PER_DAY)
        camp = stages * st * 0.03 * CAMPAIGN_MULT
        got_c += camp; got_f += camp * 0.8
        # builder constraint: each builder can burn ~ eff of its day; time-gated upgrades
        budget_c = cost_cash(L); budget_f = cost_fuel(L)
        # time gate: builders*eff*day must cover total builder-days of level (= days_target*builders*0.6)
        need_builder_days = TARGET_DAYS[L] * BUILDERS[L] * 0.75
        builder_days = BUILDERS[L] * p["eff"]
        bank_c += min(got_c, st * 2); bank_f += min(got_f, st * 2)
        spend_c = min(bank_c, budget_c - progress_c)
        spend_f = min(bank_f, budget_f - progress_f)
        # pace limited by builders
        pace = builder_days / need_builder_days
        spend_c = min(spend_c, budget_c * pace); spend_f = min(spend_f, budget_f * pace)
        progress_c += spend_c; progress_f += spend_f; bank_c -= spend_c; bank_f -= spend_f
        if progress_c >= budget_c * 0.999 and progress_f >= budget_f * 0.999:
            day_at[L] = day; L += 1; progress_c = progress_f = 0.0
    return day_at

def _load(name):
    import os, yaml
    return yaml.safe_load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "content", name), encoding="utf-8"))

def price_book():
    """content/building_costs.yaml -> (file, {id: (cost, time_s, cost_growth, time_growth, max_level)}); the single source of prices."""
    f = _load("building_costs.yaml")
    book = {i: (e["cost"], e["time_s"], e.get("cost_growth", f["cost_growth"]), e.get("time_growth", f["time_growth"]), e.get("max_level"))
            for i, e in f["buildings"].items()}
    return f, book

def structure_bills(f=None, book=None):
    """Cash and builder-seconds paid while sitting at each HQ level L if every active structure is kept at its level cap.
    Rules (mirrored in docs/design/economy.md): a type is listed in buildings.yaml/defences.yaml; level cap at HQ L is
    min(levels, L - unlock_hq + 1); HQ pays the price of level L+1; new copies pay levels 1..cap, existing copies the new levels.
    Walls/wire/fences/mines/traps (passive_obstacles) are bulk items bought as needed and are not in the bill."""
    if book is None: f, book = price_book()
    b, d = _load("buildings.yaml"), _load("defences.yaml")
    defs = [x for g in ("core", "resources", "military") for x in b[g]] + d["active"]
    def count(x, L):
        if x["id"] == "b_hq": return 1
        n = 0
        for k, v in sorted((x.get("max_by_hq") or x.get("max_count_by_hq")).items()):
            if k <= L: n = v
        return n
    def lv_max(x):
        return x.get("levels") or book[x["id"]][4] or 1
    def cap(x, L):
        if L < x.get("unlock_hq", 1): return 0
        return min(L, HQ_MAX) if x["id"] == "b_hq" else min(lv_max(x), L - x.get("unlock_hq", 1) + 1)
    cash, secs = {}, {}
    for L in range(1, HQ_MAX + 1):
        c = t = 0
        for x in defs:
            cost, t1, cg, tg, _ = book[x["id"]]
            if x["id"] == "b_hq":
                if L < HQ_MAX: c += cost["cash"] * cg ** L; t += t1 * tg ** L
                continue
            nL, nP = count(x, L), (count(x, L - 1) if L > 1 else 0)
            cL, cP = cap(x, L), (cap(x, L - 1) if L > 1 else 0)
            lv = list(range(cP + 1, cL + 1)) * nP + list(range(1, cL + 1)) * (nL - nP)
            for n in lv:
                c += cost.get("cash", 0) * cg ** (n - 1); t += t1 * tg ** (n - 1)
        cash[L], secs[L] = c, t
    return cash, secs

def check_building_costs():
    """content/building_costs.yaml vs schema, catalog and pacing: every buildable type priced, shared growth knobs equal
    economy.yaml, structure bill within 40-140% of the cash bill and builder time within 50-170% of the builder budget."""
    import os, json
    errs = []
    f, book = price_book()
    try:
        import jsonschema
        jsonschema.validate(f, json.load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "content", "schema", "building_costs.schema.json"))))
    except ImportError: pass
    except jsonschema.ValidationError as e: errs.append(f"building_costs.yaml schema: {e.message}")
    b, d = _load("buildings.yaml"), _load("defences.yaml")
    want = {x["id"] for g in ("core", "resources", "military") for x in b[g]} | {x["id"] for g in ("passive_obstacles", "active") for x in d[g]}
    for i in sorted(want - set(book)): errs.append(f"building_costs.yaml: no price for {i}")
    for i in sorted(set(book) - want): errs.append(f"building_costs.yaml: unknown type {i}")
    lv = {x["id"]: x.get("levels") for g in ("core", "resources", "military") for x in b[g]}
    for i, (cost, t, cg, tg, ml) in book.items():
        if not lv.get(i) and not ml: errs.append(f"building_costs.yaml: {i} has no levels in buildings.yaml and no max_level")
    y = _load("economy.yaml")
    if abs(f["cost_growth"] - y["costs"]["cash"]["growth"]) > 1e-9: errs.append("building_costs cost_growth != economy.yaml costs.cash.growth")
    if abs(f["time_growth"] - y["costs"]["time_growth_per_level"]) > 1e-9: errs.append("building_costs time_growth != economy.yaml time_growth_per_level")
    cash, secs = structure_bills(f, book)
    for L in range(1, HQ_MAX + 1):
        r = cash[L] / cost_cash(L)
        if not 0.4 <= r <= 1.4: errs.append(f"HQ{L}: structure cash bill {cash[L]:,.0f} is {r:.2f}x of the cash bill (want 0.4-1.4)")
        if L < HQ_MAX:
            tr = secs[L] / (TARGET_DAYS[L] * BUILDERS[L] * 0.75 * 86400)
            if not 0.5 <= tr <= 1.7: errs.append(f"HQ{L}: build time {secs[L]/86400:.1f} builder-days is {tr:.2f}x of the builder budget (want 0.5-1.7)")
            hq_t = book["b_hq"][1] * book["b_hq"][3] ** L
            if hq_t > 0.4 * TARGET_DAYS[L] * 86400: errs.append(f"HQ{L}: HQ upgrade takes {hq_t/86400:.1f} d, over 40% of the days spent at this level")
    return errs

def report():
    cal = calibrate()
    out = dict(calibration=cal, split=SPLIT, sim={})
    tot_t = 0; cum = {}
    for r in cal: tot_t += r["days"]; cum[r["hq"]] = tot_t
    out["target_cum"] = cum
    for prof in PROFILES:
        out["sim"][prof] = simulate(prof)
    return out

def check(o):
    """Acceptance checks (exit 1 on failure): casual within +-20% of targets, active <= 1.35x faster than casual,
    and content/economy.yaml passive tables agree with the constants above (the Go server reads the yaml)."""
    import os, yaml
    errs = []
    cas, act = o["sim"]["casual"], o["sim"]["active"]
    for L, tgt in o["target_cum"].items():
        if L < 3: continue   # first levels are 1-3 days: whole-day rounding dominates, see docs/design/economy.md
        if abs(cas[L] - tgt) > 0.2 * tgt: errs.append(f"casual HQ{L}: {cas[L]}d vs target {tgt}d (>20%)")
    for L in cas:
        if L >= 3 and cas[L] / act[L] > 1.35: errs.append(f"active only {cas[L]/act[L]:.2f}x faster at HQ{L} (>1.35x)")
    y = yaml.safe_load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "content", "economy.yaml"), encoding="utf-8"))["passive"]
    c = y["collectors_count"]
    for L in range(1, HQ_MAX + 1):
        if min(c["base"] + L // c["hq_divisor"], c["max"]) != collectors(L)[0]: errs.append(f"yaml collectors_count != sim at HQ{L}")
    r, st = y["rate_per_hour_each"], y["storage_per_resource"]
    for L in range(1, HQ_MAX + 1):
        want = r["base"] if L == 1 else round(r["base"] * r["growth"] ** (L - 1), -1)
        if want != collectors(L)[1]: errs.append(f"yaml rate != sim at HQ{L}")
        if round(st["base"] * st["growth"] ** (L - 1), -2) != storage(L): errs.append(f"yaml storage != sim at HQ{L}")
    # start_grant: yaml == sim, fits HQ1 storage, leaves a real HQ1 bill (<= 75%) and covers the onboarding spend
    g = yaml.safe_load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "content", "economy.yaml"), encoding="utf-8")).get("start_grant", {})
    for k in ("cash", "fuel", "gems", "alloy", "trophies"):
        if g.get(k, 0) != START_GRANT[k]: errs.append(f"yaml start_grant.{k} != sim")
    for k, bill in (("cash", cost_cash(1)), ("fuel", cost_fuel(1))):
        if START_GRANT[k] > storage(1): errs.append(f"start_grant.{k} exceeds HQ1 storage")
        if START_GRANT[k] > 0.75 * bill: errs.append(f"start_grant.{k} > 75% of HQ1 bill")
    if START_GRANT["cash"] < ONBOARD_CASH or START_GRANT["fuel"] < ONBOARD_FUEL: errs.append("start_grant below onboarding spend")
    errs += check_building_costs()
    return errs

if __name__ == "__main__":
    o = report()
    print("HQ | days | cash total | fuel total | cash/d | fuel/d | collectors x rate/h | storage | loot cap")
    for r in o["calibration"]:
        print(f'{r["hq"]:>2} | {r["days"]:>4} | {r["cash_total"]:>10,} | {r["fuel_total"]:>10,} | {r["need_cash_day"]:>6,} | {r["need_fuel_day"]:>6,} | {r["collectors"]} x {r["rate_h"]:>6,} | {r["storage"]:>7,} | {r["loot_cap"]:>7,}')
    cash, secs = structure_bills()
    print("\nHQ | structure cash bill | vs cash bill | builder-days | vs builder budget   (content/building_costs.yaml)")
    for L in range(1, HQ_MAX + 1):
        print(f"{L:>2} | {cash[L]:>19,.0f} | {cash[L]/cost_cash(L):>11.2f}x | {secs[L]/86400:>12.1f} | {secs[L]/(TARGET_DAYS[L]*BUILDERS[L]*0.75*86400):>10.2f}x")
    print("\nTarget cumulative days (casual):", o["target_cum"])
    for prof, d in o["sim"].items(): print(f"sim {prof:8s}: day reaching HQ(L+1) =", d)
    if "--json" in sys.argv: json.dump(o, open(sys.argv[sys.argv.index("--json")+1], "w"), ensure_ascii=False, indent=1)
    if "--check" in sys.argv:
        e = check(o)
        ratio = {L: round(o["sim"]["casual"][L] / o["sim"]["active"][L], 2) for L in o["sim"]["casual"]}
        print("casual/active speed ratio per HQ:", ratio)
        print("CHECK:", ("FAIL" + chr(10) + "  " + (chr(10) + "  ").join(e)) if e else "OK")
        sys.exit(1 if e else 0)
