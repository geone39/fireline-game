# CHANGELOG `raid`

## Unreleased
- модуль заведён
- RAID-0 (#73): пакет `server/svc/raid` без HTTP: `LoadRules`/`LoadContent` (content/*.yaml → `sim.Rules` в единицах ядра, ошибки на отсутствующих ключах, `Content.Version`), `FullSnapshot`/`ClientSnapshot` (клиентский без скрытых ловушек, OQ-7.8), `Verify` (пересчёт на native sim, строгая политика: расхождение → результат клиента отклонён, засчитан серверный). Тесты: все golden `sim/testdata/replays` + реальный content. `server/go.mod`: `replace github.com/OWNER/fireline/sim => ../sim`; `deploy/server.Dockerfile` копирует `sim/` (правка ops, см. PR).
