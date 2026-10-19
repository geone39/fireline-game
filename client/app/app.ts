// Ядро шелла: вход гостя, загрузка /v1/me и /v1/base, восстановление сессии, content_outdated.
// Без DOM и Pixi — всё тестируется моками fetch/PlatformAdapter.
import { ApiClient, ApiError, type ContentOutdated } from '../core/api';
import { createBaseApi, createBaseController, serverClock, type BaseController } from '../core/base';
import { createStore, type Store } from '../core/store';
import { UnsupportedError, type PlatformAdapter } from '../platform/adapter';
import type { Content } from './content';
import { createPersistentTokens } from './session';

export interface Me {
  profile: {
    id: string;
    nickname: string;
    hq_level: number;
    trophies: number;
    league: string;
    shield_until: string | null;
    created_at: string;
  };
  /** Кошельки по ресурсам (cash/fuel/alloy/gems). */
  wallets: Record<string, number>;
  /** По элементу на слот билдера; busy_until === null — свободен. */
  builders: { id: string; busy_until: string | null; target_id?: string }[];
  server_time?: string;
}

interface GuestResponse {
  player_id: string;
  device_id: string;
  tokens: { access_token: string; refresh_token: string; access_expires_at?: string; refresh_expires_at?: string };
}

export type Phase = 'booting' | 'ready' | 'error' | 'outdated';

export interface AppState {
  phase: Phase;
  me: Me | null;
  error: { code: string; message: string } | null;
  outdated: ContentOutdated | null;
}

export interface AppDeps {
  platform: PlatformAdapter;
  /** '' — тот же origin (dev: vite proxy на /v1). */
  baseUrl: string;
  /** Версия контента клиента → X-Content-Version. Пусто — сервер проверку пропускает. */
  contentVersion?: string;
  content: Content;
  fetch?: typeof fetch;
  now?: () => number;
  newKey?: () => string;
  newUid?: () => string;
  sleep?: (ms: number) => Promise<void>;
}

export interface App {
  store: Store<AppState>;
  api: ApiClient;
  base: BaseController;
  /** Восстановить сессию (или войти гостем) и загрузить me + base. Не бросает: итог — в store.phase. */
  start(): Promise<void>;
  /** Обновить кошельки/билдеров (/v1/me). */
  refreshMe(): Promise<void>;
  /** Текущее время сервера, мс: server_time последнего ответа + прошедшее локально. */
  serverNowMs(): number;
  dispose(): void;
}

/** Технические интервалы синхронизации таймеров (не игровые числа). */
const TIMER_SYNC_MARGIN_MS = 300;
const TIMER_SYNC_MIN_MS = 1000;
const MAX_TIMEOUT_MS = 2 ** 31 - 1;

