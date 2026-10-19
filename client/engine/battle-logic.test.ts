import { describe, expect, it } from 'vitest';
import { cheb, hpRatio, pickEntity, rangeCells, routeTap, showHpBar, sortBattle } from './battle-logic';
import { sampleBattle } from './battle-fixture';
import type { BattleEntity } from './battle-types';
import { footprints, grid } from './test-content';

const state = () => sampleBattle(grid.size, footprints);
const byId = (s: ReturnType<typeof state>, id: number) => s.entities.find((e) => e.id === id) as BattleEntity;

describe('cheb / rangeCells', () => {
  it('uses Chebyshev metric (diagonal = 1)', () => {
    expect(cheb({ x: 0, y: 0 }, { x: 3, y: 3 })).toBe(3);
    expect(cheb({ x: 2, y: 5 }, { x: 4, y: 4 })).toBe(2);
  });
  it('ring between min and max, clipped to the grid', () => {
    const cells = rangeCells({ x: 5, y: 5, min: 1, max: 2 }, 44);
    expect(cells).toHaveLength(25 - 1);
    expect(cells.some((c) => c.x === 5 && c.y === 5)).toBe(false);
    expect(rangeCells({ x: 5, y: 5, min: 2, max: 2 }, 44)).toHaveLength(16);
    expect(rangeCells({ x: 0, y: 0, min: 1, max: 1 }, 44)).toHaveLength(3);
    expect(rangeCells({ x: 43, y: 43, min: 0, max: 1 }, 44)).toHaveLength(4);
  });
});

describe('pickEntity', () => {
  it('finds a building by any cell of its footprint', () => {
    const s = state();
    expect(pickEntity(22, 22, s.entities)?.id).toBe(1);
    expect(pickEntity(23, 23, s.entities)?.id).toBe(1);
    expect(pickEntity(24, 24, s.entities)).toBeUndefined();
  });
  it('unit wins over what it stands on; air wins over ground', () => {
    const s = state();
    const on = [...s.entities, { ...byId(s, 10), id: 99, x: 20, y: 20 }];
    expect(pickEntity(20, 20, on)?.id).toBe(99);
    const air = [...on, { ...byId(s, 13), id: 100, x: 20, y: 20 }];
    expect(pickEntity(20, 20, air)?.id).toBe(100);
  });
  it('hidden traps are not pickable, revealed ones are', () => {
    const s = state();
    expect(pickEntity(15, 22, s.entities)).toBeUndefined();
    expect(pickEntity(15, 21, s.entities)?.id).toBe(8);
  });
});

describe('routeTap', () => {
  it('own unit -> battleSelect', () => {
    expect(routeTap(state(), 11, 24)).toEqual({ type: 'battleSelect', id: 11 });
  });
  it('defence entity -> battleTarget, any footprint cell', () => {
    expect(routeTap(state(), 17, 25)).toEqual({ type: 'battleTarget', id: 3, x: 17, y: 25 });
  });
  it('empty cell and hidden trap cell -> battleCell', () => {
    expect(routeTap(state(), 12, 12)).toEqual({ type: 'battleCell', x: 12, y: 12 });
    expect(routeTap(state(), 15, 22)).toEqual({ type: 'battleCell', x: 15, y: 22 });
  });
  it('revealed trap is not a target', () => {
    expect(routeTap(state(), 15, 21)).toEqual({ type: 'battleCell', x: 15, y: 21 });
  });
  it('own unit listed in highlight.targets (heal) -> battleTarget, the selected one itself -> select', () => {
    const s = { ...state(), selectedId: 12, highlight: { targets: [10] } };
    expect(routeTap(s, 10, 22)).toEqual({ type: 'battleTarget', id: 10, x: 10, y: 22 });
    expect(routeTap({ ...s, highlight: { targets: [12] } }, 10, 20)).toEqual({ type: 'battleSelect', id: 12 });
  });
  it('outside the grid -> nothing', () => {
    expect(routeTap(state(), -1, 3)).toBeUndefined();
    expect(routeTap(state(), 3, 44)).toBeUndefined();
  });
});

describe('draw order and hp bars', () => {
  it('sorts back to front: walls before hq, air last, hidden traps skipped', () => {
    const order = sortBattle(state().entities).map((e) => e.id);
    expect(order.at(-1)).toBe(13);
    expect(order.indexOf(4)).toBeLessThan(order.indexOf(1));
    expect(order).not.toContain(9);
  });
  it('hp ratio is clamped; building bar only when damaged, unit bar always', () => {
    const s = state();
    expect(hpRatio(byId(s, 1))).toBeCloseTo(0.875);
    expect(hpRatio({ ...byId(s, 1), hp: -5 })).toBe(0);
    expect(hpRatio({ ...byId(s, 1), maxHp: 0 })).toBe(0);
    expect(showHpBar(byId(s, 2))).toBe(false);
    expect(showHpBar(byId(s, 1))).toBe(true);
    expect(showHpBar({ ...byId(s, 10), hp: byId(s, 10).maxHp })).toBe(true);
    expect(showHpBar(byId(s, 8))).toBe(false);
  });
});

describe('fixture highlight', () => {
  it('move cells are within 3 (Chebyshev) and never inside a building', () => {
    const s = state();
    const solid = s.entities.filter((e) => e.kind !== 'trap');
    for (const c of s.highlight?.moves ?? []) {
      expect(cheb({ x: 10, y: 22 }, c)).toBeLessThanOrEqual(3);
      expect(pickEntity(c.x, c.y, solid.filter((e) => e.side === 'defender'))).toBeUndefined();
    }
  });
});
