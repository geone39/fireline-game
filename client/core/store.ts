// Единый store (Zustand-стиль, без зависимости) + командная очередь
// с оптимистичным применением и откатом при ошибке сервера (docs/01-architecture.md §7).
import { ApiError } from './api';

export type Listener<S> = (state: S, prev: S) => void;

export interface Store<S> {
  getState(): S;
  setState(patch: Partial<S> | ((s: S) => Partial<S>)): void;
  subscribe(l: Listener<S>): () => void;
}

export function createStore<S extends object>(initial: S): Store<S> {
  let state = initial;
  const ls = new Set<Listener<S>>();
  return {
    getState: () => state,
    setState(patch) {
      const p = typeof patch === 'function' ? patch(state) : patch;
      const prev = state;
      state = { ...state, ...p };
      ls.forEach((l) => l(state, prev));
    },
    subscribe(l) {
      ls.add(l);
      return () => ls.delete(l);
    },
  };
}

/** Команда базы, как в POST /v1/base/commands. */
export interface Command {
  op: string;
  args: Record<string, unknown>;
}

export interface QueuedCommand extends Command {
  /** Локальный id команды. */
  id: number;
  queuedAt: number;
}

export interface GameState<W> {
  /** Состояние, подтверждённое сервером. */
  confirmed: W;
  /** Оптимистичное состояние = confirmed + pending (то, что видит UI). */
  view: W;
  pending: QueuedCommand[];
  syncing: boolean;
  /** Последняя отклонённая сервером команда (для показа пользователю). */
  lastRejection: { command: Command; code: string; message: string } | null;
  /** Есть команды, ожидающие синхронизации (оффлайн/ретрай). */
  offline: boolean;
}

export interface GameStoreOptions<W> {
  initial: W;
  /** Чистая функция оптимистичного применения; бросать нельзя — вернуть state как есть, если не применимо. */
  apply: (world: W, cmd: Command) => W;
  /** Отправка пакета. idempotencyKey стабилен для одного пакета при повторах. Возвращает авторитетное состояние, если сервер его отдал. */
  send: (commands: Command[], idempotencyKey: string) => Promise<W | void>;
  now?: () => number;
  newKey?: () => string;
  /** Максимальный возраст команды в очереди до отбрасывания (по умолчанию 5 мин, §7 «Оффлайн»). */
  maxPendingAgeMs?: number;
}

export interface GameStore<W> extends Store<GameState<W>> {
  /** Применить оптимистично и поставить в очередь. */
  enqueue(cmd: Command): void;
  /** Отправить очередь на сервер; безопасно вызывать многократно. */
  flush(): Promise<void>;
  /** Заменить подтверждённое состояние (например, после GET /v1/base). */
  reset(world: W): void;
}

const DEFAULT_MAX_AGE = 5 * 60_000;

export function createGameStore<W>(o: GameStoreOptions<W>): GameStore<W> {
  const now = o.now ?? Date.now;
  const newKey = o.newKey ?? (() => globalThis.crypto.randomUUID());
  const maxAge = o.maxPendingAgeMs ?? DEFAULT_MAX_AGE;
  const store = createStore<GameState<W>>({
    confirmed: o.initial,
    view: o.initial,
    pending: [],
    syncing: false,
    lastRejection: null,
    offline: false,
  });
  let seq = 0;
  /** Пакет в полёте: ключ фиксируется до окончательного ответа. */
  let inflight: { key: string; ids: Set<number> } | null = null;

  const project = (confirmed: W, pending: QueuedCommand[]) => pending.reduce<W>((w, c) => o.apply(w, c), confirmed);

  function enqueue(cmd: Command) {
    const q: QueuedCommand = { op: cmd.op, args: cmd.args, id: ++seq, queuedAt: now() };
    const s = store.getState();
    store.setState({ pending: [...s.pending, q], view: o.apply(s.view, q) });
  }

  async function flush() {
    if (store.getState().syncing) return;
    store.setState({ syncing: true });
    try {
      for (;;) {
        // Протухшие команды отбрасываем (с откатом view).
        const s0 = store.getState();
        const fresh = s0.pending.filter((c) => now() - c.queuedAt <= maxAge);
        if (fresh.length !== s0.pending.length) {
          const dropped = s0.pending.find((c) => now() - c.queuedAt > maxAge)!;
          store.setState({
            pending: fresh,
            view: project(s0.confirmed, fresh),
            lastRejection: { command: dropped, code: 'expired', message: 'sync timeout' },
          });
        }
        const { pending } = store.getState();
        if (pending.length === 0) {
          inflight = null;
          return;
        }
        // Повтор после сетевой ошибки шлёт тот же состав пакета с тем же ключом.
        if (!inflight) inflight = { key: newKey(), ids: new Set(pending.map((c) => c.id)) };
        const batch = pending.filter((c) => inflight!.ids.has(c.id));
        if (batch.length === 0) {
          inflight = null;
          continue;
        }
        const batchIds = inflight.ids;
        let authoritative: W | void;
        try {
          authoritative = await o.send(
            batch.map(({ op, args }) => ({ op, args })),
            inflight.key,
          );
        } catch (e) {
          if (e instanceof ApiError && e.retryable) {
            // Сервер недоступен: команды остаются в очереди, ключ сохраняется.
            store.setState({ offline: true });
            return;
          }
          // Отказ сервера: откатываем весь пакет (серверная обработка пакета атомарна).
          inflight = null;
          const cur = store.getState();
          const rest = cur.pending.filter((c) => !batchIds.has(c.id));
          const code = e instanceof ApiError ? e.code : 'internal';
          store.setState({
            pending: rest,
            view: project(cur.confirmed, rest),
            offline: false,
            lastRejection: { command: batch[0]!, code, message: e instanceof Error ? e.message : String(e) },
          });
          continue;
        }
        inflight = null;
        const cur = store.getState();
        const rest = cur.pending.filter((c) => !batchIds.has(c.id));
        const confirmed = authoritative !== undefined ? (authoritative as W) : project(cur.confirmed, batch);
        store.setState({ confirmed, pending: rest, view: project(confirmed, rest), offline: false });
      }
    } finally {
      store.setState({ syncing: false });
    }
  }

  return {
    ...store,
    enqueue,
    flush,
    reset(world) {
      const p = store.getState().pending;
      store.setState({ confirmed: world, view: project(world, p) });
    },
  };
}
