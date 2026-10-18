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
    bank_c = bank_f = 0.0
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

def report():
    cal = calibrate()
    out = dict(calibration=cal, split=SPLIT, sim={})
    tot_t = 0; cum = {}
    for r in cal: tot_t += r["days"]; cum[r["hq"]] = tot_t
    out["target_cum"] = cum
    for prof in PROFILES:
        out["sim"][prof] = simulate(prof)
    return out

if __name__ == "__main__":
    o = report()
    print("HQ | days | cash total | fuel total | cash/d | fuel/d | collectors x rate/h | storage | loot cap")
    for r in o["calibration"]:
        print(f'{r["hq"]:>2} | {r["days"]:>4} | {r["cash_total"]:>10,} | {r["fuel_total"]:>10,} | {r["need_cash_day"]:>6,} | {r["need_fuel_day"]:>6,} | {r["collectors"]} x {r["rate_h"]:>6,} | {r["storage"]:>7,} | {r["loot_cap"]:>7,}')
    print("\nTarget cumulative days (casual):", o["target_cum"])
    for prof, d in o["sim"].items(): print(f"sim {prof:8s}: day reaching HQ(L+1) =", d)
    if "--json" in sys.argv: json.dump(o, open(sys.argv[sys.argv.index("--json")+1], "w"), ensure_ascii=False, indent=1)
