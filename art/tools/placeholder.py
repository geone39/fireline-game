#!/usr/bin/env python3
"""Заглушки (ART-0): нейтральные цветные блоки для сущностей, у которых ещё нет модели. Детерминированно, только stdlib.

Библиотека для art/tools/build_art.py (раньше gen_placeholders.py: реестр сущностей переехал в artlib.py, упаковка — в build_art.py).
Любое изменение этого файла пересобирает заглушки всех категорий (он входит в хэш сборки).
"""
import colorsys, struct, zlib

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


def render(e):
    """Сущность из artlib.entities() -> (frames[Img], anchor(x, y), fps)."""
    eid, kind = e["id"], e["category"]
    col = hsv(hue_for(e["idx"]), .55, .85)
    foot = (UNIT_FRAME // 2, UNIT_FRAME - 5)
    if kind == "heroes":
        return [unit("INF", col, 1.25, f, hero=True) for f in range(UNIT_FRAMES)], foot, UNIT_FPS
    if kind in ("units", "vehicles", "ships"):
        scale = 1.0 + min(e["hp"], 1000) / 2500   # tougher = visibly bigger silhouette
        return [unit(e["cls"], col, scale, f) for f in range(UNIT_FRAMES)], foot, UNIT_FPS
    if kind == "spells_fx":
        return [unit("SPELL", col)], (UNIT_FRAME // 2, UNIT_FRAME // 2), 0
    w, h = e["size"]
    if e.get("decor") or e.get("obstacle"):
        w, h = 1, 1   # ART-0: заглушки декора/препятствий всегда 1x1 (результат сборки не менялся); у настоящих моделей футпринт из content
    if eid == "b_hq":
        col = hsv(.12, .75, .9)
    if e.get("passive"):
        im, a = building(w, h, col, 6 if e["hp"] < 100 else 12, low=True)
    elif e.get("decor") or e.get("obstacle"):
        im, a = building(w, h, hsv(.33 if e.get("decor") else .08, .35, .7), 10 if e.get("decor") else 8, low=True)
    else:
        im, a = building(w, h, col, 14 + 8 * max(w, h))
    return [im], a, 0
