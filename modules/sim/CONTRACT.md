# CONTRACT — `sim`

**Версия контракта:** 0.2.2 (0.2 — `contract-change` SIM-0; основание — ADR-003, `docs/design/battle-sim.md` §13; 0.2.1 — аддитивно, SIM-1; 0.2.2 — аддитивно, SIM-2). Реализовано: мир, высадка, `Move`, поиск пути, выбор целей (SIM-1); `Attack`, урон/сплэш, разрушение, скрытые ловушки, конец боя, звёзды, `LootBasis` (SIM-2). `Ability` и `Cast` возвращают `ErrNotImplemented` (команда состояния не меняет) до SIM-5; фаза обороны (`EndTurn`) — пока без действий (SIM-3).

_Изменения контракта — отдельным PR с меткой `contract-change`. Любое изменение правил = повышение `sim.EngineVersion` + пересоздание golden._

## API (Go, пакет `sim`)
```go
func Run(snap Snapshot, cmds []Command, seed uint64) (Result, error)   // полный реплей журнала
func NewBattle(snap Snapshot, seed uint64) (*Battle, error)            // пошаговая форма для клиента
func (b *Battle) Apply(cmd Command) ([]Event, error)                   // отклонённая команда — не error, а Result.Rejected
func (b *Battle) Result() Result
const EngineVersion = "0.2.2"
var ErrNotImplemented error                                            // Ability/Cast, пока нет SIM-5
var ErrBadSnapshot error                                                // NewBattle: неиграбельный Snapshot/Rules
```

## Типы
- `Cell{X, Y int16}`, `EntityID uint32` (0 = нет). Время — **ходы** (`Command.Turn`): 0 = развёртывание, 1…N. Тиков и fixed-point нет.
- `Snapshot{ContentVersion, BaseID, Map, Rules, Buildings []PlacedBuilding{ID, Level, Pos Cell, Revealed}, Garrison, Army, Spells, Hero, HeroLevel}`. Порядок `Buildings`/`Garrison` значения не имеет (нормализуется движком, §2.4). **Клиентский Snapshot не содержит скрытых ловушек** (OQ-7.8): его строит сервер; серверный пересчёт идёт по полному.
- `Command{Turn, Op, Unit, Target EntityID, ID string, Level int, To Cell, Path []Cell}`; `Op`: `Deploy, Load, Start, Move, Attack, Ability, Cast, EndTurn, Retreat`. `Turn` в журнале не убывает.
- `Result{Stars, Destruction, StarFlags, HQDestroyed, TurnsUsed, EndReason, LootBasis, UnitsLost, SpellsUsed, HeroDamageTaken, HeroSurvived, Rejected []Rejection{Index, Code}, EngineVersion, StateHash}`. Списки отсортированы, `map` в контракте нет (`LootBasis.Storages []StorageHit`).
- `Event{Kind, Actor, Target, Cell, Value}` — только для анимации, в `StateHash` не входят.

## Rules (0.2.1, additive; 0.2.2 — поля боя)
`Snapshot.Rules` — числа из content уже **в целых «за ход»** (спека §3.2): `GridW/H`, `BuildZone`, `Maps[]{ID, Water[]Rect}`, `MaxTurns`, `ParadropEdgeCells`, `Units[]UnitDef`, `Buildings[]BuildingDef`. sim не читает файлы и не знает yaml; вызывающий (server / WASM-обёртка) строит `Rules` из `content/*.yaml` (OQ-SIM3, OQ-SIM5). `Rules` неизменяемы на время боя; в `StateHash` не входят (идентичность — `ContentVersion`).
`NewBattle` возвращает `ErrBadSnapshot`: нет/неверные `Rules`, неизвестные ids/карта, здание вне `build_zone`/на воде/с перекрытием.

**0.2.2 (аддитивно, нулевые значения = «нет»):** `Rules.SplashFalloffPct` (`battle.splash_falloff_pct`, 0..100); `UnitDef.SplashCells, Burst, WallMult, VsMult []{Tag, Permille}`; `BuildingDef.SplashCells, Trap (TrapMine|TrapSpring), Burst, MaxHPFlung, FlingCells` (классы, на которые срабатывает ловушка, — `BuildingDef.Vs`). Теги `VsMult`: `VsWire, VsWall, VsBuilding, VsDefence, VsINF, VsVEH, VsAIR, VsSEA`; `Permille` — множитель (1500 = ×1,5), бонусы суммируются, один `floor` (спека §6.1). Брони (`armor_pct`) нет — вне R1 (§16).

## Что НЕ в sim (берут из `content/battle.yaml`, CONTENT-REQ §14)
`max_turns` (предохранитель конечности), `time_limit_s` (лимит реального времени — **только сервер**: по истечении дописывает `Retreat` в журнал), `turn_s`, `splash_falloff_pct` и прочие числа. В коде sim игровых констант нет.

## Поведение
- `Attack{Unit, Target}` (SIM-2): порядок проверок — id, владелец, жив/на поле/есть атака (`invalid_target`), `no_ap`, цель допустима (`invalid_target`: своя, мёртвая, декор, скрытая ловушка, юнит внутри бункера/на борту), `below_min_range`, `out_of_range`. Расстояние — Чебышёв до footprint. Урон: `Dmg` (или `Burst × WallMult` по стенам/проволоке) × `(1000 + Σ(vs_mult−1000))/1000`; сплэш — `dmg × (100·r − falloff·d)/(100·r)` по всем врагам в `r` от ближайшей к стрелку клетки цели (основная цель всегда, затем по возрастанию `EntityID`). `Burst`-юнит (`u_breacher`) уничтожается после подрыва. Разрушенное освобождает клетки сразу; гибель перевозчика высаживает пассажиров на соседние свободные клетки по `(Y,X)` (не поместившиеся гибнут).
- Скрытые ловушки: `Move` останавливается на первой сработавшей (путь после неё отбрасывается, AP тратится). Мина — `Burst` + сплэш по врагам классов из `Vs`; пружина — INF с `max_hp ≤ MaxHPFlung` отбрасывается против последнего шага на `FlingCells` (над препятствиями, на самую дальнюю свободную клетку луча). Вскрытая (`Revealed`) ловушка срабатывает так же, но может быть целью `Attack` (тогда просто убирается без взрыва).
- Конец боя (после каждой команды): `full_destruction` (раньше `no_units`), `no_units`, `turn_limit` (в `EndTurn`), `retreat`. `UnitsLost` не включает героя (спека §9.2); `HeroDamageTaken` — сумма реально снятых HP героя.
- Отклонённая команда не меняет состояние (единственное исключение — неявный `EndTurn`, который выполняет любая команда с `Turn = t+1`; `Turn` вне `{t, t+1}` → `bad_turn`); код — `RejectCode`. Строгая политика (OQ-7.7): клиентский результат с `Rejected` ≠ ∅ сервер не принимает, засчитывает пересчёт.
- Детерминизм: без `float`, `time`, `math/rand`, итерации по `map`; проверяется `TestDeterminismLint`.
- Потребители, которых нужно обновить: `server/svc/raid` (ещё не создан), `client/sim-wasm` (SIM-4/client-engine), `server` finish-валидация — см. `docs/open-questions.md` (OQ-SIM1…).
