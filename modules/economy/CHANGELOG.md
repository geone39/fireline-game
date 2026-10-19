# CHANGELOG `economy`

## Unreleased
- feat(economy): стартовый грант гостя (ECON-3): `start_grant` в `content/economy.yaml` (cash 1500, fuel 600), `Ledger.GrantStart/GrantStartTx` (idemKey `start_grant:<playerID>`, один раз), валидация в `Parse`, тесты; `econ_sim` стартует с грантом и проверяет его в `--check`
- contract-change: OWNS += `content/building_costs.yaml` и его схема (цены/время стройки, ECON-4; решение владельца 2026-10-19)
- feat(economy): Ledger.Apply/ApplyTx — идемпотентный (idemKey на игрока), append-only, единственный писатель wallets; тесты повтора и конкурентных вызовов (testcontainers)
- feat(economy): сборщики и хранилища (Accrue/Collect/StorageCap/PoolCap) по `content/economy.yaml`
- data(economy): `passive.*` в yaml структурирован (collectors_count, round_to, production_share_pct) вместо строки-формулы
- test(economy): `econ_sim.py --check` — цели темпа и сверка yaml с симулятором
- модуль заведён