export function createApp(d: AppDeps): App {
  const store = createStore<AppState>({ phase: 'booting', me: null, error: null, outdated: null });
  const tokens = createPersistentTokens(d.platform.storage);
  const setOutdated = (info: ContentOutdated): void => store.setState({ phase: 'outdated', outdated: info });

  const api = new ApiClient({
    baseUrl: d.baseUrl,
    tokens,
    contentVersion: () => d.contentVersion ?? '',
    fetch: d.fetch,
    newKey: d.newKey,
    sleep: d.sleep,
    onContentOutdated: setOutdated,
    onUnauthorized: () => {
      if (store.getState().phase === 'ready') void recover();
    },
  });
  const base = createBaseController({
    api: createBaseApi(api),
    catalog: d.content.catalog,
    pricing: d.content.pricing,
    grid: d.content.grid,
    now: d.now,
    newUid: d.newUid,
  });

  let signing: Promise<void> | null = null;
  /** Вход гостем; параллельные вызовы делят один запрос. */
  function signIn(): Promise<void> {
    signing ??= (async () => {
      const auth = await d.platform.signedAuth();
      if (auth.provider !== 'guest') throw new UnsupportedError(`sign-in with ${auth.provider}`);
      const r = await api.post<GuestResponse>('/v1/auth/guest', { device_id: auth.payload.device_id }, { anonymous: true });
      tokens.set(r.tokens);
    })().finally(() => {
      signing = null;
    });
    return signing;
  }

  async function loadAll(): Promise<void> {
    const [me] = await Promise.all([api.get<Me>('/v1/me'), base.model.load()]);
    store.setState({ me });
  }

  /** Загрузка; при потерянной сессии — один повторный вход. */
  async function loadWithRelogin(): Promise<void> {
    try {
      await loadAll();
    } catch (e) {
      if (!(e instanceof ApiError) || e.code !== 'unauthorized') throw e;
      await signIn();
      await loadAll();
    }
  }

  function fail(e: unknown): void {
    if (store.getState().phase === 'outdated') return;
    if (e instanceof ApiError && e.code === 'content_outdated') {
      setOutdated((e.details ?? {}) as ContentOutdated);
      return;
    }
    store.setState({
      phase: 'error',
      error: {
        code: e instanceof ApiError ? e.code : e instanceof UnsupportedError ? 'unsupported' : 'internal',
        message: e instanceof Error ? e.message : String(e),
      },
    });
  }

  async function start(): Promise<void> {
    store.setState({ phase: 'booting', error: null });
    try {
      await tokens.restore();
      if (!tokens.get()) await signIn();
      await loadWithRelogin();
      store.setState({ phase: 'ready' });
    } catch (e) {
      fail(e);
    }
  }

  /** Сессия потеряна посреди игры (refresh не удался): тихо входим тем же устройством и перезагружаем состояние. */
  async function recover(): Promise<void> {
    try {
      await signIn();
      await loadAll();
    } catch (e) {
      fail(e);
    }
  }

  let refreshing: Promise<void> | null = null;
  let again = false;
  function refreshMe(): Promise<void> {
    if (refreshing) {
      again = true; // пока шёл запрос, состояние изменилось — повторим один раз
      return refreshing;
    }
    refreshing = (async () => {
      do {
        again = false;
        try {
          store.setState({ me: await api.get<Me>('/v1/me') });
        } catch {
          // сеть/5xx/outdated: HUD остаётся со старыми числами, следующее событие повторит запрос
        }
      } while (again);
    })().finally(() => {
      refreshing = null;
    });
    return refreshing;
  }

  const now = d.now ?? Date.now;
  let receivedAt = now();
  let lastServerTime = base.model.store.getState().confirmed.serverTime;
  const serverNowMs = (): number => serverClock(base.model.store.getState().confirmed, receivedAt, now());

  // Один отложенный таймер на ближайший finish_at (#60): по срабатыванию перезапрашиваем /v1/base (→ и /v1/me).
  // Поправка на смещение часов — через server_time ответа (serverNowMs). Пересоздаётся при каждом подтверждённом состоянии.
  let timerHandle: ReturnType<typeof setTimeout> | null = null;
  const clearTimerHandle = (): void => {
    if (timerHandle !== null) clearTimeout(timerHandle);
    timerHandle = null;
  };
  function armTimerSync(): void {
    clearTimerHandle();
    if (store.getState().phase !== 'ready') return;
    const timers = base.model.store.getState().confirmed.timers;
    if (timers.length === 0) return;
    const nextFinish = Math.min(...timers.map((t) => Date.parse(t.finish_at)).filter((n) => !Number.isNaN(n)));
    if (!Number.isFinite(nextFinish)) return;
    // запас: сервер завершает таймер лениво/по тику; минимальный интервал защищает от цикла при рассинхроне
    const delay = Math.max(nextFinish - serverNowMs() + TIMER_SYNC_MARGIN_MS, TIMER_SYNC_MIN_MS);
    timerHandle = setTimeout(() => {
      timerHandle = null;
      if (store.getState().phase !== 'ready') return;
      base.model
        .load()
        .catch(() => undefined)
        .finally(armTimerSync);
    }, Math.min(delay, MAX_TIMEOUT_MS));
  }

  // Кошельки меняются от команд базы (списание, сбор): после каждого подтверждения сервера обновляем me.
  const offBase = base.model.store.subscribe((s, prev) => {
    if (s.confirmed.serverTime !== lastServerTime) {
      lastServerTime = s.confirmed.serverTime;
      receivedAt = now();
    }
    if (s.confirmed !== prev.confirmed && store.getState().phase === 'ready') {
      void refreshMe();
      armTimerSync();
    }
  });
  const offPhase = store.subscribe((s, prev) => {
    if (s.phase !== prev.phase) armTimerSync();
  });
  const offLife = [
    d.platform.lifecycle.on('visible', () => {
      // load() меняет confirmed → подписка ниже сама обновит /v1/me
      if (store.getState().phase === 'ready') void base.model.load().catch(() => undefined);
    }),
    d.platform.lifecycle.on('online', () => {
      if (store.getState().phase === 'ready') void base.model.store.flush();
    }),
  ];

  return {
    store,
    api,
    base,
    start,
    refreshMe,
    serverNowMs,
    dispose() {
      clearTimerHandle();
      offBase();
      offPhase();
      offLife.forEach((f) => f());
    },
  };
}
