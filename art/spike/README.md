# art/spike — ART-1: два пути рендера стиля A01-lite

Гайд по стилю: [`../STYLE.md`](../STYLE.md). Сравнение и рекомендация: [`COMPARISON.md`](COMPARISON.md). Просмотр: откройте **`preview.html`** в браузере (самодостаточный файл, без сервера и зависимостей).

Тестовые ассеты: `b_hq` (штаб 4×4), `b_cash_mine` (шахта 3×3), `u_rifleman` (idle / walk / attack / die). Общие входы: `models.json` (детали, группы, позы), `palette.json` (P01+P03, свет). Готовые результаты лежат в репозитории (`*/out/`), перегенерация — командами ниже (PowerShell, из корня репозитория).

## (б) Процедурный 2D — Node ≥ 18, без пакетов, 4 направления

```powershell
node art/spike/procedural/render.mjs            # -> art/spike/procedural/out/ (≈ 1–2 с)
```
Параметры: `--out DIR`, `--ss N` (суперсэмплинг, по умолчанию 4).

## (а) Blender — 8 направлений, toon-шейдер + inverted-hull обводка

Blender ставит владелец: `winget install BlenderFoundation.Blender` (проверено на 5.2.2 LTS; скрипт поддерживает и `BLENDER_EEVEE_NEXT` из 4.2–4.5).

```powershell
& "C:\Program Files\Blender Foundation\Blender 5.2\blender.exe" -b -P art\spike\blender\render_spike.py -- --out art\spike\blender\out
```
Параметры после `--`: `--out DIR`, `--ss N`, `--only b_hq,u_rifleman`, `--keep-raw`. Нужны GPU/драйвер, способные поднять Eevee в фоне (RTX 3060 — ок, ≈ 12 с на всё). Путь к `blender.exe` другой — подставьте свой.

## Проверка, замеры, превью

```powershell
python art/spike/check_spike.py                 # манифесты по контракту art 0.1, размеры кадров, PNG
python art/spike/measure.py                     # размеры атласов: PNG и WebP (нужен Pillow, необязателен)
node art/spike/build_preview.mjs                # пересобрать preview.html из out/ обоих путей
```

## Формат результата

В `*/out/`: атласы `spike_{proc|blender}_{units|buildings}.png` + TexturePacker-JSON, `manifest.json` (ключи `{id, atlas, frames[], anchors, fps}` как в контракте `art` 0.1; атласы названы с префиксом `spike_*`, чтобы не пересекаться с реальными), `timings.json`.
Контракт 0.1 не знает про направления и анимации, поэтому они закодированы в имени кадра: `u_rifleman_{anim}_d{dir}_{n}` (dir 0 = лицом к камере, далее по часовой стрелке на экране), а метаданные — в служебном ключе `_anims` манифеста спайка (не входит в контракт). Предложение расширить контракт — OQ-ART1-1 в `docs/open-questions.md`.

## Показать в клиенте

С ART-2 `merge_spike.py` и `gen_placeholders.py` заменены пайплайном (`art/README.md`). Спайк заморожен как запись ART-1; его три ассета лежат в `art/src/{buildings,units}/models.json` со статусом `wip`. Подменить заглушки в `art/manifest.json`: поставить `"status": "done"` и `python art/tools/build_art.py --id b_hq,b_cash_mine,u_rifleman`; откат — вернуть `wip` и пересобрать.

(Ниже — исторические замечания ART-1.) Атласы подтягиваются по путям `spike/<path>/out/*.png` относительно `art/`. Клиент пока не умеет выбирать анимацию/направление (кадры идут подряд) — отдельная dev-страница принадлежит client-engine, запрос — OQ-ART1-4. `art/manifest.json` в этом PR **не изменён**.
