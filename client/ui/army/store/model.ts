// Чистая логика слайса: оптимистичное применение команд и ленивое «дозревание» очередей по finish_at
// (зеркало серверных правил army-lab.md §4.2, §4.4, §4.5, §5). Сервер авторитарен: подтверждённое состояние приходит в ответах.
import type { ArmyCatalog } from '../content';
import type { ArmyWorld, CountDto, OrderDto, PoolDto, QueueDto, World } from '../types';

export type Op =
  | { kind: 'train'; buildingId: string; unitId: string; count: number; localId: string }
  | { kind: 'cancel'; orderId: string; count?: number }
  | { kind: 'speedup'; buildingId: string }
  | { kind: 'research'; nodeId: string }
  | { kind: 'cancelResearch' }
  | { kind: 'speedupResearch' };

export interface ApplyCtx {
  catalog: ArmyCatalog;
  /** Серверное «сейчас», секунды. */
  nowS: number;
}

/** Когда будет готова вся очередь: голова (unit_finish_at) + оставшиеся юниты по train_s. */
export function queueEnd(q: Pick<QueueDto, 'orders'>, catalog: ArmyCatalog, fallbackNow: number): number {
  let end = fallbackNow;
  q.orders.forEach((o, i) => {
    const train = catalog.unit(o.unit_id)?.trainS ?? 0;
    if (i === 0) end = o.unit_finish_at + Math.max(0, o.remaining - 1) * train;
    else end += o.remaining * train;
  });
  return end;
}

function addCount(list: readonly CountDto[], id: string, n: number, level: number): CountDto[] {
  const has = list.some((c) => c.id === id);
  const out = list.map((c) => (c.id === id ? { ...c, count: c.count + n } : c));
  return has ? out : [...out, { id, count: n, level }];
}

function addToStock(a: ArmyWorld, id: string, n: number, done: readonly string[], catalog: ArmyCatalog): ArmyWorld {
  const isSpell = catalog.unit(id)?.kind === 'spell';
  const level = catalog.levelOf(id, done);
  return isSpell ? { ...a, spells: addCount(a.spells, id, n, level) } : { ...a, stock: addCount(a.stock, id, n, level) };
}

function withPool(a: ArmyWorld, pool: string, delta: number): ArmyWorld {
  const p: PoolDto | undefined = a.pools[pool];
  if (!p) return a;
  const used = Math.max(0, p.used + delta);
  return { ...a, pools: { ...a.pools, [pool]: { ...p, used, free: Math.max(p.capacity - used, 0) } } };
}

function findOrder(a: ArmyWorld, orderId: string): { q: QueueDto; o: OrderDto } | null {
  for (const q of a.queues) {
    const o = q.orders.find((x) => x.order_id === orderId);
    if (o) return { q, o };
  }
  return null;
}

function replaceQueue(a: ArmyWorld, q: QueueDto, orders: OrderDto[], ctx: ApplyCtx): ArmyWorld {
  if (orders.length === 0) return { ...a, queues: a.queues.filter((x) => x.building_id !== q.building_id) };
  const next: QueueDto = { ...q, orders, finish_at: queueEnd({ orders }, ctx.catalog, ctx.nowS) };
  const exists = a.queues.some((x) => x.building_id === q.building_id);
  return { ...a, queues: exists ? a.queues.map((x) => (x.building_id === q.building_id ? next : x)) : [...a.queues, next] };
}

