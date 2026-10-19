import {
  BATTLE_END_MS,
  BATTLE_EXPLOSION_MS,
  BATTLE_HEAL_MS,
  BATTLE_HIT_MS,
  BATTLE_MOVE_STEP_MS,
  BATTLE_SHOT_MS,
  BATTLE_SMOKE_MS,
  BATTLE_SPAWN_MS,
  BATTLE_SPELL_MS,
  BATTLE_TICK_MS,
  BATTLE_TRAP_MS,
} from './config';
import { EVENT, footprintSize, type BattleEntity, type BattleEvent, type BattleWorldState } from './battle-types';

export type FxKind = 'shot' | 'hit' | 'explosion' | 'smoke' | 'spawn' | 'spell' | 'tick' | 'heal' | 'trap';

/** Кратковременный эффект. x,y — клетки (дробные допустимы); у 'shot' from* — клетка стрелка. */
export interface Fx {
  kind: FxKind;
  x: number;
  y: number;
  fromX?: number;
  fromY?: number;
  /** Крупный взрыв (разрушение постройки) против малого (гибель юнита). */
  big?: boolean;
  age: number;
  duration: number;
}

export interface MoveAnim {
  fromX: number;
  fromY: number;
  toX: number;
  toY: number;
  age: number;
  duration: number;
}

interface Step {
  total: number;
  age: number;
  /** Действие в конце шага (урон от выстрела засчитывается, когда «долетел» снаряд). */
  end?: () => void;
}

/**
 * Очередь событий ядра sim и их воспроизведение. Без Pixi и DOM: тестируется в node.
 *
 * Модель: `setState(s)` — авторитетное состояние «после» (то, что даёт core). `enqueue(events)` проигрывает события
 * поверх показанного состояния: двигает/удаляет/ранит сущности и порождает эффекты. Когда очередь пуста, показанное
 * состояние снова равно последнему `setState` (анимация не может разойтись с ядром). Если `setState` пришёл во время
 * воспроизведения, он откладывается до конца очереди. Обратный порядок (сначала setState, потом enqueue) тоже
 * допустим: события отработают по уже обновлённому состоянию, недостающие сущности пропускаются.
 */
export class BattlePlayback {
  private target: BattleWorldState | null = null;
  /** Авторитетное состояние, принятое к показу (метаданные: выбор, подсветка); сущности — из display. */
  private shown: BattleWorldState | null = null;
  private display = new Map<number, BattleEntity>();
  private queue: BattleEvent[] = [];
  private current: Step | null = null;
  private instant = false;
  readonly moves = new Map<number, MoveAnim>();
  fx: Fx[] = [];

  constructor(private readonly onIdle: () => void = () => undefined) {}

  /** Идёт ли воспроизведение событий (эффекты-«хвосты» не считаются). */
  get busy(): boolean {
    return this.current !== null || this.queue.length > 0;
  }

  /** Есть что перерисовывать каждый кадр. */
  get animating(): boolean {
    return this.busy || this.fx.length > 0 || this.moves.size > 0;
  }

  /** Режим «пропустить»: события применяются мгновенно, эффекты не создаются. */
  setInstant(on: boolean): void {
    this.instant = on;
    if (on) this.skip();
  }

  /** Показываемое состояние (сущности — с учётом проигранных событий). */
  get state(): BattleWorldState | null {
    if (!this.shown) return null;
    return { ...this.shown, entities: [...this.display.values()] };
  }

  /** Авторитетное состояние «после». Откладывается, пока проигрываются события. */
  setState(state: BattleWorldState): void {
    this.target = state;
    if (!this.busy) this.adopt();
  }

  enqueue(events: readonly BattleEvent[]): void {
    if (events.length === 0) return;
    for (const e of events) this.queue.push(e);
    if (this.instant) this.skip();
  }

  /** Мгновенно дорабатывает всё, что осталось, и переходит к авторитетному состоянию. */
  skip(): void {
    const wasBusy = this.busy;
    if (this.current) this.current.end?.();
    this.current = null;
    while (this.queue.length > 0) {
      const ev = this.queue.shift() as BattleEvent;
      this.begin(ev, true).end?.();
    }
    this.moves.clear();
    this.fx = [];
    this.adopt();
    if (wasBusy) this.onIdle();
  }

  /** Продвигает время. Возвращает true, если нужна перерисовка. */
  tick(dtMs: number): boolean {
    if (!this.animating) return false;
    for (const f of this.fx) f.age += dtMs;
    this.fx = this.fx.filter((f) => f.age < f.duration);
    for (const [id, m] of this.moves) {
      m.age += dtMs;
      if (m.age >= m.duration) this.moves.delete(id);
    }
    const wasBusy = this.busy;
    let left = dtMs;
    while (this.busy) {
      if (!this.current) {
        const ev = this.queue.shift() as BattleEvent;
        this.current = this.begin(ev, false);
      }
      const need = this.current.total - this.current.age;
      if (left < need) {
        this.current.age += left;
        break;
      }
      left -= need;
      this.current.end?.();
      this.current = null;
    }
    if (wasBusy && !this.busy) {
      this.adopt();
      this.onIdle();
    }
    return true;
  }

