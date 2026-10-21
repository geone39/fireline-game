#!/usr/bin/env python3
"""Проверки art (ART-2): `make art-check`. Только stdlib + PyYAML, без Blender/Pillow/Node. Код 1 при ошибках.

  1. id в частях манифеста = id из content (+ свободные t_*/ui_* из models.json); модели не ссылаются на лишние id;
  2. art/manifest.json == сборка из art/src/<cat>/manifest.part.json (никто не правил его руками);
  3. спрайты: ключи, атлас, кадры внутри атласа и без наложений, размеры кадра, якорь, число кадров, направления/анимации;
  4. атласы: PNG (и WebP) на месте, размер ≤ 2048, совпадает с манифестом и TexturePacker-JSON;
  5. бюджеты: первая загрузка < 3 МБ, полный < 40 МБ (по факту; по плану каталога — предупреждение);
  6. модели: структура, палитра, статусы; хэш входов совпадает с lock (изменил модель — пересобери);
  7. шаблоны art/templates; актуальность art/CATALOG.md.
"""
import json
import os
import struct
import sys

sys.stdout.reconfigure(encoding="utf-8", errors="replace")
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import artlib as A  # noqa: E402
import gen_catalog  # noqa: E402

errs, warns = [], []
E = errs.append
W = warns.append
SHAPES = ("box", "cyl", "cone", "sphere")
KINDS = ("building", "unit")


def png_size(path):
    with open(path, "rb") as f:
        d = f.read(32)
    if d[:8] != b"\x89PNG\r\n\x1a\n":
        return None
    return list(struct.unpack(">II", d[16:24]))


def is_webp(path):
    with open(path, "rb") as f:
        d = f.read(12)
    return d[:4] == b"RIFF" and d[8:12] == b"WEBP"


def check_palette(pal):
    for k, v in pal.items():
        if k in ("_doc", "light"):
            continue
        if not (isinstance(v, str) and len(v) == 7 and v[0] == "#" and all(c in "0123456789abcdefABCDEF" for c in v[1:])):
            E(f"palette.json: {k}={v!r} — не #rrggbb")


def check_model(m, where, pal, frame_px=None):
    i = m.get("id", "?")
    p = f"{where}/{i}"
    if m.get("kind") not in KINDS:
        E(f"{p}: kind должен быть {'|'.join(KINDS)}")
        return
    fr, an = m.get("frame"), m.get("anchor")
    if not (isinstance(fr, list) and len(fr) == 2 and all(isinstance(x, int) and 0 < x <= 1024 for x in fr)):
        E(f"{p}: frame должен быть [w, h] целых ≤ 1024")
        return
    if not (isinstance(an, list) and len(an) == 2 and 0 <= an[0] <= fr[0] and 0 <= an[1] <= fr[1]):
        E(f"{p}: anchor вне кадра")
    if frame_px and fr != frame_px:
        E(f"{p}: frame {fr} != {frame_px} (art/src/categories.json)")
    if m.get("status", "wip") not in A.STATUSES:
        E(f"{p}: status {m.get('status')!r} не из {A.STATUSES}")
    if m.get("renderer", "blender") not in A.RENDERERS:
        E(f"{p}: renderer {m.get('renderer')!r} не из {A.RENDERERS}")
    if not m.get("parts"):
        E(f"{p}: нет parts")
    groups = m.get("groups", {})
    names = set()
    for pt in m.get("parts", []):
        n = pt.get("n")
        if n in names:
            E(f"{p}: дубль детали {n}")
        names.add(n)
        if pt.get("shape") not in SHAPES:
            E(f"{p}/{n}: shape {pt.get('shape')!r} не из {SHAPES}")
        if pt.get("col") not in pal:
            E(f"{p}/{n}: цвет {pt.get('col')!r} не из palette.json")
        if not all(isinstance(pt.get(k), list) and len(pt[k]) == 3 for k in ("c", "s")):
            E(f"{p}/{n}: c и s — по 3 числа")
        if pt.get("g") and pt["g"] not in groups:
            E(f"{p}/{n}: группа {pt['g']} не объявлена")
    if m["kind"] == "unit":
        for g, d in groups.items():
            if d.get("parent") and d["parent"] not in groups:
                E(f"{p}: у группы {g} нет родителя {d['parent']}")
            if "pivot" not in d:
                E(f"{p}: у группы {g} нет pivot")
        anims = m.get("anims") or {}
        if not anims:
            E(f"{p}: unit без anims")
        for a, d in anims.items():
            if not d.get("frames") or not isinstance(d.get("fps"), int):
                E(f"{p}: анимация {a}: нужны fps (целое) и frames")
                continue
            for k, f in enumerate(d["frames"]):
                for g in f:
                    if not g.startswith("_") and g not in groups:
                        E(f"{p}: анимация {a} кадр {k}: группа {g} не объявлена")
    elif groups or m.get("anims"):
        W(f"{p}: у building/prop groups/anims игнорируются рендерером")


