import { describe, expect, it, vi } from 'vitest';
import { ApiClient, ApiError, memoryTokenStore, type Tokens } from './api';

const tok = (n: number): Tokens => ({ access_token: `a${n}`, refresh_token: `r${n}` });
const json = (status: number, body: unknown) =>
  Promise.resolve(new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }));
const err = (status: number, code: string, details?: unknown) => json(status, { error: { code, message: code, details } });

function make(fetchMock: ReturnType<typeof vi.fn>, extra = {}) {
  let n = 0;
  const tokens = memoryTokenStore(tok(1));
  const api = new ApiClient({
    baseUrl: 'http://x',
    tokens,
    contentVersion: () => 'cv1',
    fetch: fetchMock as unknown as typeof fetch,
    newKey: () => `key${++n}`,
    sleep: () => Promise.resolve(),
    ...extra,
  });
  return { api, tokens };
}
const hdr = (call: unknown[], name: string) => ((call[1] as RequestInit).headers as Record<string, string>)[name];

describe('ApiClient', () => {
  it('sends bearer, content version and idempotency key on mutations only', async () => {
    const f = vi.fn().mockImplementation(() => json(200, { ok: 1 }));
    const { api } = make(f);
    await api.get('/v1/me');
    await api.post('/v1/base/commands', [{ op: 'build' }]);
    expect(hdr(f.mock.calls[0]!, 'Authorization')).toBe('Bearer a1');
    expect(hdr(f.mock.calls[0]!, 'X-Content-Version')).toBe('cv1');
    expect(hdr(f.mock.calls[0]!, 'Idempotency-Key')).toBeUndefined();
    expect(hdr(f.mock.calls[1]!, 'Idempotency-Key')).toBe('key1');
  });

  it('retries a mutation with the same Idempotency-Key', async () => {
    const f = vi
      .fn()
      .mockImplementationOnce(() => Promise.reject(new TypeError('offline')))
      .mockImplementationOnce(() => err(500, 'internal'))
      .mockImplementation(() => json(200, { done: true }));
    const { api } = make(f);
    await expect(api.post('/v1/army/train', {})).resolves.toEqual({ done: true });
    expect(f).toHaveBeenCalledTimes(3);
    const keys = f.mock.calls.map((c) => hdr(c, 'Idempotency-Key'));
    expect(new Set(keys)).toEqual(new Set(['key1']));
  });

  it('gives up after maxRetries and does not retry business errors', async () => {
    const f = vi.fn().mockImplementation(() => err(500, 'internal'));
    const { api } = make(f, { maxRetries: 1 });
    await expect(api.post('/x', {})).rejects.toMatchObject({ code: 'internal' });
    expect(f).toHaveBeenCalledTimes(2);
    const g = vi.fn().mockImplementation(() => err(409, 'insufficient_resources'));
    await expect(make(g).api.post('/x', {})).rejects.toMatchObject({ code: 'insufficient_resources' });
    expect(g).toHaveBeenCalledTimes(1);
  });

  it('refreshes on 401, rotates tokens and replays with the same key', async () => {
    const f = vi
      .fn()
      .mockImplementationOnce(() => err(401, 'unauthorized'))
      .mockImplementationOnce(() => json(200, tok(2)))
      .mockImplementation(() => json(200, { ok: true }));
    const { api, tokens } = make(f);
    await expect(api.post('/v1/base/commands', [])).resolves.toEqual({ ok: true });
    expect(f.mock.calls[1]![0]).toBe('http://x/v1/auth/refresh');
    expect(JSON.parse((f.mock.calls[1]![1] as RequestInit).body as string)).toEqual({ refresh_token: 'r1' });
    expect(tokens.get()).toEqual(tok(2));
    expect(hdr(f.mock.calls[2]!, 'Authorization')).toBe('Bearer a2');
    expect(hdr(f.mock.calls[2]!, 'Idempotency-Key')).toBe(hdr(f.mock.calls[0]!, 'Idempotency-Key'));
  });

  it('shares one refresh between concurrent 401s', async () => {
    let refreshes = 0;
    const f = vi.fn().mockImplementation((url: string, init: RequestInit) => {
      if (url.endsWith('/auth/refresh')) {
        refreshes++;
        return json(200, tok(2));
      }
      const auth = (init.headers as Record<string, string>).Authorization;
      return auth === 'Bearer a1' ? err(401, 'unauthorized') : json(200, {});
    });
    const { api } = make(f);
    await Promise.all([api.get('/v1/me'), api.get('/v1/base')]);
    expect(refreshes).toBe(1);
  });

  it('clears session and calls onUnauthorized when refresh is rejected', async () => {
    const f = vi.fn().mockImplementation(() => err(401, 'unauthorized'));
    const onUnauthorized = vi.fn();
    const { api, tokens } = make(f, { onUnauthorized });
    await expect(api.get('/v1/me')).rejects.toMatchObject({ code: 'unauthorized' });
    expect(tokens.get()).toBeNull();
    expect(onUnauthorized).toHaveBeenCalledOnce();
    expect(f).toHaveBeenCalledTimes(2); // запрос + refresh, без цикла
  });

  it('reports content_outdated with bundle url and does not retry', async () => {
    const f = vi
      .fn()
      .mockImplementation(() => err(409, 'content_outdated', { content_version: 'cv2', bundle_url: 'http://cdn/b' }));
    const onContentOutdated = vi.fn();
    const { api } = make(f, { onContentOutdated });
    const e = await api.post('/x', {}).catch((x: unknown) => x);
    expect(e).toBeInstanceOf(ApiError);
    expect((e as ApiError).code).toBe('content_outdated');
    expect(onContentOutdated).toHaveBeenCalledWith({ content_version: 'cv2', bundle_url: 'http://cdn/b' });
    expect(f).toHaveBeenCalledTimes(1);
  });
});
