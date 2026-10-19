import { inGrid } from './picking';
import { footprintSize, type BattleCell, type BattleEntity, type BattleWorldState } from './battle-types';

/** Расстояние Чебышёва (диагональ = 1) — единая метрика боя (docs/design/battle-sim.md §2.2). */
export function cheb(a: BattleCell, b: BattleCell): number {
  return Math.max(Math.abs(a.x - b.x), Math.abs(a.y - b.y));
}

/** Клетки сетки с min ≤ cheb(центр, клетка) ≤ max. Центр не входит при min ≥ 1. */
export function rangeCells(
  range: { x: number; y: number; min: number; max: number },
  gridSize: number,
): BattleCell[] {
  const out: BattleCell[] = [];
  for (let y = range.y - range.max; y <= range.y + range.max; y++) {
    for (let x = range.x - range.max; x <= range.x + range.max; x++) {
      if (!inGrid(x, y, gridSize)) continue;
      const d = cheb(range, { x, y });
      if (d >= range.min && d <= range.max) out.push({ x, y });
    }
  }
  return out;
}

export function covers(e: BattleEntity, cx: number, cy: number): boolean {
  const { w, h } = footprintSize(e);
  return cx >= e.x && cx < e.x + w && cy >= e.y && cy < e.y + h;
}

function isUnit(e: BattleEntity): boolean {
  return e.kind === 'troop' || e.kind === 'vehicle' || e.kind === 'ship' || e.kind === 'hero';
}

/** Приоритет при выборе под пальцем: воздух > юниты > постройки/стены > ловушки. */
function pickPriority(e: BattleEntity): number {
  if (e.layer === 'air') return 4;
  if (isUnit(e)) return 3;
  if (e.kind === 'trap') return 1;
  return 2;
}

/** Видна ли сущность вообще: скрытая ловушка без `revealed` не рисуется и не выбирается. */
export function isVisible(e: BattleEntity): boolean {
  return e.kind !== 'trap' || e.revealed === true;
}

/** Сущность под клеткой (с учётом приоритета), либо undefined. Ничьи — по меньшему id. */
export function pickEntity(cx: number, cy: number, entities: readonly BattleEntity[]): BattleEntity | undefined {
  let best: BattleEntity | undefined;
  for (const e of entities) {
    if (!isVisible(e) || !covers(e, cx, cy)) continue;
    if (!best || pickPriority(e) > pickPriority(best) || (pickPriority(e) === pickPriority(best) && e.id < best.id)) {
      best = e;
    }
  }
  return best;
}

export type BattleTap =
  | { type: 'battleSelect'; id: number }
  | { type: 'battleTarget'; id: number; x: number; y: number }
  | { type: 'battleCell'; x: number; y: number };

/**
 * Что значит тап по клетке:
 *  - свой юнит -> battleSelect (кроме случая, когда он в highlight.targets выбранного — лечение -> battleTarget);
 *  - сущность обороны (кроме декора) -> battleTarget; декор и ловушки не цели -> battleCell;
 *  - иначе -> battleCell.
 * Допустимость хода/цели решает потребитель (ядро sim отвергнет недопустимое).
 */
export function routeTap(state: BattleWorldState, cx: number, cy: number): BattleTap | undefined {
  if (!inGrid(cx, cy, state.gridSize)) return undefined;
  const hit = pickEntity(cx, cy, state.entities);
  if (hit) {
    const targeted = state.highlight?.targets?.includes(hit.id) === true;
    if (hit.side === 'attacker') {
      if (targeted && hit.id !== state.selectedId) return { type: 'battleTarget', id: hit.id, x: cx, y: cy };
      return { type: 'battleSelect', id: hit.id };
    }
    if (hit.kind !== 'decor' && hit.kind !== 'trap') return { type: 'battleTarget', id: hit.id, x: cx, y: cy };
  }
  return { type: 'battleCell', x: cx, y: cy };
}

/** Порядок отрисовки: ключ глубины (ближняя клетка), юниты поверх построек той же глубины, воздух — выше всего. */
export function battleDepth(e: BattleEntity, at?: BattleCell): number {
  const { w, h } = footprintSize(e);
  const x = at?.x ?? e.x;
  const y = at?.y ?? e.y;
  const base = (x + w - 1 + (y + h - 1)) * 4;
  return base + (isUnit(e) ? 1 : 0) + (e.layer === 'air' ? 1_000_000 : 0);
}

export function sortBattle(entities: readonly BattleEntity[]): BattleEntity[] {
  return entities
    .filter(isVisible)
    .slice()
    .sort((a, b) => battleDepth(a) - battleDepth(b) || a.x - b.x || a.id - b.id);
}

/** Доля hp 0..1 (maxHp ≤ 0 -> 0). */
export function hpRatio(e: BattleEntity): number {
  return e.maxHp > 0 ? Math.min(1, Math.max(0, e.hp / e.maxHp)) : 0;
}

/** Полоску hp показываем у юнитов всегда, у построек — только после повреждения. */
export function showHpBar(e: BattleEntity): boolean {
  if (e.kind === 'decor' || e.kind === 'trap' || e.maxHp <= 0) return false;
  return isUnit(e) || e.hp < e.maxHp;
}
