#!/usr/bin/env python3
"""Подмена заглушек в art/manifest.json спрайтами спайка ART-1 (временно, для просмотра в клиенте).
  python art/tools/merge_spike.py procedural|blender [--write]
Без --write только печатает, что изменится. Откат: python art/tools/gen_placeholders.py."""
import sys as _s; _s.stdout.reconfigure(encoding="utf-8")
import json, os, sys
ART = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
path = sys.argv[1]
sp = json.load(open(os.path.join(ART, "spike", path, "out", "manifest.json"), encoding="utf-8"))
mp = os.path.join(ART, "manifest.json")
m = json.load(open(mp, encoding="utf-8"))
ids = {s["id"] for s in sp["sprites"]}
m["sprites"] = [s for s in m["sprites"] if s["id"] not in ids] + sp["sprites"]
m["atlases"].update(sp["atlases"])
print("заменяются:", ", ".join(sorted(ids)), "| новые атласы:", ", ".join(sp["atlases"]))
if "--write" in sys.argv:
    json.dump(m, open(mp, "w", encoding="utf-8", newline="\n"), indent=1, sort_keys=True)
    print("art/manifest.json обновлён")
