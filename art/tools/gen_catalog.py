#!/usr/bin/env python3
"""Генерирует art/CATALOG.md из content/*.yaml + art/src (категории, модели, части манифеста).

  python art/tools/gen_catalog.py            # переписать art/CATALOG.md
  python art/tools/gen_catalog.py --check    # код 1, если файл устарел (для check_art.py / CI)
Приоритет и сложность считаются правилами artlib.py; точечные правки — art/src/catalog_overrides.json ({id: {priority, complexity}}).
"""
import os
import sys

sys.stdout.reconfigure(encoding="utf-8", errors="replace")
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import artlib as A  # noqa: E402

PATH = os.path.join(A.ART, "CATALOG.md")
ORDER = ["terrain", "decor", "buildings", "defences", "units", "vehicles", "ships", "heroes", "spells_fx", "ui"]


def overrides():
    p = os.path.join(A.SRC, "catalog_overrides.json")
    return A.jread(p) if os.path.exists(p) else {}


def rows():
    ov = overrides()
    cats = A.categories()
    out = []
    for cat in ORDER:
        part = A.read_part(cat)
        lock = part.get("lock", {})
        for e in A.category_entities(cat):
            m = e["model"]
            pl = A.plan(e)
            status = "done" if m and m.get("status") == "done" else "wip" if m else "placeholder"
            out.append({"e": e, "cat": cat, "plan": pl, "status": status, "pri": A.priority(e, ov), "cx": A.complexity(e, ov),
                        "built": lock.get(e["id"], {}).get("renderer", "—")})
    return out


def px(r):
    """Планируемая площадь кадров сущности, px."""
    c = A.categories()[r["cat"]]
    e, pl = r["e"], r["plan"]
    if c["kind"] == "building" and e["size"]:
        n = e["size"][0] + e["size"][1]
        return pl["frames"] * n * 32 * (n * 16 + 48)
    fp = c.get("frame_px") or [64, 64]
    return pl["frames"] * fp[0] * fp[1]


def projection():
    """(полный, первой загрузки) — оценка WebP в байтах по плану каталога."""
    bpp = A.config()["budget"]["webp_bytes_per_px"]
    rs = rows()
    return (sum(px(r) for r in rs) * bpp, sum(px(r) for r in rs if A.categories()[r["cat"]]["first_load"]) * bpp)


