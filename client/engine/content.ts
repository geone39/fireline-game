import { parse } from 'yaml';
import type { Footprints, Rect } from './base-types';

export interface BaseGridConfig {
  size: number;
  defaultMap: string;
  maps: Readonly<Record<string, readonly Rect[]>>;
}

type Obj = Record<string, unknown>;

function isObj(v: unknown): v is Obj {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** Разбор content/base_grid.yaml. Сетка должна быть квадратной. */
export function parseBaseGrid(text: string): BaseGridConfig {
  const doc: unknown = parse(text);
  if (!isObj(doc) || !isObj(doc.grid) || !isObj(doc.maps)) throw new Error('base_grid: bad document');
  const { w, h } = doc.grid;
  if (typeof w !== 'number' || w !== h) throw new Error('base_grid: grid must be square');
  const maps: Record<string, Rect[]> = {};
  for (const [id, m] of Object.entries(doc.maps)) {
    if (!isObj(m) || !Array.isArray(m.water)) throw new Error(`base_grid: bad map ${id}`);
    maps[id] = m.water.map((r: unknown) => {
      if (!Array.isArray(r) || r.length !== 4) throw new Error(`base_grid: bad water rect in ${id}`);
      const [x, y, rw, rh] = r as number[];
      return { x: x as number, y: y as number, w: rw as number, h: rh as number };
    });
  }
  const defaultMap = typeof doc.default_map === 'string' ? doc.default_map : Object.keys(maps)[0];
  if (defaultMap === undefined || !(defaultMap in maps)) throw new Error('base_grid: bad default_map');
  return { size: w, defaultMap, maps };
}

/** Собирает размеры футпринтов из buildings.yaml / defences.yaml: любые записи с `id` и `size: [w,h]`. */
export function parseFootprints(...texts: string[]): Footprints {
  const out: Record<string, readonly [number, number]> = {};
  const walk = (v: unknown): void => {
    if (Array.isArray(v)) {
      for (const e of v) walk(e);
    } else if (isObj(v)) {
      if (typeof v.id === 'string' && Array.isArray(v.size) && v.size.length === 2) {
        out[v.id] = [Number(v.size[0]), Number(v.size[1])];
      } else {
        for (const e of Object.values(v)) walk(e);
      }
    }
  };
  for (const t of texts) walk(parse(t));
  return out;
}
