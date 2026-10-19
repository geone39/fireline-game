#!/usr/bin/env python3
"""Generate neutral placeholder sprites (flat colour blocks) + art/manifest.json from content/*.yaml.

Deterministic, stdlib + PyYAML only. Style-agnostic on purpose (OQ-2: style not chosen yet).
Run:  python3 art/tools/gen_placeholders.py        (rewrites art/atlases/*, art/manifest.json)
"""
import colorsys, json, os, struct, zlib
import yaml

ROOT = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
ART = os.path.join(ROOT, "art")
load = lambda n: yaml.safe_load(open(os.path.join(ROOT, "content", n), encoding="utf-8"))

# ---- size spec (ASSUMPTION: no size spec exists yet; see PR description) --------------------
TILE_W, TILE_H = 64, 32          # iso tile diamond, px
UNIT_FRAME = 48                  # units/heroes/spell icons: 48x48 frame
UNIT_FRAMES, UNIT_FPS = 2, 6     # 2-frame idle/walk bob
OUTLINE = (20, 20, 28, 255)


class Img:
    """Tiny RGBA canvas."""
    def __init__(s, w, h):
        s.w, s.h, s.px = w, h, [[(0, 0, 0, 0)] * w for _ in range(h)]

    def set(s, x, y, c):
        if 0 <= x < s.w and 0 <= y < s.h:
            s.px[y][x] = c

    def poly(s, pts, c):
        ys = [p[1] for p in pts]
        n = len(pts)
        for y in range(max(0, int(min(ys))), min(s.h, int(max(ys)) + 1)):
            yc, xs = y + 0.5, []
            for i in range(n):
                (x0, y0), (x1, y1) = pts[i], pts[(i + 1) % n]
                if (y0 <= yc < y1) or (y1 <= yc < y0):
                    xs.append(x0 + (yc - y0) * (x1 - x0) / (y1 - y0))
            xs.sort()
            for a, b in zip(xs[0::2], xs[1::2]):
                for x in range(int(a + 0.5), int(b + 0.5)):
                    s.set(x, y, c)

    def rect(s, x, y, w, h, c):
        s.poly([(x, y), (x + w, y), (x + w, y + h), (x, y + h)], c)

    def ellipse(s, cx, cy, rx, ry, c):
        for y in range(int(cy - ry) - 1, int(cy + ry) + 2):
            for x in range(int(cx - rx) - 1, int(cx + rx) + 2):
                if ((x + .5 - cx) / rx) ** 2 + ((y + .5 - cy) / ry) ** 2 <= 1:
                    s.set(x, y, c)

    def outline(s, c=OUTLINE):
        o = [r[:] for r in s.px]
        for y in range(s.h):
            for x in range(s.w):
                if s.px[y][x][3] == 0 and any(
                        0 <= x + dx < s.w and 0 <= y + dy < s.h and s.px[y + dy][x + dx][3]
                        for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1))):
                    o[y][x] = c
        s.px = o

    def blit(s, o, ox, oy):
        for y in range(o.h):
            for x in range(o.w):
                if o.px[y][x][3]:
                    s.set(ox + x, oy + y, o.px[y][x])

    def png(s):
        raw = b"".join(b"\x00" + b"".join(bytes(p) for p in r) for r in s.px)
        ch = lambda t, d: struct.pack(">I", len(d)) + t + d + struct.pack(">I", zlib.crc32(t + d) & 0xffffffff)
        return (b"\x89PNG\r\n\x1a\n" + ch(b"IHDR", struct.pack(">IIBBBBB", s.w, s.h, 8, 6, 0, 0, 0))
                + ch(b"IDAT", zlib.compress(raw, 9)) + ch(b"IEND", b""))


def hsv(h, s, v):
    r, g, b = colorsys.hsv_to_rgb(h % 1, s, v)
    return (int(r * 255), int(g * 255), int(b * 255), 255)


def shade(c, k):
    return (min(255, int(c[0] * k)), min(255, int(c[1] * k)), min(255, int(c[2] * k)), 255)


def hue_for(i):
    return (i * 0.381966) % 1   # golden angle: neighbouring ids stay distinct


