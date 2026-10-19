import { describe, expect, it } from 'vitest';
import type { BaseItem } from './base-types';
import { depthKey, sortByDepth } from './depth';

const fp = { big: [4, 4], small: [1, 1] } as const;
const mk = (uid: string, type: string, x: number, y: number, rot = 0): BaseItem => ({ uid, type, level: 1, x, y, rot });

describe('depth sorting', () => {
  it('nearer to the camera (larger x+y) is drawn later', () => {
    const a = mk('a', 'small', 5, 5);
    const b = mk('b', 'small', 6, 5);
    const c = mk('c', 'small', 1, 1);
    expect(sortByDepth([b, a, c], fp).map((i) => i.uid)).toEqual(['c', 'a', 'b']);
  });
  it('uses the front corner of the footprint', () => {
    expect(depthKey(mk('h', 'big', 0, 0), fp)).toBe(6);
    expect(sortByDepth([mk('h', 'big', 0, 0), mk('s', 'small', 5, 0)], fp).map((i) => i.uid)).toEqual(['s', 'h']);
  });
  it('rotation swaps footprint sides', () => {
    const fp2 = { bar: [3, 1] } as const;
    expect(depthKey(mk('r0', 'bar', 0, 0, 0), fp2)).toBe(2);
    expect(depthKey(mk('r1', 'bar', 0, 0, 1), fp2)).toBe(2);
    expect(depthKey(mk('r2', 'bar', 4, 0, 1), fp2)).toBe(6);
  });
  it('is deterministic for ties and does not mutate input', () => {
    const items = [mk('b', 'small', 3, 2), mk('a', 'small', 2, 3), mk('c', 'small', 3, 2)];
    const before = items.map((i) => i.uid);
    const sorted = sortByDepth(items, fp).map((i) => i.uid);
    expect(sorted).toEqual(['a', 'b', 'c']);
    expect(sortByDepth([...items].reverse(), fp).map((i) => i.uid)).toEqual(sorted);
    expect(items.map((i) => i.uid)).toEqual(before);
  });
});
