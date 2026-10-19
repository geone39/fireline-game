import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import type { Command, Event, Result, Snapshot } from '../../sim-wasm/api';
import { createSim, type Sim, type SimBattle } from '../../sim-wasm/sim';
import { loadFirelineSim } from '../../sim-wasm/loader';
import { nodeSources } from '../../sim-wasm/node';
import { existsSync } from 'node:fs';
import { createBattle } from './controller';
import { actionOn, deployCells, moveCells, pathTo, targetsOf } from './highlight';
import { applyEvents, clientView, createWorld, rulesOf } from './world';
import { toRenderState } from './view';

const here = dirname(fileURLToPath(import.meta.url));
const replay = (n: string): { snapshot: Snapshot } =>
  JSON.parse(readFileSync(resolve(here, `../../../sim/testdata/replays/${n}.json`), 'utf8')) as { snapshot: Snapshot };
const g06 = replay('G06').snapshot;

describe('clientView (OQ-7.8)', () => {
  it('скрытые ловушки не попадают в клиентский Snapshot, вскрытые и обычные — остаются', () => {
    const snap = replay('G12').snapshot;
    const rules = rulesOf(snap);
    const hidden = new Set(rules.Buildings.filter((b) => b.Hidden).map((b) => b.ID));
    expect(snap.Buildings?.some((b) => hidden.has(b.ID))).toBe(true);
    const v = clientView(snap);
    expect(v.Buildings?.some((b) => hidden.has(b.ID) && !b.Revealed)).toBe(false);
    expect(v.Buildings?.length).toBeLessThan(snap.Buildings?.length ?? 0);
    expect(v.Buildings?.some((b) => b.ID === 'b_hq')).toBe(true);
  });
});

describe('мир и подсветка (без ядра)', () => {
  const w0 = createWorld(g06);
  it('ids построек в каноническом порядке (Y, X): гнездо 1, штаб 2', () => {
    expect(w0.entities.map((e) => e.contentId)).toEqual(['d_mg_nest', 'b_hq']);
    expect(w0.entities[0]).toMatchObject({ id: 1, x: 12, y: 10, hp: w0.entities[0]!.maxHp });
  });

  it('высадка: события Moved создают юнита, ids продолжают нумерацию', () => {
    const ev: Event = { Kind: 1, Actor: 3, Target: 0, Cell: { X: 1, Y: 10 }, Value: 0 };
    const w = applyEvents(w0, [ev], { command: { op: 'deploy', id: 'u_rifleman', level: 1 } });
    expect(w.entities[2]).toMatchObject({ id: 3, owner: 'attacker', contentId: 'u_rifleman', x: 1, y: 10 });
    expect(w0.entities).toHaveLength(2); // чистая функция
  });

  it('клетки высадки — вне зоны базы, не заняты; ходы — не дальше move_cells, без построек', () => {
    const rules = rulesOf(g06);
    const cells = deployCells(w0, 'u_rifleman');
    expect(cells.length).toBeGreaterThan(0);
    const z = rules.BuildZone;
    expect(cells.every((c) => !(c.x >= z.X && c.x < z.X + z.W && c.y >= z.Y && c.y < z.Y + z.H))).toBe(true);

    const w = applyEvents(w0, [{ Kind: 1, Actor: 3, Target: 0, Cell: { X: 10, Y: 10 }, Value: 0 }], { command: { op: 'deploy', id: 'u_rifleman', level: 1 } });
    const u = w.entities[2]!;
    const mv = moveCells(w, u);
    const reach = u.unit!.MoveCells;
    expect(mv.length).toBeGreaterThan(0);
    expect(mv.every((c) => Math.max(Math.abs(c.x - u.x), Math.abs(c.y - u.y)) <= reach)).toBe(true);
    expect(mv.some((c) => c.x >= 12 && c.x <= 13 && c.y >= 10 && c.y <= 11)).toBe(false); // гнездо непроходимо
    const p = pathTo(w, u, 9, 10);
    expect(p).toEqual([{ X: 9, Y: 10 }]);
    expect(pathTo(w, u, 30, 30)).toBeNull();
  });

  it('цели: гнездо в дальности стрелка, штаб — нет; heal/attack по правилам юнита', () => {
    const put = (x: number) =>
      applyEvents(w0, [{ Kind: 1, Actor: 3, Target: 0, Cell: { X: x, Y: 10 }, Value: 0 }], { command: { op: 'deploy', id: 'u_rifleman', level: 1 } });
    const far = put(1);
    expect(targetsOf(far, far.entities[2]!)).toEqual([]);
    const near = put(9);
    expect(targetsOf(near, near.entities[2]!)).toEqual([1]);
    expect(actionOn(near.entities[2]!, near.entities[0]!)).toEqual({ op: 'attack' });
    expect(actionOn(near.entities[2]!, near.entities[2]!)).toBeNull();
  });

  it('toRenderState: уничтоженные и скрытые на борту/в бункере не рисуются, ap только у своих', () => {
    const w = applyEvents(w0, [{ Kind: 1, Actor: 3, Target: 0, Cell: { X: 1, Y: 10 }, Value: 0 }, { Kind: 4, Actor: 3, Target: 1, Cell: { X: 12, Y: 10 }, Value: 0 }], {
      command: { op: 'deploy', id: 'u_rifleman', level: 1 },
    });
    const s = toRenderState(w, { selectedId: null, armedDeploy: null });
    expect(s.entities.map((e) => e.id)).toEqual([2, 3]);
    expect(s.entities.find((e) => e.id === 3)).toMatchObject({ side: 'attacker', kind: 'troop', ap: 2 });
    expect(s.entities.find((e) => e.id === 2)?.ap).toBeUndefined();
  });
});