# ---- painters ------------------------------------------------------------------------------
def building(w, h, col, height, low=False):
    """Iso box over a w x h footprint -> (Img, anchor). Anchor = footprint centre on the ground."""
    n = w + h
    W, H = n * TILE_W // 2, n * TILE_H // 2 + height + 2
    im = Img(W, H)
    cx, gy = W / 2, H - n * TILE_H / 4 - 1
    hw, hh = n * TILE_W / 4, n * TILE_H / 4
    im.poly([(cx, gy - hh), (cx + hw, gy), (cx, gy + hh), (cx - hw, gy)], shade(col, .45))  # ground
    k = .78
    bw, bh = hw * k, hh * k
    L, R, B, T = (cx - bw, gy), (cx + bw, gy), (cx, gy + bh), (cx, gy - bh)
    up = lambda p: (p[0], p[1] - height)
    im.poly([L, B, up(B), up(L)], shade(col, .70))
    im.poly([B, R, up(R), up(B)], shade(col, .55))
    im.poly([up(L), up(T), up(R), up(B)], shade(col, 1.15))
    if not low:   # roof marker so tall buildings are not plain blobs
        im.poly([(cx - 5, gy - height - 4), (cx, gy - height - 9), (cx + 5, gy - height - 4), (cx, gy - height + 1)],
                shade(col, 1.4))
    im.outline()
    return im, (int(cx), int(gy))


def unit(cls, col, scale=1.0, frame=0, hero=False):
    F = UNIT_FRAME
    im = Img(F, F)
    b = -1 if frame else 0
    cx, fy = F / 2, F - 5 + b
    dark, light = shade(col, .6), shade(col, 1.25)
    if cls == "INF":
        bw, bh = 7 * scale, 13 * scale
        im.ellipse(cx, fy - 1, bw + 2, 3, (0, 0, 0, 90))
        im.rect(cx - bw, fy - bh - 2, bw * 2, bh, col)
        im.rect(cx - bw, fy - 6, bw * 2, 4, dark)
        im.ellipse(cx, fy - bh - 6 * scale, 5 * scale, 5 * scale, light)
    elif cls == "VEH":
        w, h = 17 * scale, 7 * scale
        im.rect(cx - w, fy - h - 4, w * 2, h, col)
        im.rect(cx - w * .5, fy - h - 4 - 7 * scale, w, 7 * scale, light)
        im.rect(cx + w * .2, fy - h - 4 - 5 * scale, w * 1.1, 3, dark)
        for dx in (-w * .6, w * .6):
            im.ellipse(cx + dx, fy - 2, 4, 4, (45, 45, 50, 255))
    elif cls == "AIR":
        cy = fy - 20 + b * 2
        im.ellipse(cx, fy - 1, 9, 2, (0, 0, 0, 70))
        im.poly([(cx, cy - 11), (cx + 15, cy), (cx, cy + 6), (cx - 15, cy)], col)
        im.poly([(cx, cy - 11), (cx + 15, cy), (cx, cy)], light)
        im.rect(cx - 2, cy - 3, 4, 4, dark)
    elif cls == "SEA":
        w = 17 * scale
        im.poly([(cx - w, fy - 9), (cx + w * 1.1, fy - 9), (cx + w * .7, fy - 1), (cx - w * .8, fy - 1)], col)
        im.rect(cx - w * .4, fy - 17 * scale, w * .8, 8 * scale, light)
        im.rect(cx + w * .4, fy - 13 * scale, w * .6, 3, dark)
    else:   # spell icon: disc with ring
        im.ellipse(cx, F / 2, 18, 18, dark)
        im.ellipse(cx, F / 2, 14, 14, col)
        im.ellipse(cx, F / 2, 6, 6, light)
    if hero:
        gold = (255, 214, 64, 255)
        im.poly([(cx, 3), (cx + 3, 9), (cx + 9, 9), (cx + 4, 13), (cx + 6, 19), (cx, 15),
                 (cx - 6, 19), (cx - 4, 13), (cx - 9, 9), (cx - 3, 9)], gold)
    im.outline()
    return im


# ---- content -> entities -------------------------------------------------------------------
def entities():
    """[(id, atlas, kind, params)] for every entity that needs a sprite."""
    T, D, B, H = load("troops.yaml"), load("defences.yaml"), load("buildings.yaml"), load("heroes.yaml")
    out = []
    for sect in ("troops", "vehicles", "ships"):
        for u in T[sect]:
            out.append((u["id"], "units", "unit", {"cls": u["class"], "hp": u["hp"]}))
    for s in T["spells"]:
        out.append((s["id"], "spells", "spell", {}))
    for h in H["heroes"]:
        out.append((h["id"], "heroes", "hero", {}))
    for sect in ("passive_obstacles", "active"):
        for d in D[sect]:
            out.append((d["id"], "defences", "building",
                        {"size": d["size"], "passive": sect != "active", "hp": d["hp"]}))
    for sect in ("core", "resources", "military"):
        for b in B[sect]:
            out.append((b["id"], "buildings", "building", {"size": b["size"]}))
    for d in B["decor"]:
        out.append((d["id"], "decor", "building", {"size": [1, 1], "decor": True}))
    for o in B["obstacles"]:
        out.append((o["id"], "decor", "building", {"size": [1, 1], "obstacle": True}))
    return out


