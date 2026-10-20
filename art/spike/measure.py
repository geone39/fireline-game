#!/usr/bin/env python3
"""Размеры атласов спайка: PNG как есть и webp (нужен Pillow; без него webp пропускается). Печатает markdown-таблицу."""
import sys as _s; _s.stdout.reconfigure(encoding="utf-8")
import io, json, os, sys
HERE = os.path.dirname(os.path.abspath(__file__))
try:
    from PIL import Image
except ImportError:
    Image = None
print("| путь | атлас | размер, px | кадров | PNG, КБ | webp q90, КБ | webp lossless, КБ |\n|---|---|---|---|---|---|---|")
for path in ("procedural", "blender"):
    out = os.path.join(HERE, path, "out")
    m = json.load(open(os.path.join(out, "manifest.json"), encoding="utf-8"))
    for key, a in m["atlases"].items():
        p = os.path.join(out, key + ".png")
        n = sum(len(s["frames"]) for s in m["sprites"] if s["atlas"] == key)
        w = ll = "—"
        if Image:
            im = Image.open(p).convert("RGBA")
            b = io.BytesIO(); im.save(b, "WEBP", quality=90, method=6); w = f"{b.tell()/1024:.1f}"
            b = io.BytesIO(); im.save(b, "WEBP", lossless=True, method=6); ll = f"{b.tell()/1024:.1f}"
        print(f"| {path} | {key} | {a['size'][0]}×{a['size'][1]} | {n} | {os.path.getsize(p)/1024:.1f} | {w} | {ll} |")
