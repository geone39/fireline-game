# CHANGELOG `client-engine`

## Unreleased
- модуль заведён
- OWNS: добавлены корневые файлы сборки `client/` (package.json, vite.config, tsconfig и т.д.)
- CLIENT-1: каркас Vite + Preact + PixiJS v8, интерфейс Renderer (v0.1), изометрическая сетка 44x44 и hello tile, vitest/eslint/tsc
- CLIENT-2b: рендер базы по BaseState. Изометрическая сетка из `content/base_grid.yaml` (размер, вода карты plain/coastal/canyon), здания спрайтами-заглушками из `art/manifest.json` по якорю `ground`, сортировка по глубине, камера (pan/zoom/pinch, ограничение краями и зумом), culling спрайтов, подсветка клетки под курсором и выбранного здания, события `select/hover/dragStart/dragMove/dragEnd`. Чистая логика (iso, depth, camera, picking, gestures, interaction) покрыта vitest без Pixi. Контракт Renderer 0.1 -> 0.2 (аддитивно, `contract-change`). Новая зависимость: `yaml`.
