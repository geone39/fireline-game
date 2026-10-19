# CHANGELOG `economy`

## Unreleased
- data(economy): `content/building_costs.yaml` + `content/schema/building_costs.schema.json` (ECON-4): цена и время для 17 зданий и 16 защитных сооружений; формат для `svc/base.LoadPriceBook` без изменений (закрывает OQ-B6); `econ_sim --check` читает файл (схема, полнота, ростовые ручки = economy.yaml, счёт структур 0.4–1.4× счёта cash, время 0.5–1.7× бюджета билдеров); тест `building_costs_test.go`; запросы в OQ-E5..E7
- feat(economy): стартовый грант гостя (ECON-3): `start_grant` в `content/economy.yaml` (cash 1500, fuel 600), `Ledger.GrantStart/GrantStartTx` (idemKey `start_grant:<playerID>`, один раз), валидация в `Parse`, тесты; `econ_sim` стартует с грантом и проверяет его в `--check`
- contract-change: OWNS += `content/building_costs.yaml` и его схема (цены/время стройки, ECON-4; решение владельца 2026-10-19)
- feat(economy): Ledger.Apply/ApplyTx — идемпотентный (idemKey на игрока), append-only, единственный писатель wallets; тесты повтора и конкурентных вызовов (testcontainers)
- feat(economy): сборщики и хранилища (Accrue/Collect/StorageCap/PoolCap) по `content/economy.yaml`
- data(economy): `passive.*` в yaml структурирован (collectors_count, round_to, production_share_pct) вместо строки-формулы
- test(economy): `econ_sim.py --check` — цели темпа и сверка yaml с симулятором
- модуль заведён