def render():
    cfg = A.config()
    cats = A.categories()
    rows = globals()["rows"]()
    bpp = cfg["budget"]["webp_bytes_per_px"]
    L = ["# art/CATALOG.md — каталог сущностей art", "",
         "**Файл сгенерирован** `python art/tools/gen_catalog.py` из `content/*.yaml` и `art/src/`; руками не править (`check_art.py` проверяет актуальность). "
         "Точечные правки приоритета/сложности — `art/src/catalog_overrides.json`.", "",
         "- **Приоритет.** P0 — нужен экранам M1–M3 (база, армия, бой): ASSUMPTION — открывается на HQ ≤ 3 (первые уровни прогрессии), штаб, препятствия карты, terrain/ui; "
         "P1 — HQ 4–6, командир, препятствия/декор карты; P2 — остальное (HQ ≥ 7, остальные герои, декор). Определения M1–M3 в репозитории нет — см. `docs/open-questions.md` OQ-ART2-2.",
         "- **Сложность модели.** S — простая форма (пехота, малые здания/оборона ≤ 2 клеток, декор, иконки); M — техника, крупные юниты, здания 3×3; L — здания 4×4, корабли, герои.",
         "- **Статус.** `placeholder` — нет модели, в атласе заглушка ART-0; `wip` — модель есть в `art/src/<cat>/models.json`, в игру не идёт (рендер: `build_art.py --preview`); `done` — собрана в атлас и манифест.",
         "- **Направления/анимации/состояния** — цель по `art/STYLE.md` §6 (план), фактический состав — в `art/manifest.json`. Тиры зданий (ярусы L1–3, L4–6…) — оценка `ceil(levels/3)`, максимум 4; "
         "схема именования тиров в манифесте — предложение OQ-ART2-3.", ""]
    tot = {s: 0 for s in ("placeholder", "wip", "done")}
    for r in rows:
        tot[r["status"]] += 1
    full = sum(px(r) for r in rows) * bpp
    L += ["## Сводка", "", "| Категория | Сущностей | P0 | P1 | P2 | placeholder | wip | done | Планируемых кадров | Оценка WebP, МБ |", "|---|---|---|---|---|---|---|---|---|---|"]
    for cat in ORDER:
        rs = [r for r in rows if r["cat"] == cat]
        c = lambda f: sum(1 for r in rs if f(r))
        L.append(f"| `{cat}` | {len(rs)} | {c(lambda r: r['pri'] == 'P0')} | {c(lambda r: r['pri'] == 'P1')} | {c(lambda r: r['pri'] == 'P2')} | "
                 f"{c(lambda r: r['status'] == 'placeholder')} | {c(lambda r: r['status'] == 'wip')} | {c(lambda r: r['status'] == 'done')} | "
                 f"{sum(r['plan']['frames'] for r in rs)} | {sum(px(r) for r in rs) * bpp / 1e6:.1f} |")
    L.append(f"| **всего** | {len(rows)} | {sum(1 for r in rows if r['pri'] == 'P0')} | {sum(1 for r in rows if r['pri'] == 'P1')} | {sum(1 for r in rows if r['pri'] == 'P2')} | "
             f"{tot['placeholder']} | {tot['wip']} | {tot['done']} | {sum(r['plan']['frames'] for r in rows)} | {full / 1e6:.1f} |")
    L += ["", f"Оценка размера — кадры × площадь × {bpp} Б/px (замер WebP q90 в спайке ART-1); бюджет: первая загрузка < {cfg['budget']['first_load_mb']} МБ, полный < {cfg['budget']['full_mb']} МБ "
          "(проверяет `check_art.py` по факту и по плану).", ""]
    for cat in ORDER:
        rs = [r for r in rows if r["cat"] == cat]
        c = cats[cat]
        L += [f"## `{cat}` — {c['doc']}", "", f"Атлас: `{c['atlas']}`; первая загрузка: {'да' if c['first_load'] else 'нет (лениво)'}; исходники: `art/src/{cat}/`.", ""]
        if not rs:
            L += ["_Сущностей пока нет — id добавляются в `art/src/%s/models.json` с префиксом `%s`._" % (cat, c.get("id_prefix", "")), ""]
            continue
        L += ["| id | Футпринт | Напр. | Анимации (кадров) | Состояния × тиры | Кадров (план) | Приоритет | Сложность | Статус |", "|---|---|---|---|---|---|---|---|---|"]
        for r in sorted(rs, key=lambda r: (r["pri"], r["e"]["id"])):
            e, pl = r["e"], r["plan"]
            fp = f"{e['size'][0]}×{e['size'][1]}" if e["size"] else "—"
            an = ", ".join(f"{k} {v}" for k, v in pl["anims"].items()) or "—"
            st = (f"{'/'.join(pl['states'])} × {pl['tiers']}" if pl["states"] else "—")
            L.append(f"| `{e['id']}` | {fp} | {pl['dirs']} | {an} | {st} | {pl['frames']} | {r['pri']} | {r['cx']} | {r['status']} |")
        L.append("")
    return "\n".join(L).rstrip() + "\n"


def write():
    with open(PATH, "w", encoding="utf-8", newline="\n") as f:
        f.write(render())
    print("записан", os.path.relpath(PATH, A.ROOT))


def main():
    if "--check" in sys.argv:
        cur = open(PATH, encoding="utf-8", newline="").read().replace("\r\n", "\n") if os.path.exists(PATH) else ""
        if cur != render():
            print("art/CATALOG.md устарел: python art/tools/gen_catalog.py")
            return 1
        print("art/CATALOG.md актуален")
        return 0
    write()
    return 0


if __name__ == "__main__":
    sys.exit(main())