const dist = resolve(here, '../../public/sim');
const haveWasm = existsSync(resolve(dist, 'sim.wasm')) && existsSync(resolve(dist, 'wasm_exec.js'));

describe.skipIf(!haveWasm)('контроллер боя на реальном sim.wasm (G06)', () => {
  const sim = async (): Promise<Sim> => createSim(await loadFirelineSim(nodeSources(dist)));

  it('от высадки до отступления: стор, AP, отказ ядра, итог', async () => {
    const b = createBattle({ sim: await sim(), snapshot: g06, seed: '6' });
    const st = () => b.store.getState();
    b.armDeploy('u_rifleman', 1);
    expect(b.renderState().highlight?.moves?.length).toBeGreaterThan(0);
    b.cell(1, 10);
    expect(st().armed).toBeNull();
    expect(st().stock.units.find((s) => s.ID === 'u_rifleman')?.Count).toBe(4);
    expect(st().step?.events[0]).toMatchObject({ Kind: 1, Actor: 3 });
    b.armDeploy('u_brawler', 1);
    b.cell(1, 12);
    b.start();
    expect(st().world).toMatchObject({ phase: 'player', turn: 1 });

    // ход стрелка
    b.select(3);
    const moves = b.renderState().highlight?.moves ?? [];
    expect(moves.some((m) => m.x === 4 && m.y === 10)).toBe(true);
    b.cell(4, 10);
    expect(st().world.entities[2]).toMatchObject({ x: 4, y: 10, ap: 1 });
    expect(st().step?.seq).toBeGreaterThan(0);
    expect(st().log.at(-1)).toEqual({ command: expect.objectContaining({ op: 'move', unit: 3 }) as Command, rejected: null });

    // нехватка дальности -> отказ ядра out_of_range (11)
    b.select(3);
    b.target(1);
    expect(st().lastReject?.code).toBe(11);
    expect(st().world.entities[2]?.ap).toBe(1);
    expect(st().log.at(-1)?.rejected).toBe(11);

    // конец хода: событие TurnEnded -> ход 2, ap восстановлен
    b.endTurn();
    expect(st().world.turn).toBe(2);
    expect(st().world.entities[2]?.ap).toBe(2);

    b.retreat();
    expect(st().world.phase).toBe('over');
    const r = st().result as Result;
    expect(r.EndReason).toBe(4);
    expect(r.Stars).toBe(0);
    b.dispose();
  }, 60_000);

  it('играется до разрушения: цель в дальности, атака уменьшает hp гнезда на сумму событий', async () => {
    const b = createBattle({ sim: await sim(), snapshot: g06, seed: '6' });
    const st = () => b.store.getState();
    b.armDeploy('u_rifleman', 1);
    b.cell(1, 10);
    b.start();
    for (const x of [4, 7, 10]) {
      b.select(3);
      b.cell(x, 10);
      b.endTurn();
      expect(st().lastReject).toBeNull();
    }
    b.select(3);
    expect(b.renderState().highlight?.targets).toContain(1);
    b.target(1);
    const ev = st().step!.events.filter((e) => e.Kind === 2 && e.Target === 1);
    expect(ev.length).toBe(1);
    const nest = st().world.entities[0]!;
    expect(nest.hp).toBe(nest.maxHp - ev[0]!.Value);
    expect(st().step!.before.entities[0]!.hp).toBe(nest.maxHp);
    b.dispose();
  }, 60_000);
});

describe('контроллер: ошибки и конец боя (мок ядра)', () => {
  it('после конца боя команды не отправляются', () => {
    const calls: Command[] = [];
    const battle: SimBattle = {
      apply(c) {
        calls.push(c);
        return { events: [{ Kind: 11, Actor: 0, Target: 0, Cell: { X: 0, Y: 0 }, Value: 4 }], rejected: null };
      },
      result: () => ({ EndReason: 4, Stars: 0 }) as Result,
      free: () => undefined,
    };
    const sim: Sim = { engineVersion: 'x', run: () => ({}) as Result, newBattle: () => battle };
    const b = createBattle({ sim, snapshot: g06, seed: 1 });
    b.retreat();
    b.endTurn();
    b.start();
    expect(calls).toHaveLength(1);
    expect(b.store.getState().result?.EndReason).toBe(4);
  });
});
