# CHANGELOG `economy`

## Unreleased
- contract-change: OWNS += `content/building_costs.yaml` и его схема (цены/время стройки, ECON-4; решение владельца 2026-10-19)
- feat(economy): Ledger.Apply/ApplyTx — идемпотентный (idemKey на игрока), append-only, единственный писатель wallets; тесты повтора и конкурентных вызовов (testcontainers)
- feat(economy): сборщики и хранилища (Accrue/Collect/StorageCap/PoolCap) по `content/economy.yaml`
- data(economy): `passive.*` в yaml структурирован (collectors_count, round_to, production_share_pct) вместо строки-формулы
- test(economy): `econ_sim.py --check` — цели темпа и сверка yaml с симулятором
- модуль заведён
