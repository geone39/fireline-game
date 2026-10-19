// Bootstrap и вход: createApp против мока сервера (fetch), без DOM и Pixi.
import { describe, expect, it } from 'vitest';
import { baseState, startItems } from '../core/base/test-kit';
import { createBrowserAdapter, memoryKV } from '../platform/browser';
import { createApp, type Me } from './app';
import { loadContent } from './content';
import { TOKENS_KEY } from './session';

const content = loadContent();
const DEVICE = 'dev_0123456789abcdef0123';

const me = (cash = 500): Me => ({
  profile: { id: 'p1', nickname: 'Guest', hq_level: 1, trophies: 0, league: 'bronze', shield_until: null, created_at: '2026-01-01T00:00:00Z' },
  wallets: { cash, fuel: 200, alloy: 0, gems: 50 },
  builders: [{ id: 'b1', busy_until: null }],
});
const tokens = (n: number) => ({ access_token: `a${n}`, refresh_token: `r${n}`, access_expires_at: 'x', refresh_expires_at: 'y' });
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const apiError = (status: number, code: string, details?: unknown) => json(status, { error: { code, message: code, details } });

interface Call {
  method: string;
  path: string;
  auth: string | null;
  body: unknown;
}

/** Мини-сервер: по умолчанию «здоровый»; override перехватывает отдельные запросы. */
function server(override: (c: Call) => Response | undefined = () => undefined) {
  const calls: Call[] = [];
  let walletCash = 500;
  let version = 1;
  let loginN = 0;
  const fetchFn = ((url: string, init: RequestInit = {}) => {
    const headers = new Headers(init.headers);
    const c: Call = {
      method: init.method ?? 'GET',
      path: url,
      auth: headers.get('Authorization'),
      body: init.body ? JSON.parse(init.body as string) : undefined,
    };
    calls.push(c);
    const o = override(c);
    if (o) return Promise.resolve(o);
    if (c.path === '/v1/auth/guest') return Promise.resolve(json(200, { player_id: 'p1', device_id: DEVICE, tokens: tokens(++loginN) }));
    if (c.path === '/v1/auth/refresh') return Promise.resolve(json(200, { player_id: 'p1', tokens: tokens(100 + ++loginN) }));
    if (c.path === '/v1/me') return Promise.resolve(json(200, me(walletCash)));
    if (c.path === '/v1/base') return Promise.resolve(json(200, baseState(startItems(), { version })));
    if (c.path === '/v1/base/commands') {
      version++;
      walletCash += 25; // collect
      return Promise.resolve(json(200, baseState(startItems(), { version, collectors: {} })));
    }
    return Promise.resolve(apiError(404, 'not_found'));
  }) as unknown as typeof fetch;
  return { calls, fetchFn };
}

function setup(s = server(), kv = memoryKV()) {
  const platform = createBrowserAdapter({ storage: kv, newDeviceId: () => DEVICE });
  const app = createApp({ platform, baseUrl: '', content, fetch: s.fetchFn, sleep: () => Promise.resolve(), newKey: () => 'k' });
  return { app, kv, ...s };
}
const paths = (calls: Call[]) => calls.map((c) => `${c.method} ${c.path}`);

