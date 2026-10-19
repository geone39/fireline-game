import { TILE_HEIGHT, TILE_WIDTH } from './config';

export interface Point {
  x: number;
  y: number;
}

/** Тайловые координаты -> экранные (центр верхней грани тайла). */
export function tileToScreen(tx: number, ty: number): Point {
  return {
    x: ((tx - ty) * TILE_WIDTH) / 2,
    y: ((tx + ty) * TILE_HEIGHT) / 2,
  };
}

/** Экранные -> тайловые (дробные). */
export function screenToTile(sx: number, sy: number): Point {
  const a = (2 * sx) / TILE_WIDTH;
  const b = (2 * sy) / TILE_HEIGHT;
  return { x: (a + b) / 2, y: (b - a) / 2 };
}

/** Вершины ромба тайла (плоский массив x,y) с центром в (cx, cy): верх, право, низ, лево. */
export function diamond(cx: number, cy: number): number[] {
  const w = TILE_WIDTH / 2;
  const h = TILE_HEIGHT / 2;
  return [cx, cy - h, cx + w, cy, cx, cy + h, cx - w, cy];
}
