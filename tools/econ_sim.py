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

# --- army and laboratory (ECON-5, docs/design/army-lab.md). Unit stats, prices and capacities come from content/*.yaml; the values below are
# assumptions of the player model (not game balance): see docs/design/economy.md "Армия и лаборатория".
RAID_ARMY_LOSS = dict(unit=0.15, garage=0.10, spell=0.25)   # ASSUMPTION: share of the full pool lost/used per raid (OQ-ECON5-1)
RES_SHARE = 0.35           # ASSUMPTION: share of the fuel bill of an HQ level that a player puts into research (rest: army build-up, heroes, reserve)
RES_TIER = {1:1, 2:1, 3:1, 4:2, 5:2, 6:2, 7:3, 8:3, 9:3, 10:4}   # ASSUMPTION: tier of the node taken at HQ L = tier of the nodes whose lab level (HQ-1) is open: 1 -> lab<=2, 2 -> lab 3..5, 3 -> lab 6..8, 4 -> lab 9+ (tier 5: R2)
RAID_COST_MAX = 0.15       # economy.md criterion 3: a lost attack costs at most this share of the daily income
PAY_CAP_X = 1.6            # OQ-ARMY-4 / economy.md criterion 4: an active payer must not beat the casual pace by more than this
PAY_REF_GEMS_DAY = 10      # gems/day an active payer turns into timer time in the check: ~ the free gems (economy.md: ~60 a week); monetization is not designed yet
PAYERS = {"free": 0, "ref": PAY_REF_GEMS_DAY, "100/day": 100, "500/day": 500, "unlimited": math.inf}   # reported; only `free` and `ref` are checked


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

