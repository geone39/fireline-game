// Клиентская копия контента армии и лаборатории (troops.yaml, research.yaml, army.yaml, building_costs.yaml).
// Для подписей, предпроверок и подсветки. Авторитетно всё считает сервер; чисел баланса в коде нет.
import { parse } from 'yaml';

export type UnitKind = 'unit' | 'vehicle' | 'ship' | 'spell';

export interface UnitDef {
  id: string;
  nameKey: string;
  kind: UnitKind;
  /** INF / VEH / AIR / SEA; у заклинаний — null. */
  cls: string | null;
  housing: number;
  trainS: number;
  cost: Readonly<Record<string, number>>;
  /** unlock.hq (0 — нет условия). */
  unlockHq: number;
  /** Условия по уровню производящего здания: { b_barracks: 3 }. */
  unlockBuilding: Readonly<Record<string, number>>;
  /** Размер отряда (кратность заказа), 1 — без группы. */
  group: number;
  pool: string;
  /** Тип производящего здания (b_barracks…). */
  producer: string;
}

export interface Tier {
  fuel: number;
  hours: number;
}

export interface ResearchNode {
  id: string;
  nameKey: string;
  /** Предмет (юнит или заклинание), null у перков. */
  item: string | null;
  perk: { key: string; value: number | boolean } | null;
  toLevel: number;
  tier: number;
  lab: number;
  requires: readonly string[];
  soon: boolean;
}

export interface ArmyCatalog {
  units: ReadonlyMap<string, UnitDef>;
  /** Порядок как в troops.yaml: troops, vehicles, ships, spells. */
  unitOrder: readonly string[];
  nodes: readonly ResearchNode[];
  tiers: ReadonlyMap<number, Tier>;
  /** Пулы housing в порядке показа. */
  pools: readonly string[];
  queueMaxOrders: number;
  labBuilding: string;
  /** Типы зданий-производителей. */
  producers: readonly string[];
  unit(id: string): UnitDef | undefined;
  node(id: string): ResearchNode | undefined;
  /** Юниты, которые обучает здание данного типа. */
  unitsOf(producerType: string): readonly UnitDef[];
  /** Уровень предмета по выполненным узлам (army-lab.md §2.1). */
  levelOf(item: string, done: readonly string[]): number;
  /** gems = ceil(hours^exp × factor) — та же функция, что у построек (building_costs.yaml). */
  speedupGems(secondsLeft: number): number;
}

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === 'object' && v !== null && !Array.isArray(v);
const num = (v: unknown, d = 0): number => (typeof v === 'number' && Number.isFinite(v) ? v : d);
const str = (v: unknown, d = ''): string => (typeof v === 'string' ? v : d);
const numMap = (v: unknown): Record<string, number> => {
  const out: Record<string, number> = {};
  if (isObj(v)) for (const [k, x] of Object.entries(v)) if (typeof x === 'number') out[k] = x;
  return out;
};

export function parseArmyCatalog(troopsYaml: string, researchYaml: string, armyYaml: string, costsYaml: string): ArmyCatalog {
  const troops: unknown = parse(troopsYaml);
  const research: unknown = parse(researchYaml);
  const army: unknown = parse(armyYaml);
  const costs: unknown = parse(costsYaml);
  if (!isObj(troops) || !isObj(research) || !isObj(army) || !isObj(costs)) throw new Error('army content: bad document');

  const poolByClass = isObj(army.pool_by_class) ? (army.pool_by_class as Record<string, string>) : {};
  const poolSpell = str(army.pool_spell, 'spell');
  const producersCfg = isObj(army.producers) ? (army.producers as Record<string, string>) : {};
  const poolBuilding = isObj(army.pool_building) ? (army.pool_building as Record<string, string>) : {};

  const units = new Map<string, UnitDef>();
  const order: string[] = [];
  const sections: [string, UnitKind][] = [['troops', 'unit'], ['vehicles', 'vehicle'], ['ships', 'ship'], ['spells', 'spell']];
  for (const [section, kind] of sections) {
    const list = troops[section];
    if (!Array.isArray(list)) continue;
    for (const raw of list) {
      if (!isObj(raw) || typeof raw.id !== 'string') continue;
      const cls = kind === 'spell' ? null : str(raw.class) || null;
      const unlock = isObj(raw.unlock) ? raw.unlock : {};
      const unlockBuilding: Record<string, number> = {};
      for (const [k, v] of Object.entries(unlock)) if (k !== 'hq' && typeof v === 'number') unlockBuilding[`b_${k}`] = v;
      units.set(raw.id, {
        id: raw.id,
        nameKey: str(raw.name_key, raw.id),
        kind,
        cls,
        housing: num(raw.housing, 1),
        trainS: num(raw.train_s),
        cost: numMap(raw.cost),
        unlockHq: num(unlock.hq),
        unlockBuilding,
        group: Math.max(1, num(raw.group, 1)),
        pool: kind === 'spell' ? poolSpell : (poolByClass[cls ?? ''] ?? ''),
        producer: kind === 'spell' ? str(producersCfg.spell) : str(producersCfg[cls ?? '']),
      });
      order.push(raw.id);
    }
  }

  const tiers = new Map<number, Tier>();
  if (isObj(research.tiers)) {
    for (const [k, v] of Object.entries(research.tiers)) if (isObj(v)) tiers.set(Number(k), { fuel: num(v.fuel), hours: num(v.hours) });
  }
  const nodes: ResearchNode[] = [];
  if (Array.isArray(research.nodes)) {
    for (const raw of research.nodes) {
      if (!isObj(raw) || typeof raw.id !== 'string') continue;
      const item = typeof raw.unit === 'string' ? raw.unit : typeof raw.spell === 'string' ? raw.spell : null;
      const perk = isObj(raw.perk) && typeof raw.perk.key === 'string' ? { key: raw.perk.key, value: raw.perk.value as number | boolean } : null;
      nodes.push({
        id: raw.id,
        nameKey: str(raw.name_key, raw.id),
        item,
        perk,
        toLevel: num(raw.to_level),
        tier: num(raw.tier),
        lab: num(raw.lab, 1),
        requires: Array.isArray(raw.requires) ? raw.requires.filter((x): x is string => typeof x === 'string') : [],
        soon: raw.soon === true,
      });
    }
  }

  const sg = isObj(costs.speedup_gems) ? costs.speedup_gems : {};
  const factor = num(sg.factor);
  const exponent = num(sg.exponent, 1);
  const queue = isObj(army.queue) ? army.queue : {};
  const poolsSeen: string[] = [];
  for (const p of [...Object.keys(poolBuilding)]) if (!poolsSeen.includes(p)) poolsSeen.push(p);
  const producers = [...new Set(Object.values(producersCfg))];

  return {
    units,
    unitOrder: order,
    nodes,
    tiers,
    pools: poolsSeen,
    queueMaxOrders: num(queue.max_orders_per_building, Number.POSITIVE_INFINITY),
    labBuilding: str(army.lab_building, 'b_laboratory'),
    producers,
    unit: (id) => units.get(id),
    node: (id) => nodes.find((n) => n.id === id),
    unitsOf: (type) => order.map((id) => units.get(id)!).filter((u) => u.producer === type),
    levelOf: (item, done) =>
      Math.max(1, ...nodes.filter((n) => n.item === item && done.includes(n.id)).map((n) => n.toLevel)),
    speedupGems: (secondsLeft) => (secondsLeft <= 0 ? 0 : Math.ceil(Math.pow(secondsLeft / 3600, exponent) * factor)),
  };
}
