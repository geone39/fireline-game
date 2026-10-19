import { TILE_HEIGHT, TILE_WIDTH } from './config';

export interface Point {
  x: number;
  y: number;
}

/** Тайловые координаты -> экранные (центр верхней грани тайла). Дробные значения допустимы. */
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

/** Целая клетка под точкой мира (центр клетки (i,j) = tileToScreen(i,j)). */
export function screenToCell(sx: number, sy: number): Point {
  const t = screenToTile(sx, sy);
  return { x: Math.round(t.x), y: Math.round(t.y) };
}

/** Вершины ромба тайла (плоский массив x,y) с центром в (cx, cy): верх, право, низ, лево. */
export function diamond(cx: number, cy: number): number[] {
  const w = TILE_WIDTH / 2;
  const h = TILE_HEIGHT / 2;
  return [cx, cy - h, cx + w, cy, cx, cy + h, cx - w, cy];
}

/** Центр прямоугольного футпринта (левый верхний тайл x,y; размер w×h) в тайловых координатах. */
export function footprintCenter(x: number, y: number, w: number, h: number): Point {
  return { x: x + (w - 1) / 2, y: y + (h - 1) / 2 };
}

/** Вершины ромба футпринта w×h (верх, право, низ, лево) в координатах мира. */
export function footprintDiamond(x: number, y: number, w: number, h: number): number[] {
  const top = tileToScreen(x - 0.5, y - 0.5);
  const right = tileToScreen(x + w - 0.5, y - 0.5);
  const bottom = tileToScreen(x + w - 0.5, y + h - 0.5);
  const left = tileToScreen(x - 0.5, y + h - 0.5);
  return [top.x, top.y, right.x, right.y, bottom.x, bottom.y, left.x, left.y];
}
