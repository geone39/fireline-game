// «Сервер» армии и лаборатории в памяти по правилам army-lab.md (фикстура F-ARMY-1, §9): для тестов и dev-страницы.
// Не зависит от node/DOM. Проверки упрощены: housing (§4.3 п.5), кошелёк, гейты лаборатории (§5.1), отмена, ускорение.
import { ApiError } from '../../core/api';
import type { ArmyCatalog } from './content';
import type { ArmyApi, ArmyStateDto, LabStateDto, OrderDto, QueueDto } from './types';

export const NOW_MS = Date.parse('2026-01-01T00:00:00Z');

/** Фикстура F-ARMY-1 (army-lab.md §9.1): unit 40 мест, занято 28 (12 стрелков + 2 снайпера + 3 штурмовика в очереди A). */
export function fixtureArmy(nowMs = NOW_MS): ArmyStateDto {
  const nowS = nowMs / 1000;
  return {
    stock: [
      { id: 'u_rifleman', count: 12, level: 1 },
      { id: 'u_sniper', count: 2, level: 1 },
    ],
    spells: [],
    deployed: [],
    pools: {
      unit: { capacity: 40, used: 28, free: 12, base: 40, perk_pct: 0 },
      garage: { capacity: 20, used: 0, free: 20 },
      spell: { capacity: 10, used: 0, free: 10 },
    },
    queues: [{ building_id: 'br1', orders: [{ order_id: 'o1', unit_id: 'u_brawler', count: 3, remaining: 3, unit_finish_at: nowS + 45 }], finish_at: nowS + 135 }],
    perks: {},
    server_time: new Date(nowMs).toISOString(),
  };
}

export function fixtureLab(nowMs = NOW_MS): LabStateDto {
  return { lab_level: 2, active: null, done: [], server_time: new Date(nowMs).toISOString() };
}

export type Wallet = Record<string, number>;

/** «Сервер» в памяти: проверки housing/кошелька/лаборатории по правилам §4.3, §5.1 (упрощённо) и подмена отказов. */
export interface FakeServer {
  api: ArmyApi;
  army: ArmyStateDto;
  lab: LabStateDto;
  wallet: Wallet;
  calls: { name: string; body?: unknown; key?: string }[];
  /** Следующий вызов мутации с этим именем упадёт с этой ошибкой. */
  failNext(name: string, e: ApiError): void;
  /** Остановить ответы мутаций до resolve() — чтобы увидеть оптимистичное состояние. */
  hold(): () => void;
}

