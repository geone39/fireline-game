# Golden-реплеи sim (план)

Файл `Gxx.json` = `{snapshot, commands, seed, content_version, engine_version, expect:{result, state_hash}}`. Пересоздаются только при повышении `EngineVersion` (отдельный коммит). Хеш сверяется на native и WASM. Фикстура F01 — `docs/design/battle-sim.md` §12.

**Состояние (SIM-1, EngineVersion 0.2.1):** файлы `G01…G05.json` лежат здесь и генерируются тестом `go test -run TestGolden -update` из сценариев `sim/golden_test.go` (только вместе с повышением `EngineVersion`, отдельным коммитом; CI сверяет `Result` и `StateHash`). Правила `Rules` встроены в каждый файл (тестовая фикстура, **не баланс**). В SIM-1 это сценарии без урона:

| ID | Что проверяет (SIM-1) |
|---|---|
| G01 | F01-база на plain: высадка двух юнитов, 3 хода ходьбы по явным путям, `Retreat` на ходу 4 |
| G02 | Местность `Move`: дрон над стенами, проволока (стоимость 2), танковый ёж (пехота проходит, техника нет), неявный `EndTurn` |
| G03 | Отклонения: `bad_phase, bad_turn, no_stock, out_of_zone, cell_occupied, hero_limit, unknown_entity, not_owner, invalid_target, no_ap, bad_path, path_cost, battle_over` — состояние не меняется |
| G04 | Особая высадка: десант группой по 4, герой L2, БТР `carries` (третий пассажир → `capacity`) |
| G05 | `Retreat` в фазе 0 (`TurnsUsed=0`), следующая команда → `battle_over` |

Таблица ниже — исходный план G01..G26; её ID **не совпадают** с файлами SIM-1 (OQ-SIM7), пересборка плана — при SIM-2.

ASSUMPTION: распределение G01..G26 (AGENT.md требует 26) — предложение sim, уточняет balance/QA.

| ID | Что проверяет |
|---|---|
| G01 | F01 из спеки §12: стрелок + штурмовик против гнезда, `Retreat` (Destruction 50, 1★) |
| G02 | Полное разрушение, 3★, `EndReason=FullDestruction` |
| G03 | `no_units`: все юниты погибли |
| G04 | `turn_limit` (предохранитель `max_turns`) |
| G05 | Retreat в фазе 0 (`TurnsUsed=0`) |
| G06 | Высадка: зона каймы, корабли в воде, герой один, `Start` без юнитов |
| G07 | AP: Move+Attack при ap=1 отклоняется (`no_ap`), при ap=2 проходит |
| G08 | Move: диагональная «лесенка» стен, стоимость проволоки |
| G09 | Дальность: `min_range`, ближний бой, сплэш с затуханием |
| G10 | Цепь `d_tesla` |
| G11 | Стены, `u_breacher`, `c_breach` |
| G12 | Мина: прерывание пути, сплэш |
| G13 | `d_spring_trap`: бросок INF, тяжёлый юнит не срабатывает |
| G14 | Проволока: урон в фазе обороны |
| G15 | Бункер: триггер по урону и по `trigger_cells`, гарнизон идёт по §7.4 |
| G16 | Саботёр `jam`, `c_emp`, `d_jammer` |
| G17 | `c_airstrike`/`c_medkit`/`c_smoke`/`c_adrenaline` |
| G18 | Герой + способности (`a_*`), аура `h_commander` |
| G19 | БТР: `Load`/`unload`, гибель перевозчика |
| G20 | Корабли: вода, береговые цели, `s_lander` |
| G21 | Перегрев `d_flame_trap` |
| G22 | `u_paratrooper` (группа из 4) |
| G23 | Отклонения: все `RejectCode`, состояние не меняется |
| G24 | Перестановка `Snapshot.Buildings` даёт тот же `StateHash` |
| G25 | Скрытые ловушки: полный Snapshot vs клиентский вид (расхождение ожидаемо) |
| G26 | Большая база HQ10 (150 построек, 120 юнитов) — также бенчмарк |
