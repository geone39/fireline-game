#!/usr/bin/env python3
"""Единый сборщик art (ART-2): модели/заглушки -> кадры -> атласы по категориям -> art/src/<cat>/manifest.part.json -> art/manifest.json.

  python art/tools/build_art.py                           # все категории (заглушки + модели со status "done"), кэш
  python art/tools/build_art.py --category units          # одна категория (можно несколько: --category units,heroes)
  python art/tools/build_art.py --id u_rifleman           # один id (остальное в категории берётся как есть)
  python art/tools/build_art.py --preview --renderer blender --id b_hq,u_rifleman
                                                          # рендер моделей любого статуса в art/.cache/preview/<renderer>/, репозиторий не трогает
  python art/tools/build_art.py --templates [--category ui]  # собрать шаблоны art/templates/ в art/.cache/templates/<renderer>/
  python art/tools/build_art.py --list                    # состояние: сущности, статусы, рендереры

Рендереры: blender (путь: --blender, BLENDER_EXE, PATH, стандартные места Windows/macOS/Linux) и procedural (Node >= 18, запасной).
Нужны PyYAML и Pillow (pip install pyyaml pillow). Подробности — art/README.md.
"""
import argparse
import glob
import hashlib
import json
import os
import shutil
import subprocess
import sys
import time

sys.stdout.reconfigure(encoding="utf-8", errors="replace")
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import artlib as A  # noqa: E402

try:
    from PIL import Image
except ImportError:
    sys.exit("build_art.py: нужен Pillow (pip install pillow)")

PACK_VERSION = 1   # менять при изменении упаковки/формата частей: сбрасывает «ничего не изменилось»
TEMPLATES = os.path.join(A.ART, "templates")


# ---- поиск Blender ----------------------------------------------------------------------------
def find_blender(explicit=None):
    cands = [explicit, os.environ.get("BLENDER_EXE")]
    for c in cands:
        if c:
            if os.path.isfile(c):
                return c
            sys.exit(f"Blender не найден по пути: {c}")
    w = shutil.which("blender")
    if w:
        return w
    pats = []
    for base in (os.environ.get("ProgramFiles"), os.environ.get("ProgramFiles(x86)"), os.environ.get("ProgramW6432"),
                 os.path.join(os.environ.get("LOCALAPPDATA", ""), "Programs")):
        if base:
            pats.append(os.path.join(base, "Blender Foundation", "Blender *", "blender.exe"))
    pats += ["/Applications/Blender.app/Contents/MacOS/Blender", "/usr/bin/blender", "/snap/bin/blender", "/usr/local/bin/blender"]
    found = sorted((p for pat in pats for p in glob.glob(pat)), key=lambda p: [int(x) if x.isdigit() else 0 for x in p.replace("\\", "/").split("/")[-2].split(".")], reverse=True)
    return found[0] if found else None


# ---- кадры ------------------------------------------------------------------------------------
def pil_from_placeholder(im):
    return Image.frombytes("RGBA", (im.w, im.h), b"".join(bytes(p) for row in im.px for p in row))


class Frames:
    """Результат для одной сущности: упорядоченные кадры + метаданные спрайта."""
    def __init__(self, frames, anchor, fps, renderer, hash_=None, dirs=None, anims=None):
        self.frames, self.anchor, self.fps = frames, list(anchor), fps
        self.renderer, self.hash, self.dirs, self.anims = renderer, hash_, dirs, anims


def placeholder_frames(e):
    import placeholder as P
    ims, anchor, fps = P.render(e)
    return Frames([(f"{e['id']}_{n}", pil_from_placeholder(im)) for n, im in enumerate(ims)], anchor, fps, "placeholder")


def load_cached(renderer, eid, h):
    d = os.path.join(A.CACHE, "frames", renderer, eid, h)
    mp = os.path.join(d, "meta.json")
    if not os.path.exists(mp):
        return None
    meta = A.jread(mp)
    fr = []
    for n in meta["frames"]:
        with Image.open(os.path.join(d, "frames", n + ".png")) as im:
            fr.append((n, im.convert("RGBA")))
    return real_frames(meta, fr, renderer, h)


