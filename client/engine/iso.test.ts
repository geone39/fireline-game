import { describe, expect, it } from 'vitest';
import { GRID_SIZE } from './config';
import { diamond, screenToTile, tileToScreen } from './iso';

describe('iso', () => {
  it('origin maps to origin', () => {
    expect(tileToScreen(0, 0)).toEqual({ x: 0, y: 0 });
  });
  it('roundtrips every tile of the grid', () => {
    for (let ty = 0; ty < GRID_SIZE; ty++) {
      for (let tx = 0; tx < GRID_SIZE; tx++) {
        const p = tileToScreen(tx, ty);
        const t = screenToTile(p.x, p.y);
        expect(t.x).toBeCloseTo(tx);
        expect(t.y).toBeCloseTo(ty);
      }
    }
  });
  it('diamond has four vertices', () => {
    expect(diamond(0, 0)).toHaveLength(8);
  });
});
