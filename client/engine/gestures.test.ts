import { describe, expect, it } from 'vitest';
import { TAP_SLOP_PX } from './config';
import { GestureController, type Gesture } from './gestures';

function setup(canDrag = false) {
  const out: Gesture[] = [];
  return { out, g: new GestureController((e) => out.push(e), () => canDrag) };
}

describe('GestureController', () => {
  it('tap: down+up without leaving the slop radius', () => {
    const { g, out } = setup();
    g.down(1, 10, 10);
    g.move(1, 10 + TAP_SLOP_PX, 10);
    g.up(1, 10, 10);
    expect(out).toEqual([{ type: 'tap', sx: 10, sy: 10 }]);
  });
  it('pan beyond slop, no tap', () => {
    const { g, out } = setup();
    g.down(1, 0, 0);
    g.move(1, 20, 0);
    g.move(1, 30, 5);
    g.up(1, 30, 5);
    expect(out).toEqual([
      { type: 'pan', dx: 20, dy: 0 },
      { type: 'pan', dx: 10, dy: 5 },
    ]);
  });
  it('drag when canDragAt, ends at release point', () => {
    const { g, out } = setup(true);
    g.down(1, 5, 5);
    g.move(1, 50, 5);
    g.move(1, 60, 9);
    g.up(1, 60, 9);
    expect(out.map((e) => e.type)).toEqual(['dragStart', 'dragMove', 'dragMove', 'dragEnd']);
    expect(out[0]).toEqual({ type: 'dragStart', sx: 5, sy: 5 });
    expect(out[3]).toEqual({ type: 'dragEnd', sx: 60, sy: 9 });
  });
  it('cancel finishes a drag but never taps', () => {
    const { g, out } = setup(true);
    g.down(1, 0, 0);
    g.move(1, 30, 0);
    g.cancel(1);
    expect(out.at(-1)).toEqual({ type: 'dragEnd', sx: 30, sy: 0 });
    const s = setup();
    s.g.down(2, 0, 0);
    s.g.cancel(2);
    s.g.up(2, 0, 0);
    expect(s.out).toEqual([]);
  });
  it('hover only without pressed pointers', () => {
    const { g, out } = setup();
    g.move(1, 4, 4);
    g.down(1, 4, 4);
    g.move(2, 9, 9);
    expect(out).toEqual([{ type: 'hover', sx: 4, sy: 4 }]);
  });
  it('pinch zooms around the midpoint', () => {
    const { g, out } = setup();
    g.down(1, 0, 100);
    g.down(2, 100, 100);
    g.move(2, 200, 100);
    expect(out.find((e) => e.type === 'zoom')).toEqual({ type: 'zoom', factor: 2, sx: 100, sy: 100 });
  });
  it('wheel zoom direction', () => {
    const { g, out } = setup();
    g.wheel(-1, 3, 4, 1.1);
    g.wheel(1, 3, 4, 1.1);
    g.wheel(0, 3, 4, 1.1);
    expect(out).toEqual([
      { type: 'zoom', factor: 1.1, sx: 3, sy: 4 },
      { type: 'zoom', factor: 1 / 1.1, sx: 3, sy: 4 },
    ]);
  });
});