describe('вход гостя и загрузка', () => {
  it('новый гость: guest → me + base, токены и device_id сохранены в storage', async () => {
    const { app, kv, calls } = setup();
    await app.start();
    const st = app.store.getState();
    expect(st.phase).toBe('ready');
    expect(st.me?.wallets.cash).toBe(500);
    expect(app.base.model.store.getState().confirmed.items).toHaveLength(startItems().length);

    const guest = calls.find((c) => c.path === '/v1/auth/guest')!;
    expect(guest.body).toEqual({ device_id: DEVICE });
    expect(guest.auth).toBeNull();
    expect(calls.filter((c) => c.path === '/v1/me' || c.path === '/v1/base').every((c) => c.auth === 'Bearer a1')).toBe(true);
    expect(JSON.parse((await kv.get(TOKENS_KEY))!).refresh_token).toBe('r1');
    expect(await kv.get('fireline.device_id')).toBe(DEVICE);
  });

  it('повторный запуск с сохранёнными токенами: без /auth/guest', async () => {
    const kv = memoryKV();
    await kv.set(TOKENS_KEY, JSON.stringify(tokens(7)));
    const { app, calls } = setup(server(), kv);
    await app.start();
    expect(app.store.getState().phase).toBe('ready');
    expect(paths(calls)).not.toContain('POST /v1/auth/guest');
    expect(calls.find((c) => c.path === '/v1/me')?.auth).toBe('Bearer a7');
  });

  it('access протух: refresh с ротацией, новая пара сохраняется', async () => {
    const kv = memoryKV();
    await kv.set(TOKENS_KEY, JSON.stringify(tokens(7)));
    let first = true;
    const s = server((c) => {
      if (c.path === '/v1/me' && c.auth === 'Bearer a7' && first) {
        first = false;
        return apiError(401, 'unauthorized');
      }
      return undefined;
    });
    const { app, calls } = setup(s, kv);
    await app.start();
    expect(app.store.getState().phase).toBe('ready');
    expect(paths(calls)).toContain('POST /v1/auth/refresh');
    expect(paths(calls)).not.toContain('POST /v1/auth/guest');
    expect(JSON.parse((await kv.get(TOKENS_KEY))!).refresh_token).toMatch(/^r10\d/);
  });

  it('refresh отозван: повторный вход тем же device_id', async () => {
    const kv = memoryKV();
    await kv.set(TOKENS_KEY, JSON.stringify(tokens(7)));
    const s = server((c) => (c.path === '/v1/auth/refresh' || c.auth === 'Bearer a7' ? apiError(401, 'unauthorized') : undefined));
    const { app, calls } = setup(s, kv);
    await app.start();
    expect(app.store.getState().phase).toBe('ready');
    expect(calls.find((c) => c.path === '/v1/auth/guest')?.body).toEqual({ device_id: DEVICE });
    expect(JSON.parse((await kv.get(TOKENS_KEY))!).access_token).toBe('a1');
  });

  it('content_outdated: экран обновления с bundle_url', async () => {
    const s = server((c) => (c.path === '/v1/me' ? apiError(409, 'content_outdated', { content_version: 'v2', bundle_url: 'https://cdn/b' }) : undefined));
    const { app } = setup(s);
    await app.start();
    const st = app.store.getState();
    expect(st.phase).toBe('outdated');
    expect(st.outdated).toEqual({ content_version: 'v2', bundle_url: 'https://cdn/b' });
  });

  it('сеть недоступна: phase=error (network), повторный start после восстановления', async () => {
    let down = true;
    const s = server();
    const flaky = ((u: string, i?: RequestInit) => (down ? Promise.reject(new TypeError('offline')) : s.fetchFn(u, i))) as typeof fetch;
    const platform = createBrowserAdapter({ storage: memoryKV(), newDeviceId: () => DEVICE });
    const app = createApp({ platform, baseUrl: '', content, fetch: flaky, sleep: () => Promise.resolve() });
    await app.start();
    expect(app.store.getState().phase).toBe('error');
    expect(app.store.getState().error?.code).toBe('network');
    down = false;
    await app.start();
    expect(app.store.getState().phase).toBe('ready');
  });
});

describe('сессия в игре', () => {
  it('после команды базы кошелёк из /v1/me обновляется', async () => {
    const { app } = setup();
    await app.start();
    expect(app.store.getState().me?.wallets.cash).toBe(500);
    await app.base.model.commands.collect();
    await app.refreshMe(); // дожидаемся уже запущенного/повторного запроса
    expect(app.store.getState().me?.wallets.cash).toBe(525);
  });

  it('потеря сессии посреди игры: тихий повторный вход и перезагрузка', async () => {
    let dead = false;
    const s = server((c) => (dead && (c.auth === 'Bearer a1' || c.path === '/v1/auth/refresh') ? apiError(401, 'unauthorized') : undefined));
    const { app, calls } = setup(s);
    await app.start();
    dead = true;
    await app.refreshMe(); // 401 → refresh 401 → onUnauthorized → recover
    await new Promise((r) => setTimeout(r, 10));
    expect(app.store.getState().phase).toBe('ready');
    expect(calls.filter((c) => c.path === '/v1/auth/guest')).toHaveLength(2);
  });
});
