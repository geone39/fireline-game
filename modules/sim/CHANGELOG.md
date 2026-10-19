# CHANGELOG `sim`

## Unreleased
### 0.2.1 — SIM-1: мир, высадка, Move, поиск пути, цели (EngineVersion 0.2.1)
- Мир 44×44 по `Snapshot.Rules`: вода (`maps.<id>.water`), здания по `(Y,X,ID)` (канонический порядок EntityID, §2.4), слои земля/вода/воздух, проходимость (стены, `d_tank_trap`, диагональная «лесенка»), гарнизон бункеров получает EntityID (юниты внутри, высвобождение — SIM-3).
- Фаза 0: `Deploy` (зона высадки = вне `build_zone`, корабли — вода, герой один, десант группой по `Group`), `Load`, `Start`; `Retreat`. Отклонения по `RejectCode`, состояние не меняется.
- `Move`: явный путь, AP, `move_cells`, стоимость проволоки `ceil(100/(100-slow))`; неявный `EndTurn` при смене `Turn`; `max_turns` как предохранитель.
- Дейкстра на клетках (тайбрейк N,NE,E,SE,S,SW,W,NW; очередь `(cost, idx)`), `approach` (§7.4 шаг 2) и выбор целей `(Chebyshev, hp, EntityID)` с `min_range`, `Vs`, `LineOfFire`-заглушкой — внутренние, под SIM-2/3.
- `StateHash` = xxhash64 (§11.5, собственная реализация без зависимостей); `Result` (Destruction/Stars/LootBasis считаются, пока всегда 0).
- **Additive contract-change 0.2.1:** `Snapshot.Rules` (+ типы `Rules`, `UnitDef`, `BuildingDef`, …), `ErrBadSnapshot`; `Attack/Ability/Cast` → `ErrNotImplemented` (SIM-2/5).
- Тесты: property (1000 случайных логов: Run дважды, Run = Apply-цикл, перестановка `Buildings`), golden G01–G05, бенчмарк пути 44×44, `TestDeterminismLint`.
### 0.2.0 — SIM-0, contract-change (только контракт, правил нет)
- Пошаговый бой на сетке (ADR-003): `Cell` вместо fixed-point `Vec`, `Turn` вместо тиков; удалены `TickMs`, `MaxTicks`, `TileUnits`, `OpHeroAbility`.
- Новые `Command` (Unit/Target/ID/Level/To/Path), `Op` (Deploy, Load, Start, Move, Attack, Ability, Cast, EndTurn, Retreat), расширенный `Result` (EndReason, StarFlags, потери, Rejected, StateHash), `LootBasis` без `map`.
- `Run` 0.2 + `NewBattle/Apply/Result`; пока возвращают `ErrNotImplemented`.
- `EngineVersion = "0.2.0"`; тест формы API и `TestDeterminismLint`; план golden — `sim/testdata/replays/README.md`.

## 0.1
- модуль заведён
