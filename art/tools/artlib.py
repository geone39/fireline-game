"""Общая библиотека пайплайна art: пути, категории, реестр сущностей из content/*.yaml, хэши, сборка art/manifest.json из частей.

Только stdlib + PyYAML (её используют и проверки CI). Pillow нужен лишь build_art.py.
"""
import hashlib
import json
import math
import os

import yaml

ROOT = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
ART = os.path.join(ROOT, "art")
SRC = os.path.join(ART, "src")
CACHE = os.path.join(ART, ".cache")
TOOLS = os.path.join(ART, "tools")
RENDER = os.path.join(ART, "render")

STATUSES = ("wip", "done")           # статус модели в models.json; нет модели = placeholder
RENDERERS = ("blender", "procedural")


def jdump(obj):
    """Канонический JSON репозитория: отступ 1, ключи по алфавиту, LF, перевод строки в конце."""
    return json.dumps(obj, indent=1, sort_keys=True, ensure_ascii=False) + "\n"


def jwrite(path, obj):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write(jdump(obj))


def jread(path):
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def sha(*parts):
    h = hashlib.sha256()
    for p in parts:
        h.update(p if isinstance(p, bytes) else json.dumps(p, sort_keys=True, ensure_ascii=False).encode("utf-8"))
        h.update(b"\x00")
    return h.hexdigest()[:16]


def file_sha(path):
    with open(path, "rb") as f:
        return hashlib.sha256(f.read().replace(b"\r\n", b"\n")).hexdigest()[:16]


# ---- категории --------------------------------------------------------------------------------
def config():
    return jread(os.path.join(SRC, "categories.json"))


def categories():
    return config()["categories"]


def cat_dir(cat):
    return os.path.join(SRC, cat)


def part_path(cat):
    return os.path.join(cat_dir(cat), "manifest.part.json")


def palette():
    return jread(os.path.join(SRC, "palette.json"))


def models(cat):
    """{id: model} из art/src/<cat>/models.json (файла может не быть)."""
    p = os.path.join(cat_dir(cat), "models.json")
    if not os.path.exists(p):
        return {}
    return {m["id"]: m for m in jread(p).get("models", [])}


def model_hash(model, renderer, pal=None):
    """Вход рендера: модель + палитра + код рендерера + суперсэмплинг. Меняется любое — кадры пересобираются."""
    script = "blender_render.py" if renderer == "blender" else "proc_render.mjs"
    m = {k: v for k, v in model.items() if k not in ("status", "note")}
    return sha(m, pal if pal is not None else palette(), file_sha(os.path.join(RENDER, script)), renderer, config().get("ss", 4))


# ---- реестр сущностей из content --------------------------------------------------------------
def _load(name):
    with open(os.path.join(ROOT, "content", name), encoding="utf-8") as f:
        return yaml.safe_load(f)


def entities():
    """[{id, category, size, cls, hp, hq, levels, params}] в порядке gen_placeholders (ART-0): от порядка зависит цвет заглушки."""
    T, D, B, H = _load("troops.yaml"), _load("defences.yaml"), _load("buildings.yaml"), _load("heroes.yaml")
    out = []

    def add(x, cat, **kw):
        hq = x.get("unlock_hq")
        if hq is None:
            hq = (x.get("unlock") or {}).get("hq") if isinstance(x.get("unlock"), dict) else None
        out.append({"id": x["id"], "category": cat, "size": x.get("size"), "cls": x.get("class"), "hp": x.get("hp"),
                    "hq": hq, "levels": x.get("levels"), **kw})

    for sect, cat in (("troops", "units"), ("vehicles", "vehicles"), ("ships", "ships")):
        for u in T[sect]:
            add(u, cat)
    for s in T["spells"]:
        add(s, "spells_fx")
    for h in H["heroes"]:
        add(h, "heroes", hq=None)
    for sect in ("passive_obstacles", "active"):
        for d in D[sect]:
            add(d, "defences", passive=sect != "active")
    for sect in ("core", "resources", "military"):
        for b in B[sect]:
            add(b, "buildings")
    for d in B["decor"]:
        add(d, "decor", size=[1, 1], decor=True)
    for o in B["obstacles"]:
        add(o, "decor", size=[1, 1], obstacle=True)
    ids = [e["id"] for e in out]
    assert len(ids) == len(set(ids)), "duplicate ids in content"
    for i, e in enumerate(out):
        e["idx"] = i
    return out


