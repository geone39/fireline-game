// Контроллер армии и лаборатории: подтверждённое состояние + оптимистичные команды с откатом, ленивое обновление
// по finish_at и селекторы для UI. Точка интеграции для шелла (CLIENT-5): createArmyController(...) — один на сессию.
// Команды идут по одной (у каждой свой эндпоинт и Idempotency-Key): отказ откатывает только её и запрашивает свежее состояние.
import { ApiError } from '../../../core/api';
import { createStore, type Store } from '../../../core/store';
import type { ArmyCatalog } from '../content';
import type { ArmyApi, ArmyStateDto, BuildingRef, LabStateDto, World } from '../types';
import { EMPTY_WORLD, armyFromDto, labFromDto } from '../types';
import { applyOp, opSpend, settleArmy, type ApplyCtx, type Op } from './model';
import { INITIAL_UI, selectViewModel, type ArmyViewModel, type Notice, type Tab, type UiState, type Wallet } from './view-model';

export interface ArmyControllerOptions {
  api: ArmyApi;
  catalog: ArmyCatalog;
  /** Здания игрока из слайса базы (uid, type, level): производители, штаб и лаборатория. */
  buildings: () => readonly BuildingRef[];
  /** Кошелёк из /v1/me (cash/fuel/alloy/gems) — только для предпроверок; не задан — отключены. */
  wallet?: () => Wallet | undefined;
  /** После любой мутации (успех или отказ): шелл обновляет кошелёк (/v1/me) и таймеры базы. */
  onSettled?: () => void;
  now?: () => number;
  newKey?: () => string;
  /** Минимальный интервал между автообновлениями по finish_at, мс. */
  minRefreshGapMs?: number;
}

interface Pending {
  id: number;
  op: Op;
  key: string;
}

export interface ControllerState {
  confirmed: World;
  view: World;
  pending: Pending[];
  notice: Notice | null;
  refreshing: boolean;
  ui: UiState;
}

export interface ArmyActions {
  refresh(): Promise<void>;
  selectTab(tab: Tab): void;
  selectBuilding(uid: string): void;
  setCount(unitId: string, n: number): void;
  setMax(unitId: string): void;
  /** Поставить заказ выбранному зданию (количество — из ui.counts). */
  train(unitId: string): Promise<void>;
  askCancel(key: string | null): void;
  cancelOrder(orderId: string, count?: number): Promise<void>;
  speedupQueue(buildingUid: string): Promise<void>;
  research(nodeId: string): Promise<void>;
  cancelResearch(): Promise<void>;
  speedupResearch(): Promise<void>;
  dismissNotice(): void;
}

export interface ArmyController {
  store: Store<ControllerState>;
  actions: ArmyActions;
  viewModel(): ArmyViewModel;
  subscribe(l: () => void): () => void;
  /** Серверное «сейчас» в секундах (server_time последнего ответа + прошедшее локально). */
  serverNowS(): number;
  /** Вызывать периодически (хук делает это раз в тик): если что-то дозрело по finish_at — обновить состояние. */
  checkDue(): boolean;
}

function noticeFrom(e: unknown): Notice {
  if (e instanceof ApiError) {
    const details = e.details && typeof e.details === 'object' ? (e.details as Record<string, unknown>) : {};
    const reason = typeof details.reason === 'string' ? details.reason : null;
    return { code: e.code, reason, message: e.message, details };
  }
  return { code: 'internal', reason: null, message: e instanceof Error ? e.message : String(e), details: {} };
}

