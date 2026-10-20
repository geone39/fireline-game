# CHANGELOG `army`

## Unreleased
- feat(army): ARMY-1 (#102) `server/svc/army` — очереди обучения по зданиям (`timers kind=train`, ленивый пересчёт вперёд/назад, офлайн-прогресс по секундам), housing по пулам unit/garage/spell из `capacity_by_level` зданий игрока и перка `army_housing_pct`, оплата при постановке через `Ledger.ApplyTx` (идемпотентно по `Idempotency-Key`), отмена с возвратом (100% не начатых / 50% начатого, ключи `army.yaml`), ускорение за gems (`base.PriceBook.SpeedupGems`), `ArmyState`, `FormArmy`/`Resolve`/`Release` (escrow рейда), chi `Mount` (`GET /army`, `POST /army/train|cancel|speedup`). Допущения и запросы — OQ-ARMY-11.
- модуль заведён
