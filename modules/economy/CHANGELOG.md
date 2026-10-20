# CHANGELOG `economy`

## Unreleased
- feat(economy): ECON-5 армия и лаборатория в `tools/econ_sim.py` (#104): полная армия по HQ (housing из `capacity_by_level`/`max_by_hq`, цены и `train_s` из `troops.yaml`), замена потерь рейда и поиск соперника из кошелька, исследования лаборатории (один узел за раз, tier по HQ), ускорения за gems единой функцией `ceil(h^0.8×4)` и модель платящего; `--check` проверяет: fuel-счёт ≥10% свободен после армии и исследований, потерю среднего рейда ≤15% дневного дохода, время дозаказа ≤50% паузы между сессиями, занятость лаборатории ≤90%, платящий (10 gems/сутки + 6-й билдер) ≤1.6× казуала, `speedup_gems` = 4/0.8; цели темпа не изменились (казуал HQ10 — день 185, активный ≤1.30×)
- data(economy): `content/economy.yaml` += `army_lab` (допущения модели игрока) и `payer_check`; сверяются с симулятором, серверный код их не читает
- docs(economy): `docs/design/economy.md` «Армия и лаборатория»; OQ-E7 закрыт, запросы OQ-ECON5-1…4 (потери рейда, gems/платящий — needs-owner, критерий 3 и узлы лаборатории — для content)
- data(economy): `content/building_costs.yaml` + `content/schema/building_costs.schema.json` (ECON-4): цена и время для 17 зданий и 16 защитных сооружений; формат для `svc/base.LoadPriceBook` без изменений (закрывает OQ-B6); `econ_sim --check` читает файл (схема, полнота, ростовые ручки = economy.yaml, счёт структур 0.4–1.4× счёта cash, время 0.5–1.7× бюджета билдеров); тест `building_costs_test.go`; запросы в OQ-E5..E7
- feat(economy): стартовый грант гостя (ECON-3): `start_grant` в `content/economy.yaml` (cash 1500, fuel 600), `Ledger.GrantStart/GrantStartTx` (idemKey `start_grant:<playerID>`, один раз), валидация в `Parse`, тесты; `econ_sim` стартует с грантом и проверяет его в `--check`
- contract-change: OWNS += `content/building_costs.yaml` и его схема (цены/время стройки, ECON-4; решение владельца 2026-10-19)
- feat(economy): Ledger.Apply/ApplyTx — идемпотентный (idemKey на игрока), append-only, единственный писатель wallets; тесты повтора и конкурентных вызовов (testcontainers)
- feat(economy): сборщики и хранилища (Accrue/Collect/StorageCap/PoolCap) по `content/economy.yaml`
- data(economy): `passive.*` в yaml структурирован (collectors_count, round_to, production_share_pct) вместо строки-формулы
- test(economy): `econ_sim.py --check` — цели темпа и сверка yaml с симулятором
- модуль заведён