def simulate(profile, seed=1, gems_day=0.0, stats=None):
    """Day-by-day agent. Besides the HQ upgrade bill it pays the army (raid losses are trained again out of the wallet), researches in the
    laboratory (one node at a time, fuel up front, ready after the tier hours) and, if gems_day > 0, spends gems on timers
    (speedup = ceil(hours^0.8 * 4), the same function for builds, queues and research). stats[L] collects what was spent at HQ L."""
    import random
    rnd = random.Random(seed)
    p = PROFILES[profile]
    plan, tiers = army_plan(), _load("research.yaml")["tiers"]
    typ = typical_timer_h()
    day = 0; L = 1; progress_c = 0.0; progress_f = 0.0
    bank_c = float(START_GRANT['cash']); bank_f = float(START_GRANT['fuel'])   # new guest starts with the grant
    day_at = {}
    lab_free = 0.0
    new_st = lambda: dict(res_fuel=0.0, op_fuel=0.0, op_cash=0.0, inc_f=0.0, inc_c=0.0, gems=0.0, lab_h=0.0, nodes=0, days=0)
    st_l = new_st()
    while L <= HQ_MAX and day < 600:
        day += 1; st_l["days"] += 1
        cnt, rate = collectors(L)
        st = storage(L)
        # passive: accumulates between sessions, capped by storage between collections
        gap_h = 24 / p["sessions"]
        per_collect = min(cnt * rate * gap_h, st * 0.5)   # half of storage per resource pool is collector share
        got_c = got_f = 0.0
        for _ in range(p["sessions"]):
            got_c += per_collect; got_f += per_collect * 0.55
        # raids: each raid wins avg ~1.9 stars, loot = pct of victim stock (peer-level) capped
        n_raids = int(p["sessions"] * p["raids"])
        pl = plan[L]
        exp_loot = min(st * 0.55 * LOOT_PCT * 3 * EXP_STAR_MULT, LOOT_CAP_PER_HQ(L))   # cash of an average raid, fuel is 0.6 of it
        if exp_loot * 0.6 < pl["raid_fuel"] or exp_loot < pl["raid_cash"] + search_cost(L): n_raids = 0   # no raiding at a loss: each wallet must come out ahead
        for _ in range(n_raids):
            stars = rnd.choices([0,1,2,3], weights=[.15,.30,.35,.20])[0]
            victim_stock = st * 0.55
            loot = min(victim_stock * LOOT_PCT * 3 * STAR_MULT[stars], LOOT_CAP_PER_HQ(L))
            got_c += loot; got_f += loot * 0.6
        # campaign (energy-gated)
        stages = min(p["campaign"] * p["sessions"] / 2, CAMPAIGN_ENERGY_PER_DAY)
        camp = stages * st * 0.03 * CAMPAIGN_MULT
        got_c += camp; got_f += camp * 0.8
        st_l["inc_f"] += got_f; st_l["inc_c"] += got_c
        bank_c = min(bank_c + got_c, st * 2); bank_f = min(bank_f + got_f, st * 2)   # a day's intake is spent between collections: the wallet holds at most 2 vault sets
        # army upkeep: the units spent in the day's raids are trained again, every raid also pays the opponent search (operating cost, not progress)
        op_f = min(bank_f, n_raids * pl["raid_fuel"]); op_c = min(bank_c, n_raids * (pl["raid_cash"] + search_cost(L)))
        bank_f -= op_f; bank_c -= op_c; st_l["op_fuel"] += op_f; st_l["op_cash"] += op_c
        budget_c = cost_cash(L); budget_f = cost_fuel(L)
        # laboratory: at each session, if the lab is free, start the next node while the research share of the level's fuel bill allows it
        gems_left = gems_day
        tier = tiers[RES_TIER[L]]
        for k in range(p["sessions"]):
            t = day - 1 + (k + 0.5) / p["sessions"]
            if (L >= 2 and lab_free <= t and bank_f >= tier["fuel"] and progress_f + tier["fuel"] <= budget_f
                    and st_l["res_fuel"] + 0.5 * tier["fuel"] <= RES_SHARE * budget_f):
                bank_f -= tier["fuel"]; progress_f += tier["fuel"]; st_l["res_fuel"] += tier["fuel"]; st_l["nodes"] += 1
                g = speedup_gems(tier["hours"])
                if gems_left >= g: gems_left -= g; st_l["gems"] += g; lab_free = t
                else: lab_free = t + tier["hours"] / 24; st_l["lab_h"] += tier["hours"]
        # builder constraint: each builder can burn ~ eff of its day; time-gated upgrades
        need_builder_days = TARGET_DAYS[L] * BUILDERS[L] * 0.75
        builder_days = (BUILDERS[L] + (1 if gems_day else 0)) * p["eff"]   # a payer also owns the extra builder (economy.yaml extra_builder_gems)
        if gems_left == math.inf: builder_days = math.inf
        elif gems_left > 0:    # gems skip builder time: a timer of t hours costs speedup_gems(t), i.e. speedup_gems(t) / t per builder-hour
            builder_days += gems_left / (24 * speedup_gems(typ[L]) / typ[L]); st_l["gems"] += gems_left
        spend_c = min(bank_c, budget_c - progress_c)
        spend_f = min(bank_f, budget_f - progress_f)
        # pace limited by builders
        pace = builder_days / need_builder_days
        spend_c = min(spend_c, budget_c * pace); spend_f = min(spend_f, budget_f * pace)
        progress_c += spend_c; progress_f += spend_f; bank_c -= spend_c; bank_f -= spend_f
        if progress_c >= budget_c * 0.999 and progress_f >= budget_f * 0.999:
            day_at[L] = day; L += 1; progress_c = progress_f = 0.0
            if stats is not None: stats[L - 1] = st_l
            st_l = new_st()
    return day_at

EXP_STAR_MULT = 0.30 * STAR_MULT[1] + 0.35 * STAR_MULT[2] + 0.20 * STAR_MULT[3]   # mean of the raid star weights used in simulate()
def search_cost(L): return round(20 * 1.9 ** (L - 1), -1)   # = economy.yaml raids.matchmaking.search_cost_cash_by_hq