/** Оптимистичное применение команды; неприменимое возвращает мир как есть (сервер всё равно решит сам). */
export function applyOp(w: World, op: Op, ctx: ApplyCtx): World {
  const { catalog, nowS } = ctx;
  switch (op.kind) {
    case 'train': {
      const u = catalog.unit(op.unitId);
      if (!u || op.count <= 0) return w;
      let army = withPool(w.army, u.pool, op.count * u.housing);
      const q = army.queues.find((x) => x.building_id === op.buildingId) ?? { building_id: op.buildingId, orders: [], finish_at: nowS };
      const start = q.orders.length === 0 ? nowS : queueEnd(q, catalog, nowS);
      const order: OrderDto = { order_id: op.localId, unit_id: op.unitId, count: op.count, remaining: op.count, unit_finish_at: start + u.trainS };
      army = replaceQueue(army, q, [...q.orders, order], ctx);
      return { ...w, army };
    }
    case 'cancel': {
      const hit = findOrder(w.army, op.orderId);
      if (!hit) return w;
      const k = Math.min(Math.max(1, op.count ?? hit.o.remaining), hit.o.remaining);
      const u = catalog.unit(hit.o.unit_id);
      const army = u ? withPool(w.army, u.pool, -k * u.housing) : w.army;
      const orders = hit.q.orders
        .map((o) => (o.order_id === op.orderId ? { ...o, remaining: o.remaining - k, count: Math.max(o.count - k, 0) } : o))
        .filter((o) => o.remaining > 0);
      return { ...w, army: replaceQueue(army, hit.q, orders, ctx) };
    }
    case 'speedup': {
      const q = w.army.queues.find((x) => x.building_id === op.buildingId);
      if (!q) return w;
      let army = w.army;
      for (const o of q.orders) army = addToStock(army, o.unit_id, o.remaining, w.lab.done, catalog);
      return { ...w, army: { ...army, queues: army.queues.filter((x) => x.building_id !== op.buildingId) } };
    }
    case 'research': {
      const n = catalog.node(op.nodeId);
      const tier = n ? catalog.tiers.get(n.tier) : undefined;
      if (!n || !tier || w.lab.active) return w;
      return { ...w, lab: { ...w.lab, active: { nodeId: n.id, startedAt: nowS, finishAt: nowS + tier.hours * 3600 } } };
    }
    case 'cancelResearch':
      return w.lab.active ? { ...w, lab: { ...w.lab, active: null } } : w;
    case 'speedupResearch': {
      const a = w.lab.active;
      if (!a) return w;
      const done = w.lab.done.includes(a.nodeId) ? w.lab.done : [...w.lab.done, a.nodeId];
      return { ...w, lab: { ...w.lab, active: null, done } };
    }
  }
}

/** Что команда оптимистично тратит (для предпроверки кошелька следующих): ресурсы за обучение, fuel за исследование, gems за ускорение. */
export function opSpend(w: World, op: Op, ctx: ApplyCtx): Record<string, number> {
  const { catalog, nowS } = ctx;
  const out: Record<string, number> = {};
  const add = (res: string, n: number): void => void (out[res] = (out[res] ?? 0) + n);
  switch (op.kind) {
    case 'train': {
      const u = catalog.unit(op.unitId);
      if (u) for (const [r, c] of Object.entries(u.cost)) add(r, c * op.count);
      break;
    }
    case 'research': {
      const n = catalog.node(op.nodeId);
      const tier = n ? catalog.tiers.get(n.tier) : undefined;
      if (tier) add('fuel', tier.fuel);
      break;
    }
    case 'speedup': {
      const q = w.army.queues.find((x) => x.building_id === op.buildingId);
      if (q) add('gems', catalog.speedupGems(q.finish_at - nowS));
      break;
    }
    case 'speedupResearch':
      if (w.lab.active) add('gems', catalog.speedupGems(w.lab.active.finishAt - nowS));
      break;
    default:
      break;
  }
  return out;
}

/**
 * Ленивый пересчёт очередей (army-lab.md §4.2): пока unit_finish_at ≤ now → юнит в запас, remaining−1,
 * следующий юнит стартует в момент окончания предыдущего (в том числе первый юнит следующего заказа).
 */
export function settleArmy(a: ArmyWorld, nowS: number, done: readonly string[], catalog: ArmyCatalog): ArmyWorld {
  let out = a;
  const queues: QueueDto[] = [];
  for (const q of a.queues) {
    const orders: OrderDto[] = [];
    // carry — момент, когда закончился предыдущий заказ: с него стартует следующий; null — голова ещё идёт.
    let carry: number | null = null;
    let open = true;
    for (const [i, o] of q.orders.entries()) {
      const train = catalog.unit(o.unit_id)?.trainS ?? 0;
      let { remaining, unit_finish_at: next } = o;
      let k = 0;
      if (open && train > 0) {
        if (i > 0 && carry !== null) next = carry + train;
        while (remaining > 0 && next <= nowS) {
          remaining -= 1;
          k += 1;
          next += train;
        }
        if (remaining === 0) carry = next - train;
        else open = false;
      } else open = false;
      if (k > 0) out = addToStock(out, o.unit_id, k, done, catalog);
      if (remaining > 0) orders.push(k > 0 || next !== o.unit_finish_at ? { ...o, remaining, unit_finish_at: next } : o);
    }
    const unchanged = orders.length === q.orders.length && orders.every((o, i) => o === q.orders[i]);
    if (orders.length > 0) queues.push(unchanged ? q : { ...q, orders });
  }
  const same = queues.length === a.queues.length && queues.every((q, i) => q === a.queues[i]);
  return same && out === a ? a : { ...out, queues };
}
