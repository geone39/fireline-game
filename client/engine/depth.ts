import { footprintOf, type BaseItem, type Footprints } from './base-types';

/** Ключ глубины: сумма координат ближней (нижней на экране) клетки футпринта. Больше = ближе к камере. */
export function depthKey(item: BaseItem, footprints: Footprints): number {
  const { w, h } = footprintOf(item, footprints);
  return item.x + w - 1 + (item.y + h - 1);
}

function cmp(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

/** Порядок отрисовки сзади вперёд. Детерминирован: при равной глубине — по x, затем по uid. */
export function sortByDepth(items: readonly BaseItem[], footprints: Footprints): BaseItem[] {
  return items
    .map((item) => ({ item, key: depthKey(item, footprints) }))
    .sort((a, b) => a.key - b.key || a.item.x - b.item.x || cmp(a.item.uid, b.item.uid))
    .map((e) => e.item);
}
