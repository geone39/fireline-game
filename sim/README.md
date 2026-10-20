Боевое ядро. См. `modules/sim/AGENT.md`, `modules/sim/CONTRACT.md`, `docs/design/battle-sim.md`. Тесты: `go test ./...` (из каталога `sim/`; для паритета WASM нужны Go и Node).

## WASM
`sim/wasm/` — сборка ядра для браузера (`build.ps1` / `build.sh` → `sim/wasm/dist/sim.wasm`), JSON-API `fireline_sim` (`run`, `newBattle`, `apply`, `result`, `state`, `free`) и типы `api.d.ts`. Проверка паритета с native: `go test ./wasm/...` — все golden через node, `Result`, `StateHash` и `state(handle)` после каждой команды совпадают байт-в-байт.

`state(handle)` (контракт 0.2.5) — клиентский снимок мира сейчас: сущности, hp, ap, позиции, зоны эффектов; без скрытых ловушек (OQ-7.8). Заменяет «теневой мир» клиента (OQ-C4b-1).

## Definition of Done (SIM-6)
| Пункт DoD | Где выполнено |
|---|---|
| `go test ./...` зелёный | CI: `go vet`, `gofmt`, `go test ./...` в `sim/` |
| golden G01…G26 (+G27…G32), план → файл → что проверяет | `testdata/replays/README.md`; `TestGoldenPlanIsDocumented`, `TestGoldenCoversEveryRejectCode` |
| golden не меняются без bump `EngineVersion` | `TestGolden` (расхождение = падение), `-update` отказывает переписывать реплей текущей версии (`TestGoldenUpdatePolicy`); пересоздание — отдельным коммитом (README реплеев) |
| property: Run дважды = одинаковые `Result`/`StateHash` на 1000 случайных боёв | `TestPropertyRunTwiceSameResult1000`, `TestPropertySupportRunTwiceSameResult1000` (+ Run = Apply-цикл, перестановка `Buildings`, инварианты мира и `State()`) |
| фазз случайных команд: без паник, невалидные → `RejectCode` | `TestFuzzGarbageCommands1000`, `TestFuzzDegenerateBattles`, `FuzzApply` (`go test -fuzz FuzzApply -fuzztime 60s`) |
| нет float/time/rand/map-iter в логике | `TestDeterminismLint` (белый список импортов; пакеты `sim` и `sim/wasm/wire`), `TestLintCoversAllCoreFiles`, `TestLintDetectsViolations` |
| паритет native/WASM для всех golden | `TestWasmParity` (`go test ./wasm/...`) |
| бенчмарк | см. ниже |

## Бенчмарк
Единица — ход боя: один `EndTurn` (закрытие хода игрока и вся фаза обороны). Старая формулировка DoD «≥5000 тиков/с» относилась к тиковой модели до ADR-003 (OQ-SIM11, OQ-SIM12).

`go test -run xxx -bench TurnsPerSecond -benchtime 300x .` (Ryzen 7 5800X, go1.27.0, Windows):

| Нагрузка | время на 10 ходов | ходов/с |
|---|---|---|
| худший случай спеки §11.8: 150 построек (12-местные бункеры), 120 гарнизона, 120 атакующих | ≈ 4,8 мс | ≈ 2100 |
| поменьше: 50 построек, 30 гарнизона, 30 атакующих (живут дольше, все 10 ходов с целями) | ≈ 6,2 мс | ≈ 1600 |

Порог регрессии в тесте — **500 ходов/с** (`TestBenchmarkThreshold`, пропускается в `-short` и `-race`): бой в 30 ходов на худшей нагрузке укладывается в ≈ 15 мс (порог ≈ 60 мс). Буквальные 5000 ходов/с на худшей нагрузке **не достигнуты**; узкие места — Дейкстра гарнизона (≈ 40%) и выбор цели обороны (≈ 25%), ускорение без изменения правил — отдельная задача (OQ-SIM12).