def real_frames(meta, fr, renderer, h):
    anims = {k: {"fps": a["fps"], "frames": a["frames"], "loop": a["loop"]} for k, a in meta["anims"].items()}
    fps = anims["walk"]["fps"] if "walk" in anims else 0
    return Frames(fr, meta["anchor"], fps, renderer, h, meta["dirs"], anims)


def run_renderer(renderer, models_file, ids, work, blender):
    os.makedirs(work, exist_ok=True)
    common = ["--models", models_file, "--palette", os.path.join(A.SRC, "palette.json"), "--out", work,
              "--ids", ",".join(ids), "--ss", str(A.config().get("ss", 4))]
    if renderer == "blender":
        exe = find_blender(blender)
        if not exe:
            sys.exit("Blender не найден: задайте BLENDER_EXE или --blender (art/README.md §1), либо --renderer procedural")
        cmd = [exe, "-b", "--python-exit-code", "1", "-P", os.path.join(A.RENDER, "blender_render.py"), "--"] + common
    else:
        node = shutil.which("node")
        if not node:
            sys.exit("Node.js не найден (нужен для --renderer procedural)")
        cmd = [node, os.path.join(A.RENDER, "proc_render.mjs")] + common
    print(f"  [{renderer}] {', '.join(ids)}", flush=True)
    t = time.time()
    r = subprocess.run(cmd, capture_output=True, text=True, encoding="utf-8", errors="replace", cwd=A.ROOT)
    out = (r.stdout or "") + (r.stderr or "")
    for line in out.splitlines():
        if line.startswith(("WARN", "Traceback", "Error", "  File")) or ": " in line and "frames" in line:
            print("    " + line.strip())
    if r.returncode != 0:
        print(out[-3000:])
        sys.exit(f"рендерер {renderer} завершился с кодом {r.returncode}")
    for i in ids:
        if not os.path.exists(os.path.join(work, i, "meta.json")):
            print(out[-3000:])
            sys.exit(f"рендерер {renderer} не создал {i}")
    print(f"  [{renderer}] готово за {time.time() - t:.1f} с", flush=True)


def store_cache(renderer, eid, h, work):
    base = os.path.join(A.CACHE, "frames", renderer, eid)
    shutil.rmtree(base, ignore_errors=True)
    os.makedirs(base, exist_ok=True)
    shutil.move(os.path.join(work, eid), os.path.join(base, h))


# ---- упаковка ---------------------------------------------------------------------------------
def shelf(frames, row_w, pad):
    """frames: [(name, Image)] -> ({name: (x,y,w,h)}, H). Порядок и правила как в ART-0 (по высоте убыв., затем имя)."""
    x = y = rowh = 0
    place = {}
    for k, im in sorted(frames, key=lambda f: (-f[1].height, f[0])):
        w, h = im.size
        if w > row_w:
            sys.exit(f"кадр {k} шире ряда атласа ({w} > {row_w})")
        if x + w + pad > row_w:
            x, y, rowh = 0, y + rowh + pad, 0
        place[k] = (x, y, w, h)
        x += w + pad
        rowh = max(rowh, h)
    H = 1
    while H < y + rowh:
        H *= 2
    return place, H


def paginate(groups, cfg):
    """groups: [(eid, [(name, Image)])] по порядку -> [[(eid, frames)]] страницы атласа ≤ atlas_max_px по высоте."""
    row_w, pad, mx = cfg["atlas_row_px"], cfg["atlas_pad_px"], cfg["atlas_max_px"]
    pages, cur = [], []
    for eid, fr in groups:
        trial = [f for _, g in cur for f in g] + fr
        if cur and shelf(trial, row_w, pad)[1] > mx:
            pages.append(cur)
            cur = []
        cur.append((eid, fr))
        if shelf([f for _, g in cur for f in g], row_w, pad)[1] > mx:
            sys.exit(f"{eid}: кадры не помещаются в атлас {mx}px по высоте")
    if cur:
        pages.append(cur)
    return pages


