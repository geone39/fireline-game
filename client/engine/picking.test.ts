import { describe, expect, it } from 'vitest';
import { mockBaseState } from './base-source';
import { inGrid, pickItem } from './picking';
import { footprints } from './test-content';

const items = mockBaseState().layout.items;

describe('picking', () => {
  it('finds the building covering any cell of its footprint', () => {
    for (const [x, y] of [[20, 20], [23, 23], [21, 22]] as const) {
      expect(pickItem(x, y, items, footprints)?.uid).toBe('hq');
    }
  });
  it('misses outside footprints', () => {
    expect(pickItem(24, 20, items, footprints)).toBeUndefined();
    expect(pickItem(19, 19, items, footprints)).toBeUndefined();
  });
  it('respects rotation', () => {
    const rotated = [{ uid: 'r', type: 'x', level: 1, x: 0, y: 0, rot: 1 }];
    const fp = { x: [3, 1] } as const;
    expect(pickItem(0, 2, rotated, fp)?.uid).toBe('r');
    expect(pickItem(2, 0, rotated, fp)).toBeUndefined();
  });
  it('inGrid bounds', () => {
    expect(inGrid(0, 0, 44)).toBe(true);
    expect(inGrid(43, 43, 44)).toBe(true);
    expect(inGrid(44, 0, 44)).toBe(false);
    expect(inGrid(-1, 5, 44)).toBe(false);
  });
});
