// Синхронизация базы по finish_at (#60): таймер истёк → рефетч → строитель свободен, уровень обновлён.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { baseState, startItems } from '../core/base/test-kit';
import { createBrowserAdapter, memoryKV } from '../platform/browser';
import { createApp, type Me } from './app';
import { loadContent } from './content';

const content = loadContent();
const SERVER_T0 = Date.parse('2026-01-01T00:00:00.000Z');
const json = (body: unknown) => new Response(JSON.stringify(body), { status: 200 });
const me = (busy: boolean): Me => ({
  profile: { id: 'p1', nickname: 'G', hq_level: 1, trophies: 0, league: 'bronze', shield_until: null, created_at: '2026-01-01T00:00:00Z' },
  wallets: { cash: 1, fuel: 1, alloy: 0, gems: 0 },
  builders: [{ id: 'b1', busy_until: busy ? '2026-01-01T00:00:05.000Z' : null }],
});

/** Сервер с одним таймером улучшения; finishInS — через сколько секунд серверного времени он завершится. */
function setup(clientSkewMs: number, finishInS = 5) {
  vi.setSystemTime(SERVER_T0 + clientSkewMs);
  const start = Date.now(); // клиентское время старта
  const items = startItems();
  const target = items.find((i) => i.type === 'b_cash_mine')!;
  const paths: string[] = [];
  const serverNow = () => SERVER_T0 + (Date.now() - start);
  const fetchFn = ((url: string) => {
    paths.push(url);
    if (url === '/v1/auth/guest') return Promise.resolve(json({ player_id: 'p1', device_id: 'd', tokens: { access_token: 'a', refresh_token: 'r' } }));
    const done = serverNow() >= SERVER_T0 + finishInS * 1000;
    if (url === '/v1/me') return Promise.resolve(json(me(!done)));
    if (url === '/v1/base') {
      const its = items.map((i) => (i.uid === target.uid && done ? { ...i, level: i.level + 1 } : i));
      return Promise.resolve(
        json(
          baseState(its, {
            server_time: new Date(serverNow()).toISOString(),
            timers: done ? [] : [{ id: 't1', kind: 'upgrade', target_id: target.uid, finish_at: new Date(SERVER_T0 + finishInS * 1000).toISOString(), to_level: target.level + 1 }],
            builders: { total: 1, busy: done ? 0 : 1 },
          }),
        ),
      );
    }
    return Promise.resolve(new Response('{}', { status: 404 }));
  }) as unknown as typeof fetch;
  const doc = new EventTarget() as EventTarget & { visibilityState: string };
  doc.visibilityState = 'visible';
  const platform = createBrowserAdapter({ storage: memoryKV(), newDeviceId: () => 'dev_0123456789abcdef0123', document: doc, target: new EventTarget() });
  const app = createApp({ platform, baseUrl: '', content, fetch: fetchFn, sleep: () => Promise.resolve() });
  const count = (p: string) => paths.filter((x) => x === p).length;
  return { app, doc, count, target };
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe('синхронизация таймеров базы (#60)', () => {
  it('по finish_at рефетчит base и me: строитель свободен, уровень обновлён', async () => {
    const { app, count, target } = setup(0);
    await app.start();
    expect(app.base.model.store.getState().confirmed.timers).toHaveLength(1);
    expect(app.base.model.store.getState().confirmed.builders.busy).toBe(1);
    expect(count('/v1/base')).toBe(1);

    await vi.advanceTimersByTimeAsync(4000);
    expect(count('/v1/base')).toBe(1); // рано
    await vi.advanceTimersByTimeAsync(1500);

    const w = app.base.model.store.getState().confirmed;
    expect(count('/v1/base')).toBe(2);
    expect(w.timers).toHaveLength(0);
    expect(w.builders.busy).toBe(0);
    expect(w.items.find((i) => i.uid === target.uid)!.level).toBe(target.level + 1);
    expect(app.store.getState().me!.builders[0]!.busy_until).toBeNull();
    // таймеров больше нет — дальше запросов нет
    await vi.advanceTimersByTimeAsync(60_000);
    expect(count('/v1/base')).toBe(2);
    app.dispose();
  });

  it.each([3_600_000, -3_600_000])('смещение часов клиента %i мс: ждём по server_time, не по локальным часам', async (skew) => {
    const { app, count } = setup(skew);
    await app.start();
    await vi.advanceTimersByTimeAsync(4000);
    expect(count('/v1/base')).toBe(1);
    await vi.advanceTimersByTimeAsync(1500);
    expect(count('/v1/base')).toBe(2);
    expect(app.base.model.store.getState().confirmed.builders.busy).toBe(0);
    app.dispose();
  });

  it('возврат вкладки после долгого простоя обновляет базу', async () => {
    const { app, doc, count } = setup(0);
    await app.start();
    vi.setSystemTime(Date.now() + 30_000); // вкладка «спала», таймеры не тикали
    doc.visibilityState = 'visible';
    doc.dispatchEvent(new Event('visibilitychange'));
    await vi.advanceTimersByTimeAsync(0);
    expect(count('/v1/base')).toBe(2);
    expect(app.base.model.store.getState().confirmed.builders.busy).toBe(0);
    app.dispose();
  });

  it('dispose отменяет отложенный таймер', async () => {
    const { app, count } = setup(0);
    await app.start();
    app.dispose();
    await vi.advanceTimersByTimeAsync(20_000);
    expect(count('/v1/base')).toBe(1);
  });
});
