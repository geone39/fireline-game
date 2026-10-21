# art/CATALOG.md — каталог сущностей art

**Файл сгенерирован** `python art/tools/gen_catalog.py` из `content/*.yaml` и `art/src/`; руками не править (`check_art.py` проверяет актуальность). Точечные правки приоритета/сложности — `art/src/catalog_overrides.json`.

- **Приоритет.** P0 — нужен экранам M1–M3 (база, армия, бой): ASSUMPTION — открывается на HQ ≤ 3 (первые уровни прогрессии), штаб, препятствия карты, terrain/ui; P1 — HQ 4–6, командир, препятствия/декор карты; P2 — остальное (HQ ≥ 7, остальные герои, декор). Определения M1–M3 в репозитории нет — см. `docs/open-questions.md` OQ-ART2-2.
- **Сложность модели.** S — простая форма (пехота, малые здания/оборона ≤ 2 клеток, декор, иконки); M — техника, крупные юниты, здания 3×3; L — здания 4×4, корабли, герои.
- **Статус.** `placeholder` — нет модели, в атласе заглушка ART-0; `wip` — модель есть в `art/src/<cat>/models.json`, в игру не идёт (рендер: `build_art.py --preview`); `done` — собрана в атлас и манифест.
- **Направления/анимации/состояния** — цель по `art/STYLE.md` §6 (план), фактический состав — в `art/manifest.json`. Тиры зданий (ярусы L1–3, L4–6…) — оценка `ceil(levels/3)`, максимум 4; схема именования тиров в манифесте — предложение OQ-ART2-3.

## Сводка

| Категория | Сущностей | P0 | P1 | P2 | placeholder | wip | done | Планируемых кадров | Оценка WebP, МБ |
|---|---|---|---|---|---|---|---|---|---|
| `terrain` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0.0 |
| `decor` | 5 | 0 | 2 | 3 | 5 | 0 | 0 | 5 | 0.0 |
| `buildings` | 18 | 11 | 7 | 0 | 16 | 2 | 0 | 181 | 1.1 |
| `defences` | 16 | 6 | 8 | 2 | 16 | 0 | 0 | 78 | 0.2 |
| `units` | 12 | 5 | 5 | 2 | 11 | 1 | 0 | 1248 | 0.6 |
| `vehicles` | 5 | 1 | 1 | 3 | 5 | 0 | 0 | 520 | 0.2 |
| `ships` | 3 | 0 | 2 | 1 | 3 | 0 | 0 | 312 | 0.1 |
| `heroes` | 4 | 0 | 1 | 3 | 4 | 0 | 0 | 512 | 0.2 |
| `spells_fx` | 6 | 2 | 4 | 0 | 6 | 0 | 0 | 42 | 0.0 |
| `ui` | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0.0 |
| **всего** | 69 | 25 | 30 | 14 | 66 | 3 | 0 | 2898 | 2.6 |

Оценка размера — кадры × площадь × 0.2 Б/px (замер WebP q90 в спайке ART-1); бюджет: первая загрузка < 3 МБ, полный < 40 МБ (проверяет `check_art.py` по факту и по плану).

## `terrain` — тайлы земли, воды, берега, пропсы карты (не из content: id свободные с префиксом t_)

Атлас: `terrain`; первая загрузка: да; исходники: `art/src/terrain/`.

_Сущностей пока нет — id добавляются в `art/src/terrain/models.json` с префиксом `t_`._

## `decor` — декор и препятствия (x_*, o_*), футпринт из content

Атлас: `decor`; первая загрузка: нет (лениво); исходники: `art/src/decor/`.

| id | Футпринт | Напр. | Анимации (кадров) | Состояния × тиры | Кадров (план) | Приоритет | Сложность | Статус |
|---|---|---|---|---|---|---|---|---|
| `o_rock` | 1×1 | 1 | — | normal × 1 | 1 | P1 | S | placeholder |
| `o_wreck` | 1×1 | 1 | — | normal × 1 | 1 | P1 | S | placeholder |
| `x_flag` | 1×1 | 1 | — | normal × 1 | 1 | P2 | S | placeholder |
| `x_flowerbed` | 1×1 | 1 | — | normal × 1 | 1 | P2 | S | placeholder |
| `x_statue` | 1×1 | 1 | — | normal × 1 | 1 | P2 | S | placeholder |

## `buildings` — здания базы (b_*), ширина кадра (fw+fh)*32

Атлас: `buildings`; первая загрузка: да; исходники: `art/src/buildings/`.

