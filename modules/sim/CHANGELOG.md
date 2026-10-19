# CHANGELOG `sim`

## Unreleased
### 0.2.4 — SIM-5: техника, корабли, герои, заклинания (EngineVersion 0.2.4)
- `Ability`: `heal` (медик по INF, инженер по VEH: цель своя раненая, не себя, `+Heal` до `max_hp`), `jam` (саботёр вплотную к постройке с атакой: `disabled` на `JamTurns`), `unload` (все/один пассажир, свободные клетки `cheb ≤ 1` по `(Y,X)`, у вышедших `ap_left = 0`; десантный катер высаживает только на сушу), `hero` (`zone_damage`, `spawn_decoy`, `stealth`; кулдаун в ходах, 1 AP).
- `Cast`: `c_airstrike`, `c_medkit`, `c_smoke`, `c_adrenaline`, `c_emp`, `c_breach`; зоны по Чебышёву, эффект `t…t+turns−1`, шаг в фазе обороны (до проволоки), `spells_per_turn`; джаммер `d_jammer`; дым и невидимость исключают цель из выбора обороны; аура героя и адреналин (бонус к `move_cells` и урону).
- Приманка героя — `KindDecoy`: цель обороны, но не армия (не считается в `no_units`, не в `UnitsLost`).
- **Additive contract-change 0.2.4:** `Rules.{SpellsPerTurn, Spells}`, `SpellDef`, `AbilityDef`, `UnitDef.{DmgGrowthPermille, Heal, HealVs, JamTurns, Ability, AuraCells, AuraDmgPermille, AuraMovePermille}`, `BuildingDef.{AuraCells, JamPct}`, `VsHQ`, `KindDecoy`, `EventSpawned`; `ErrNotImplemented` больше не возвращается (переменная осталась). `StateHash` включает зоны, запас заклинаний, `stealthUntil`, `decoyUntil`, порядок груза. Урон теперь растёт с уровнем юнита, если задан `DmgGrowthPermille` (§3.2; 0 = как раньше).
- Golden: G21–G29 (G01–G20 пересозданы отдельным коммитом из-за bump версии: `Result` без `EngineVersion/StateHash` совпал с прежним до пересоздания); `TestWasmParity` берёт все `G*.json` (паритет native/WASM для G21–G29). Property: `TestPropertySupportRunTwiceSameResult1000` (журналы, полные Ability/Cast; Run дважды, Run = Apply-цикл, перестановка `Buildings`, баланс запаса заклинаний), прежний property с героями/заклинаниями/новыми постройками; unit-тесты `support_test.go`; бенчмарк `BenchmarkSupportTenTurnsFullBase`.
- Не сделано (вне R1, OQ-7.6): герой-страж (`h_warden`, `a_bulwark`, `buff_aura`) и ПВО-зонтик эсминца; корабли обороны. Допущения — `docs/open-questions.md` OQ-SIM11.
### 0.2.3+ — SIM-4: WASM-сборка (EngineVersion не менялся, контракт не менялся)
- `sim/wasm/`: `main.go` (`//go:build js && wasm`, глобал `fireline_sim`: `engineVersion`, `run`, `newBattle`, `apply`, `result`, `free`), `wire/` (JSON-фасад без build-тегов: Command с именами op, `StateHash` и seed строками), `build.ps1`/`build.sh` (`GOOS=js GOARCH=wasm`, `-s -w`, + `wasm_exec.js` из GOROOT), `run_golden.js`, `api.d.ts`.
- Тест `TestWasmParity`: собирает WASM, прогоняет G01…G20 через node (`run` и пошаговый `newBattle/apply`) и требует байт-в-байт тот же ответ, что нативный фасад, и StateHash из golden. `TestWireMatchesGolden`, `TestWireErrors`.
- Размер `sim.wasm`: 4 638 851 байт (≈1,29 МБ gzip). Пакет `sim` не тронут (`TestDeterminismLint` зелёный); обёртка вне пакета `sim`.
### 0.2.3 — SIM-3: фаза обороны (EngineVersion 0.2.3)
- `EndTurn` запускает фазу обороны (спека §7): (1) триггеры бункера — гарнизон выходит при уроне бункеру или атакующем в `TriggerCells` (по умолчанию — наибольшая дальность гарнизона), на кольцо клеток вокруг footprint по `(Y,X)`, действует в ту же фазу; (2) проволока `DotDmg`; (3) постройки по `EntityID`: цель `(расстояние Чебышёва, hp, EntityID)`, `min_range`, классы `Vs`, сплэш, цепь `d_tesla`, перегрев `d_flame_trap` (`OverheatFireTurns`/`CoolTurns`, счёт в ходах); (4) гарнизон по `EntityID`: выстрел или один шаг по Дейкстре (§7.4) и выстрел вторым AP; (5) конец боя проверяется после каждого действия (`no_units` во время обороны обрывает фазу), затем `max_turns`.
- События для анимации: `Released`, `EffectTick` (проволока), `Attacked/Killed/Destroyed/Moved` с `Actor` обороны.
- **Additive contract-change 0.2.3:** `BuildingDef.{TriggerCells, DotDmg, OverheatFireTurns, CoolTurns, Chain, ChainCells}`; `StateHash` теперь включает `heat`, «получал урон», `disabled`; `pickTarget`/`reach` (без аллокаций) вынесены из `targetsInRange`.
- Тесты: golden G13–G20 (G01–G12 пересозданы отдельным коммитом из-за bump версии: оборона теперь стреляет), unit-тесты порядка фазы, дальности/классов, `min_range`+сплэш, цепи, проволоки, перегрева, бункера (радиус, урон, разрушенный, `disabled`, нет места), ИИ гарнизона, конца боя/`max_turns`; property на 1000 журналов с обороной (в т.ч. `pickTarget == targetsInRange[0]`), `TestDeterminismLint`, бенчмарк полной базы (150 построек, 120 гарнизона, 120 атакующих).
- Не сделано (SIM-5): заклинания, герой и аура, `Ability` (jam/EMP → `disabled`), дым/`stealth` как условие выбора цели, джаммер `d_jammer`, корабли обороны. Герой-страж вне R1 (OQ-7.6).
### 0.2.2 — SIM-2: Attack, урон, разрушение, ловушки, конец боя (EngineVersion 0.2.2)
- `Attack`: дальность/`min_range` по Чебышёву до footprint, AP, допустимые цели, целочисленный урон со множителями `vs_mult`, `Burst × wall_mult` (u_breacher), сплэш с затуханием `splash_falloff_pct` (центр — ближайшая к стрелку клетка цели; враги по возрастанию `EntityID`), мгновенное разрушение и освобождение клеток, события `Attacked/Killed/Destroyed`.
- Скрытые ловушки при `Move`: `d_mine` (взрыв + сплэш по классам из `Vs`), `d_spring_trap` (бросок лёгкого INF назад над препятствиями), прерывание пути, вскрытая ловушка (радар) — цель `Attack`; гибель перевозчика высаживает пассажиров (§5.4).
- Конец боя после каждой команды: 100% (приоритетнее `no_units`), нет юнитов атакующего, `Retreat`, `max_turns`. Звёзды HQ/50%/100%, `Destruction`, `LootBasis` (хранилища), `HeroDamageTaken`; потеря героя не входит в `UnitsLost`.
- **Additive contract-change 0.2.2:** `Rules.SplashFalloffPct`, `UnitDef.{SplashCells,Burst,WallMult,VsMult}`, `BuildingDef.{SplashCells,Trap,Burst,MaxHPFlung,FlingCells}`; `Attack` больше не `ErrNotImplemented`.
- Тесты: golden G06–G12 (G01–G05 пересозданы отдельным коммитом из-за bump версии), unit-тесты урона/сплэша/ловушек/перевозчика/окончаний, property на 1000 журналов с Attack и ловушками (Run дважды, Run = Apply-цикл, перестановка `Buildings`), `TestDeterminismLint`.
- Не сделано (SIM-3/5): фаза обороны (выстрелы построек, проволока dot, бункер, ИИ гарнизона), `Ability`, `Cast`, герой-аура; броня вне R1.
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
