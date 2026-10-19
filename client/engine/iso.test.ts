import { describe, expect, it } from 'vitest';
import { TILE_HEIGHT, TILE_WIDTH } from './config';
import { diamond, footprintCenter, footprintDiamond, screenToCell, screenToTile, tileToScreen } from './iso';
import { grid } from './test-content';

describe('iso', () => {
  it('origin maps to origin', () => {
    expect(tileToScreen(0, 0)).toEqual({ x: 0, y: 0 });
  });
  it('x axis goes right-down, y axis left-down', () => {
    expect(tileToScreen(1, 0)).toEqual({ x: TILE_WIDTH / 2, y: TILE_HEIGHT / 2 });
    expect(tileToScreen(0, 1)).toEqual({ x: -TILE_WIDTH / 2, y: TILE_HEIGHT / 2 });
  });
  it('roundtrips every tile of the grid from base_grid.yaml', () => {
    for (let ty = 0; ty < grid.size; ty++) {
      for (let tx = 0; tx < grid.size; tx++) {
        const p = tileToScreen(tx, ty);
        const t = screenToTile(p.x, p.y);
        expect(t.x).toBeCloseTo(tx);
        expect(t.y).toBeCloseTo(ty);
        expect(screenToCell(p.x, p.y)).toEqual({ x: tx, y: ty });
      }
    }
  });
  it('points inside a diamond pick that cell', () => {
    const p = tileToScreen(5, 7);
    expect(screenToCell(p.x + 10, p.y)).toEqual({ x: 5, y: 7 });
    expect(screenToCell(p.x, p.y - 10)).toEqual({ x: 5, y: 7 });
  });
  it('diamond has four vertices', () => {
    expect(diamond(0, 0)).toHaveLength(8);
  });
  it('footprint centre and outline', () => {
    expect(footprintCenter(10, 20, 1, 1)).toEqual({ x: 10, y: 20 });
    expect(footprintCenter(10, 20, 4, 2)).toEqual({ x: 11.5, y: 20.5 });
    const p = tileToScreen(3, 4);
    expect(footprintDiamond(3, 4, 1, 1)).toEqual(diamond(p.x, p.y));
  });
});
