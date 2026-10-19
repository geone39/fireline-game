// API-клиент по docs/03-api.md.

export interface Tokens {
  access_token: string;
  refresh_token: string;
  access_expires_at?: string;
  refresh_expires_at?: string;
}

export interface TokenStore {
  get(): Tokens | null;
  set(t: Tokens | null): void;
}

export function memoryTokenStore(initial: Tokens | null = null): TokenStore {
  let cur = initial;
  return { get: () => cur, set: (t) => void (cur = t) };
}

export type ErrorCode =
  | 'invalid' | 'unauthorized' | 'not_found' | 'insufficient_resources' | 'conflict'
  | 'shielded' | 'cooldown' | 'content_outdated' | 'rate_limited' | 'internal'
  | 'network';

export class ApiError extends Error {
  constructor(
    readonly code: ErrorCode | (string & {}),
    readonly status: number,
    message: string,
    readonly details?: unknown,
  ) {
    super(message);
    this.name = 'ApiError';
  }
  /** Ошибка, после которой повтор того же запроса имеет смысл. */
  get retryable(): boolean {
    return this.code === 'network' || this.code === 'rate_limited' || this.status >= 500;
  }
}

export interface ContentOutdated {
  content_version?: string;
  bundle_url?: string;
}

export interface ApiOptions {
  baseUrl: string;
  tokens: TokenStore;
  /** Версия контента клиента → X-Content-Version. */
  contentVersion: () => string;
  fetch?: typeof fetch;
  newKey?: () => string;
  onContentOutdated?: (info: ContentOutdated) => void;
  /** Сессия потеряна (refresh не удался): нужен повторный вход. */
  onUnauthorized?: () => void;
  /** Сколько раз повторять запрос при network/429/5xx (с тем же Idempotency-Key). */
  maxRetries?: number;
  sleep?: (ms: number) => Promise<void>;
}

export interface RequestOptions {
  /** Принудительно задать/не задавать Idempotency-Key. По умолчанию: для всего, кроме GET. */
  idempotencyKey?: string | false;
  /** Не пытаться обновить токен и не слать Authorization (auth/*). */
  anonymous?: boolean;
}

export class ApiClient {
  private readonly f: typeof fetch;
  private readonly newKey: () => string;
  private readonly maxRetries: number;
  private readonly sleep: (ms: number) => Promise<void>;
  private refreshing: Promise<void> | null = null;

  constructor(private readonly o: ApiOptions) {
    this.f = o.fetch ?? ((...a) => globalThis.fetch(...a));
    this.newKey = o.newKey ?? (() => globalThis.crypto.randomUUID());
    this.maxRetries = o.maxRetries ?? 2;
    this.sleep = o.sleep ?? ((ms) => new Promise((r) => setTimeout(r, ms)));
  }

  get<T>(path: string, opts?: RequestOptions): Promise<T> {
    return this.request<T>('GET', path, undefined, opts);
  }
  post<T>(path: string, body?: unknown, opts?: RequestOptions): Promise<T> {
    return this.request<T>('POST', path, body, opts);
  }
  put<T>(path: string, body?: unknown, opts?: RequestOptions): Promise<T> {
    return this.request<T>('PUT', path, body, opts);
  }

  async request<T>(method: string, path: string, body?: unknown, opts: RequestOptions = {}): Promise<T> {
    const key =
      opts.idempotencyKey === false ? undefined : (opts.idempotencyKey ?? (method === 'GET' ? undefined : this.newKey()));
    let refreshed = false;
    for (let attempt = 0; ; attempt++) {
      try {
        return await this.once<T>(method, path, body, key, opts.anonymous === true);
      } catch (e) {
        if (!(e instanceof ApiError)) throw e;
        if (e.code === 'unauthorized' && !opts.anonymous && !refreshed) {
          refreshed = true;
          await this.refresh(); // бросит unauthorized, если не вышло
          attempt--;
          continue;
        }
        // Повтор небезопасных мутаций без ключа запрещён: ключ есть → повтор идемпотентен.
        const safe = method === 'GET' || key !== undefined;
        if (e.retryable && safe && attempt < this.maxRetries) {
          await this.sleep(200 * 2 ** attempt);
          continue;
        }
        throw e;
      }
    }
  }

  /** Single-flight обновление пары токенов с ротацией refresh. */
  refresh(): Promise<void> {
    this.refreshing ??= this.doRefresh().finally(() => {
      this.refreshing = null;
    });
    return this.refreshing;
  }

  private async doRefresh(): Promise<void> {
    const cur = this.o.tokens.get();
    if (!cur) return this.lost('no session');
    try {
      const res = await this.once<Tokens | { tokens: Tokens }>('POST', '/v1/auth/refresh', { refresh_token: cur.refresh_token }, this.newKey(), true);
      // Сервер отвечает {player_id, tokens:{...}} (как /auth/guest); плоская форма допускается для обратной совместимости.
      this.o.tokens.set('tokens' in res ? res.tokens : res); // старый refresh отозван сервером — сохраняем новую пару
    } catch (e) {
      if (e instanceof ApiError && e.code === 'unauthorized') return this.lost(e.message);
      throw e;
    }
  }

  private lost(message: string): never {
    this.o.tokens.set(null);
    this.o.onUnauthorized?.();
    throw new ApiError('unauthorized', 401, message);
  }

  private async once<T>(method: string, path: string, body: unknown, key: string | undefined, anonymous: boolean): Promise<T> {
    const headers: Record<string, string> = {
      Accept: 'application/json',
      'X-Content-Version': this.o.contentVersion(),
    };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (key) headers['Idempotency-Key'] = key;
    const tok = this.o.tokens.get();
    if (!anonymous && tok) headers.Authorization = `Bearer ${tok.access_token}`;

    let res: Response;
    try {
      res = await this.f(this.o.baseUrl + path, {
        method,
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch (e) {
      throw new ApiError('network', 0, e instanceof Error ? e.message : 'network error');
    }

    const text = await res.text();
    let json: unknown;
    try {
      json = text ? JSON.parse(text) : undefined;
    } catch {
      json = undefined;
    }
    if (res.ok) return json as T;

    const err = (json as { error?: { code?: string; message?: string; details?: unknown } } | undefined)?.error;
    const code = err?.code ?? (res.status === 401 ? 'unauthorized' : res.status >= 500 ? 'internal' : 'invalid');
    const apiErr = new ApiError(code, res.status, err?.message ?? res.statusText, err?.details);
    if (code === 'content_outdated') this.o.onContentOutdated?.((err?.details ?? {}) as ContentOutdated);
    throw apiErr;
  }
}
