import {
  UnsupportedError,
  type AuthPayload,
  type Emitter,
  type KV,
  type LifecycleEvent,
  type PlatformAdapter,
  type Receipt,
  type SharePayload,
} from './adapter';

const DEVICE_KEY = 'fireline.device_id';

/** Память на случай, когда localStorage недоступен (приватный режим, SSR/тесты). */
export function memoryKV(): KV {
  const m = new Map<string, string>();
  return {
    get: (k) => Promise.resolve(m.get(k) ?? null),
    set: (k, v) => {
      m.set(k, v);
      return Promise.resolve();
    },
    remove: (k) => {
      m.delete(k);
      return Promise.resolve();
    },
  };
}

export function localStorageKV(ls: Storage): KV {
  return {
    get: (k) => Promise.resolve(ls.getItem(k)),
    set: (k, v) => {
      ls.setItem(k, v);
      return Promise.resolve();
    },
    remove: (k) => {
      ls.removeItem(k);
      return Promise.resolve();
    },
  };
}

export function randomDeviceId(): string {
  const bytes = new Uint8Array(24);
  globalThis.crypto.getRandomValues(bytes);
  // base64url без паддинга: 32 символа, подходит под ^[A-Za-z0-9_-]{16,64}$
  return btoa(String.fromCharCode(...bytes)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

export function createEmitter<E extends string>(): Emitter<E> & { emit(e: E): void } {
  const handlers = new Map<E, Set<() => void>>();
  return {
    on(event, handler) {
      let set = handlers.get(event);
      if (!set) handlers.set(event, (set = new Set()));
      set.add(handler);
      return () => set.delete(handler);
    },
    emit(event) {
      handlers.get(event)?.forEach((h) => h());
    },
  };
}

export interface BrowserOptions {
  storage?: KV;
  /** Источник событий окна (по умолчанию window/document, если есть). */
  target?: Pick<EventTarget, 'addEventListener'>;
  document?: { visibilityState: string; addEventListener: EventTarget['addEventListener'] };
  newDeviceId?: () => string;
  nav?: { share?: (d: { text?: string; url?: string }) => Promise<void> };
}

export function createBrowserAdapter(opts: BrowserOptions = {}): PlatformAdapter {
  const hasWindow = typeof window !== 'undefined';
  const storage = opts.storage ?? (hasWindow ? localStorageKV(window.localStorage) : memoryKV());
  const target = opts.target ?? (hasWindow ? window : undefined);
  const doc = opts.document ?? (typeof document !== 'undefined' ? document : undefined);
  const newDeviceId = opts.newDeviceId ?? randomDeviceId;
  const lifecycle = createEmitter<LifecycleEvent>();

  target?.addEventListener('online', () => lifecycle.emit('online'));
  target?.addEventListener('offline', () => lifecycle.emit('offline'));
  doc?.addEventListener('visibilitychange', () =>
    lifecycle.emit(doc.visibilityState === 'hidden' ? 'hidden' : 'visible'),
  );

  return {
    id: () => 'browser',
    async signedAuth(): Promise<AuthPayload> {
      let deviceId = await storage.get(DEVICE_KEY);
      if (!deviceId) {
        deviceId = newDeviceId();
        await storage.set(DEVICE_KEY, deviceId);
      }
      return { provider: 'guest', payload: { device_id: deviceId } };
    },
    purchase(): Promise<Receipt> {
      // Real-money покупок в браузерной сборке R1 нет (premium-валюта покупается внутри игры).
      return Promise.reject(new UnsupportedError('purchase on browser'));
    },
    async share(p: SharePayload) {
      const share = opts.nav?.share ?? (typeof navigator !== 'undefined' ? navigator.share?.bind(navigator) : undefined);
      if (!share) throw new UnsupportedError('share on browser');
      await share({ text: p.text, url: p.url });
    },
    storage,
    lifecycle,
    adsPolicy: { rewardedAllowed: true, interstitialAllowed: false },
  };
}
