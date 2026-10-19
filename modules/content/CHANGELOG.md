# CHANGELOG `content`

## Unreleased
- модуль заведён
- OWNS: `content/economy.yaml` исключён (принадлежит `economy`, устранено пересечение)
- OWNS: `content/building_costs.yaml` и `content/schema/building_costs.schema.json` исключены (принадлежат `economy`, ECON-4)
- feat(content): CONTENT-2 — `content/strings/{ru,en}/names.yaml` (85 `name_key`, нейтральные плейсхолдеры, D-10)
- data(content): добавлены `name_key` для `o_rock`/`o_wreck`, всех узлов `research.yaml`, глав `ch2..ch4` (аддитивно, `schema: 1` не меняется — миграция не требуется; `title_placeholder` оставлен)
- feat(tools): `validate_content.py` проверяет наличие `name_key` у каждого id, их уникальность, полноту ru/en и отсутствие «сирот» в strings
- feat(content): CONTENT-3 — `content/base_grid.yaml` + `content/schema/base_grid.schema.json`: сетка 44×44, `build_zone`, карты plain/coastal/canyon с прямоугольниками воды (аддитивно)
- data(content): `size` у decor (`x_flag` 1×1, `x_statue`/`x_flowerbed` 2×2) и obstacles (`o_rock` 1×1, `o_wreck` 2×2); `max_by_hq` у ресурсных зданий (аддитивно, `schema: 1` без миграции)
- data(content): строки `map.plain/coastal/canyon` (ru/en)
- feat(tools): `validate_content.py` проверяет `base_grid.yaml` (схема, границы, name_key карт), наличие `size` у всех размещаемых и корректность `max_by_hq`
- feat(content): CONTENT-4 (#71) — battle keys for sim (additive, `schema: 1` unchanged, no migration): new `content/battle.yaml` (+`battle.schema.json`: `time_limit_s=300`, `turn_s`, `max_turns`, `splash_falloff_pct`, `spells_per_turn`, `paradrop_edge_cells`); `ap` (1; 2 only `u_rifleman`/`u_dasher`), `move_cells`, `range_cells`, `splash_cells`, `vs_mult`, `heal_vs`, `group`/`paradrop` in `troops.yaml`; spell `kind`/`duration_s`/`radius_cells`/adrenaline permille; `level_growth` in troops/defences/heroes; defences: `trap`/`hidden`/`fling_cells`/`overheat_fire_turns`/`cool_turns`/`chain_cells`/`jam_pct`/`range_cells`/`min_range_cells`/`splash_cells`/`destruction_weight`; buildings: `hp_l1`/`hp_growth`/`destruction_weight`, `untargetable`; heroes: `ap`, cells, `ability.effect`, aura keys, `in_r1`, `decoys` section (`u_decoy_turret`, name_key ru/en)
- feat(tools): `validate_content.py` checks battle.yaml, every sim-facing key (presence + consistency of `*_cells` with `speed`/`range` at `battle.turn_s`), the `ap: 2` rule, per-kind effect fields, and that every `battle.<key>` named in `sim/*.go` exists in `battle.yaml`; new schemas `spell`/`defence`/`hero`

