import { describe, expect, it } from 'vitest';
import { ApiClient, ApiError, memoryTokenStore } from '../api';
import { createBaseApi } from './api';
import { createBaseModel } from './model';
import { toWorld } from './types';
import { baseState, catalog, item, mockApi, pricing, startItems } from './test-kit';

function setup() {
  const initial = baseState(startItems());
  const api = mockApi(initial);
  let n = 0;
  const model = createBaseModel({ api, catalog, pricing, newUid: () => `n${++n}`, newKey: () => `k${n}`, now: () => Date.parse('2026-01-01T00:00:00Z') });
  model.store.reset(toWorld(initial));
  return { api, model, initial };
}

describe('base model: оптимистичное применение', () => {
  it('build сразу виден во view, confirmed не меняется до ответа сервера', async () => {
    const { api, model } = setup();
    let release!: (s: ReturnType<typeof baseState>) => void;
    api.onSend = () => new Promise((r) => (release = r));
    const p = model.commands.build('b_vault', 30, 30);
    const s = model.store.getState();
    expect(s.view.items.find((i) => i.uid === 'n1')).toMatchObject({ type: 'b_vault', x: 30, y: 30, level: 1 });
    expect(s.confirmed.items.some((i) => i.uid === 'n1')).toBe(false);
    expect(s.pending).toHaveLength(1);
    // локальный таймер стройки из building_costs.yaml и занятый билдер
    expect(s.view.timers[0]).toMatchObject({ kind: 'build', target_id: 'n1', id: 'local:n1' });
    expect(s.view.builders.busy).toBe(1);
    release(baseState([...startItems(), item('n1', 'b_vault', 30, 30)], { timers: [{ id: 't1', kind: 'build', target_id: 'n1', finish_at: '2026-01-01T02:00:00Z', to_level: 1 }], builders: { total: 2, busy: 1 }, version: 2 }));
    await p;
    const after = model.store.getState();
    expect(after.pending).toHaveLength(0);
    expect(after.confirmed.version).toBe(2);
    expect(after.view.timers[0]?.id).toBe('t1'); // авторитетный таймер заменил локальный
  });

  it('отправляет пакет с Idempotency-Key и полями команды в args', async () => {
    const { api, model } = setup();
    await model.commands.move('cm1', 14, 24, 0);
    expect(api.calls.sent).toHaveLength(1);
    expect(api.calls.sent[0]).toEqual({ key: 'k0', commands: [{ op: 'move', args: { uid: 'cm1', x: 14, y: 24, rot: 0 } }] });
  });

  it('отказ сервера откатывает команду и сохраняет причину', async () => {
    const { api, model } = setup();
    api.onSend = () => Promise.reject(new ApiError('insufficient_resources', 422, 'no cash'));
    await model.commands.build('b_vault', 30, 30);
    const s = model.store.getState();
    expect(s.view.items.some((i) => i.type === 'b_vault')).toBe(false);
    expect(s.view.builders.busy).toBe(0);
    expect(s.pending).toHaveLength(0);
    expect(s.lastRejection?.code).toBe('insufficient_resources');
  });

  it('при сетевой ошибке команды остаются в очереди, view сохраняет оптимистичное состояние', async () => {
    const { api, model } = setup();
    api.onSend = () => Promise.reject(new ApiError('network', 0, 'down'));
    await model.commands.remove('cm1');
    const s = model.store.getState();
    expect(s.offline).toBe(true);
    expect(s.pending).toHaveLength(1);
    expect(s.view.items.some((i) => i.uid === 'cm1')).toBe(false);
  });

  it('upgrade несёт целевой уровень (to), speedup завершает апгрейд оптимистично', async () => {
    const { api, model } = setup();
    api.onSend = () => new Promise(() => undefined);
    void model.commands.upgrade('cm1');
    const q = model.store.getState().pending[0];
    expect(q).toMatchObject({ op: 'upgrade', args: { uid: 'cm1', to: 2 } });
    expect(model.store.getState().view.timers[0]).toMatchObject({ kind: 'upgrade', to_level: 2 });
  });

  it('speedup: уровень растёт, таймер и билдер освобождаются', () => {
    const { model } = setup();
    model.store.reset(
      toWorld(baseState(startItems(), { timers: [{ id: 't', kind: 'upgrade', target_id: 'cm1', finish_at: '2026-01-01T05:00:00Z', to_level: 2 }], builders: { total: 2, busy: 1 } })),
    );
    model.store.enqueue({ op: 'speedup', args: { uid: 'cm1' } });
    const v = model.store.getState().view;
    expect(v.items.find((i) => i.uid === 'cm1')?.level).toBe(2);
    expect(v.timers).toHaveLength(0);
    expect(v.builders.busy).toBe(0);
  });

  it('clear_obstacle и collect', () => {
    const { model } = setup();
    model.store.reset(toWorld(baseState(startItems(), { collectors: { cash: 120 } })));
    model.store.enqueue({ op: 'clear_obstacle', args: { uid: 'rock' } });
    model.store.enqueue({ op: 'collect', args: {} });
    const v = model.store.getState().view;
    expect(v.items.some((i) => i.uid === 'rock')).toBe(false);
    expect(v.collectors.cash).toBe(0);
  });

  it('build без цены не выдумывает таймер (декор)', () => {
    const { model } = setup();
    model.store.enqueue({ op: 'build', args: { uid: 'd1', type: 'x_flag', x: 5, y: 5, rot: 0 } });
    expect(model.store.getState().view.timers).toHaveLength(0);
  });

  it('load() заменяет подтверждённое состояние', async () => {
    const { model, api } = setup();
    api.fetchBase = () => Promise.resolve(baseState([item('hq', 'b_hq', 1, 1)], { version: 9 }));
    await model.load();
    expect(model.store.getState().confirmed.version).toBe(9);
  });
});

describe('base api adapter', () => {
  it('ходит по путям docs/03-api.md', async () => {
    const seen: { method: string; url: string; body?: string; key?: string }[] = [];
    const fetchMock = ((url: string, init: RequestInit) => {
      seen.push({ method: String(init.method), url, body: init.body as string | undefined, key: (init.headers as Record<string, string>)['Idempotency-Key'] });
      return Promise.resolve(new Response(JSON.stringify(baseState([])), { status: 200 }));
    }) as unknown as typeof fetch;
    const http = new ApiClient({ baseUrl: '', tokens: memoryTokenStore(), contentVersion: () => 'v', fetch: fetchMock, newKey: () => 'auto' });
    const api = createBaseApi(http);
    await api.fetchBase();
    await api.sendCommands([{ op: 'collect', args: {} }], 'my-key');
    await api.saveLayout([item('a', 'b_hq', 1, 1)]);
    expect(seen.map((s) => `${s.method} ${s.url}`)).toEqual(['GET /v1/base', 'POST /v1/base/commands', 'PUT /v1/base/layout']);
    expect(seen[1]?.key).toBe('my-key');
    expect(JSON.parse(seen[1]?.body ?? '')).toEqual([{ op: 'collect', args: {} }]);
    expect(JSON.parse(seen[2]?.body ?? '')).toEqual({ layout: { items: [item('a', 'b_hq', 1, 1)] } });
  });
});