| id | Футпринт | Напр. | Анимации (кадров) | Состояния × тиры | Кадров (план) | Приоритет | Сложность | Статус |
|---|---|---|---|---|---|---|---|---|
| `b_barracks` | 3×3 | 1 | — | normal/damaged/ruin × 4 | 12 | P0 | M | placeholder |
| `b_camp` | 4×4 | 1 | — | normal/damaged/ruin × 4 | 12 | P0 | L | placeholder |
| `b_cash_mine` | 3×3 | 1 | work 4 | normal/damaged/ruin × 4 | 16 | P0 | M | wip |
| `b_factory` | 4×4 | 1 | — | normal/damaged/ruin × 2 | 6 | P0 | L | placeholder |
| `b_fuel_refinery` | 3×3 | 1 | work 4 | normal/damaged/ruin × 4 | 16 | P0 | M | placeholder |
| `b_fuel_tank` | 3×3 | 1 | — | normal/damaged/ruin × 4 | 12 | P0 | M | placeholder |
| `b_garage` | 3×3 | 1 | — | normal/damaged/ruin × 2 | 6 | P0 | M | placeholder |
| `b_hq` | 4×4 | 1 | — | normal/damaged/ruin × 4 | 12 | P0 | L | wip |
| `b_laboratory` | 4×4 | 1 | — | normal/damaged/ruin × 4 | 12 | P0 | L | placeholder |
| `b_support_bay` | 3×3 | 1 | — | normal/damaged/ruin × 2 | 6 | P0 | M | placeholder |
| `b_vault` | 3×3 | 1 | — | normal/damaged/ruin × 4 | 12 | P0 | M | placeholder |
| `b_alloy_depot` | 2×2 | 1 | — | normal/damaged/ruin × 3 | 9 | P1 | M | placeholder |
| `b_alloy_foundry` | 3×3 | 1 | work 4 | normal/damaged/ruin × 3 | 13 | P1 | M | placeholder |
| `b_command` | 3×3 | 1 | — | normal/damaged/ruin × 3 | 9 | P1 | M | placeholder |
| `b_defense_barracks` | 3×3 | 1 | — | normal/damaged/ruin × 2 | 6 | P1 | M | placeholder |
| `b_market` | 3×3 | 1 | — | normal/damaged/ruin × 2 | 6 | P1 | M | placeholder |
| `b_radar` | 2×2 | 1 | work 4 | normal/damaged/ruin × 2 | 10 | P1 | M | placeholder |
| `b_shipyard` | 4×4 | 1 | — | normal/damaged/ruin × 2 | 6 | P1 | L | placeholder |

## `defences` — оборона (d_*): пассивные препятствия и активные башни

Атлас: `defences`; первая загрузка: нет (лениво); исходники: `art/src/defences/`.

| id | Футпринт | Напр. | Анимации (кадров) | Состояния × тиры | Кадров (план) | Приоритет | Сложность | Статус |
|---|---|---|---|---|---|---|---|---|
| `d_fence` | 1×1 | 1 | — | normal/damaged/ruin × 1 | 3 | P0 | S | placeholder |
| `d_mg_nest` | 2×2 | 1 | attack 3 | normal/damaged/ruin × 1 | 6 | P0 | M | placeholder |
| `d_mine` | 1×1 | 1 | — | normal/damaged/ruin × 1 | 3 | P0 | S | placeholder |
| `d_mortar` | 3×3 | 1 | attack 3 | normal/damaged/ruin × 1 | 6 | P0 | M | placeholder |
| `d_sniper_tower` | 2×2 | 1 | attack 3 | normal/damaged/ruin × 1 | 6 | P0 | M | placeholder |
| `d_wire` | 1×1 | 1 | — | normal/damaged/ruin × 1 | 3 | P0 | S | placeholder |
| `d_aa` | 2×2 | 1 | attack 3 | normal/damaged/ruin × 1 | 6 | P1 | M | placeholder |
| `d_at_cannon` | 2×2 | 1 | attack 3 | normal/damaged/ruin × 1 | 6 | P1 | M | placeholder |
| `d_bunker` | 3×3 | 1 | attack 3 | normal/damaged/ruin × 1 | 6 | P1 | M | placeholder |
| `d_coastal` | 3×3 | 1 | attack 3 | normal/damaged/ruin × 1 | 6 | P1 | M | placeholder |
| `d_flame_trap` | 2×2 | 1 | attack 3 | normal/damaged/ruin × 1 | 6 | P1 | M | placeholder |
| `d_spring_trap` | 1×1 | 1 | — | normal/damaged/ruin × 1 | 3 | P1 | S | placeholder |
| `d_tank_trap` | 1×1 | 1 | — | normal/damaged/ruin × 1 | 3 | P1 | S | placeholder |
| `d_wall` | 1×1 | 1 | — | normal/damaged/ruin × 1 | 3 | P1 | S | placeholder |
| `d_jammer` | 2×2 | 1 | attack 3 | normal/damaged/ruin × 1 | 6 | P2 | M | placeholder |
| `d_tesla` | 2×2 | 1 | attack 3 | normal/damaged/ruin × 1 | 6 | P2 | M | placeholder |

