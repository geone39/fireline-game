// Форма данных базы, нужная рендеру. Источник — BaseState (docs/03-api.md, раздел Base): layout.items.
export interface BaseItem {
  uid: string;
  type: string;
  level: number;
  x: number;
  y: number;
  rot: number;
}

/** Прямоугольник в тайлах: [x, y, w, h] из content/base_grid.yaml. */
export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** Размеры футпринта по id типа ([w,h] из content/buildings.yaml и defences.yaml). */
export type Footprints = Readonly<Record<string, readonly [number, number]>>;

/** Размер футпринта с учётом поворота (нечётный rot меняет w и h). Неизвестный тип -> 1×1. */
export function footprintOf(item: BaseItem, footprints: Footprints): { w: number; h: number } {
  const f = footprints[item.type] ?? [1, 1];
  return item.rot % 2 === 0 ? { w: f[0], h: f[1] } : { w: f[1], h: f[0] };
}
