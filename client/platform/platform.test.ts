import { describe, expect, it, vi } from 'vitest';
import { UnsupportedError } from './adapter';
import { createBrowserAdapter, memoryKV } from './browser';
import { createTelegramAdapter } from './telegram';
import { createVKAdapter } from './vk';

function target() {
  const h = new Map<string, () => void>();
  return { addEventListener: (e: string, f: () => void) => void h.set(e, f), fire: (e: string) => h.get(e)?.() };
}

describe('browser adapter', () => {
  it('creates a device id once and reuses it', async () => {
    const storage = memoryKV();
    const a = createBrowserAdapter({ storage, newDeviceId: () => 'dev-1234567890abcdef' });
    const p1 = await a.signedAuth();
    const b = createBrowserAdapter({ storage, newDeviceId: () => 'other-xxxxxxxxxxxxxx' });
    expect(p1).toEqual({ provider: 'guest', payload: { device_id: 'dev-1234567890abcdef' } });
    expect(await b.signedAuth()).toEqual(p1);
    expect(a.id()).toBe('browser');
  });

  it('default device id matches server pattern', async () => {
    const a = createBrowserAdapter({ storage: memoryKV() });
    expect((await a.signedAuth()).payload.device_id).toMatch(/^[A-Za-z0-9_-]{16,64}$/);
  });

  it('kv roundtrip', async () => {
    const kv = createBrowserAdapter({ storage: memoryKV() }).storage;
    await kv.set('a', '1');
    expect(await kv.get('a')).toBe('1');
    await kv.remove('a');
    expect(await kv.get('a')).toBeNull();
  });

  it('emits lifecycle events', () => {
    const t = target();
    const d = { visibilityState: 'hidden', ...target() };
    const a = createBrowserAdapter({ storage: memoryKV(), target: t, document: d as never });
    const on = vi.fn();
    const off = a.lifecycle.on('offline', on);
    const hid = vi.fn();
    a.lifecycle.on('hidden', hid);
    t.fire('offline');
    d.fire('visibilitychange');
    off();
    t.fire('offline');
    expect(on).toHaveBeenCalledOnce();
    expect(hid).toHaveBeenCalledOnce();
  });

  it('purchase is unsupported, share uses navigator.share when present', async () => {
    const share = vi.fn().mockResolvedValue(undefined);
    const a = createBrowserAdapter({ storage: memoryKV(), nav: { share } });
    await expect(a.purchase('sku')).rejects.toBeInstanceOf(UnsupportedError);
    await a.share({ text: 't', url: 'u' });
    expect(share).toHaveBeenCalledWith({ text: 't', url: 'u' });
    await expect(createBrowserAdapter({ storage: memoryKV() }).share({ text: 't' })).rejects.toBeInstanceOf(
      UnsupportedError,
    );
  });
});

describe('stubs', () => {
  it.each([
    ['vk', createVKAdapter()],
    ['telegram', createTelegramAdapter()],
  ])('%s throws UnsupportedError', (id, a) => {
    expect(a.id()).toBe(id);
    expect(() => a.signedAuth()).toThrow(UnsupportedError);
    expect(a.adsPolicy.rewardedAllowed).toBe(false);
  });
});
