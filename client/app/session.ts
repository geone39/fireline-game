// Токены сессии: синхронный TokenStore для ApiClient + сохранение в PlatformAdapter.storage.
import type { TokenStore, Tokens } from '../core/api';
import type { KV } from '../platform/adapter';

export const TOKENS_KEY = 'fireline.tokens';

export interface PersistentTokens extends TokenStore {
  /** Прочитать сохранённые токены из storage (один раз при старте). */
  restore(): Promise<Tokens | null>;
}

function parseTokens(raw: string | null): Tokens | null {
  if (!raw) return null;
  try {
    const v = JSON.parse(raw) as Partial<Tokens>;
    return typeof v.access_token === 'string' && typeof v.refresh_token === 'string' ? (v as Tokens) : null;
  } catch {
    return null;
  }
}

export function createPersistentTokens(kv: KV): PersistentTokens {
  let cur: Tokens | null = null;
  const swallow = (): void => undefined; // storage недоступен — сессия живёт в памяти
  return {
    get: () => cur,
    set(t) {
      cur = t;
      void (t ? kv.set(TOKENS_KEY, JSON.stringify(t)) : kv.remove(TOKENS_KEY)).catch(swallow);
    },
    async restore() {
      cur = parseTokens(await kv.get(TOKENS_KEY).catch(() => null));
      return cur;
    },
  };
}
