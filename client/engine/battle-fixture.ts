import type { Footprints } from './base-types';
import { cheb } from './battle-logic';
import { EVENT, type BattleCell, type BattleEntity, type BattleEvent, type BattleWorldState } from './battle-types';

// ТЕСТОВАЯ ФИКСТУРА для vitest и dev-страницы. hp/ap/дальности здесь — произвольные числа для проверки рендера,
// НЕ игровой баланс (баланс живёт в content/*.yaml). Размеры построек берутся из контента (footprints).

/** Тестовый бой: оборона в центре, атакующий слева; выбран стрелок (ap 2/2) с подсветкой ходов, дальности и целей. */
export function sampleBattle(gridSize: number, footprints: Footprints, water: BattleWorldState['water'] = []): BattleWorldState {
  const fp = (type: string): { w: number; h: number } => {
    const f = footprints[type] ?? [1, 1];
    return { w: f[0], h: f[1] };
  };
  const def = (id: number, type: string, kind: BattleEntity['kind'], x: number, y: number, hp: number, maxHp = hp): BattleEntity => ({
    id, side: 'defender', type, kind, x, y, ...fp(type), hp, maxHp,
  });
  const atk = (id: number, type: string, kind: BattleEntity['kind'], x: number, y: number, hp: number, ap: number, layer?: BattleEntity['layer']): BattleEntity => ({
    id, side: 'attacker', type, kind, x, y, hp, maxHp: hp, ap, maxAp: 2, ...(layer ? { layer } : {}),
  });
  const entities: BattleEntity[] = [
    def(1, 'b_hq', 'building', 20, 20, 700, 800),
    def(2, 'd_mg_nest', 'defence', 25, 20, 420),
    def(3, 'd_mortar', 'defence', 16, 24, 150, 600),
    def(4, 'd_wall', 'wall', 18, 18, 900),
    def(5, 'd_wall', 'wall', 19, 18, 900),
    def(6, 'd_wall', 'wall', 20, 18, 300, 900),
    def(7, 'd_wall', 'wall', 21, 18, 900),
    { id: 8, side: 'defender', type: 'd_mine', kind: 'trap', x: 15, y: 21, hp: 1, maxHp: 1, revealed: true },
    { id: 9, side: 'defender', type: 'd_mine', kind: 'trap', x: 15, y: 22, hp: 1, maxHp: 1 },
    atk(10, 'u_rifleman', 'troop', 10, 22, 90, 2),
    atk(11, 'v_tank', 'vehicle', 11, 24, 400, 1),
    atk(12, 'u_medic', 'troop', 10, 20, 60, 0),
    atk(13, 'u_drone', 'troop', 13, 21, 30, 2, 'air'),
  ];
  const blocked = (c: BattleCell) =>
    entities.some((e) => e.kind !== 'trap' && c.x >= e.x && c.x < e.x + (e.w ?? 1) && c.y >= e.y && c.y < e.y + (e.h ?? 1));
  const from = { x: 10, y: 22 };
  const moves: BattleCell[] = [];
  for (let y = from.y - 3; y <= from.y + 3; y++)
    for (let x = from.x - 3; x <= from.x + 3; x++)
      if ((x !== from.x || y !== from.y) && cheb(from, { x, y }) <= 3 && !blocked({ x, y })) moves.push({ x, y });
  return {
    gridSize,
    water,
    entities,
    selectedId: 10,
    highlight: { moves, range: { ...from, min: 1, max: 6 }, targets: [3, 8] },
  };
}

const cell = (x: number, y: number) => ({ X: x, Y: y });
const ev = (Kind: number, Actor: number, Target: number, x: number, y: number, Value = 0): BattleEvent => ({
  Kind, Actor, Target, Cell: cell(x, y), Value,
});

/** Тестовая серия: ход, выстрел, добивание, взрыв здания, срабатывание ловушки, высвобождение, конец хода. */
export function sampleEvents(): BattleEvent[] {
  return [
    ev(EVENT.moved, 10, 0, 13, 22, 3),
    ev(EVENT.attacked, 10, 3, 16, 24, 60),
    ev(EVENT.attacked, 11, 3, 16, 24, 90),
    ev(EVENT.destroyed, 11, 3, 16, 24),
    ev(EVENT.trapTriggered, 8, 10, 15, 21),
    ev(EVENT.healed, 12, 10, 13, 22, 20),
    ev(EVENT.turnEnded, 0, 0, 0, 0, 1),
  ];
}

/** Состояние после sampleEvents(): стрелок дошёл до (13,22), миномёт уничтожен, ловушка раскрыта, ap потрачены. */
export function sampleAfter(before: BattleWorldState): BattleWorldState {
  return {
    ...before,
    selectedId: 10,
    highlight: undefined,
    entities: before.entities
      .filter((e) => e.id !== 3)
      .map((e) =>
        e.id === 10 ? { ...e, x: 13, y: 22, ap: 0, hp: Math.min(e.maxHp, e.hp + 20) } : e.id === 8 ? { ...e, revealed: true } : e,
      ),
  };
}