def render(eid, kind, p, idx):
    """-> (frames[Img], anchor(x, y), fps)"""
    col = hsv(hue_for(idx), .55, .85)
    foot = (UNIT_FRAME // 2, UNIT_FRAME - 5)
    if kind == "hero":
        return [unit("INF", col, 1.25, f, hero=True) for f in range(UNIT_FRAMES)], foot, UNIT_FPS
    if kind == "unit":
        scale = 1.0 + min(p["hp"], 1000) / 2500   # tougher = visibly bigger silhouette
        return [unit(p["cls"], col, scale, f) for f in range(UNIT_FRAMES)], foot, UNIT_FPS
    if kind == "spell":
        return [unit("SPELL", col)], (UNIT_FRAME // 2, UNIT_FRAME // 2), 0
    w, h = p["size"]
    if eid == "b_hq":
        col = hsv(.12, .75, .9)
    if p.get("passive"):
        im, a = building(w, h, col, 6 if p["hp"] < 100 else 12, low=True)
    elif p.get("decor") or p.get("obstacle"):
        im, a = building(w, h, hsv(.33 if p.get("decor") else .08, .35, .7), 10 if p.get("decor") else 8, low=True)
    else:
        im, a = building(w, h, col, 14 + 8 * max(w, h))
    return [im], a, 0


def pack(frames, maxw=1024):
    """Shelf packing. frames: [(key, Img)] -> (atlas Img, {key: (x, y, w, h)})."""
    x = y = rowh = 0
    place = {}
    for k, im in sorted(frames, key=lambda f: (-f[1].h, f[0])):
        if x + im.w + 1 > maxw:
            x, y, rowh = 0, y + rowh + 1, 0
        place[k] = (x, y, im.w, im.h)
        x += im.w + 1
        rowh = max(rowh, im.h)
    H = 1
    while H < y + rowh:
        H *= 2
    atlas = Img(maxw, H)
    for k, im in frames:
        atlas.blit(im, place[k][0], place[k][1])
    return atlas, place


def main():
    ents = entities()
    ids = [e[0] for e in ents]
    assert len(ids) == len(set(ids)), "duplicate ids in content"
    by_atlas = {}
    for i, (eid, atlas, kind, p) in enumerate(ents):
        frames, anchor, fps = render(eid, kind, p, i)
        by_atlas.setdefault(atlas, []).append((eid, frames, anchor, fps))
    os.makedirs(os.path.join(ART, "atlases"), exist_ok=True)
    manifest = {
        "schema": 1, "placeholder": True,
        "spec": {"tile_px": [TILE_W, TILE_H], "unit_frame_px": UNIT_FRAME,
                 "building_px": "w=(fw+fh)*32, h=(fw+fh)*16+height; anchor = footprint centre on ground"},
        "atlases": {}, "sprites": [],
    }
    for atlas, items in sorted(by_atlas.items()):
        flat = [(f"{eid}_{n}", im) for eid, frames, _, _ in items for n, im in enumerate(frames)]
        img, place = pack(flat)
        fn = f"atlases/{atlas}.png"
        with open(os.path.join(ART, fn), "wb") as f:
            f.write(img.png())
        tp = {"frames": {k: {"frame": dict(zip("xywh", v)), "rotated": False, "trimmed": False,
                             "spriteSourceSize": {"x": 0, "y": 0, "w": v[2], "h": v[3]},
                             "sourceSize": {"w": v[2], "h": v[3]}} for k, v in sorted(place.items())},
              "meta": {"image": f"{atlas}.png", "format": "RGBA8888",
                       "size": {"w": img.w, "h": img.h}, "scale": "1"}}
        with open(os.path.join(ART, f"atlases/{atlas}.json"), "w", newline="\n") as f:
            json.dump(tp, f, indent=1, sort_keys=True)
            f.write("\n")
        manifest["atlases"][atlas] = {"image": fn, "data": f"atlases/{atlas}.json", "size": [img.w, img.h]}
        for eid, frames, anchor, fps in items:
            manifest["sprites"].append({
                "id": eid, "atlas": atlas,
                "frames": [{"name": f"{eid}_{n}", **dict(zip("xywh", place[f"{eid}_{n}"]))}
                           for n in range(len(frames))],
                "anchors": {"ground": {"x": anchor[0], "y": anchor[1]}}, "fps": fps})
    manifest["sprites"].sort(key=lambda s: s["id"])
    with open(os.path.join(ART, "manifest.json"), "w", encoding="utf-8", newline="\n") as f:
        json.dump(manifest, f, indent=1, sort_keys=True)
        f.write("\n")
    print(f"{len(manifest['sprites'])} sprites, {len(by_atlas)} atlases")


if __name__ == "__main__":
    main()
