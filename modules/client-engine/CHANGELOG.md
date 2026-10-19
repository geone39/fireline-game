# CHANGELOG `client-engine`

## Unreleased
- модуль заведён
- OWNS: добавлены корневые файлы сборки `client/` (package.json, vite.config, tsconfig и т.д.)
- CLIENT-1: каркас Vite + Preact + PixiJS v8, интерфейс Renderer (v0.1), изометрическая сетка 44x44 и hello tile, vitest/eslint/tsc
- CLIENT-2b: рендер базы по BaseState. Изометрическая сетка из `content/base_grid.yaml` (размер, вода карты plain/coastal/canyon), здания спрайтами-заглушками из `art/manifest.json` по якорю `ground`, сортировка по глубине, камера (pan/zoom/pinch, ограничение краями и зумом), culling спрайтов, подсветка клетки под курсором и выбранного здания, события `select/hover/dragStart/dragMove/dragEnd`. Чистая логика (iso, depth, camera, picking, gestures, interaction) покрыта vitest без Pixi. Контракт Renderer 0.1 -> 0.2 (аддитивно, `contract-change`). Новая зависимость: `yaml`.
- CLIENT-4a: рендер поля боя (Renderer 0.2 -> 0.3, аддитивно, `contract-change`): `BattleRenderer` (`setBattle`, `playEvents`, `skipEvents`, `setSkipAnimations`), `BattleWorldState`, ввод `battleSelect/battleTarget/battleCell/battleEventsDone`, очередь событий ядра с анимациями (move, attack, hit, destroy, spawn, smoke, trap, spell, heal) и режимом «пропустить», подсветка ходов/дальности (Чебышёв)/целей, полоски hp и маркеры AP. Чистая логика (battle-logic, battle-playback, battle-input) покрыта vitest без Pixi; mock BattleRenderer; dev-страница `engine/battle-dev.html`.
