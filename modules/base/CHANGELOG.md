# CHANGELOG `base`

## Unreleased
- feat: BASE-2 — команды upgrade/remove/collect/speedup/clear_obstacle (`Engine.Plan`), ленивый пересчёт таймеров по серверному времени (`Settle`: вперёд/назад, повтор идемпотентен), билдеры и цены из content (`PriceBook`, OQ-B6), `Service` (GET/Apply/SaveLayout) с списанием только через `economy.Ledger.ApplyTx`, записью в `base_events`/`base_state`/`timers`/`collector_pools`, атомарным пакетом и идемпотентностью по `Idempotency-Key`/uid; chi `Mount` для `/v1/base`, `/v1/base/commands`, `PUT /v1/base/layout` (OQ-B7, OQ-B8, OQ-B9)
- feat: `LoadGrid`/`ParseGrid` читают размер сетки и воду из content/base_grid.yaml (OQ-B1/B5); тесты каталога не закрепляют отсутствие size/лимитов у decor, obstacles и ресурсных зданий (BASE-2)
- feat: server/svc/base — сетка, каталог из content/*.yaml, валидация build/move и раскладки (границы, пересечения, лимиты по HQ, requires_water, запрет полной замкнутости HQ без разрушаемых стен), идемпотентность по uid, типы остальных BaseCommand (BASE-1)
- модуль заведён
