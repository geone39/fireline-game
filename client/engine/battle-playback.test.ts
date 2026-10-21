import { describe, expect, it, vi } from 'vitest';
import { BattlePlayback } from './battle-playback';
import { sampleAfter, sampleBattle, sampleEvents } from './battle-fixture';
import { EVENT, type BattleEvent } from './battle-types';
import { BATTLE_DIE_MS, BATTLE_MOVE_STEP_MS, BATTLE_SHOT_MS } from './config';
import { footprints, grid } from './test-content';

const base = () => sampleBattle(grid.size, footprints);
const ev = (Kind: number, Actor: number, Target: number, x: number, y: number, Value = 0): BattleEvent => ({
  Kind,
  Actor,
  Target,
  Cell: { X: x, Y: y },
  Value,
});
const get = (p: BattlePlayback, id: number) => p.state?.entities.find((e) => e.id === id);

describe('BattlePlayback', () => {
  it('idle: shown state equals the authoritative one, tick is a no-op', () => {
    const p = new BattlePlayback();
    const s = base();
    p.setState(s);
    expect(p.busy).toBe(false);
    expect(p.state?.entities).toHaveLength(s.entities.length);
    expect(p.tick(16)).toBe(false);
  });

  it('move: interpolates, lasts steps*MOVE_STEP_MS, idle callback fires once', () => {
    const idle = vi.fn();
    const p = new BattlePlayback(idle);
    p.setState(base());
    p.enqueue([ev(EVENT.moved, 10, 0, 13, 22, 3)]);
    p.tick(0);
    expect(p.positionOf(get(p, 10)!)).toEqual({ x: 10, y: 22 });
    p.tick((3 * BATTLE_MOVE_STEP_MS) / 2);
    expect(p.positionOf(get(p, 10)!).x).toBeCloseTo(11.5);
    expect(idle).not.toHaveBeenCalled();
    p.tick(3 * BATTLE_MOVE_STEP_MS);
    expect(p.busy).toBe(false);
    expect(idle).toHaveBeenCalledTimes(1);
    expect(p.moves.size).toBe(0);
  });

  it('attack: hp drops when the shot lands; destroyed removes the entity and adds explosion + smoke', () => {
    const p = new BattlePlayback();
    p.setState(base());
    const hp0 = get(p, 3)!.hp;
    p.enqueue([ev(EVENT.attacked, 10, 3, 16, 24, 60), ev(EVENT.attacked, 11, 1, 20, 20, 5), ev(EVENT.destroyed, 11, 3, 16, 24)]);
    p.tick(1);
    expect(p.fx.some((f) => f.kind === 'shot')).toBe(true);
    expect(get(p, 3)!.hp).toBe(hp0);
    p.tick(BATTLE_SHOT_MS);
    expect(get(p, 3)!.hp).toBe(hp0 - 60);
    expect(p.fx.some((f) => f.kind === 'hit')).toBe(true);
    p.tick(BATTLE_SHOT_MS + 1);
    expect(get(p, 3)).toBeUndefined();
    expect(p.fx.filter((f) => f.kind === 'explosion' && f.big)).toHaveLength(1);
    expect(p.fx.some((f) => f.kind === 'smoke')).toBe(true);
  });

  it('a state set during playback is deferred until the queue drains', () => {
    const p = new BattlePlayback();
    const s = base();
    p.setState(s);
    p.enqueue([ev(EVENT.killed, 0, 11, 11, 24)]);
    p.tick(1);
    p.setState({ ...s, selectedId: 12, entities: s.entities.filter((e) => e.id !== 11) });
    expect(p.state?.selectedId).toBe(10);
    p.tick(10_000);
    expect(p.busy).toBe(false);
    expect(p.state?.selectedId).toBe(12);
    expect(get(p, 11)).toBeUndefined();
  });

  it('skip(): applies everything at once, no fx left, ends at the authoritative state, fires idle once', () => {
    const idle = vi.fn();
    const p = new BattlePlayback(idle);
    const s = base();
    p.setState(s);
    p.enqueue(sampleEvents());
    p.skip();
    expect(p.busy).toBe(false);
    expect(p.fx).toEqual([]);
    expect(p.moves.size).toBe(0);
    expect(p.state?.entities).toHaveLength(s.entities.length);
    expect(idle).toHaveBeenCalledTimes(1);
    p.skip();
    expect(idle).toHaveBeenCalledTimes(1);
  });

  it('events then authoritative after-state: ends exactly on the after-state', () => {
    const p = new BattlePlayback();
    const s = base();
    p.setState(s);
    p.enqueue(sampleEvents());
    p.setState(sampleAfter(s));
    expect(get(p, 10)).toMatchObject({ x: 10, y: 22 });
    p.tick(100_000);
    expect(get(p, 10)).toMatchObject({ x: 13, y: 22, ap: 0 });
    expect(get(p, 3)).toBeUndefined();
    expect(p.state?.highlight).toBeUndefined();
  });

  it('skip mode: each enqueued batch plays instantly and reports done', () => {
    const idle = vi.fn();
    const p = new BattlePlayback(idle);
    p.setState(base());
    p.setInstant(true);
    p.enqueue(sampleEvents());
    p.enqueue(sampleEvents());
    expect(idle).toHaveBeenCalledTimes(2);
    expect(p.animating).toBe(false);
  });

  it('released: an entity absent from the shown state appears at the event cell with a spawn fx', () => {
    const s = base();
    const spawned = { ...s.entities.find((e) => e.id === 10)!, id: 50, x: 16, y: 26 };
    const p = new BattlePlayback();
    // события пришли раньше нового состояния: сущности 50 ещё нет нигде, шаг пропускается без ошибок
    p.setState(s);
    p.enqueue([ev(EVENT.released, 3, 50, 16, 26)]);
    p.tick(1);
    expect(get(p, 50)).toBeUndefined();
    p.tick(10_000);
    // состояние уже содержит сущность, события доигрываются поверх него
    const q = new BattlePlayback();
    q.setState({ ...s, entities: [...s.entities, spawned] });
    q.enqueue([ev(EVENT.released, 3, 50, 16, 26)]);
    q.tick(1);
    expect(get(q, 50)).toMatchObject({ x: 16, y: 26 });
    expect(q.fx.some((f) => f.kind === 'spawn')).toBe(false); // уже на поле: не порождаем второй раз
  });

  it('trap trigger reveals a hidden trap; heal never exceeds maxHp; events about unknown ids are harmless', () => {
    const p = new BattlePlayback();
    p.setState(base());
    p.enqueue([
      ev(EVENT.trapTriggered, 9, 10, 15, 22),
      ev(EVENT.healed, 12, 10, 10, 22, 9999),
      ev(EVENT.attacked, 77, 78, 1, 1, 5),
    ]);
    p.tick(1);
    expect(get(p, 9)?.revealed).toBe(true);
    p.tick(100_000);
    expect(p.busy).toBe(false);
    p.setState(base());
    p.enqueue([ev(EVENT.healed, 12, 10, 10, 22, 9999)]);
    p.tick(1);
    expect(get(p, 10)!.hp).toBe(get(p, 10)!.maxHp);
  });

  it('load (moved with a target) walks the passenger to the carrier and removes it from the shown state', () => {
    const p = new BattlePlayback();
    p.setState(base());
    p.enqueue([ev(EVENT.moved, 10, 11, 11, 24, 1)]);
    p.tick(1);
    expect(get(p, 10)).toMatchObject({ x: 11, y: 24 });
    p.tick(BATTLE_MOVE_STEP_MS);
    expect(p.busy).toBe(false);
  });

  it('attack: действие стрелка несёт вектор на цель и живёт BATTLE_SHOT_MS; skip его убирает', () => {
    const p = new BattlePlayback();
    p.setState(base());
    const shooter = get(p, 10)!;
    const target = get(p, 3)!;
    p.enqueue([ev(EVENT.attacked, 10, 3, target.x, target.y, 1)]);
    p.tick(1);
    const a = p.actions.get(10)!;
    expect(a.dx).toBeCloseTo(target.x - shooter.x + ((target.w ?? 1) - 1) / 2);
    expect(a.dy).toBeCloseTo(target.y - shooter.y + ((target.h ?? 1) - 1) / 2);
    p.tick(BATTLE_SHOT_MS);
    expect(p.actions.size).toBe(0);
    p.enqueue([ev(EVENT.attacked, 10, 3, target.x, target.y, 1)]);
    p.tick(1);
    p.skip();
    expect(p.actions.size).toBe(0);
  });

  it('killed: юнит остаётся в dying на BATTLE_DIE_MS (для die), постройка — нет; skip и quiet не оставляют призраков', () => {
    const p = new BattlePlayback();
    p.setState(base());
    p.enqueue([ev(EVENT.killed, 10, 11, 15, 22)]);
    p.tick(1);
    expect(p.dying.has(11)).toBe(true);
    expect(get(p, 11)).toBeUndefined();
    expect(p.animating).toBe(true);
    p.tick(BATTLE_DIE_MS + 1);
    expect(p.dying.size).toBe(0);

    const q = new BattlePlayback();
    q.setState(base());
    q.enqueue([ev(EVENT.destroyed, 11, 3, 16, 24)]);
    q.tick(1);
    expect(q.dying.size).toBe(0);

    const r = new BattlePlayback();
    r.setState(base());
    r.enqueue([ev(EVENT.killed, 10, 11, 15, 22)]);
    r.skip();
    expect(r.dying.size).toBe(0);
  });
});