def write_atlas(out_root, name, page, cfg, prev_px, force):
    """-> (атлас-описание для манифеста, {имя кадра: (x,y,w,h)}, px_hash)."""
    row_w, pad = cfg["atlas_row_px"], cfg["atlas_pad_px"]
    flat = [f for _, g in page for f in g]
    place, H = shelf(flat, row_w, pad)
    atlas = Image.new("RGBA", (row_w, H), (0, 0, 0, 0))
    for k, im in flat:
        atlas.paste(im, place[k][:2])
    px = hashlib.sha256(f"{row_w}x{H}".encode() + atlas.tobytes()).hexdigest()[:16]
    rel = f"atlases/{name}"
    png, webp, js = (os.path.join(out_root, rel + e) for e in (".png", ".webp", ".json"))
    os.makedirs(os.path.dirname(png), exist_ok=True)
    same = prev_px == px and not force
    if not (same and os.path.exists(png)):
        atlas.save(png, "PNG", optimize=False, compress_level=9)
    if not (same and os.path.exists(webp)):
        atlas.save(webp, "WEBP", quality=cfg["webp"]["quality"], method=cfg["webp"]["method"])
    tp = {"frames": {k: {"frame": dict(zip("xywh", v)), "rotated": False, "trimmed": False,
                         "spriteSourceSize": {"x": 0, "y": 0, "w": v[2], "h": v[3]},
                         "sourceSize": {"w": v[2], "h": v[3]}} for k, v in sorted(place.items())},
          "meta": {"image": f"{name}.png", "format": "RGBA8888", "size": {"w": row_w, "h": H}, "scale": "1"}}
    A.jwrite(js, tp)
    return {"image": rel + ".png", "webp": rel + ".webp", "data": rel + ".json", "size": [row_w, H]}, place, px


# ---- сборка категории -------------------------------------------------------------------------
class Ctx:
    def __init__(self, a):
        self.mode = "templates" if a.templates else "preview" if a.preview else "normal"
        self.renderer_arg = a.renderer
        self.blender = a.blender
        self.force = a.force
        self.ids = set(x for v in (a.id or []) for x in v.split(",") if x)
        self.cfg = A.config()
        self.pal = A.palette()
        rn = a.renderer or "blender"
        self.out_root = {"normal": A.ART, "preview": os.path.join(A.CACHE, "preview", rn),
                         "templates": os.path.join(A.CACHE, "templates", rn)}[self.mode]

    def part_path(self, cat):
        return A.part_path(cat) if self.mode == "normal" else os.path.join(self.out_root, "parts", cat + ".part.json")

    def models_file(self, cat):
        return os.path.join(TEMPLATES, "models.json") if self.mode == "templates" else os.path.join(A.cat_dir(cat), "models.json")


def entities_for(ctx, cat):
    """-> [(entity, source)] source: None (заглушка) | модель."""
    cfg = A.categories()[cat]
    if ctx.mode == "templates":
        ms = [m for m in A.jread(ctx.models_file(cat))["models"] if m.get("category") == cat]
        return [({"id": m["id"], "category": cat, "size": m.get("footprint"), "cls": None, "hp": None, "hq": None, "levels": None, "idx": 0}, m) for m in ms]
    ms = A.models(cat)
    if cfg["content"]:
        es = {e["id"]: e for e in A.category_entities(cat, with_models=False)}
        for i in ms:
            if i not in es:
                sys.exit(f"{cat}: модель {i} не соответствует id из content/ (ожидаются: {', '.join(sorted(es)[:5])}…)")
    else:
        es = {i: {"id": i, "category": cat, "size": m.get("footprint"), "cls": None, "hp": None, "hq": None, "levels": None, "idx": 0} for i, m in ms.items()}
    out = []
    for i in sorted(es):
        m = ms.get(i)
        if ctx.mode == "preview":
            if m and (not ctx.ids or i in ctx.ids):
                out.append((es[i], m))
        elif cfg["content"]:
            out.append((es[i], m if m and m.get("status") == "done" else None))
        elif m and m.get("status") == "done":
            out.append((es[i], m))
    return out


def crop_prev(ctx, old, sprite, cache):
    """Кадры из ранее собранного атласа (когда сущность не пересобирается)."""
    a = old["atlases"][sprite["atlas"]]
    if a["image"] not in cache:
        cache[a["image"]] = Image.open(os.path.join(ctx.out_root, a["image"])).convert("RGBA")
    im = cache[a["image"]]
    return [(f["name"], im.crop((f["x"], f["y"], f["x"] + f["w"], f["y"] + f["h"]))) for f in sprite["frames"]]


