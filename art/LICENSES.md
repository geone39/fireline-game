# art/LICENSES.md — лицензии ассетов

| Ассет | Источник | Лицензия |
|---|---|---|
| `art/atlases/*.png`, `*.webp`, `*.json`, `art/manifest.json`, `art/src/*/manifest.part.json` (заглушки ART-0 и сборка ART-2) | Заглушки: `art/tools/placeholder.py` (цветные блоки, без внешних материалов); сборка: `art/tools/build_art.py` | Лицензия проекта (см. репозиторий) |
| `art/src/**/models.json`, `art/src/palette.json`, `art/templates/**`, `art/render/*` (ART-2) | Собственные модели из примитивов (образцы — производные от моделей спайка ART-1); рендереры `art/render/blender_render.py` (Blender 5.2, bpy; лицензия GPL относится к Blender, не к рендерам) и `art/render/proc_render.mjs` (Node) | Лицензия проекта (см. репозиторий) |
| `art/spike/**` (модели `models.json`, палитра, атласы `*/out/*`, `preview.html`, скрипты) | ART-1: собственные модели из примитивов; рендер `art/spike/procedural/render.mjs` (Node, свой растеризатор) и `art/spike/blender/render_spike.py` (Blender 5.2, bpy; лицензия GPL относится к Blender, не к рендерам) | Лицензия проекта (см. репозиторий) |

Сторонних ассетов, шрифтов, текстур и референсов нет (в ART-1 шрифты не используются, текстур нет, ИИ-генерация не применялась). Заглушки нейтральны по стилю (OQ-2 не решён) и будут заменены после выбора A/P/U.
Каждый новый внешний ассет добавлять строкой: файл, автор, источник/URL, лицензия.
