import { describe, expect, it } from 'vitest';
import { validatePlacement } from './validate';
import { catalog, coastalGrid, grid, item, startItems } from './test-kit';

const ctx = { catalog, grid };
const items = startItems();

describe('validatePlacement (только подсветка)', () => {
  it('свободное место — ok, все клетки зелёные', () => {
    const p = validatePlacement(ctx, items, { uid: 'n', type: 'b_vault', x: 30, y: 30, rot: 0 }, 'build');
    expect(p.ok).toBe(true);
    expect(p.tiles).toHaveLength(9); // 3×3 из buildings.yaml
    expect(p.tiles.every((t) => t.ok)).toBe(true);
  });

  it('пересечение краснит только занятые клетки', () => {
    const p = validatePlacement(ctx, items, { uid: 'n', type: 'b_vault', x: 18, y: 18, rot: 0 }, 'build'); // 18..20 касается HQ (20..23) одной клеткой
    expect(p.reason).toBe('overlap');
    expect(p.tiles.filter((t) => !t.ok).map((t) => `${t.x},${t.y}`)).toEqual(['20,20']);
  });

  it('границы сетки', () => {
    const p = validatePlacement(ctx, items, { uid: 'n', type: 'b_vault', x: grid.size - 1, y: 0, rot: 0 }, 'build');
    expect(p.reason).toBe('out_of_bounds');
    expect(validatePlacement(ctx, items, { uid: 'n', type: 'd_wall', x: -1, y: 0, rot: 0 }, 'build').ok).toBe(false);
  });

  it('вода и верфь «у берега»', () => {
    const water = { catalog, grid: coastalGrid };
    expect(validatePlacement(water, items, { uid: 'n', type: 'd_wall', x: 41, y: 5, rot: 0 }, 'build').reason).toBe('on_water');
    // верфь разблокируется на HQ 4: для проверки берега поднимаем уровень штаба в раскладке
    const hq4 = [item('hq', 'b_hq', 20, 20, 0, 4)];
    expect(validatePlacement(water, hq4, { uid: 'n', type: 'b_shipyard', x: 30, y: 5, rot: 0 }, 'build').reason).toBe('needs_water');
    expect(validatePlacement(water, hq4, { uid: 'n', type: 'b_shipyard', x: 36, y: 5, rot: 0 }, 'build').ok).toBe(true); // 36..39, вода с x=40
  });

  it('разблокировка и лимит по уровню штаба', () => {
    expect(validatePlacement(ctx, items, { uid: 'n', type: 'b_radar', x: 30, y: 30, rot: 0 }, 'build').reason).toBe('locked');
    // у b_cash_mine на HQ1 лимит из buildings.yaml; ставим столько же, сколько уже стоит + 1
    const max = catalog.maxCount(catalog.get('b_cash_mine')!, 1)!;
    const full = [item('hq', 'b_hq', 20, 20)];
    for (let k = 0; k < max; k++) full.push(item(`m${k}`, 'b_cash_mine', 2 + k * 4, 2));
    expect(validatePlacement(ctx, full, { uid: 'n', type: 'b_cash_mine', x: 2, y: 30, rot: 0 }, 'build').reason).toBe('limit');
  });

  it('нет штаба — build невозможен', () => {
    expect(validatePlacement(ctx, [], { uid: 'n', type: 'd_wall', x: 1, y: 1, rot: 0 }, 'build').reason).toBe('no_hq');
  });

  it('move: сам с собой не пересекается, препятствия не двигаются, rot 0..3', () => {
    expect(validatePlacement(ctx, items, { uid: 'cm1', type: 'b_cash_mine', x: 15, y: 20, rot: 0 }, 'move').ok).toBe(true);
    expect(validatePlacement(ctx, items, { uid: 'rock', type: 'o_rock', x: 9, y: 9, rot: 0 }, 'move').reason).toBe('not_movable');
    expect(validatePlacement(ctx, items, { uid: 'cm1', type: 'b_cash_mine', x: 15, y: 20, rot: 7 }, 'move').reason).toBe('bad_rot');
    expect(validatePlacement(ctx, items, { uid: 'cm1', type: 'b_cash_mine', x: 19, y: 20, rot: 0 }, 'move').reason).toBe('overlap');
  });

  it('неизвестный тип', () => {
    expect(validatePlacement(ctx, items, { uid: 'n', type: 'zzz', x: 1, y: 1, rot: 0 }, 'build').reason).toBe('unknown_type');
  });
});