def main():
    cfg = A.config()
    cats = cfg["categories"]
    pal = A.palette()
    check_palette(pal)
    content = A.entities()
    cids = {e["id"]: e for e in content}
    parts = {}
    # ---- модели ----
    seen = {}
    for cat, c in cats.items():
        for k in ("atlas", "kind", "dirs", "first_load", "doc"):
            if k not in c:
                E(f"categories.json: {cat}: нет {k}")
        parts[cat] = A.read_part(cat)
        mp = os.path.join(A.cat_dir(cat), "models.json")
        if not os.path.isdir(A.cat_dir(cat)):
            E(f"art/src/{cat}/ отсутствует")
        ms = A.models(cat)
        for i, m in ms.items():
            if i in seen:
                E(f"модель {i} в двух категориях: {seen[i]}, {cat}")
            seen[i] = cat
            if c["content"]:
                if i not in cids:
                    E(f"{cat}: модель {i} — нет такого id в content/")
                elif cids[i]["category"] != cat:
                    E(f"{cat}: модель {i} лежит не в своей категории (по content: {cids[i]['category']})")
                elif m.get("footprint") and cids[i]["size"] and m["footprint"] != cids[i]["size"]:
                    E(f"{cat}/{i}: footprint {m['footprint']} != content {cids[i]['size']}")
            elif not i.startswith(c["id_prefix"]):
                E(f"{cat}: id {i} должен начинаться с {c['id_prefix']}")
            if c["kind"] == "unit" and m.get("kind") != "unit":
                E(f"{cat}/{i}: в категории-риге kind должен быть unit")
            fpx = c.get("frame_px")
            check_model(m, f"src/{cat}", pal, fpx if (m.get("kind") == "unit" or cat == "spells_fx") else None)
            if c["kind"] == "building" and m.get("footprint") and m.get("kind") == "building":
                fw, fh = m["footprint"]
                if m["frame"][0] != (fw + fh) * 32:
                    E(f"{cat}/{i}: ширина кадра {m['frame'][0]} != (fw+fh)*32 = {(fw + fh) * 32}")
    # ---- части манифеста ----
    lockbad = 0
    for cat, c in cats.items():
        part = parts[cat]
        sprites = {s["id"]: s for s in part["sprites"]}
        if len(sprites) != len(part["sprites"]):
            E(f"{cat}: дубль id в manifest.part.json")
        ms = A.models(cat)
        if c["content"]:
            want = {e["id"] for e in content if e["category"] == cat}
        else:
            want = {i for i, m in ms.items() if m.get("status") == "done"}
        for i in sorted(want - set(sprites)):
            E(f"{cat}: нет спрайта {i} (python art/tools/build_art.py --category {cat})")
        for i in sorted(set(sprites) - want):
            E(f"{cat}: лишний спрайт {i} (нет в content/ или модель не done)")
        for i, s in sprites.items():
            lk = part["lock"].get(i)
            if not lk:
                E(f"{cat}/{i}: нет записи lock")
                continue
            r = lk.get("renderer")
            m = ms.get(i)
            if r == "placeholder":
                if m and m.get("status") == "done":
                    E(f"{cat}/{i}: модель done, а в атласе заглушка — пересоберите")
                if "dirs" in s or "anims" in s:
                    E(f"{cat}/{i}: у заглушки не должно быть dirs/anims")
            else:
                if r not in A.RENDERERS:
                    E(f"{cat}/{i}: renderer {r!r} в lock")
                elif not m or m.get("status") != "done":
                    E(f"{cat}/{i}: в атласе рендер, а модель не done — пересоберите")
                elif A.model_hash(m, r, pal) != lk.get("hash"):
                    lockbad += 1
                    E(f"{cat}/{i}: модель/палитра/рендерер изменились после сборки — python art/tools/build_art.py --id {i}")
        # ---- спрайты ----
        for i, s in sprites.items():
            check_sprite(cat, c, s, part, cids.get(i), ms.get(i))
        # ---- атласы ----
        for name, a in part["atlases"].items():
            check_atlas(cat, name, a, part)
    # ---- корневой манифест ----
    mp = os.path.join(A.ART, "manifest.json")
    try:
        want = A.jdump(A.assemble(parts))
    except AssertionError as e:
        E(f"манифест: {e}")
        want = None
    if want is not None:
        cur = open(mp, encoding="utf-8", newline="").read().replace("\r\n", "\n") if os.path.exists(mp) else ""
        if cur != want:
            E("art/manifest.json не совпадает со сборкой из частей — не править руками: python art/tools/build_art.py")
    # ---- бюджеты ----
    check_budget(cfg, parts)
    # ---- шаблоны ----
    check_templates(cats, pal)
    # ---- каталог ----
    if gen_catalog.render() != (open(gen_catalog.PATH, encoding="utf-8", newline="").read().replace("\r\n", "\n") if os.path.exists(gen_catalog.PATH) else ""):
        E("art/CATALOG.md устарел: python art/tools/gen_catalog.py")
    n = sum(len(p["sprites"]) for p in parts.values())
    done = sum(1 for p in parts.values() for l in p["lock"].values() if l["renderer"] != "placeholder")
    print(f"art-check: {n} спрайтов ({done} из моделей, {n - done} заглушек), {len(errs)} ошибок, {len(warns)} предупреждений")
    for w in warns:
        print(" ? ", w)
    for e in errs:
        print(" - ", e)
    return 1 if errs else 0