export function createArmyController(o: ArmyControllerOptions): ArmyController {
  const now = o.now ?? Date.now;
  const newKey = o.newKey ?? (() => globalThis.crypto.randomUUID());
  const gap = o.minRefreshGapMs ?? 1000;
  const store = createStore<ControllerState>({
    confirmed: EMPTY_WORLD,
    view: EMPTY_WORLD,
    pending: [],
    notice: null,
    refreshing: false,
    ui: INITIAL_UI,
  });

  // Часы: от server_time последнего ответа.
  let clock: { serverS: number; atMs: number } | null = null;
  const serverNowS = (): number => (clock ? clock.serverS + Math.floor((now() - clock.atMs) / 1000) : Math.floor(now() / 1000));
  const syncClock = (serverTime: string | undefined): void => {
    const ms = serverTime ? Date.parse(serverTime) : Number.NaN;
    if (!Number.isNaN(ms)) clock = { serverS: Math.floor(ms / 1000), atMs: now() };
  };
  const ctx = (): ApplyCtx => ({ catalog: o.catalog, nowS: serverNowS() });

  const project = (confirmed: World, pending: readonly Pending[]): World => pending.reduce<World>((w, p) => applyOp(w, p.op, ctx()), confirmed);
  const setConfirmed = (patch: (w: World) => World): void => {
    const s = store.getState();
    const confirmed = patch(s.confirmed);
    store.setState({ confirmed, view: project(confirmed, s.pending) });
  };
  const patchUi = (p: Partial<UiState>): void => store.setState((s) => ({ ui: { ...s.ui, ...p } }));

  const applyArmy = (dto: ArmyStateDto): void => {
    syncClock(dto.server_time);
    setConfirmed((w) => ({ ...w, army: armyFromDto(dto), armyLoaded: true }));
  };
  const applyLab = (dto: LabStateDto): void => {
    syncClock(dto.server_time);
    setConfirmed((w) => ({ ...w, lab: labFromDto(dto, w.lab), labLoaded: true }));
  };

  async function refreshArmy(): Promise<void> {
    applyArmy(await o.api.fetchArmy());
  }
  async function refreshLab(): Promise<void> {
    applyLab(await o.api.fetchLab());
  }

  let lastRefreshMs = Number.NEGATIVE_INFINITY;
  let refreshing: Promise<void> | null = null;
  /** GET /v1/army и /v1/lab; лаборатория может быть ещё не подключена на сервере — тогда её отсутствие не ошибка экрана армии. */
  function refresh(): Promise<void> {
    refreshing ??= (async () => {
      lastRefreshMs = now();
      store.setState({ refreshing: true });
      try {
        const [a, l] = await Promise.allSettled([refreshArmy(), refreshLab()]);
        if (a.status === 'rejected') store.setState({ notice: noticeFrom(a.reason) });
        else if (l.status === 'rejected' && !(l.reason instanceof ApiError && (l.reason.status === 404 || l.reason.status === 405))) {
          store.setState({ notice: noticeFrom(l.reason) });
        }
        if (l.status === 'rejected') setConfirmed((w) => ({ ...w, labLoaded: true }));
      } finally {
        refreshing = null;
        store.setState({ refreshing: false });
      }
    })();
    return refreshing;
  }

  // --- Команды: очередь по одной, откат только своей ---
  let seq = 0;
  let chain: Promise<void> = Promise.resolve();

  function exec(op: Op, call: (key: string) => Promise<void>, alsoRefreshArmy = false): Promise<void> {
    const p: Pending = { id: ++seq, op, key: newKey() };
    const s = store.getState();
    const pending = [...s.pending, p];
    store.setState({ pending, view: project(s.confirmed, pending), notice: null });
    const run = async (): Promise<void> => {
      let failed = false;
      try {
        await call(p.key);
        if (alsoRefreshArmy) await refreshArmy().catch(() => undefined);
      } catch (e) {
        failed = true;
        store.setState({ notice: noticeFrom(e) });
      } finally {
        const cur = store.getState();
        const rest = cur.pending.filter((x) => x.id !== p.id);
        store.setState({ pending: rest, view: project(cur.confirmed, rest) });
      }
      // Отказ: локальные предположения могли разойтись с сервером — берём авторитетное состояние.
      if (failed) await Promise.allSettled([refreshArmy(), refreshLab()]);
      o.onSettled?.();
    };
    chain = chain.then(run, run);
    return chain;
  }

  const actions: ArmyActions = {
    refresh,
    selectTab: (tab) => patchUi({ tab, confirm: null }),
    selectBuilding: (uid) => patchUi({ buildingUid: uid, confirm: null }),
    setCount(unitId, n) {
      const v = Number.isFinite(n) ? Math.max(0, Math.trunc(n)) : 0;
      patchUi({ counts: { ...store.getState().ui.counts, [unitId]: v } });
    },
    setMax(unitId) {
      const unit = selectViewModel(vmInput()).train.units.find((x) => x.id === unitId);
      if (unit) patchUi({ counts: { ...store.getState().ui.counts, [unitId]: Math.max(unit.maxCount, unit.group) } });
    },
    train(unitId) {
      const vm = selectViewModel(vmInput()).train;
      const unit = vm.units.find((x) => x.id === unitId);
      const building = vm.selectedUid;
      if (!unit || !building || unit.blocked !== null) return Promise.resolve();
      const op: Op = { kind: 'train', buildingId: building, unitId, count: unit.count, localId: `local:${seq + 1}` };
      return exec(op, async (key) => applyArmy(await o.api.train({ building_id: building, unit_id: unitId, count: unit.count }, key)));
    },
    askCancel: (key) => patchUi({ confirm: key }),
    cancelOrder(orderId, count) {
      patchUi({ confirm: null });
      return exec({ kind: 'cancel', orderId, count }, async (key) => applyArmy(await o.api.cancel(count === undefined ? { order_id: orderId } : { order_id: orderId, count }, key)));
    },
    speedupQueue: (buildingUid) =>
      exec({ kind: 'speedup', buildingId: buildingUid }, async (key) => applyArmy(await o.api.speedup({ building_id: buildingUid }, key))),
    research: (nodeId) =>
      exec({ kind: 'research', nodeId }, async (key) => applyLab(await o.api.research({ node_id: nodeId }, key))),
    cancelResearch() {
      patchUi({ confirm: null });
      return exec({ kind: 'cancelResearch' }, async (key) => applyLab(await o.api.cancelResearch(key)));
    },
    // Ускорение исследования мгновенно повышает уровень юнитов — подтягиваем армию (уровни и перки).
    speedupResearch: () => exec({ kind: 'speedupResearch' }, async (key) => applyLab(await o.api.speedupResearch(key)), true),
    dismissNotice: () => store.setState({ notice: null }),
  };

  function vmInput() {
    const s = store.getState();
    const nowS = serverNowS();
    const world: World = { ...s.view, army: settleArmy(s.view.army, nowS, s.view.lab.done, o.catalog) };
    let wallet = o.wallet?.();
    if (wallet) {
      const spent: Record<string, number> = {};
      for (const p of s.pending) for (const [r, n] of Object.entries(opSpend(s.confirmed, p.op, { catalog: o.catalog, nowS }))) spent[r] = (spent[r] ?? 0) + n;
      wallet = Object.fromEntries(Object.entries(wallet).map(([r, n]) => [r, n - (spent[r] ?? 0)]));
    }
    return { world, ui: s.ui, notice: s.notice, busy: s.pending.length > 0 || s.refreshing, catalog: o.catalog, buildings: o.buildings(), wallet, nowS };
  }
  function checkDue(): boolean {
    const s = store.getState();
    if (refreshing || s.pending.length > 0 || now() - lastRefreshMs < gap) return false;
    const t = serverNowS();
    const c = s.confirmed;
    const dueArmy = c.army.queues.some((q) => q.orders[0] !== undefined && q.orders[0].unit_finish_at <= t);
    const dueLab = c.lab.active !== null && c.lab.active.finishAt <= t;
    if (!dueArmy && !dueLab) return false;
    void refresh().then(() => (dueLab ? refreshArmy() : undefined)).catch(() => undefined);
    return true;
  }

  return {
    store,
    actions,
    viewModel: () => selectViewModel(vmInput()),
    subscribe: (l) => store.subscribe(l),
    serverNowS,
    checkDue,
  };
}
