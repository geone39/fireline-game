# CHANGELOG `economy`

## Unreleased
- feat(economy): Ledger.Apply/ApplyTx — идемпотентный (idemKey на игрока), append-only, единственный писатель wallets; тесты повтора и конкурентных вызовов (testcontainers)
- feat(economy): сборщики и хранилища (Accrue/Collect/StorageCap/PoolCap) по `content/economy.yaml`
- data(economy): `passive.*` в yaml структурирован (collectors_count, round_to, production_share_pct) вместо строки-формулы
- test(economy): `econ_sim.py --check` — цели темпа и сверка yaml с симулятором
- модуль заведён