export function createFakeServer(catalog: ArmyCatalog, init?: { army?: ArmyStateDto; lab?: LabStateDto; wallet?: Wallet; clock?: () => number }): FakeServer {
  const clock = init?.clock ?? Date.now;
  const s: FakeServer = {
    army: init?.army ?? fixtureArmy(clock()),
    lab: init?.lab ?? fixtureLab(clock()),
    wallet: init?.wallet ?? { fuel: 2000, cash: 0, alloy: 0, gems: 50 },
    calls: [],
    failNext: (name, e) => void fails.set(name, e),
    hold: () => {
      let release: () => void = () => undefined;
      gate = new Promise<void>((r) => (release = r));
      return () => {
        release();
        gate = null;
      };
    },
    api: null as unknown as ArmyApi,
  };
  const fails = new Map<string, ApiError>();
  let gate: Promise<void> | null = null;
  let seq = 0;
  const nowS = (): number => Math.floor(clock() / 1000);
  const stamp = <T extends { server_time?: string }>(o: T): T => ({ ...o, server_time: new Date(clock()).toISOString() });
  const queueEnd = (q: QueueDto): number =>
    q.orders.reduce((t, o, i) => (i === 0 ? o.unit_finish_at + (o.remaining - 1) * (catalog.unit(o.unit_id)?.trainS ?? 0) : t + o.remaining * (catalog.unit(o.unit_id)?.trainS ?? 0)), 0);

  async function mutate<T>(name: string, body: unknown, key: string, f: () => T): Promise<T> {
    s.calls.push({ name, body, key });
    if (gate) await gate;
    const e = fails.get(name);
    if (e) {
      fails.delete(name);
      throw e;
    }
    return f();
  }
  const bad = (code: string, status: number, reason: string, details: Record<string, unknown> = {}): ApiError =>
    new ApiError(code, status, reason, { reason, ...details });

  s.api = {
    fetchArmy: () => {
      s.calls.push({ name: 'fetchArmy' });
      return Promise.resolve(stamp(structuredClone(s.army)));
    },
    fetchLab: () => {
      s.calls.push({ name: 'fetchLab' });
      return Promise.resolve(stamp(structuredClone(s.lab)));
    },
    train: (req, key) =>
      mutate('train', req, key, () => {
        const u = catalog.unit(req.unit_id)!;
        const pool = s.army.pools[u.pool]!;
        const need = req.count * u.housing;
        if (need > pool.free) {
          throw bad('conflict', 409, 'housing_full', { pool: u.pool, capacity: pool.capacity, used: pool.used, free: pool.free, need, max_count: Math.floor(pool.free / u.housing) });
        }
        const cost = Object.entries(u.cost).map(([r, c]) => [r, c * req.count] as const);
        const missing = Object.fromEntries(cost.filter(([r, c]) => (s.wallet[r] ?? 0) < c));
        if (Object.keys(missing).length > 0) throw new ApiError('insufficient_resources', 422, 'not enough resources', { missing });
        for (const [r, c] of cost) s.wallet[r] = (s.wallet[r] ?? 0) - c;
        pool.used += need;
        pool.free -= need;
        const q = s.army.queues.find((x) => x.building_id === req.building_id) ?? (s.army.queues.push({ building_id: req.building_id, orders: [], finish_at: nowS() }), s.army.queues.at(-1)!);
        const start = q.orders.length === 0 ? nowS() : queueEnd(q);
        const order: OrderDto = { order_id: `srv${++seq}`, unit_id: req.unit_id, count: req.count, remaining: req.count, unit_finish_at: start + u.trainS };
        q.orders.push(order);
        q.finish_at = queueEnd(q);
        return stamp(structuredClone(s.army));
      }),
    cancel: (req, key) =>
      mutate('cancel', req, key, () => {
        const q = s.army.queues.find((x) => x.orders.some((o) => o.order_id === req.order_id));
        const o = q?.orders.find((x) => x.order_id === req.order_id);
        if (!q || !o) throw new ApiError('not_found', 404, 'order not found');
        const u = catalog.unit(o.unit_id)!;
        const pool = s.army.pools[u.pool]!;
        pool.used -= o.remaining * u.housing;
        pool.free += o.remaining * u.housing;
        q.orders = q.orders.filter((x) => x !== o);
        if (q.orders.length === 0) s.army.queues = s.army.queues.filter((x) => x !== q);
        return stamp(structuredClone(s.army));
      }),
    speedup: (req, key) =>
      mutate('speedup', req, key, () => {
        const q = s.army.queues.find((x) => x.building_id === req.building_id);
        if (!q) throw bad('invalid', 400, 'queue_empty');
        const gems = catalog.speedupGems(q.finish_at - nowS());
        if ((s.wallet.gems ?? 0) < gems) throw new ApiError('insufficient_resources', 422, 'gems', { missing: { gems } });
        s.wallet.gems = (s.wallet.gems ?? 0) - gems;
        for (const o of q.orders) {
          const row = s.army.stock.find((c) => c.id === o.unit_id);
          if (row) row.count += o.remaining;
          else s.army.stock.push({ id: o.unit_id, count: o.remaining, level: 1 });
        }
        s.army.queues = s.army.queues.filter((x) => x !== q);
        return stamp(structuredClone(s.army));
      }),
    research: (req, key) =>
      mutate('research', req, key, () => {
        const n = catalog.node(req.node_id);
        if (!n) throw bad('invalid', 400, 'unknown_node');
        if (n.lab > (s.lab.lab_level ?? 0)) throw bad('conflict', 409, 'lab_level', { need: n.lab, have: s.lab.lab_level ?? 0 });
        if (s.lab.active) throw bad('conflict', 409, 'lab_busy');
        const tier = catalog.tiers.get(n.tier)!;
        if ((s.wallet.fuel ?? 0) < tier.fuel) throw new ApiError('insufficient_resources', 422, 'fuel', { missing: { fuel: tier.fuel } });
        s.wallet.fuel = (s.wallet.fuel ?? 0) - tier.fuel;
        s.lab.active = { node_id: n.id, started_at: nowS(), finish_at: nowS() + tier.hours * 3600, paid: { fuel: tier.fuel } };
        return stamp(structuredClone(s.lab));
      }),
    cancelResearch: (key) =>
      mutate('cancelResearch', {}, key, () => {
        s.lab.active = null;
        return stamp(structuredClone(s.lab));
      }),
    speedupResearch: (key) =>
      mutate('speedupResearch', {}, key, () => {
        const a = s.lab.active;
        if (!a) throw bad('invalid', 400, 'no_research');
        const gems = catalog.speedupGems(Number(a.finish_at) - nowS());
        if ((s.wallet.gems ?? 0) < gems) throw new ApiError('insufficient_resources', 422, 'gems', { missing: { gems } });
        s.wallet.gems = (s.wallet.gems ?? 0) - gems;
        s.lab.done = [...s.lab.done, a.node_id];
        s.lab.active = null;
        const n = catalog.node(a.node_id);
        for (const c of [...s.army.stock, ...s.army.spells]) if (n?.item === c.id) c.level = catalog.levelOf(c.id, s.lab.done);
        return stamp(structuredClone(s.lab));
      }),
  };
  return s;
}