def build_category(ctx, cat):
    cfg = A.categories()[cat]
    old = A.jread(ctx.part_path(cat)) if ctx.mode == "normal" and os.path.exists(ctx.part_path(cat)) else A.empty_part(cat)
    old_sprites = {s["id"]: s for s in old["sprites"]}
    todo = entities_for(ctx, cat)
    results, lock, to_render = {}, {}, []
    if ctx.mode != "normal" and ctx.ids and not todo:
        return None
    for e, m in todo:
        sel = not ctx.ids or e["id"] in ctx.ids
        if m is None:
            results[e["id"]] = placeholder_frames(e)
            lock[e["id"]] = {"renderer": "placeholder"}
            continue
        r = ctx.renderer_arg or m.get("renderer") or "blender"
        h = A.model_hash(m, r, ctx.pal)
        prev = old["lock"].get(e["id"])
        if not sel and prev and prev.get("renderer") != "placeholder" and e["id"] in old_sprites:
            results[e["id"]] = ("carry", old_sprites[e["id"]])          # не выбран: оставляем как собрано
            lock[e["id"]] = prev
            continue
        cached = None if ctx.force else load_cached(r, e["id"], h)
        if cached:
            results[e["id"]] = cached
        else:
            to_render.append((e["id"], r, h))
        lock[e["id"]] = {"renderer": r, "hash": h}
    # рендер недостающего: один запуск рендерера на категорию
    for r in sorted({r for _, r, _ in to_render}):
        ids = [i for i, rr, _ in to_render if rr == r]
        work = os.path.join(A.CACHE, "work", r, cat)
        shutil.rmtree(work, ignore_errors=True)
        run_renderer(r, ctx.models_file(cat), ids, work, ctx.blender)
        for i, rr, h in to_render:
            if rr == r:
                store_cache(r, i, h, work)
                results[i] = load_cached(r, i, h)
    # упаковка
    prevcache, groups = {}, []
    for e, _ in todo:
        res = results[e["id"]]
        if isinstance(res, tuple):
            groups.append((e["id"], crop_prev(ctx, old, res[1], prevcache)))
        else:
            groups.append((e["id"], res.frames))
    part = A.empty_part(cat)
    part["lock"] = lock
    part["px_hash"] = {}
    pages = paginate(groups, ctx.cfg) if groups else []
    base = cfg["atlas"]
    for pi, page in enumerate(pages):
        name = base if pi == 0 else f"{base}_{pi + 1}"
        prev_px = old.get("px_hash", {}).get(name)
        adesc, place, px = write_atlas(ctx.out_root, name, page, ctx.cfg, prev_px, ctx.force)
        part["atlases"][name] = adesc
        part["px_hash"][name] = px
        for eid, fr in page:
            res = results[eid]
            frames = [{"name": n, **dict(zip("xywh", place[n]))} for n, _ in fr]
            if isinstance(res, tuple):
                s = dict(res[1])
                s["atlas"], s["frames"] = name, frames
            else:
                s = {"id": eid, "atlas": name, "frames": frames, "fps": res.fps,
                     "anchors": {"ground": {"x": res.anchor[0], "y": res.anchor[1]}}}
                if res.renderer != "placeholder":
                    s["dirs"] = res.dirs
                    s["anims"] = res.anims
            part["sprites"].append(s)
    # удалить страницы атласа, которых больше нет
    for name, a in old["atlases"].items():
        if name not in part["atlases"]:
            for ext in ("png", "webp", "json"):
                p = os.path.join(ctx.out_root, "atlases", f"{name}.{ext}")
                if os.path.exists(p):
                    os.remove(p)
    A.jwrite(ctx.part_path(cat), part)
    return part


def write_manifest(ctx, cats):
    if ctx.mode == "normal":
        m = A.assemble()
        A.jwrite(os.path.join(A.ART, "manifest.json"), m)
        return m
    parts = {c: A.jread(ctx.part_path(c)) for c in cats if os.path.exists(ctx.part_path(c))}
    m = A.assemble(parts)
    A.jwrite(os.path.join(ctx.out_root, "manifest.json"), m)
    return m


