# CHANGELOG `sim`

## Unreleased
### 0.2.0 — SIM-0, contract-change (только контракт, правил нет)
- Пошаговый бой на сетке (ADR-003): `Cell` вместо fixed-point `Vec`, `Turn` вместо тиков; удалены `TickMs`, `MaxTicks`, `TileUnits`, `OpHeroAbility`.
- Новые `Command` (Unit/Target/ID/Level/To/Path), `Op` (Deploy, Load, Start, Move, Attack, Ability, Cast, EndTurn, Retreat), расширенный `Result` (EndReason, StarFlags, потери, Rejected, StateHash), `LootBasis` без `map`.
- `Run` 0.2 + `NewBattle/Apply/Result`; пока возвращают `ErrNotImplemented`.
- `EngineVersion = "0.2.0"`; тест формы API и `TestDeterminismLint`; план golden — `sim/testdata/replays/README.md`.

## 0.1
- модуль заведён
