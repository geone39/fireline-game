// Клиентский каталог из content/*.yaml: размеры, лимиты по штабу, цены. Нужен только для подсветки и подписей —
// авторитетно всё считает сервер (AGENTS.md п.5).
import { parse } from 'yaml';
import type { Def, Grid, Kind, LimitStep, Rect } from './types';

type Obj = Record<string, unknown>;
const isObj = (v: unknown): v is Obj => typeof v === 'object' && v !== null && !Array.isArray(v);
const num = (v: unknown): number | null => (typeof v === 'number' ? v : null);

function numMap(v: unknown): Record<string, number> {
  const out: Record<string, number> = {};
  if (isObj(v)) for (const [k, x] of Object.entries(v)) if (typeof x === 'number') out[k] = x;
  return out;
}

function limitsOf(r: Obj): LimitStep[] | null {
  const t = isObj(r.max_by_hq) ? r.max_by_hq : isObj(r.max_count_by_hq) ? r.max_count_by_hq : null;
  if (!t) return null;
  return Object.entries(t)
    .map(([at, max]) => ({ at: Number(at), max: Number(max) }))
    .sort((a, b) => a.at - b.at);
}

export class Catalog {
  private readonly defs = new Map<string, Def>();
  constructor(defs: Def[]) {
    for (const d of defs) this.defs.set(d.id, d);
  }
  get(id: string): Def | undefined {
    return this.defs.get(id);
  }
  all(): Def[] {
    return [...this.defs.values()];
  }
  get hqId(): string | null {
    return this.all().find((d) => d.kind === 'core')?.id ?? null;
  }
  /** Максимум штук типа при данном уровне штаба; null — без лимита. HQ — ровно один. */
  maxCount(d: Def, hq: number): number | null {
    if (d.kind === 'core') return 1;
    if (!d.limits) return null;
    let n = 0;
    for (const s of d.limits) if (s.at <= hq) n = s.max;
    return n;
  }
  footprints(): Record<string, readonly [number, number]> {
    const out: Record<string, readonly [number, number]> = {};
    for (const d of this.defs.values()) out[d.id] = [d.w, d.h];
    return out;
  }
}

function parseSection(rows: unknown, kind: Kind, out: Def[]): void {
  if (!Array.isArray(rows)) return;
  for (const r of rows) {
    if (!isObj(r) || typeof r.id !== 'string') continue;
    const size = Array.isArray(r.size) ? r.size : [1, 1]; // ASSUMPTION как на сервере: нет size → 1×1
    out.push({
      id: r.id,
      kind,
      nameKey: typeof r.name_key === 'string' ? r.name_key : r.id,
      w: Number(size[0] ?? 1),
      h: Number(size[1] ?? 1),
      unlockHq: num(r.unlock_hq) ?? 0,
      requiresWater: r.requires_water === true,
      limits: limitsOf(r) as readonly LimitStep[] | null,
      levels: num(r.levels),
      range: num(r.range),
      minRange: num(r.min_range),
      auraR: num(r.aura_r),
      clearCost: numMap(r.clear_cost),
      reward: numMap(r.reward),
      produces: typeof r.produces === 'string' ? r.produces : null,
    });
  }
}

/** content/buildings.yaml + content/defences.yaml. */
export function parseCatalog(buildingsYaml: string, defencesYaml: string): Catalog {
  const b: unknown = parse(buildingsYaml);
  const d: unknown = parse(defencesYaml);
  if (!isObj(b) || !isObj(d)) throw new Error('catalog: bad content');
  const out: Def[] = [];
  parseSection(b.core, 'core', out);
  parseSection(b.resources, 'building', out);
  parseSection(b.military, 'building', out);
  parseSection(b.decor, 'decor', out);
  parseSection(b.obstacles, 'obstacle', out);
  parseSection(d.passive_obstacles, 'passive', out);
  parseSection(d.active, 'active', out);
  return new Catalog(out);
}

/** content/base_grid.yaml для выбранной карты (по умолчанию default_map). */
export function parseGrid(baseGridYaml: string, map?: string): Grid {
  const doc: unknown = parse(baseGridYaml);
  if (!isObj(doc) || !isObj(doc.grid) || !isObj(doc.maps)) throw new Error('grid: bad document');
  const { w, h } = doc.grid;
  if (typeof w !== 'number' || w !== h) throw new Error('grid: must be square');
  const id = map ?? (typeof doc.default_map === 'string' ? doc.default_map : Object.keys(doc.maps)[0]);
  const m = id === undefined ? undefined : doc.maps[id];
  if (!isObj(m) || !Array.isArray(m.water)) throw new Error(`grid: bad map ${String(id)}`);
  const water: Rect[] = m.water.map((r: unknown) => {
    const [x, y, rw, rh] = r as number[];
    return { x: x as number, y: y as number, w: rw as number, h: rh as number };
  });
  return { size: w, water };
}

export interface PriceEntry {
  cost: Record<string, number>;
  timeS: number;
  costGrowth: number;
  timeGrowth: number;
  maxLevel: number | null;
}

/** content/building_costs.yaml. Числа для показа: сервер списывает по тем же формулам и решает сам. */
export class Pricing {
  constructor(
    private readonly entries: ReadonlyMap<string, PriceEntry>,
    private readonly speedup: { factor: number; exponent: number },
  ) {}
  has(type: string): boolean {
    return this.entries.has(type);
  }
  cost(type: string, level: number): Record<string, number> {
    const e = this.entries.get(type);
    const out: Record<string, number> = {};
    if (!e) return out;
    for (const [res, c1] of Object.entries(e.cost)) out[res] = Math.ceil(c1 * Math.pow(e.costGrowth, level - 1));
    return out;
  }
  seconds(type: string, level: number): number {
    const e = this.entries.get(type);
    return e ? Math.ceil(e.timeS * Math.pow(e.timeGrowth, level - 1)) : 0;
  }
  maxLevel(d: Def): number {
    return d.levels ?? this.entries.get(d.id)?.maxLevel ?? 1;
  }
  speedupGems(secondsLeft: number): number {
    if (secondsLeft <= 0) return 0;
    return Math.ceil(Math.pow(secondsLeft / 3600, this.speedup.exponent) * this.speedup.factor);
  }
}

export function parsePricing(costsYaml: string): Pricing {
  const doc: unknown = parse(costsYaml);
  if (!isObj(doc) || !isObj(doc.buildings) || !isObj(doc.speedup_gems)) throw new Error('pricing: bad document');
  const cg = num(doc.cost_growth) ?? 1;
  const tg = num(doc.time_growth) ?? 1;
  const entries = new Map<string, PriceEntry>();
  for (const [id, e] of Object.entries(doc.buildings)) {
    if (!isObj(e)) continue;
    entries.set(id, {
      cost: numMap(e.cost),
      timeS: num(e.time_s) ?? 0,
      costGrowth: num(e.cost_growth) ?? cg,
      timeGrowth: num(e.time_growth) ?? tg,
      maxLevel: num(e.max_level),
    });
  }
  return new Pricing(entries, {
    factor: num(doc.speedup_gems.factor) ?? 0,
    exponent: num(doc.speedup_gems.exponent) ?? 1,
  });
}

/** content/strings/<lang>/names.yaml → { name_key: текст }. */
export function parseStrings(namesYaml: string): Record<string, string> {
  const doc: unknown = parse(namesYaml);
  return isObj(doc) && isObj(doc.strings) ? (doc.strings as Record<string, string>) : {};
}
