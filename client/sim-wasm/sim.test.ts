import { spawnSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { FirelineSim, Result, Snapshot } from './api';
import { loadFirelineSim, resetSimLoader, type SimSources } from './loader';
import { nodeSources } from './node';
import { createSim, SimError } from './sim';

const ok = (result: unknown): string => JSON.stringify({ ok: true, result });
const emptyResult = (rejected: { Index: number; Code: number }[] | null): Result =>
  ({ Stars: 0, Destruction: 0, Rejected: rejected, EngineVersion: '9', StateHash: '00' }) as unknown as Result;

function mockApi(rejectIdx: number[] = []): FirelineSim {
  let n = 0;
  return {
    engineVersion: '9.9.9',
    run: () => ok(emptyResult(null)),
    newBattle: () => ok({ handle: 1 }),
    apply: () => {
      n++;
      return ok({ events: n === 1 ? null : [{ Kind: 1, Actor: 1, Target: 0, Cell: { X: 1, Y: 2 }, Value: 0 }] });
    },
    result: () => ok(emptyResult(rejectIdx.map((Index) => ({ Index, Code: 5 })))),
    free: () => ok({ freed: true }),
  };
}

const snap = { Rules: {} } as unknown as Snapshot;

describe('createSim (мок wasm)', () => {
  it('apply: события и принятая команда', () => {
    const b = createSim(mockApi()).newBattle(snap, '7');
    expect(b.apply({ turn: 0, op: 'start' })).toEqual({ events: [], rejected: null });
    expect(b.apply({ turn: 1, op: 'end_turn' }).events).toHaveLength(1);
  });

  it('apply: отказ ядра — код, не исключение; индекс команды считается по журналу', () => {
    const b = createSim(mockApi([1])).newBattle(snap, 1);
    expect(b.apply({ turn: 1, op: 'end_turn' }).rejected).toBeNull();
    expect(b.apply({ turn: 1, op: 'move', unit: 3 }).rejected).toBe(5);
  });

  it('ошибка ядра -> SimError; после free вызовы бросают', () => {
    const api = { ...mockApi(), newBattle: () => JSON.stringify({ ok: false, error: 'bad snapshot' }) };
    expect(() => createSim(api).newBattle(snap, 1)).toThrow(SimError);
    const b = createSim(mockApi()).newBattle(snap, 1);
    b.free();
    b.free();
    expect(() => b.result()).toThrow(/освобождён/);
  });

  it('engineVersion пробрасывается, snapshot сериализуется в JSON', () => {
    const api = mockApi();
    const spy = vi.spyOn(api, 'newBattle');
    const sim = createSim(api);
    sim.newBattle(snap, 'seed');
    expect(sim.engineVersion).toBe('9.9.9');
    expect(spy).toHaveBeenCalledWith(JSON.stringify(snap), 'seed');
  });
});

describe('loadFirelineSim (мок источников)', () => {
  afterEach(() => resetSimLoader());

  it('ждёт fireline_sim_ready и кэширует', async () => {
    const api = mockApi();
    class FakeGo {
      importObject = {};
      run = async (): Promise<void> => {
        globalThis.fireline_sim = api;
        globalThis.fireline_sim_ready?.();
        return new Promise<void>(() => undefined);
      };
    }
    const src: SimSources = {
      loadRuntime: async () => void ((globalThis as unknown as { Go: unknown }).Go = FakeGo),
      instantiate: async () => ({}) as WebAssembly.Instance,
    };
    const spy = vi.spyOn(src, 'loadRuntime');
    expect(await loadFirelineSim(src)).toBe(api);
    expect(await loadFirelineSim(src)).toBe(api);
    expect(spy).toHaveBeenCalledTimes(1);
  });

  it('ошибка загрузки не кэшируется', async () => {
    const bad: SimSources = { loadRuntime: async () => Promise.reject(new Error('нет сети')), instantiate: async () => ({}) as WebAssembly.Instance };
    await expect(loadFirelineSim(bad)).rejects.toThrow('нет сети');
    await expect(loadFirelineSim(bad)).rejects.toThrow('нет сети');
  });
});

// Реальный sim.wasm: собирается `npm run sim:build` (нужен Go); если артефакта нет и Go недоступен — тест пропускается.
const here = dirname(fileURLToPath(import.meta.url));
const dist = resolve(here, '../public/sim');
function ensureBuilt(): boolean {
  if (existsSync(resolve(dist, 'sim.wasm')) && existsSync(resolve(dist, 'wasm_exec.js'))) return true;
  return spawnSync(process.execPath, [resolve(here, 'build.mjs')], { stdio: 'ignore' }).status === 0;
}

describe('реальный sim.wasm', () => {
  const built = ensureBuilt();
  it.skipIf(!built)('golden G06: пошагово == полный run (StateHash), отказ приходит кодом', async () => {
    const g = JSON.parse(readFileSync(resolve(here, '../../sim/testdata/replays/G06.json'), 'utf8')) as {
      snapshot: Snapshot;
      commands: import('./api').Command[];
      seed: number;
      expect: { result: { StateHash?: unknown } };
    };
    const sim = createSim(await loadFirelineSim(nodeSources(dist)));
    expect(sim.engineVersion).toMatch(/^\d+\.\d+\.\d+$/);
    const full = sim.run(g.snapshot, g.commands, String(g.seed));
    const b = sim.newBattle(g.snapshot, String(g.seed));
    for (const c of g.commands) expect(b.apply(c).rejected).toBeNull();
    const stepped = b.result();
    expect(stepped.StateHash).toBe(full.StateHash);
    expect(stepped.Stars).toBe(full.Stars);
    expect(b.apply({ turn: 4, op: 'end_turn' }).rejected).toBe(17); // battle_over
    b.free();
  }, 120_000);
});
