// Слайс базы: GameStore<BaseWorld> (оптимистичная очередь + откат из client/core/store.ts) и команды BaseCommand.
// Сервер авторитарен: оптимистичное применение только прячет задержку, подтверждённое состояние приходит в ответе.
import { createGameStore, type Command, type GameStore } from '../store';
import type { Catalog, Pricing } from './catalog';
import { EMPTY_WORLD, toWorld, type BaseApi, type BaseItem, type BaseStateDto, type BaseTimer, type BaseWorld } from './types';

export interface BaseModelOptions {
  api: BaseApi;
  catalog: Catalog;
  /** Цены нужны только для локального таймера в оптимистичном состоянии; без них таймер не выдумываем. */
  pricing?: Pricing;
  now?: () => number;
  newKey?: () => string;
  newUid?: () => string;
  initial?: BaseWorld;
}

const str = (v: unknown): string => (typeof v === 'string' ? v : '');
const int = (v: unknown, d = 0): number => (typeof v === 'number' && Number.isFinite(v) ? Math.trunc(v) : d);

export const LOCAL_TIMER_PREFIX = 'local:';

/** Чистая функция оптимистичного применения BaseCommand. Не применимое — возвращает мир как есть. */
export function applyBaseCommand(world: BaseWorld, cmd: Command, o: { catalog: Catalog; pricing?: Pricing; now: number }): BaseWorld {
  const a = cmd.args;
  const uid = str(a.uid);
  const idx = world.items.findIndex((i) => i.uid === uid);
  const timerOf = (u: string): BaseTimer | undefined => world.timers.find((t) => t.target_id === u);
  const withoutTimer = (u: string): Pick<BaseWorld, 'timers' | 'builders'> => {
    const had = timerOf(u);
    return {
      timers: world.timers.filter((t) => t.target_id !== u),
      builders: had ? { ...world.builders, busy: Math.max(0, world.builders.busy - 1) } : world.builders,
    };
  };
  const startTimer = (kind: string, u: string, type: string, to: number): Pick<BaseWorld, 'timers' | 'builders'> => {
    if (!o.pricing?.has(type)) return { timers: world.timers, builders: world.builders };
    const t: BaseTimer = {
      id: `${LOCAL_TIMER_PREFIX}${u}`,
      kind,
      target_id: u,
      finish_at: new Date(o.now + o.pricing.seconds(type, to) * 1000).toISOString(),
      to_level: to,
    };
    return { timers: [...world.timers, t], builders: { ...world.builders, busy: world.builders.busy + 1 } };
  };

  switch (cmd.op) {
    case 'build': {
      const type = str(a.type);
      if (!uid || idx >= 0 || !o.catalog.get(type)) return world;
      const item: BaseItem = { uid, type, level: 1, x: int(a.x), y: int(a.y), rot: int(a.rot) };
      return { ...world, items: [...world.items, item], ...startTimer('build', uid, type, 1) };
    }
    case 'move': {
      if (idx < 0) return world;
      const items = world.items.map((i, k) => (k === idx ? { ...i, x: int(a.x, i.x), y: int(a.y, i.y), rot: int(a.rot, i.rot) } : i));
      return { ...world, items };
    }
    case 'remove':
    case 'clear_obstacle': {
      if (idx < 0) return world;
      return { ...world, items: world.items.filter((i) => i.uid !== uid), ...withoutTimer(uid) };
    }
    case 'upgrade': {
      const it = world.items[idx];
      if (!it || timerOf(uid)) return world;
      return { ...world, ...startTimer('upgrade', uid, it.type, it.level + 1) };
    }
    case 'speedup': {
      const t = timerOf(uid);
      const it = world.items[idx];
      if (!t || !it) return world;
      const items = t.kind === 'upgrade' ? world.items.map((i) => (i.uid === uid ? { ...i, level: t.to_level } : i)) : world.items;
      return { ...world, items, ...withoutTimer(uid) };
    }
    case 'collect': {
      const collectors: Record<string, number> = {};
      for (const k of Object.keys(world.collectors)) collectors[k] = 0;
      return { ...world, collectors };
    }
  }
  return world;
}

export interface BaseCommands {
  build(type: string, x: number, y: number, rot?: number): Promise<string>;
  move(uid: string, x: number, y: number, rot: number): Promise<void>;
  upgrade(uid: string): Promise<void>;
  remove(uid: string): Promise<void>;
  collect(): Promise<void>;
  speedup(uid: string): Promise<void>;
  clearObstacle(uid: string): Promise<void>;
}

export interface BaseModel {
  store: GameStore<BaseWorld>;
  commands: BaseCommands;
  /** GET /v1/base → подтверждённое состояние. */
  load(): Promise<void>;
  /** Принять авторитетное состояние, полученное вне очереди команд (ответ PUT /v1/base/layout). */
  applyServerState(state: BaseStateDto): void;
  api: BaseApi;
}

export function createBaseModel(o: BaseModelOptions): BaseModel {
  const now = o.now ?? Date.now;
  const newUid = o.newUid ?? (() => `u_${globalThis.crypto.randomUUID()}`);
  const store = createGameStore<BaseWorld>({
    initial: o.initial ?? EMPTY_WORLD,
    apply: (w, c) => applyBaseCommand(w, c, { catalog: o.catalog, pricing: o.pricing, now: now() }),
    send: async (commands, key) => toWorld(await o.api.sendCommands(commands, key)),
    now,
    newKey: o.newKey,
  });
  /** Поставить команду и отправить; промис завершается, когда очередь пуста (ошибки сервера → lastRejection). */
  const run = (cmd: Command): Promise<void> => {
    store.enqueue(cmd);
    return store.flush();
  };
  const commands: BaseCommands = {
    async build(type, x, y, rot = 0) {
      const uid = newUid();
      await run({ op: 'build', args: { uid, type, x, y, rot } });
      return uid;
    },
    move: (uid, x, y, rot) => run({ op: 'move', args: { uid, x, y, rot } }),
    upgrade(uid) {
      const it = store.getState().view.items.find((i) => i.uid === uid);
      // to — целевой уровень: повтор команды после обрыва связи не купит второй апгрейд.
      return run({ op: 'upgrade', args: it ? { uid, to: it.level + 1 } : { uid } });
    },
    remove: (uid) => run({ op: 'remove', args: { uid } }),
    collect: () => run({ op: 'collect', args: {} }),
    speedup: (uid) => run({ op: 'speedup', args: { uid } }),
    clearObstacle: (uid) => run({ op: 'clear_obstacle', args: { uid } }),
  };
  return {
    store,
    commands,
    api: o.api,
    async load() {
      store.reset(toWorld(await o.api.fetchBase()));
    },
    applyServerState(state) {
      store.reset(toWorld(state));
    },
  };
}
