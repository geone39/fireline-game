// Предварительная клиентская проверка размещения — ТОЛЬКО для подсветки (зелёный/красный).
// Решает сервер (server/svc/base/validate.go); правила здесь повторяют его проверки build/move.
import type { Catalog } from './catalog';
import type { BaseItem, Grid } from './types';

export type PlacementReason =
  | 'unknown_type'
  | 'bad_rot'
  | 'out_of_bounds'
  | 'on_water'
  | 'overlap'
  | 'needs_water'
  | 'locked'
  | 'limit'
  | 'not_movable'
  | 'no_hq';

export interface TileCheck {
  x: number;
  y: number;
  ok: boolean;
}

export interface Placement {
  ok: boolean;
  reason: PlacementReason | null;
  tiles: TileCheck[];
}

export interface Candidate {
  uid: string;
  type: string;
  x: number;
  y: number;
  rot: number;
}

export interface PlacementCtx {
  catalog: Catalog;
  grid: Grid;
}

export function footprintSize(w: number, h: number, rot: number): { w: number; h: number } {
  return rot % 2 === 0 ? { w, h } : { w: h, h: w };
}

export function hqLevel(catalog: Catalog, items: readonly BaseItem[]): number {
  const id = catalog.hqId;
  return items.find((i) => i.type === id)?.level ?? 0;
}

function isWater(grid: Grid, x: number, y: number): boolean {
  return grid.water.some((r) => x >= r.x && x < r.x + r.w && y >= r.y && y < r.y + r.h);
}

/**
 * mode 'build' — новый объект (проверяются разблокировка и лимит по штабу); 'move' — существующий uid
 * (сам с собой не пересекается). items — раскладка без кандидата или с ним (для move его старое место игнорируется).
 */
export function validatePlacement(ctx: PlacementCtx, items: readonly BaseItem[], c: Candidate, mode: 'build' | 'move'): Placement {
  const def = ctx.catalog.get(c.type);
  if (!def) return { ok: false, reason: 'unknown_type', tiles: [] };
  const { w, h } = footprintSize(def.w, def.h, c.rot);
  const others = items.filter((i) => i.uid !== c.uid);
  const occupied = new Set<string>();
  for (const o of others) {
    const od = ctx.catalog.get(o.type);
    if (!od) continue;
    const s = footprintSize(od.w, od.h, o.rot);
    for (let dy = 0; dy < s.h; dy++) for (let dx = 0; dx < s.w; dx++) occupied.add(`${o.x + dx},${o.y + dy}`);
  }
  const tiles: TileCheck[] = [];
  const reasons: PlacementReason[] = [];
  const fail = (r: PlacementReason): void => {
    reasons.push(r);
  };
  if (!Number.isInteger(c.rot) || c.rot < 0 || c.rot > 3) fail('bad_rot');
  let touchesWater = false;
  for (let dy = 0; dy < h; dy++) {
    for (let dx = 0; dx < w; dx++) {
      const x = c.x + dx;
      const y = c.y + dy;
      let ok = true;
      if (x < 0 || y < 0 || x >= ctx.grid.size || y >= ctx.grid.size) {
        ok = false;
        fail('out_of_bounds');
      } else if (isWater(ctx.grid, x, y)) {
        ok = false;
        fail('on_water');
      } else if (occupied.has(`${x},${y}`)) {
        ok = false;
        fail('overlap');
      }
      if (def.requiresWater) {
        for (const [nx, ny] of [[x + 1, y], [x - 1, y], [x, y + 1], [x, y - 1]] as const) {
          if (isWater(ctx.grid, nx, ny)) touchesWater = true;
        }
      }
      tiles.push({ x, y, ok });
    }
  }
  if (def.requiresWater && !touchesWater) fail('needs_water');
  if (mode === 'move' && def.kind === 'obstacle') fail('not_movable');
  if (mode === 'build') {
    const hq = hqLevel(ctx.catalog, others);
    if (hq === 0) fail('no_hq');
    else if (hq < def.unlockHq) fail('locked');
    const max = ctx.catalog.maxCount(def, hq);
    if (max !== null && others.filter((i) => i.type === c.type).length + 1 > max) fail('limit');
  }
  const reason = reasons[0] ?? null;
  return { ok: reason === null, reason, tiles };
}