def size_report(root, m):
    tot = 0
    for k, a in sorted(m["atlases"].items()):
        p = os.path.join(root, a.get("webp") or a["image"])
        n = os.path.getsize(p)
        tot += n
        print(f"    {k:<14} {a['size'][0]}x{a['size'][1]:<5} {n / 1024:8.1f} КБ  ({os.path.basename(p)})")
    print(f"    итого {tot / 1024:.1f} КБ, спрайтов {len(m['sprites'])}")


def cmd_list():
    cats = A.categories()
    pal = A.palette()
    print(f"Blender: {find_blender() or 'не найден'}")
    for c in cats:
        ents = A.category_entities(c)
        st = {"placeholder": 0, "wip": 0, "done": 0}
        for e in ents:
            st[(e["model"] or {}).get("status", "wip") if e["model"] else "placeholder"] += 1
        print(f"{c:<10} id: {len(ents):3}  placeholder {st['placeholder']}, wip {st['wip']}, done {st['done']}")
    return 0


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--category", action="append", help="категория (можно через запятую / повторять)")
    ap.add_argument("--id", action="append", help="id сущности (через запятую / повторять)")
    ap.add_argument("--renderer", choices=A.RENDERERS, help="blender (по умолчанию) | procedural")
    ap.add_argument("--blender", help="путь к blender(.exe); иначе BLENDER_EXE, PATH, стандартные места")
    ap.add_argument("--preview", action="store_true", help="рендерить модели любого статуса в art/.cache/preview/<renderer>/")
    ap.add_argument("--templates", action="store_true", help="собрать шаблоны art/templates/ в art/.cache/templates/<renderer>/")
    ap.add_argument("--force", action="store_true", help="игнорировать кэш и перепаковать атласы")
    ap.add_argument("--no-catalog", action="store_true", help="не перегенерировать CATALOG.md")
    ap.add_argument("--list", action="store_true")
    a = ap.parse_args()
    if a.list:
        return cmd_list()
    ctx = Ctx(a)
    allc = list(A.categories())
    sel = [c for v in (a.category or []) for c in v.split(",") if c] or allc
    bad = [c for c in sel if c not in allc]
    if bad:
        sys.exit(f"неизвестная категория: {', '.join(bad)} (есть: {', '.join(allc)})")
    if ctx.ids and not a.category and ctx.mode == "normal":
        # определить категории по id
        cmap = {e["id"]: e["category"] for e in A.entities()}
        for c in allc:
            for i in A.models(c):
                cmap.setdefault(i, c)
        miss = ctx.ids - set(cmap)
        if miss:
            sys.exit(f"неизвестные id: {', '.join(sorted(miss))}")
        sel = sorted({cmap[i] for i in ctx.ids})
    if ctx.mode == "preview" and ctx.ids and not a.category:
        sel = [c for c in allc if ctx.ids & set(A.models(c))]
        miss = ctx.ids - {i for c in sel for i in A.models(c)}
        if miss:
            sys.exit(f"нет моделей для id: {', '.join(sorted(miss))} (art/src/<категория>/models.json)")
    if ctx.mode != "normal":
        shutil.rmtree(os.path.join(ctx.out_root, "parts"), ignore_errors=True)
    print(f"режим: {ctx.mode}; категории: {', '.join(sel)}" + (f"; id: {', '.join(sorted(ctx.ids))}" if ctx.ids else ""))
    built = []
    for c in sel:
        t = time.time()
        part = build_category(ctx, c)
        if part is None:
            continue
        built.append(c)
        ph = sum(1 for v in part["lock"].values() if v["renderer"] == "placeholder")
        print(f"{c}: {len(part['sprites'])} спрайтов ({ph} заглушек), атласов {len(part['atlases'])}, {time.time() - t:.1f} с")
    m = write_manifest(ctx, built if ctx.mode != "normal" else allc)
    root = ctx.out_root
    print("манифест:", os.path.relpath(os.path.join(root, "manifest.json"), A.ROOT))
    size_report(root, m)
    if ctx.mode == "normal" and not a.no_catalog:
        import gen_catalog
        gen_catalog.write()
    return 0


if __name__ == "__main__":
    sys.exit(main())
