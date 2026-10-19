import { footprintOf, type BaseItem, type Footprints } from './base-types';

/** Здание, футпринт которого покрывает клетку (cx,cy); иначе undefined. */
export function pickItem(
  cx: number,
  cy: number,
  items: readonly BaseItem[],
  footprints: Footprints,
): BaseItem | undefined {
  for (const it of items) {
    const { w, h } = footprintOf(it, footprints);
    if (cx >= it.x && cx < it.x + w && cy >= it.y && cy < it.y + h) return it;
  }
  return undefined;
}

export function inGrid(cx: number, cy: number, size: number): boolean {
  return cx >= 0 && cy >= 0 && cx < size && cy < size;
}
