# art/src — исходники по категориям

Одна папка = одна категория = (на время работы) один агент `art`. Агенты разных категорий правят **разные файлы** и не конфликтуют; общие файлы (`art/manifest.json`, `art/CATALOG.md`) генерируются из частей.

```
art/src/
  categories.json        категории, атласы, бюджеты (общий; правка — только вместе с владельцем пайплайна)
  palette.json           единая палитра P01+P03 (sRGB), свет; единственный источник цветов
  catalog_overrides.json (необязательно) {id: {priority, complexity}} — точечные правки CATALOG.md
  <категория>/
    models.json          модели (исходник правды): {"models": [...]}
    manifest.part.json   ГЕНЕРИРУЕТСЯ build_art.py: атласы и спрайты категории + lock (хэши входов); коммитится
    blend/, gltf/        (зарезервировано) .blend/glTF для сложных моделей — см. «Ограничения»
art/atlases/<атлас>.png|.webp|.json   ГЕНЕРИРУЕТСЯ; PNG — фолбэк, WebP q90 — основной
art/manifest.json       ГЕНЕРИРУЕТСЯ из всех manifest.part.json; руками не править
art/templates/models.json   образцы по одной модели на категорию
```

Категории: `terrain`, `decor`, `buildings`, `defences`, `units`, `vehicles`, `ships`, `heroes`, `spells_fx` (атлас `spells`), `ui`. Список, префиксы id и целевые анимации — `categories.json`; сущности и приоритеты — `art/CATALOG.md`.

## Правила наименования

| Что | Правило |
|---|---|
| id спрайта | равен id из `content/*.yaml` (`b_hq`, `u_rifleman`, `d_wall`, `c_airstrike`…). `terrain` и `ui` — не из content: свободные id с префиксом `t_` / `ui_` (`t_grass_a`, `ui_btn_start`), строчные, `[a-z0-9_]` |
| Шаблоны | `tpl_<категория>` — только в `art/templates/`, в манифест не попадают |
| Кадр здания/пропа | `{id}_0`; состояния — `{id}_damaged_0`, `{id}_ruin_0` |
| Кадр рига (юнит, техника, корабль, герой) | `{id}_{anim}_d{dir}_{n}`; `anim` ∈ `idle walk attack die` (+`ability` у героев, +`cast` у `spells_fx`), `dir` 0…7 (0 = лицом к камере, далее по часовой стрелке на экране), `n` с нуля |
| Тиры зданий (ярусы L1–3, L4–6…) | предложение OQ-ART2-3: `{id}_t{k}_0` (k от 2; тир 1 — без суффикса) |
| Атлас | имя из `categories.json`; при переполнении 2048 px — страницы `<атлас>_2`, `_3` (сущность целиком на одной странице) |
| Файлы | `art/src/<cat>/models.json`; `.blend` — `art/src/<cat>/blend/<id>.blend`; glTF — `…/gltf/<id>.glb` |
| Ветки/коммиты | `feat/art-<категория>-<кратко>`, `feat(art): …`; один PR = одна категория |

## Формат `models.json`

Общий для Blender- и процедурного рендерера (`art/render/*`), сохранён из ART-1. Единицы: 1 = ребро тайла по x, y и z. `x, y` — оси сетки (центр футпринта = `0, 0`), `z` вверх; низ детали в `z = 0` — земля.

```jsonc
{ "models": [ {
  "id": "b_barracks",            // id из content (или t_/ui_)
  "kind": "building",            // building (1 кадр) | unit (риг с позами, 8 направлений в Blender / 4 в процедурном)
  "status": "wip",               // wip — рендерится только --preview; done — идёт в атлас и манифест
  "renderer": "blender",         // необязательно: закрепить рендерер для сборки в репозиторий (по умолчанию blender)
  "footprint": [3, 3],           // building: клеток по x, y (обязан совпасть с content)
  "frame": [192, 136],           // px кадра; здание: ширина (fw+fh)*32, высота (fw+fh)*16 + запас H; unit: 48×48
  "anchor": [96, 87],            // якорь "ground": точка кадра на земле (центр футпринта); здание: [w/2, H+(fw+fh)*8-1], unit: [24, 43]
  "parts": [ { "n": "roof", "shape": "box|cyl|cone|sphere", "c": [x,y,z], "s": [sx,sy,sz],
               "col": "olive_dark",          // имя из palette.json
               "rot": [rx,ry,rz],            // необязательно, градусы
               "g": "armL" } ],              // unit: группа-кость
  // только unit:
  "groups": { "root": {"pivot": [0,0,0]}, "armL": {"pivot": [0,0.18,0.6], "parent": "torso"} },
  "rest":   { "armL": {"ry": -65} },                       // поза покоя
  "anims":  { "walk": { "fps": 10, "loop": true,
              "frames": [ { "legL": {"ry": -30, "dz": 0.0}, "_fall": 0 } ] } }   // поза = {группа: {rx,ry,rz,dx,dy,dz}}; _fall — падение для die, градусы
} ] }
```

Образцы: `art/templates/models.json` (здание, оборона, декор, тайл, иконка заклинания, иконка UI, юнит, герой, техника, корабль); живые примеры — `art/src/buildings/models.json` (`b_hq`, `b_cash_mine`), `art/src/units/models.json` (`u_rifleman`) из спайка ART-1.

Правила стиля при моделировании — `art/STYLE.md` (≤ 6/8 цветов, оранжевый ≤ 8 % площади, силуэт на 48 px, не выходить за кадр: рендерер печатает `WARN … вылез за кадр`).

## Ограничения

- Модели собираются из примитивов `box|cyl|cone|sphere`. Импорт `.blend`/glTF (сторонние CC0-модели, риги) **пока не реализован** (ASSUMPTION: появится вместе с первой сторонней моделью; процедурный рендерер их не читает — сознательно, см. `art/spike/COMPARISON.md`). Каждый внешний файл — строка в `art/LICENSES.md`.
- Игровые числа (hp, цены, время) в моделях не живут (AGENTS.md п.3): футпринт и состав берутся из `content/`.