def check_sprite(cat, c, s, part, ent, model):
    i = s["id"]
    real = "dirs" in s
    keys = {"id", "atlas", "frames", "anchors", "fps"} | ({"dirs", "anims"} if real else set())
    if set(s) != keys:
        E(f"{cat}/{i}: ключи {sorted(s)} != {sorted(keys)}")
        return
    a = part["atlases"].get(s["atlas"])
    if not a:
        E(f"{cat}/{i}: атлас {s['atlas']} не объявлен в части")
        return
    W_, H_ = a["size"]
    if not s["frames"]:
        E(f"{cat}/{i}: нет кадров")
        return
    for f in s["frames"]:
        if set(f) != {"name", "x", "y", "w", "h"}:
            E(f"{cat}/{i}: ключи кадра {f.get('name')}")
            continue
        if f["x"] < 0 or f["y"] < 0 or f["x"] + f["w"] > W_ or f["y"] + f["h"] > H_:
            E(f"{cat}/{i}: кадр {f['name']} вне атласа")
        if not f["name"].startswith(i + "_"):
            E(f"{cat}/{i}: имя кадра {f['name']} должно начинаться с {i}_")
    f0 = s["frames"][0]
    g = s["anchors"].get("ground")
    if not g or not (0 <= g["x"] <= f0["w"] and 0 <= g["y"] <= f0["h"]):
        E(f"{cat}/{i}: якорь ground вне кадра")
    if c["kind"] == "unit" or cat == "spells_fx":
        want = c["frame_px"]
        for f in s["frames"]:
            if [f["w"], f["h"]] != want:
                E(f"{cat}/{i}: кадр {f['name']} {f['w']}×{f['h']}, нужен {want[0]}×{want[1]}")
    if c["kind"] == "building" or (c["kind"] == "prop" and ent and ent["size"] and (real or cat == "buildings")):
        if ent and ent["size"] and (cat != "decor" or real):
            fw, fh = ent["size"]
            for f in s["frames"]:
                if f["w"] != (fw + fh) * 32:
                    E(f"{cat}/{i}: ширина {f['w']} != (fw+fh)*32 = {(fw + fh) * 32}")
    if real:
        d = s["dirs"]
        if d not in (1, 4, 8):
            E(f"{cat}/{i}: dirs {d}")
        elif d < c["dirs"]:
            W(f"{cat}/{i}: направлений {d} < {c['dirs']} (запасной процедурный рендер? STYLE §6 требует {c['dirs']})")
        an = s["anims"]
        if c["kind"] == "unit":
            total = sum(v["frames"] for v in an.values()) * d
            if total != len(s["frames"]):
                E(f"{cat}/{i}: кадров {len(s['frames'])} != направления × сумма кадров анимаций = {total}")
            for k in c.get("anims", {}):
                if k not in an:
                    W(f"{cat}/{i}: нет анимации {k} (план STYLE §6)")
            for k, v in an.items():
                for dd in range(d):
                    for n in range(v["frames"]):
                        nm = f"{i}_{k}_d{dd}_{n}"
                        if nm not in {f["name"] for f in s["frames"]}:
                            E(f"{cat}/{i}: нет кадра {nm}")
                            break
        elif len(s["frames"]) < 1:
            E(f"{cat}/{i}: нет кадров")
    elif c["kind"] == "unit" and len(s["frames"]) < 1:
        E(f"{cat}/{i}: заглушка без кадров")


