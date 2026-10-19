# CONTRACT — `sim`

**Версия контракта:** 0.2.1 (0.2 — `contract-change` SIM-0; основание — ADR-003, `docs/design/battle-sim.md` §13; 0.2.1 — аддитивно, SIM-1). Реализовано (SIM-1): мир, высадка, `Move`, поиск пути, выбор целей. `Attack`, `Ability`, `Cast` и фаза обороны возвращают `ErrNotImplemented` (команда состояния не меняет) до SIM-2/3/5.

_Изменения контракта — отдельным PR с меткой `contract-change`. Любое изменение правил = повышение `sim.EngineVersion` + пересоздание golden._

## API (Go, пакет `sim`)
```go
func Run(snap Snapshot, cmds []Command, seed uint64) (Result, error)   // полный реплей журнала
func NewBattle(snap Snapshot, seed uint64) (*Battle, error)            // пошаговая форма для клиента
func (b *Battle) Apply(cmd Command) ([]Event, error)                   // отклонённая команда — не error, а Result.Rejected
func (b *Battle) Result() Result
const EngineVersion = "0.2.1"
var ErrNotImplemented error                                            // Attack/Ability/Cast, пока нет SIM-2/5
var ErrBadSnapshot error                                                // NewBattle: неиграбельный Snapshot/Rules
```

## Типы
- `Cell{X, Y int16}`, `EntityID uint32` (0 = нет). Время — **ходы** (`Command.Turn`): 0 = развёртывание, 1…N. Тиков и fixed-point нет.
- `Snapshot{ContentVersion, BaseID, Map, Rules, Buildings []PlacedBuilding{ID, Level, Pos Cell, Revealed}, Garrison, Army, Spells, Hero, HeroLevel}`. Порядок `Buildings`/`Garrison` значения не имеет (нормализуется движком, §2.4). **Клиентский Snapshot не содержит скрытых ловушек** (OQ-7.8): его строит сервер; серверный пересчёт идёт по полному.
- `Command{Turn, Op, Unit, Target EntityID, ID string, Level int, To Cell, Path []Cell}`; `Op`: `Deploy, Load, Start, Move, Attack, Ability, Cast, EndTurn, Retreat`. `Turn` в журнале не убывает.
- `Result{Stars, Destruction, StarFlags, HQDestroyed, TurnsUsed, EndReason, LootBasis, UnitsLost, SpellsUsed, HeroDamageTaken, HeroSurvived, Rejected []Rejection{Index, Code}, EngineVersion, StateHash}`. Списки отсортированы, `map` в контракте нет (`LootBasis.Storages []StorageHit`).
- `Event{Kind, Actor, Target, Cell, Value}` — только для анимации, в `StateHash` не входят.

## Rules (0.2.1, additive)
`Snapshot.Rules` — числа из content уже **в целых «за ход»** (спека §3.2): `GridW/H`, `BuildZone`, `Maps[]{ID, Water[]Rect}`, `MaxTurns`, `ParadropEdgeCells`, `Units[]UnitDef`, `Buildings[]BuildingDef`. sim не читает файлы и не знает yaml; вызывающий (server / WASM-обёртка) строит `Rules` из `content/*.yaml` (OQ-SIM3, OQ-SIM5). `Rules` неизменяемы на время боя; в `StateHash` не входят (идентичность — `ContentVersion`).
`NewBattle` возвращает `ErrBadSnapshot`: нет/неверные `Rules`, неизвестные ids/карта, здание вне `build_zone`/на воде/с перекрытием.

## Что НЕ в sim (берут из `content/battle.yaml`, CONTENT-REQ §14)
`max_turns` (предохранитель конечности), `time_limit_s` (лимит реального времени — **только сервер**: по истечении дописывает `Retreat` в журнал), `turn_s`, `splash_falloff_pct` и прочие числа. В коде sim игровых констант нет.

## Поведение
- Отклонённая команда не меняет состояние (единственное исключение — неявный `EndTurn`, который выполняет любая команда с `Turn = t+1`; `Turn` вне `{t, t+1}` → `bad_turn`); код — `RejectCode`. Строгая политика (OQ-7.7): клиентский результат с `Rejected` ≠ ∅ сервер не принимает, засчитывает пересчёт.
- Детерминизм: без `float`, `time`, `math/rand`, итерации по `map`; проверяется `TestDeterminismLint`.
- Потребители, которых нужно обновить: `server/svc/raid` (ещё не создан), `client/sim-wasm` (SIM-4/client-engine), `server` finish-валидация — см. `docs/open-questions.md` (OQ-SIM1…).