## `units` — пехота и дроны (u_*)

Атлас: `units`; первая загрузка: да; исходники: `art/src/units/`.

| id | Футпринт | Напр. | Анимации (кадров) | Состояния × тиры | Кадров (план) | Приоритет | Сложность | Статус |
|---|---|---|---|---|---|---|---|---|
| `u_brawler` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P0 | S | placeholder |
| `u_breacher` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P0 | S | placeholder |
| `u_dasher` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P0 | S | placeholder |
| `u_rifleman` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P0 | S | wip |
| `u_sniper` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P0 | S | placeholder |
| `u_drone` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P1 | M | placeholder |
| `u_flamer` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P1 | S | placeholder |
| `u_grenadier` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P1 | S | placeholder |
| `u_medic` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P1 | S | placeholder |
| `u_saboteur` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P1 | S | placeholder |
| `u_heavy` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P2 | M | placeholder |
| `u_paratrooper` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P2 | M | placeholder |

## `vehicles` — техника (v_*); кадр 48 до решения OQ-ART1-2

Атлас: `vehicles`; первая загрузка: нет (лениво); исходники: `art/src/vehicles/`.

| id | Футпринт | Напр. | Анимации (кадров) | Состояния × тиры | Кадров (план) | Приоритет | Сложность | Статус |
|---|---|---|---|---|---|---|---|---|
| `v_humvee` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P0 | M | placeholder |
| `v_apc` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P1 | M | placeholder |
| `v_engineer` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P2 | M | placeholder |
| `v_mortar` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P2 | M | placeholder |
| `v_tank` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P2 | M | placeholder |

## `ships` — корабли (s_*); кадр 48 до решения OQ-ART1-2

Атлас: `ships`; первая загрузка: нет (лениво); исходники: `art/src/ships/`.

| id | Футпринт | Напр. | Анимации (кадров) | Состояния × тиры | Кадров (план) | Приоритет | Сложность | Статус |
|---|---|---|---|---|---|---|---|---|
| `s_gunboat` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P1 | L | placeholder |
| `s_lander` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P1 | L | placeholder |
| `s_destroyer` | — | 8 | idle 2, walk 4, attack 3, die 4 | — | 104 | P2 | L | placeholder |

## `heroes` — герои (h_*)

Атлас: `heroes`; первая загрузка: нет (лениво); исходники: `art/src/heroes/`.

| id | Футпринт | Напр. | Анимации (кадров) | Состояния × тиры | Кадров (план) | Приоритет | Сложность | Статус |
|---|---|---|---|---|---|---|---|---|
| `h_commander` | — | 8 | idle 2, walk 4, attack 3, die 4, ability 3 | — | 128 | P1 | L | placeholder |
| `h_engineer` | — | 8 | idle 2, walk 4, attack 3, die 4, ability 3 | — | 128 | P2 | L | placeholder |
| `h_ghost` | — | 8 | idle 2, walk 4, attack 3, die 4, ability 3 | — | 128 | P2 | L | placeholder |
| `h_warden` | — | 8 | idle 2, walk 4, attack 3, die 4, ability 3 | — | 128 | P2 | L | placeholder |

## `spells_fx` — заклинания (c_*): иконка + эффект применения

Атлас: `spells`; первая загрузка: нет (лениво); исходники: `art/src/spells_fx/`.

| id | Футпринт | Напр. | Анимации (кадров) | Состояния × тиры | Кадров (план) | Приоритет | Сложность | Статус |
|---|---|---|---|---|---|---|---|---|
| `c_medkit` | — | 1 | cast 6 | icon × 1 | 7 | P0 | S | placeholder |
| `c_smoke` | — | 1 | cast 6 | icon × 1 | 7 | P0 | S | placeholder |
| `c_adrenaline` | — | 1 | cast 6 | icon × 1 | 7 | P1 | S | placeholder |
| `c_airstrike` | — | 1 | cast 6 | icon × 1 | 7 | P1 | S | placeholder |
| `c_breach` | — | 1 | cast 6 | icon × 1 | 7 | P1 | S | placeholder |
| `c_emp` | — | 1 | cast 6 | icon × 1 | 7 | P1 | S | placeholder |

## `ui` — иконки и элементы интерфейса U03+U01 (id свободные с префиксом ui_)

Атлас: `ui`; первая загрузка: да; исходники: `art/src/ui/`.

_Сущностей пока нет — id добавляются в `art/src/ui/models.json` с префиксом `ui_`._
