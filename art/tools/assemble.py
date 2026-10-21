#!/usr/bin/env python3
"""Пересобрать art/manifest.json и art/CATALOG.md из частей art/src/<cat>/manifest.part.json БЕЗ рендера (только stdlib + PyYAML).

Нужен при слиянии веток разных категорий: оба файла генерируемые, при конфликте возьмите любую сторону и запустите
  python art/tools/assemble.py
"""
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import artlib as A  # noqa: E402
import gen_catalog  # noqa: E402

A.jwrite(os.path.join(A.ART, "manifest.json"), A.assemble())
print("записан art/manifest.json")
gen_catalog.write()
