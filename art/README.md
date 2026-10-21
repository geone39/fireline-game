# art — пайплайн производства спрайтов (ART-2)

Стиль — [`STYLE.md`](STYLE.md), исходники и форматы — [`src/README.md`](src/README.md), список сущностей и приоритеты — [`CATALOG.md`](CATALOG.md) (генерируется), сравнение путей рендера — [`spike/COMPARISON.md`](spike/COMPARISON.md). Модуль: [`modules/art/`](../modules/art/AGENT.md).

Схема: `content/*.yaml` (id, футпринт) + `art/src/<cat>/models.json` → рендер (Blender; процедурный Node — запасной) → кадры → атласы по категориям (`art/atlases/<атлас>.png|.webp|.json`) → `art/src/<cat>/manifest.part.json` → **`art/manifest.json`** (контракт `art` — [`modules/art/CONTRACT.md`](../modules/art/CONTRACT.md)). У id без модели в атласе заглушка ART-0, поэтому манифест всегда полный (69 id).

## 0. Зависимости

- Python ≥ 3.10, `pip install pyyaml pillow` (Pillow — только для сборки; `check_art.py` нужен лишь PyYAML).
- Node ≥ 18 — процедурный рендерер (без пакетов).
- Blender 4.2+ / 5.x — основной рендерер (§1). GPU/драйвер, способные поднять Eevee в фоне (проверено: Blender 5.2.2, RTX 3060).

## 1. Поставить Blender

```powershell
winget install BlenderFoundation.Blender
```

Путь `build_art.py` ищет сам: `--blender ПУТЬ` → переменная `BLENDER_EXE` → `blender` в `PATH` → `C:\Program Files\Blender Foundation\Blender *\blender.exe` (берётся самая новая версия), `%LOCALAPPDATA%\Programs`, macOS/Linux-пути. Свой путь:

```powershell
$env:BLENDER_EXE = "D:\tools\Blender 5.2\blender.exe"
python art/tools/build_art.py --list      # печатает найденный Blender и состояние категорий
```

## 2. Собрать одну сущность

```powershell
# посмотреть результат модели любого статуса, репозиторий не меняется (art/.cache/preview/<рендерер>/):
python art/tools/build_art.py --preview --renderer blender --id b_hq
python art/tools/build_art.py --preview --renderer procedural --id b_hq,u_rifleman     # без Blender, секунды

# в репозиторий: поставить "status": "done" у модели в art/src/<cat>/models.json и
python art/tools/build_art.py --id b_hq                  # пересоберёт категорию (остальные id берутся как есть)
python art/tools/check_art.py                            # всё зелёное -> коммит
```

Что коммитить: `art/src/<cat>/models.json`, `manifest.part.json`, `art/atlases/<атлас>.*`, `art/manifest.json`, `art/CATALOG.md`, `modules/art/CHANGELOG.md`. Кэш `art/.cache/` в git не идёт.

Основные ключи `build_art.py`: `--category units[,heroes]`, `--id a,b`, `--renderer blender|procedural`, `--blender ПУТЬ`, `--preview`, `--templates`, `--force` (игнорировать кэш), `--list`. Кэш кадров — по хэшу (модель + палитра + код рендерера + рендерер); не изменилось — не рендерится; сущности, которые не выбраны `--id`, берутся из уже собранного атласа (Blender для них не нужен). Атласы пишутся только при реальном изменении пикселей.

## 3. Добавить категорию

1. Запись в `art/src/categories.json` (`atlas`, `content` — есть ли id в `content/`, иначе `id_prefix`; `kind`; `dirs`; `anims`/`states` — план для каталога; `first_load`; `frame_px`).
2. Если id берутся из content — привязать секцию content к категории в `artlib.entities()` (`art/tools/artlib.py`), порядок не менять (от него цвет заглушек); добавить правила приоритета/сложности в `artlib.py` при необходимости.
3. `mkdir art/src/<cat>`, `models.json` с `{"models": []}`, образец в `art/templates/models.json` (по одному на категорию — проверяет `check_art.py`), строка в `ORDER` у `gen_catalog.py`.
4. `python art/tools/build_art.py --category <cat>` → `check_art.py`. Это изменение пайплайна (отдельный PR), а не контента категории.

## 4. Шаблоны

`art/templates/models.json` — по одной модели на каждую категорию (простое здание, оборона, декор, тайл, иконка UI, иконка заклинания, юнит с позами, герой, техника, корабль). Сборка всех: `python art/tools/build_art.py --templates [--renderer procedural]` → `art/.cache/templates/<рендерер>/`. Новая сущность = скопировать образец в `art/src/<cat>/models.json` (без поля `category`), сменить `id`, `footprint`, `frame`, `anchor`, детали.

## 5. Бюджеты и проверки (`make art-check`)

```powershell
python art/tools/check_art.py
```

Первая загрузка < 3 МБ (атласы категорий с `first_load`: terrain, buildings, units, ui), полный набор < 40 МБ (размер считается по WebP, если он есть); id = content; якоря в кадре; число кадров = направления × сумма кадров анимаций; ширина здания = `(fw+fh)*32`; кадр юнита 48×48; атлас ≤ 2048 px; кадры не пересекаются и совпадают с TexturePacker-JSON; модель изменена после сборки → ошибка (хэш в `lock`); `manifest.json` и `CATALOG.md` равны генерации; шаблоны валидны. Предупреждения: направлений меньше плана (запасной процедурный рендер — 4 вместо 8), нет плановой анимации, прогноз каталога выше бюджета.

Запрос ops: цель `make art-check` (`python3 art/tools/check_art.py`) и шаг CI — OQ-ART2-1 в `docs/open-questions.md` (`Makefile` и `.github/` вне OWNS art).

## 6. Параллельная работа нескольких агентов

- Один агент — одна категория (`art/src/<cat>/…` + её атласы). Общие `art/manifest.json` и `art/CATALOG.md` генерируются; при конфликте слияния возьмите любую сторону и выполните `python art/tools/assemble.py` (без рендера, только stdlib).
- Не правьте `categories.json`, `palette.json`, `art/render/`, `art/tools/` внутри контентного PR: смена палитры или рендерера пересобирает всё (хэш), это отдельный PR.
- Без Blender на машине можно править модели и собирать `--preview --renderer procedural`; в репозиторий (`status: done`) Blender-результат кладёт тот, у кого он стоит.

## 7. Лицензии и юридическая чистота

См. `STYLE.md` §10 и [`LICENSES.md`](LICENSES.md): свои модели из примитивов; сторонние — только CC0/CC-BY/OFL с записью в `LICENSES.md`; ИИ-генерация в пайплайн не попадает.