def speedup_gems(hours):
    """Gems to finish a timer with `hours` left: ceil(hours^exponent * factor) (content/building_costs.yaml speedup_gems; ux-onboarding)."""
    k = _load("building_costs.yaml")["speedup_gems"]
    return math.ceil(hours ** k["exponent"] * k["factor"])

def typical_timer_h():
    """Mean builder timer (hours) per HQ level: builder-seconds of the structure bill / number of upgrades."""
    counts = {}; _, secs = structure_bills(counts=counts)
    return {L: max(secs[L] / max(counts[L], 1) / 3600, 0.25) for L in secs}

def army_plan():
    """Per HQ level (end of the level, every pool building at its level cap): housing, the price of a full army, what a raid costs to replace,
    producer slots. Averages are over the units unlocked at that HQ with equal weight (ASSUMPTION: the player keeps a uniform mix).
    Sources: pools/producers army.yaml, capacity_by_level/max_by_hq buildings.yaml, housing/train_s/cost troops.yaml."""
    a, b, t = _load("army.yaml"), _load("buildings.yaml"), _load("troops.yaml")
    defs = {x["id"]: x for g in ("core", "resources", "military") for x in b[g]}
    def n_of(x, L): return max([v for k, v in sorted(x["max_by_hq"].items()) if k <= L] or [0])
    def lvl_of(x, L): return 0 if L < x.get("unlock_hq", 1) else min(x["levels"], L - x.get("unlock_hq", 1) + 1)
    def cap(pool, L):
        x = defs[a["pool_building"][pool]]; lv = lvl_of(x, L)
        return n_of(x, L) * x["capacity_by_level"][lv - 1] if lv else 0
    def slots(bid, L): return n_of(defs[bid], L) if lvl_of(defs[bid], L) else 0
    pools = {"unit": t["troops"], "garage": t["vehicles"] + t["ships"], "spell": t["spells"]}
    plan = {}
    for L in range(1, HQ_MAX + 1):
        row = {}
        for pool, us in pools.items():
            us = [u for u in us if u["unlock"]["hq"] <= L]
            c = cap(pool, L)
            if not us or not c: row[pool] = dict(cap=c, fuel=0, cash=0, train_s=0); continue
            row[pool] = dict(cap=c,
                fuel=c * sum(u["cost"].get("fuel", 0) / u["housing"] for u in us) / len(us),
                cash=c * sum(u["cost"].get("cash", 0) / u["housing"] for u in us) / len(us),
                train_s=c * sum(u["train_s"] / u["housing"] for u in us) / len(us))
        # garage units are produced by the factory (vehicles) and the shipyard (ships), one unit at a time per building
        row["slots"] = dict(unit=slots("b_barracks", L), garage=slots("b_factory", L) + slots("b_shipyard", L), spell=slots("b_support_bay", L))
        row["raid_fuel"] = sum(row[k]["fuel"] * RAID_ARMY_LOSS[k] for k in RAID_ARMY_LOSS)
        row["raid_cash"] = sum(row[k]["cash"] * RAID_ARMY_LOSS[k] for k in RAID_ARMY_LOSS)
        row["full_fuel"] = sum(row[k]["fuel"] for k in RAID_ARMY_LOSS); row["full_cash"] = sum(row[k]["cash"] for k in RAID_ARMY_LOSS)
        plan[L] = row
    for L in plan: plan[L]["build_fuel"] = plan[L]["full_fuel"] - (plan[L - 1]["full_fuel"] if L > 1 else 0)
    return plan