def check_atlas(cat, name, a, part):
    mx = A.config()["atlas_max_px"]
    for k in ("image", "data", "size"):
        if k not in a:
            E(f"{cat}/{name}: в атласе нет {k}")
            return
    if max(a["size"]) > mx:
        E(f"{cat}/{name}: атлас {a['size'][0]}×{a['size'][1]} > {mx}")
    p = os.path.join(A.ART, a["image"])
    if not os.path.exists(p):
        E(f"{cat}/{name}: нет файла {a['image']}")
    elif png_size(p) != a["size"]:
        E(f"{cat}/{name}: {a['image']} — не PNG или размер != {a['size']}")
    if "webp" in a:
        wp = os.path.join(A.ART, a["webp"])
        if not os.path.exists(wp):
            E(f"{cat}/{name}: нет файла {a['webp']}")
        elif not is_webp(wp):
            E(f"{cat}/{name}: {a['webp']} — не WebP")
    jp = os.path.join(A.ART, a["data"])
    if not os.path.exists(jp):
        E(f"{cat}/{name}: нет файла {a['data']}")
        return
    tp = A.jread(jp)
    if tp.get("meta", {}).get("size") != {"w": a["size"][0], "h": a["size"][1]}:
        E(f"{cat}/{name}: размер в {a['data']} != манифесту")
    boxes = []
    for s in part["sprites"]:
        if s["atlas"] != name:
            continue
        for f in s["frames"]:
            t = tp["frames"].get(f["name"])
            if not t or t["frame"] != {"x": f["x"], "y": f["y"], "w": f["w"], "h": f["h"]}:
                E(f"{cat}/{name}: кадр {f['name']} не совпадает с {a['data']}")
            boxes.append((f["x"], f["y"], f["x"] + f["w"], f["y"] + f["h"], f["name"]))
    if len(tp["frames"]) != len(boxes):
        E(f"{cat}/{name}: в {a['data']} {len(tp['frames'])} кадров, в манифесте {len(boxes)}")
    boxes.sort()
    for n, b in enumerate(boxes):
        for o in boxes[n + 1:]:
            if o[0] >= b[2]:
                break
            if o[1] < b[3] and b[1] < o[3]:
                E(f"{cat}/{name}: кадры {b[4]} и {o[4]} пересекаются")


def check_budget(cfg, parts):
    bud = cfg["budget"]
    full = first = 0
    for cat, part in parts.items():
        for name, a in part["atlases"].items():
            p = os.path.join(A.ART, a.get("webp") or a["image"])
            if not os.path.exists(p):
                continue
            n = os.path.getsize(p)
            full += n
            if cfg["categories"][cat]["first_load"]:
                first += n
    mb = 1024 * 1024
    if first >= bud["first_load_mb"] * mb:
        E(f"первая загрузка {first / mb:.2f} МБ >= {bud['first_load_mb']} МБ")
    if full >= bud["full_mb"] * mb:
        E(f"полный набор {full / mb:.2f} МБ >= {bud['full_mb']} МБ")
    pf, pfl = gen_catalog.projection()
    if pf >= bud["full_mb"] * mb:
        W(f"план каталога: полный набор ≈ {pf / mb:.1f} МБ >= {bud['full_mb']} МБ — режьте кадры/направления")
    if pfl >= bud["first_load_mb"] * mb:
        W(f"план каталога: первая загрузка ≈ {pfl / mb:.1f} МБ >= {bud['first_load_mb']} МБ")
    print(f"бюджет: первая загрузка {first / 1024:.0f} КБ из {bud['first_load_mb']} МБ, полный {full / 1024:.0f} КБ из {bud['full_mb']} МБ; "
          f"план каталога ≈ {pf / mb:.1f} / {pfl / mb:.1f} МБ")


def check_templates(cats, pal):
    p = os.path.join(A.ART, "templates", "models.json")
    if not os.path.exists(p):
        E("нет art/templates/models.json")
        return
    ms = A.jread(p)["models"]
    got = {}
    for m in ms:
        c = m.get("category")
        if c not in cats:
            E(f"templates/{m.get('id')}: category {c!r}")
            continue
        got[c] = got.get(c, 0) + 1
        check_model(m, "templates", pal)
    for c in cats:
        if got.get(c) != 1:
            E(f"templates: для категории {c} нужен ровно один шаблон (есть {got.get(c, 0)})")


if __name__ == "__main__":
    sys.exit(main())
