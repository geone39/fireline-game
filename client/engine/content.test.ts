import { describe, expect, it } from 'vitest';
import { parseBaseGrid, parseFootprints } from './content';
import { footprints, grid } from './test-content';

describe('content loader', () => {
  it('reads grid size and maps from base_grid.yaml', () => {
    expect(grid.size).toBe(44);
    expect(grid.defaultMap).toBe('plain');
    expect(grid.maps.plain).toEqual([]);
    expect(grid.maps.coastal?.[0]).toEqual({ x: 40, y: 0, w: 4, h: 44 });
    expect(Object.keys(grid.maps).sort()).toEqual(['canyon', 'coastal', 'plain']);
  });
  it('rejects non-square grids', () => {
    expect(() => parseBaseGrid('grid: {w: 4, h: 5}\nmaps: {a: {water: []}}')).toThrow();
  });
  it('collects footprints from buildings and defences', () => {
    expect(footprints.b_hq).toEqual([4, 4]);
    expect(footprints.d_wall).toEqual([1, 1]);
    expect(footprints.d_mg_nest).toEqual([2, 2]);
    expect(footprints.o_wreck).toEqual([2, 2]);
  });
  it('parseFootprints ignores entries without size', () => {
    expect(parseFootprints('a:\n  - {id: x, size: [2,3]}\n  - {id: y}')).toEqual({ x: [2, 3] });
  });
});