def research_menu():
    """Fuel and lab hours of the research.yaml nodes (not `soon`) by the HQ level where they first become available:
    lab level + 1 (the lab reaches level n at HQ n+1) and the unlock HQ of the unit/spell."""
    t, r = _load("troops.yaml"), _load("research.yaml")
    unl = {u["id"]: u["unlock"]["hq"] for g in ("troops", "vehicles", "ships", "spells") for u in t[g]}
    fuel, hours = {L: 0 for L in range(1, HQ_MAX + 1)}, {L: 0 for L in range(1, HQ_MAX + 1)}
    for n in r["nodes"]:
        if n.get("soon"): continue
        a = min(HQ_MAX, max(n["lab"] + 1, unl.get(n.get("unit") or n.get("spell"), 1)))
        fuel[a] += r["tiers"][n["tier"]]["fuel"]; hours[a] += r["tiers"][n["tier"]]["hours"]
    return fuel, hours

def _load(name):
    import os, yaml
    return yaml.safe_load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "content", name), encoding="utf-8"))

def price_book():
    """content/building_costs.yaml -> (file, {id: (cost, time_s, cost_growth, time_growth, max_level)}); the single source of prices."""
    f = _load("building_costs.yaml")
    book = {i: (e["cost"], e["time_s"], e.get("cost_growth", f["cost_growth"]), e.get("time_growth", f["time_growth"]), e.get("max_level"))
            for i, e in f["buildings"].items()}
    return f, book

def structure_bills(f=None, book=None, counts=None):
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
        c = t = 0; n_up = 1 if L < HQ_MAX else 0
        for x in defs:
            cost, t1, cg, tg, _ = book[x["id"]]
            if x["id"] == "b_hq":
                if L < HQ_MAX: c += cost["cash"] * cg ** L; t += t1 * tg ** L
                continue
            nL, nP = count(x, L), (count(x, L - 1) if L > 1 else 0)
            cL, cP = cap(x, L), (cap(x, L - 1) if L > 1 else 0)
            lv = list(range(cP + 1, cL + 1)) * nP + list(range(1, cL + 1)) * (nL - nP)
            n_up += len(lv)
            for n in lv:
                c += cost.get("cash", 0) * cg ** (n - 1); t += t1 * tg ** (n - 1)
        cash[L], secs[L] = c, t
        if counts is not None: counts[L] = n_up
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
    out = dict(calibration=cal, split=SPLIT, sim={}, stats={}, pay={})
    tot_t = 0; cum = {}
    for r in cal: tot_t += r["days"]; cum[r["hq"]] = tot_t
    out["target_cum"] = cum
    for prof in PROFILES:
        out["stats"][prof] = {}
        out["sim"][prof] = simulate(prof, stats=out["stats"][prof])
    for name, g in PAYERS.items(): out["pay"][name] = simulate("active", gems_day=g)   # an active player who also spends gems/day on timers
    out["army"], out["menu"] = army_plan(), research_menu()
    return out

def refill_hours(L, raids, plan=None):
    """Hours the producers need to train again the units lost in `raids` raids at HQ L (pools train in parallel, buildings in a pool too)."""
    pl = (plan or army_plan())[L]
    return max((pl[k]["train_s"] * RAID_ARMY_LOSS[k] * raids / max(pl["slots"][k], 1) / 3600 for k in RAID_ARMY_LOSS if pl[k]["cap"]), default=0)

