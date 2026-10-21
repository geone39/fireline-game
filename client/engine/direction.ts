import { TILE_HEIGHT, TILE_WIDTH } from './config';

// Направления спрайтов. Нумерация art (OQ-ART1-1): 0 = лицом к камере (вниз по экрану), далее по часовой стрелке на
// экране: 1 вниз-влево, 2 влево, 3 вверх-влево, 4 вверх (от камеры), 5 вверх-вправо, 6 вправо, 7 вниз-вправо.

/** Число секторов «канонического» направления (юниты с 8 направлениями). */
export const FACINGS = 8;

const cache = new Map<number, number>();
const CACHE_RANGE = 512;

/** Сектор 0..7 экранного вектора движения, заданного в тайловых координатах (dx, dy). null — нулевой вектор. */
export function facingOf(dx: number, dy: number): number | null {
  if (dx === 0 && dy === 0) return null;
  const integer = Number.isInteger(dx) && Number.isInteger(dy) && Math.abs(dx) < CACHE_RANGE && Math.abs(dy) < CACHE_RANGE;
  const key = (dx + CACHE_RANGE) * (2 * CACHE_RANGE) + (dy + CACHE_RANGE);
  if (integer) {
    const hit = cache.get(key);
    if (hit !== undefined) return hit;
  }
  const sx = ((dx - dy) * TILE_WIDTH) / 2;
  const sy = ((dx + dy) * TILE_HEIGHT) / 2;
  // Угол от «вниз» по часовой стрелке на экране (y вниз): вниз = 0, влево = 90°, вверх = 180°, вправо = 270°.
  const deg = (Math.atan2(-sx, sy) * 180) / Math.PI;
  const f = mod(Math.round(deg / (360 / FACINGS)), FACINGS);
  if (integer) cache.set(key, f);
  return f;
}

/** Сектор 0..7 -> индекс направления набора с `dirs` направлениями (1, 4 или 8; другие — пропорционально). */
export function toClipDir(facing: number, dirs: number): number {
  if (dirs <= 1) return 0;
  if (dirs === FACINGS) return mod(facing, FACINGS);
  return mod(Math.round((mod(facing, FACINGS) * dirs) / FACINGS), dirs);
}

/** Направление набора из `dirs` направлений по вектору (dx, dy) в клетках; для нулевого вектора — `fallback`. */
export function directionIndex(dx: number, dy: number, dirs: number, fallback = 0): number {
  const f = facingOf(dx, dy);
  return f === null ? mod(fallback, Math.max(1, dirs)) : toClipDir(f, dirs);
}

function mod(a: number, n: number): number {
  return ((a % n) + n) % n;
}
