// Контроллер боя: стор (Snapshot, Rules, выбор юнита, журнал команд, состояние «до/после», события, итог) и
// действия игрока. Команды применяются к ядру sim (WASM) синхронно; допустимость решает ядро — клиент лишь
// показывает код отказа (RejectCode) и не меняет состояние. Сервер потом пересчитает бой по тому же журналу (D-05).
import type { ArmySlot, Command, Event, Result, Snapshot } from '../../sim-wasm/api';
import type { Sim, SimBattle } from '../../sim-wasm/sim';
import { createStore, type Store } from '../store';
import { actionOn, pathTo } from './highlight';
import { applyEvents, clientView, createWorld, rulesOf, type World } from './world';
import { toRenderState, type UiSelection } from './view';

export type Armed =
  | { kind: 'deploy'; id: string; level: number }
  | { kind: 'spell'; id: string; level: number }
  | { kind: 'hero' };

export interface LoggedCommand {
  command: Command;
  /** Код отказа ядра, null — принята. Отклонённые тоже попадают в журнал (как в ядре: Rejection.Index). */
  rejected: number | null;
}

export interface BattleStep {
  /** Растёт с каждой принятой командой: UI проигрывает `events` и затем показывает `after`. */
  seq: number;
  before: World;
  events: Event[];
  after: World;
  command: Command;
}

export interface Stock {
  units: ArmySlot[];
  spells: ArmySlot[];
  hero: { id: string; level: number; used: boolean } | null;
}

export interface BattleState {
  world: World;
  selectedId: number | null;
  armed: Armed | null;
  stock: Stock;
  log: LoggedCommand[];
  lastReject: { code: number; command: Command } | null;
  step: BattleStep | null;
  /** Итог — когда бой окончен (EndReason ≠ 0). */
  result: Result | null;
}

export interface BattleOptions {
  sim: Sim;
  /** Snapshot с Rules; скрытые ловушки отфильтровываются (clientView, OQ-7.8). */
  snapshot: Snapshot;
  seed: string | number;
}

export interface Battle {
  readonly store: Store<BattleState>;
  /** Снимок для рендера: мир + подсветка по текущему выбору. */
  renderState(): ReturnType<typeof toRenderState>;
  armDeploy(id: string, level: number): void;
  armSpell(id: string, level: number): void;
  /** Способность выбранного героя: без цели (зона/скрытность) — сразу, с клеткой (приманка) — после тапа по клетке. */
  useHero(): void;
  unload(): void;
  /** Тап по своему юниту. */
  select(id: number): void;
  /** Тап по сущности (оборона либо своя цель лечения). */
  target(id: number): void;
  /** Тап по клетке: высадка / ход / заклинание / приманка либо снятие выбора. */
  cell(x: number, y: number): void;
  start(): void;
  endTurn(): void;
  retreat(): void;
  dispose(): void;
}

function initialStock(s: Snapshot): Stock {
  return {
    units: (s.Army ?? []).map((a) => ({ ...a })),
    spells: (s.Spells ?? []).map((a) => ({ ...a })),
    hero: s.Hero ? { id: s.Hero, level: s.HeroLevel, used: false } : null,
  };
}