def check_army_lab(o):
    """ECON-5: army, laboratory and gems against the pacing model. Returns (errors, warnings); warnings are requests to other modules."""
    errs, warns = [], []
    plan, stats, tiers = o["army"], o["stats"], _load("research.yaml")["tiers"]
    k = _load("building_costs.yaml")["speedup_gems"]
    if (k["factor"], k["exponent"]) != (4, 0.8): errs.append("building_costs speedup_gems != ceil(h^0.8*4) (OQ-ARMY-4, one function for builds, queues and research)")
    for L, t in RES_TIER.items():
        if t not in tiers: errs.append(f"RES_TIER[{L}]={t} is not a tier of research.yaml")
    for L in range(1, HQ_MAX + 1):
        bill = cost_fuel(L); pl = plan[L]
        if pl["build_fuel"] + RES_SHARE * bill > 0.9 * bill:
            errs.append(f"HQ{L}: army build-up {pl['build_fuel']:,.0f} + research share {RES_SHARE*bill:,.0f} leave < 10% of the fuel bill {bill:,.0f}")
        for prof, p in PROFILES.items():
            gap = 24 / p["sessions"]
            if refill_hours(L, p["raids"], plan) > 0.5 * gap: errs.append(f"{prof} HQ{L}: refilling {p['raids']} raids takes {refill_hours(L, p['raids'], plan):.1f} h of a {gap:.1f} h gap between sessions")
        s = stats["casual"][L]
        # economy.md criterion 3 (army-lab.md §11 p.8): one lost attack costs <= 15% of the daily income; an average attack must, a failed one (2x the average loss) is a request
        for res, inc, cost in (("fuel", s["inc_f"], pl["raid_fuel"]), ("cash", s["inc_c"], pl["raid_cash"] + search_cost(L))):
            d = inc / s["days"]
            if cost > RAID_COST_MAX * d: errs.append(f"HQ{L}: an average raid costs {cost:,.0f} {res} = {cost/d:.0%} of the casual daily income (want <= {RAID_COST_MAX:.0%})")
            elif 2 * cost > RAID_COST_MAX * d: warns.append(f"HQ{L}: a failed raid (2x the average loss) costs {2*cost/d:.0%} of the daily {res} income (criterion 3: <= {RAID_COST_MAX:.0%}); content: army housing/prices, or economy: income")
        if s["lab_h"] > 0.9 * s["days"] * 24: errs.append(f"casual HQ{L}: lab busy {s['lab_h']:.0f} h of {s['days']*24} h")
    cum_menu = cum_res = 0
    menu_fuel = o["menu"][0]
    for L in range(1, HQ_MAX + 1):
        cum_menu += menu_fuel[L]; cum_res += stats["casual"][L]["res_fuel"]
        if cum_res > cum_menu: warns.append(f"HQ{L}: casual research {cum_res:,.0f} fuel by this HQ exceeds the nodes in research.yaml {cum_menu:,.0f} (content: needs {cum_res-cum_menu:,.0f} more, tier {RES_TIER[L]})")
    cas = o["sim"]["casual"]
    for name in ("free", "ref"):
        for L in range(3, HQ_MAX + 1):
            if cas[L] / o["pay"][name][L] > PAY_CAP_X: errs.append(f"payer {name} ({PAYERS[name]} gems/day) is {cas[L]/o['pay'][name][L]:.2f}x faster than casual at HQ{L} (>{PAY_CAP_X}x)")
    for name, g in PAYERS.items():
        top = max(cas[L] / o["pay"][name][L] for L in range(3, HQ_MAX + 1))
        if top > PAY_CAP_X: warns.append(f"payer {name}: up to {top:.2f}x faster than casual (cap {PAY_CAP_X}x), not a check target")
    return errs, warns

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
    y_all = yaml.safe_load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "content", "economy.yaml"), encoding="utf-8"))
    g = y_all.get("start_grant", {})
    for k in ("cash", "fuel", "gems", "alloy", "trophies"):
        if g.get(k, 0) != START_GRANT[k]: errs.append(f"yaml start_grant.{k} != sim")
    for k, bill in (("cash", cost_cash(1)), ("fuel", cost_fuel(1))):
        if START_GRANT[k] > storage(1): errs.append(f"start_grant.{k} exceeds HQ1 storage")
        if START_GRANT[k] > 0.75 * bill: errs.append(f"start_grant.{k} > 75% of HQ1 bill")
    if START_GRANT["cash"] < ONBOARD_CASH or START_GRANT["fuel"] < ONBOARD_FUEL: errs.append("start_grant below onboarding spend")
    errs += check_building_costs()
    al, pc = y_all.get("army_lab", {}), y_all.get("payer_check", {})
    if {k: round(v * 100) for k, v in RAID_ARMY_LOSS.items()} != al.get("raid_army_loss_pct"): errs.append("yaml army_lab.raid_army_loss_pct != sim RAID_ARMY_LOSS")
    if round(RES_SHARE * 100) != al.get("research_share_of_fuel_bill_pct"): errs.append("yaml army_lab.research_share_of_fuel_bill_pct != sim RES_SHARE")
    if RES_TIER != al.get("research_tier_by_hq"): errs.append("yaml army_lab.research_tier_by_hq != sim RES_TIER")
    if (pc.get("max_speed_x_vs_casual"), pc.get("ref_gems_per_day")) != (PAY_CAP_X, PAY_REF_GEMS_DAY): errs.append("yaml payer_check != sim PAY_CAP_X / PAY_REF_GEMS_DAY")
    errs += check_army_lab(o)[0]
    return errs

