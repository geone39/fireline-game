#!/usr/bin/env python3
"""Проверка манифестов спайка по контракту art 0.1 (ключи спрайта, кадры в границах атласа, размеры кадров, PNG-сигнатура)."""
import sys as _s; _s.stdout.reconfigure(encoding="utf-8")
import json, os, struct, sys
HERE = os.path.dirname(os.path.abspath(__file__)); ART = os.path.dirname(HERE)
err = []
for path in ("procedural", "blender"):
    f = os.path.join(HERE, path, "out", "manifest.json")
    if not os.path.exists(f):
        err.append(f"{path}: нет out/manifest.json (сначала запустить рендер, см. README)"); continue
    m = json.load(open(f, encoding="utf-8"))
    for s in m["sprites"]:
        if set(s) != {"id", "atlas", "frames", "anchors", "fps"}: err.append(f"{path}/{s['id']}: bad keys")
        a = m["atlases"][s["atlas"]]
        for fr in s["frames"]:
            if fr["x"] < 0 or fr["y"] < 0 or fr["x"] + fr["w"] > a["size"][0] or fr["y"] + fr["h"] > a["size"][1]: err.append(f"{path}/{fr['name']}: вне атласа")
            if s["id"].startswith("u_") and (fr["w"], fr["h"]) != (48, 48): err.append(f"{path}/{fr['name']}: кадр не 48")
            if s["id"].startswith("b_") and fr["w"] != {"b_hq": 256, "b_cash_mine": 192}[s["id"]]: err.append(f"{path}/{fr['name']}: ширина не по футпринту")
    for k, a in m["atlases"].items():
        d = open(os.path.join(ART, a["image"]), "rb").read(32)
        if d[:8] != b"\x89PNG\r\n\x1a\n" or list(struct.unpack(">II", d[16:24])) != a["size"]: err.append(f"{k}: bad png/size")
    print(path, "спрайтов:", len(m["sprites"]), "кадров:", sum(len(s["frames"]) for s in m["sprites"]))
for e in err: print(" -", e)
sys.exit(1 if err else 0)
