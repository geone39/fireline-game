import { TAP_SLOP_PX } from './config';

/** Жесты в экранных координатах (px относительно канваса). */
export type Gesture =
  | { type: 'tap'; sx: number; sy: number }
  | { type: 'hover'; sx: number; sy: number }
  | { type: 'pan'; dx: number; dy: number }
  | { type: 'zoom'; factor: number; sx: number; sy: number }
  | { type: 'dragStart'; sx: number; sy: number }
  | { type: 'dragMove'; sx: number; sy: number }
  | { type: 'dragEnd'; sx: number; sy: number };

interface Pt {
  x: number;
  y: number;
}

type Mode = 'idle' | 'maybe' | 'pan' | 'drag' | 'pinch';

/** Конечный автомат указателей: тап / pan / drag выбранного объекта / pinch. Без DOM — тестируется в node. */
export class GestureController {
  private readonly pointers = new Map<number, Pt>();
  private mode: Mode = 'idle';
  private start: Pt = { x: 0, y: 0 };
  private last: Pt = { x: 0, y: 0 };
  private pinchDist = 0;

  constructor(
    private readonly emit: (g: Gesture) => void,
    /** Можно ли начать перетаскивание с этой экранной точки (под указателем выбранное здание). */
    private readonly canDragAt: (sx: number, sy: number) => boolean,
  ) {}

  down(id: number, sx: number, sy: number): void {
    if (this.mode === 'drag') return; // второй палец во время drag игнорируем
    this.pointers.set(id, { x: sx, y: sy });
    if (this.pointers.size === 1) {
      this.mode = 'maybe';
      this.start = { x: sx, y: sy };
      this.last = { x: sx, y: sy };
    } else if (this.pointers.size === 2) {
      this.mode = 'pinch';
      const [a, b] = [...this.pointers.values()] as [Pt, Pt];
      this.pinchDist = dist(a, b);
      this.last = mid(a, b);
    }
  }

  move(id: number, sx: number, sy: number): void {
    const p = this.pointers.get(id);
    if (!p) {
      if (this.pointers.size === 0) this.emit({ type: 'hover', sx, sy });
      return;
    }
    p.x = sx;
    p.y = sy;
    if (this.mode === 'pinch') {
      const [a, b] = [...this.pointers.values()] as [Pt, Pt];
      const m = mid(a, b);
      const d = dist(a, b);
      if (this.pinchDist > 0 && d > 0) this.emit({ type: 'zoom', factor: d / this.pinchDist, sx: m.x, sy: m.y });
      this.emit({ type: 'pan', dx: m.x - this.last.x, dy: m.y - this.last.y });
      this.pinchDist = d;
      this.last = m;
      return;
    }
    if (this.mode === 'maybe') {
      if (dist(this.start, p) <= TAP_SLOP_PX) return;
      if (this.canDragAt(this.start.x, this.start.y)) {
        this.mode = 'drag';
        this.emit({ type: 'dragStart', sx: this.start.x, sy: this.start.y });
        this.emit({ type: 'dragMove', sx, sy });
      } else {
        this.mode = 'pan';
        this.emit({ type: 'pan', dx: sx - this.start.x, dy: sy - this.start.y });
      }
      this.last = { x: sx, y: sy };
      return;
    }
    if (this.mode === 'pan') {
      this.emit({ type: 'pan', dx: sx - this.last.x, dy: sy - this.last.y });
    } else if (this.mode === 'drag') {
      this.emit({ type: 'dragMove', sx, sy });
    }
    this.last = { x: sx, y: sy };
  }

  up(id: number, sx: number, sy: number): void {
    if (!this.pointers.delete(id)) return;
    if (this.mode === 'maybe') this.emit({ type: 'tap', sx, sy });
    else if (this.mode === 'drag') this.emit({ type: 'dragEnd', sx, sy });
    if (this.mode === 'pinch' && this.pointers.size === 1) {
      // Оставшийся палец продолжает pan без скачка.
      const [rest] = [...this.pointers.values()] as [Pt];
      this.mode = 'pan';
      this.last = { x: rest.x, y: rest.y };
      return;
    }
    if (this.pointers.size === 0) this.mode = 'idle';
  }

  /** Системная отмена касания: drag завершается в последней точке, тап не срабатывает. */
  cancel(id: number): void {
    const p = this.pointers.get(id);
    if (!p) return;
    if (this.mode === 'drag') this.emit({ type: 'dragEnd', sx: p.x, sy: p.y });
    this.pointers.clear();
    this.mode = 'idle';
  }

  wheel(deltaY: number, sx: number, sy: number, step: number): void {
    if (deltaY === 0) return;
    this.emit({ type: 'zoom', factor: deltaY < 0 ? step : 1 / step, sx, sy });
  }
}

function dist(a: Pt, b: Pt): number {
  return Math.hypot(a.x - b.x, a.y - b.y);
}

function mid(a: Pt, b: Pt): Pt {
  return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
}