  /** Позиция для отрисовки (клетки, дробная при движении). */
  positionOf(e: BattleEntity): { x: number; y: number } {
    const m = this.moves.get(e.id);
    if (!m) return { x: e.x, y: e.y };
    const t = m.duration > 0 ? Math.min(1, m.age / m.duration) : 1;
    return { x: m.fromX + (m.toX - m.fromX) * t, y: m.fromY + (m.toY - m.fromY) * t };
  }

  private adopt(): void {
    this.shown = this.target;
    this.display = new Map();
    for (const e of this.target?.entities ?? []) this.display.set(e.id, { ...e });
  }

  private addFx(fx: Omit<Fx, 'age'>, quiet: boolean): void {
    if (!quiet) this.fx.push({ ...fx, age: 0 });
  }

  /** Вставляет сущность из авторитетного состояния, если её ещё нет в показанном (юнит высажен/порождён). */
  private reveal(id: number, x: number, y: number): BattleEntity | undefined {
    const have = this.display.get(id);
    if (have) return have;
    const src = this.target?.entities.find((e) => e.id === id);
    if (!src) return undefined;
    const e = { ...src, x, y };
    this.display.set(id, e);
    return e;
  }

  private begin(ev: BattleEvent, quiet: boolean): Step {
    const cx = ev.Cell.X;
    const cy = ev.Cell.Y;
    const step: Step = { total: 0, age: 0 };
    const at = (kind: FxKind, extra: Partial<Fx>, duration: number): void =>
      this.addFx({ kind, x: cx, y: cy, duration, ...extra }, quiet);
    switch (ev.Kind) {
      case EVENT.moved: {
        const unit = this.display.get(ev.Actor);
        if (!unit) {
          // Высадка (фаза 0): юнита ещё не было на поле.
          if (this.reveal(ev.Actor, cx, cy)) {
            at('spawn', {}, BATTLE_SPAWN_MS);
            step.total = BATTLE_SPAWN_MS;
          }
          break;
        }
        step.total = Math.max(1, ev.Value) * BATTLE_MOVE_STEP_MS;
        const from = this.positionOf(unit);
        if (!quiet) {
          this.moves.set(unit.id, { fromX: from.x, fromY: from.y, toX: cx, toY: cy, age: 0, duration: step.total });
        }
        unit.x = cx;
        unit.y = cy;
        if (ev.Target !== 0) {
          // Погрузка: пассажир заходит в перевозчика и исчезает с поля.
          step.end = () => this.display.delete(unit.id);
        }
        break;
      }
      case EVENT.attacked: {
        const shooter = this.display.get(ev.Actor);
        const from = shooter ? this.centre(shooter) : { x: cx, y: cy };
        at('shot', { fromX: from.x, fromY: from.y }, BATTLE_SHOT_MS);
        step.total = BATTLE_SHOT_MS;
        step.end = () => {
          const t = this.display.get(ev.Target);
          if (t) t.hp = Math.max(0, t.hp - ev.Value);
          at('hit', {}, BATTLE_HIT_MS);
        };
        break;
      }
      case EVENT.killed:
      case EVENT.destroyed: {
        const big = ev.Kind === EVENT.destroyed;
        const gone = this.display.get(ev.Target);
        const c = gone ? this.centre(gone) : { x: cx, y: cy };
        this.display.delete(ev.Target);
        this.moves.delete(ev.Target);
        this.addFx({ kind: 'explosion', x: c.x, y: c.y, big, duration: BATTLE_EXPLOSION_MS }, quiet);
        this.addFx({ kind: 'smoke', x: c.x, y: c.y, big, duration: BATTLE_SMOKE_MS }, quiet);
        step.total = BATTLE_EXPLOSION_MS;
        break;
      }
      case EVENT.trapTriggered: {
        const trap = this.display.get(ev.Actor);
        if (trap) trap.revealed = true;
        at('trap', {}, BATTLE_TRAP_MS);
        step.total = BATTLE_TRAP_MS;
        break;
      }
      case EVENT.spellCast:
        at('spell', {}, BATTLE_SPELL_MS);
        step.total = BATTLE_SPELL_MS;
        break;
      case EVENT.effectTick: {
        const t = this.display.get(ev.Target);
        if (t) t.hp = Math.max(0, t.hp - ev.Value);
        at('tick', {}, BATTLE_TICK_MS);
        step.total = BATTLE_TICK_MS;
        break;
      }
      case EVENT.healed: {
        const t = this.display.get(ev.Target);
        if (t) t.hp = Math.min(t.maxHp, t.hp + ev.Value);
        at('heal', {}, BATTLE_HEAL_MS);
        step.total = BATTLE_HEAL_MS;
        break;
      }
      case EVENT.released: {
        if (!this.display.has(ev.Target) && this.reveal(ev.Target, cx, cy)) {
          at('spawn', {}, BATTLE_SPAWN_MS);
          step.total = BATTLE_SPAWN_MS;
        }
        break;
      }
      case EVENT.battleEnded:
        step.total = BATTLE_END_MS;
        break;
      default:
        break; // turnEnded и неизвестные виды: без анимации
    }
    return step;
  }

  private centre(e: BattleEntity): { x: number; y: number } {
    const p = this.positionOf(e);
    const { w, h } = footprintSize(e);
    return { x: p.x + (w - 1) / 2, y: p.y + (h - 1) / 2 };
  }
}