def print_army_lab(o):
    plan, stats, menu = o["army"], o["stats"], o["menu"]
    print("\nArmy (content/army.yaml, troops.yaml, buildings.yaml): a full army at the end of each HQ level")
    print("HQ | housing unit/garage/spell | full army fuel | full army cash | fuel vs tank | casual fuel/d | raid loses fuel / cash+search | refill 2 raids, min | build-up fuel")
    for L in range(1, HQ_MAX + 1):
        pl, s = plan[L], stats["casual"][L]
        print(f'{L:>2} | {pl["unit"]["cap"]:>4}/{pl["garage"]["cap"]:>3}/{pl["spell"]["cap"]:>2} | {pl["full_fuel"]:>14,.0f} | {pl["full_cash"]:>14,.0f} | {pl["full_fuel"]/storage(L):>11.2f}x | {s["inc_f"]/s["days"]:>13,.0f} | {pl["raid_fuel"]:>8,.0f} / {pl["raid_cash"]+search_cost(L):>8,.0f} | {refill_hours(L, 2, plan)*60:>19.1f} | {pl["build_fuel"]:>13,.0f}')
    print("\nLaboratory (research.yaml): what the casual player researches per HQ level (tier per RES_TIER, share RES_SHARE of the fuel bill)")
    print("HQ | tier | nodes | research fuel | % of fuel bill | lab busy h / h at level | fuel of the nodes that open at this HQ (research.yaml)")
    for L in range(1, HQ_MAX + 1):
        s = stats["casual"][L]
        print(f'{L:>2} | {RES_TIER[L]:>4} | {s["nodes"]:>5} | {s["res_fuel"]:>13,.0f} | {s["res_fuel"]/cost_fuel(L):>13.0%} | {s["lab_h"]:>8.0f} / {s["days"]*24:>5} | {menu[0][L]:>9,}')
    print("\nGems (ceil(h^0.8 x 4), one function): research tier 1..5 skip =", [speedup_gems(h) for h in (2, 8, 24, 72, 168)], "gems; the whole barracks queue of a raid refill at HQ10 =", speedup_gems(refill_hours(HQ_MAX, 4) * 1) if refill_hours(HQ_MAX, 4) else 0, "gems")
    print("Active player + gems/day on timers (and the 6th builder), day reaching HQ(L+1); x = casual day / payer day (max over HQ3..10):")
    for name, d in o["pay"].items(): print(f"  {name:>9}: {list(d.values())}  max x = {max(o['sim']['casual'][L] / d[L] for L in range(3, HQ_MAX + 1)):.2f}")

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
    print_army_lab(o)
    if "--json" in sys.argv: json.dump(o, open(sys.argv[sys.argv.index("--json")+1], "w"), ensure_ascii=False, indent=1)
    if "--check" in sys.argv:
        e = check(o)
        ratio = {L: round(o["sim"]["casual"][L] / o["sim"]["active"][L], 2) for L in o["sim"]["casual"]}
        print("casual/active speed ratio per HQ:", ratio)
        for w in check_army_lab(o)[1]: print("WARN:", w)
        print("CHECK:", ("FAIL" + chr(10) + "  " + (chr(10) + "  ").join(e)) if e else "OK")
        sys.exit(1 if e else 0)