def category_entities(cat, with_models=True):
    """Сущности категории: из content (категории с content) либо только из models.json (terrain, ui).
    -> [{...entity, model: dict|None}] по возрастанию id."""
    cfg = categories()[cat]
    ms = models(cat)
    if cfg.get("content"):
        es = [e for e in entities() if e["category"] == cat]
        es = [dict(e) for e in es]
    else:
        es = [{"id": i, "category": cat, "size": m.get("footprint"), "cls": None, "hp": None, "hq": None, "levels": None, "idx": 0}
              for i, m in ms.items()]
    for e in es:
        e["model"] = ms.get(e["id"]) if with_models else None
    return sorted(es, key=lambda e: e["id"])


# ---- сборка корневого манифеста ---------------------------------------------------------------
def empty_part(cat):
    return {"category": cat, "atlases": {}, "sprites": [], "lock": {}}


def read_part(cat):
    p = part_path(cat)
    return jread(p) if os.path.exists(p) else empty_part(cat)


def assemble(parts=None):
    """Детерминированно собирает art/manifest.json из частей. parts: {cat: part} (по умолчанию читает с диска)."""
    cfg = config()
    parts = parts or {c: read_part(c) for c in cfg["categories"]}
    atlases, sprites = {}, []
    for c in sorted(parts):
        for k, a in parts[c]["atlases"].items():
            assert k not in atlases, f"атлас {k} объявлен в двух частях"
            atlases[k] = a
        sprites += parts[c]["sprites"]
    ids = [s["id"] for s in sprites]
    assert len(ids) == len(set(ids)), "id спрайта в двух частях: " + ", ".join(sorted({i for i in ids if ids.count(i) > 1}))
    sprites.sort(key=lambda s: s["id"])
    placeholder = any(l.get("renderer") == "placeholder" for p in parts.values() for l in p.get("lock", {}).values())
    return {"schema": 1, "placeholder": placeholder, "spec": cfg["spec"], "atlases": atlases, "sprites": sprites}


# ---- каталог: приоритет, сложность, планируемые кадры ------------------------------------------
def priority(e, overrides):
    if e["id"] in overrides and "priority" in overrides[e["id"]]:
        return overrides[e["id"]]["priority"]
    cat, hq = e["category"], e["hq"]
    if e["id"] == "b_hq":
        return "P0"
    if cat == "heroes":
        return "P1" if e["id"] == "h_commander" else "P2"
    if cat == "decor":
        return "P1" if e.get("obstacle") else "P2"
    if cat in ("terrain", "ui"):
        return "P0"
    if hq is None:
        return "P1"
    return "P0" if hq <= 3 else "P1" if hq <= 6 else "P2"


def complexity(e, overrides):
    if e["id"] in overrides and "complexity" in overrides[e["id"]]:
        return overrides[e["id"]]["complexity"]
    cat = e["category"]
    if cat in ("heroes", "ships"):
        return "L"
    if cat == "vehicles":
        return "M"
    if cat in ("buildings", "defences", "decor") and e["size"]:
        a = e["size"][0] * e["size"][1]
        return "S" if a <= 2 else "M" if a <= 9 else "L"
    if cat == "units":
        return "M" if e["cls"] in ("AIR",) or e["id"] in ("u_heavy", "u_paratrooper") else "S"
    return "S"


def plan(e):
    """Планируемый состав спрайта по STYLE.md: {dirs, anims{имя:кадров}, states[], tiers, frames}."""
    c = categories()[e["category"]]
    anims = dict(c.get("anims", {}))
    states = list(c.get("states", []))
    if e["category"] == "defences" and not e.get("passive"):
        anims["attack"] = 3
    if e["category"] == "buildings" and e["id"] in ("b_cash_mine", "b_fuel_refinery", "b_alloy_foundry", "b_radar"):
        anims["work"] = 4
    tiers = 1
    if e["category"] == "buildings" and e.get("levels"):
        tiers = min(4, math.ceil(e["levels"] / 3))
    dirs = c.get("dirs", 1)
    frames = dirs * sum(anims.values()) + len(states) * tiers
    if not anims and not states:
        frames = tiers
    return {"dirs": dirs, "anims": anims, "states": states, "tiers": tiers, "frames": frames}