export function createBattle(o: BattleOptions): Battle {
  const snapshot = clientView(o.snapshot);
  const rules = rulesOf(snapshot);
  const sim: SimBattle = o.sim.newBattle(snapshot, o.seed);
  const store = createStore<BattleState>({
    world: createWorld(snapshot), selectedId: null, armed: null, stock: initialStock(snapshot), log: [], lastReject: null,
    step: null, result: null,
  });
  let seq = 0;

  const get = (): BattleState => store.getState();

  function dispatch(command: Command): boolean {
    const s = get();
    if (s.world.phase === 'over') return false;
    const out = sim.apply(command);
    const log = [...s.log, { command, rejected: out.rejected }];
    if (out.rejected !== null) {
      store.setState({ log, lastReject: { code: out.rejected, command } });
      return false;
    }
    let world = applyEvents(s.world, out.events, { command });
    const stock = structuredClone(s.stock);
    if (command.op === 'start') world = { ...world, phase: 'player', turn: 1 };
    else if (command.op === 'deploy') spend(stock, command, rules);
    else if (command.op === 'cast') {
      const sl = stock.spells.find((x) => x.ID === command.id && x.Level === command.level);
      if (sl) sl.Count--;
    }
    const actor = command.unit ? world.entities[command.unit - 1] : undefined;
    if (actor && (command.op === 'move' || command.op === 'attack' || command.op === 'ability')) {
      world = { ...world, entities: world.entities.map((e) => (e === actor ? { ...e, ap: Math.max(0, e.ap - 1) } : e)) };
    }
    const over = world.phase === 'over';
    const keep = s.selectedId && world.entities[s.selectedId - 1]?.alive ? s.selectedId : null;
    store.setState({
      world, stock, log, lastReject: null, armed: null, selectedId: over ? null : keep,
      step: { seq: ++seq, before: s.world, events: out.events, after: world, command },
      result: over ? sim.result() : null,
    });
    return true;
  }

  const turn = (): number => get().world.turn;
  const selected = () => {
    const id = get().selectedId;
    return id ? get().world.entities[id - 1] : undefined;
  };

  return {
    store,
    renderState() {
      const s = get();
      const ui: UiSelection = { selectedId: s.selectedId, armedDeploy: s.armed?.kind === 'deploy' ? s.armed.id : null };
      return toRenderState(s.world, ui);
    },
    armDeploy: (id, level) => store.setState({ armed: { kind: 'deploy', id, level }, selectedId: null }),
    armSpell: (id, level) => store.setState({ armed: { kind: 'spell', id, level } }),
    useHero() {
      const u = selected();
      const a = u?.unit?.Ability;
      if (!u || !a || a.Kind === 0) return;
      // spawn_decoy (Kind 2) ставится на клетку, остальные способности действуют вокруг героя
      if (a.Kind === 2) store.setState({ armed: { kind: 'hero' } });
      else dispatch({ turn: turn(), op: 'ability', unit: u.id, id: 'hero', to: { X: u.x, Y: u.y } });
    },
    unload() {
      const u = selected();
      if (u && (u.unit?.Carries ?? 0) > 0) dispatch({ turn: turn(), op: 'ability', unit: u.id, id: 'unload' });
    },
    select(id) {
      const e = get().world.entities[id - 1];
      if (get().world.phase !== 'player' || !e || e.owner !== 'attacker' || !e.alive || !e.onField) return;
      store.setState({ selectedId: id, armed: null });
    },
    target(id) {
      const u = selected();
      const t = get().world.entities[id - 1];
      if (!u || !t || get().world.phase !== 'player') return;
      const a = actionOn(u, t);
      if (!a) return;
      if (a.op === 'attack') dispatch({ turn: turn(), op: 'attack', unit: u.id, target: t.id });
      else dispatch({ turn: turn(), op: 'ability', unit: u.id, id: a.id, target: t.id });
    },
    cell(x, y) {
      const s = get();
      const armed = s.armed;
      if (armed?.kind === 'deploy') {
        dispatch({ turn: 0, op: 'deploy', id: armed.id, level: armed.level, to: { X: x, Y: y } });
        return;
      }
      if (armed?.kind === 'spell') {
        dispatch({ turn: turn(), op: 'cast', id: armed.id, level: armed.level, to: { X: x, Y: y } });
        return;
      }
      const u = selected();
      if (armed?.kind === 'hero' && u) {
        dispatch({ turn: turn(), op: 'ability', unit: u.id, id: 'hero', to: { X: x, Y: y } });
        return;
      }
      if (!u || s.world.phase !== 'player') {
        store.setState({ selectedId: null });
        return;
      }
      const path = pathTo(s.world, u, x, y);
      if (path && path.length > 0) dispatch({ turn: turn(), op: 'move', unit: u.id, path });
      else store.setState({ selectedId: null });
    },
    start: () => void dispatch({ turn: 0, op: 'start' }),
    endTurn: () => void dispatch({ turn: turn(), op: 'end_turn' }),
    retreat: () => void dispatch({ turn: turn(), op: 'retreat' }),
    dispose: () => sim.free(),
  };
}

function spend(stock: Stock, c: Command, rules: ReturnType<typeof rulesOf>): void {
  if (stock.hero && c.id === stock.hero.id) {
    stock.hero.used = true;
    return;
  }
  const slot = stock.units.find((x) => x.ID === c.id && x.Level === c.level);
  if (slot) slot.Count -= Math.max(1, rules.Units.find((u) => u.ID === c.id)?.Group ?? 1);
}
