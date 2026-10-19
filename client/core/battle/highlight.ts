// Подсветка ходов/дальности/целей и поиск пути — ТОЛЬКО для UI. Допустимость команды решает ядро (Apply/RejectCode);
// здесь приближение правил (спека §5): Чебышёв, стоимость шага, «лестничная» диагональ. Расхождение с ядром безопасно:
// ядро отклонит команду, а UI покажет код отказа.
import type { Cell } from '../../sim-wasm/api';
import { CLASS } from './rules';
import { cellInfo, covers, footDist, type WEntity, type World } from './world';

export interface Pt {
  x: number;
  y: number;
}

export const canAttack = (u: WEntity): boolean => (u.unit?.Dmg ?? 0) > 0 || (u.unit?.Burst ?? 0) > 0;

const NEIGHBOURS: Pt[] = [
  { x: -1, y: -1 }, { x: 0, y: -1 }, { x: 1, y: -1 }, { x: -1, y: 0 }, { x: 1, y: 0 }, { x: -1, y: 1 }, { x: 0, y: 1 }, { x: 1, y: 1 },
];

/** Стоимость входа в клетку: 1, для пехоты на замедляющей постройке ceil(100/(100-slow)) (спека §5.1). */
function stepCost(w: World, u: WEntity, x: number, y: number): number {
  if (u.cls !== CLASS.INF) return 1;
  for (const e of w.entities) {
    if (e.bld && e.alive && e.bld.SlowPct > 0 && covers(e, x, y)) {
      const d = 100 - e.bld.SlowPct;
      return Math.ceil(100 / d);
    }
  }
  return 1;
}

export interface Reach {
  /** Клетка -> стоимость пути. */
  cost: Map<number, number>;
  /** Клетка -> предыдущая клетка пути. */
  prev: Map<number, number>;
}

/** Достижимые клетки юнита за один ход (Dijkstra, стоимости малы). Старт не включается. */
export function reachable(w: World, u: WEntity): Reach {
  const key = (x: number, y: number): number => y * w.rules.GridW + x;
  const limit = u.unit?.MoveCells ?? 0;
  const cost = new Map<number, number>([[key(u.x, u.y), 0]]);
  const prev = new Map<number, number>();
  const open: Pt[] = [{ x: u.x, y: u.y }];
  const blocked = (x: number, y: number): boolean => {
    const c = cellInfo(w, u.cls, x, y, u.id);
    return !c.ok || c.occupied;
  };
  while (open.length > 0) {
    let bi = 0;
    for (let i = 1; i < open.length; i++) if ((cost.get(key(open[i]!.x, open[i]!.y)) ?? 0) < (cost.get(key(open[bi]!.x, open[bi]!.y)) ?? 0)) bi = i;
    const cur = open.splice(bi, 1)[0]!;
    const cc = cost.get(key(cur.x, cur.y)) ?? 0;
    for (const d of NEIGHBOURS) {
      const nx = cur.x + d.x;
      const ny = cur.y + d.y;
      if (blocked(nx, ny)) continue;
      if (u.cls !== CLASS.AIR && d.x !== 0 && d.y !== 0 && blocked(nx, cur.y) && blocked(cur.x, ny)) continue;
      const nc = cc + stepCost(w, u, nx, ny);
      if (nc > limit) continue;
      const k = key(nx, ny);
      if (nc < (cost.get(k) ?? Infinity)) {
        cost.set(k, nc);
        prev.set(k, key(cur.x, cur.y));
        open.push({ x: nx, y: ny });
      }
    }
  }
  cost.delete(key(u.x, u.y));
  return { cost, prev };
}

export function moveCells(w: World, u: WEntity): Pt[] {
  if (u.ap < 1) return [];
  return [...reachable(w, u).cost.keys()].map((k) => ({ x: k % w.rules.GridW, y: Math.floor(k / w.rules.GridW) }));
}

/** Путь (клетки после стартовой) до (x,y) или null, если клетка не достижима по UI-оценке. */
export function pathTo(w: World, u: WEntity, x: number, y: number): Cell[] | null {
  const { prev, cost } = reachable(w, u);
  const W = w.rules.GridW;
  let k = y * W + x;
  if (!cost.has(k)) return null;
  const out: Cell[] = [];
  while (prev.has(k)) {
    out.push({ X: k % W, Y: Math.floor(k / W) });
    k = prev.get(k)!;
  }
  return out.reverse();
}

export type TargetAction = { op: 'attack' } | { op: 'ability'; id: 'heal' | 'jam' };

/** Что сделает выбранный юнит u с сущностью t (по умолчанию), либо null. Дальность проверяет вызывающий. */
export function actionOn(u: WEntity, t: WEntity): TargetAction | null {
  if (!u.unit || !t.alive || !t.onField) return null;
  if (t.owner === 'defender') {
    if (t.bld && (t.bld.Untargetable || (t.bld.Hidden && !t.revealed))) return null;
    if (canAttack(u)) return { op: 'attack' };
    if (u.unit.JamTurns > 0 && (t.bld?.Dmg ?? 0) > 0) return { op: 'ability', id: 'jam' };
    return null;
  }
  if (u.unit.Heal > 0 && t.id !== u.id && t.unit && t.hp < t.maxHp && (u.unit.HealVs & (1 << t.cls)) !== 0) {
    return { op: 'ability', id: 'heal' };
  }
  return null;
}

/** Цели, доступные u из текущей клетки (по дальности min..max для атаки/лечения, вплотную для помех). */
export function targetsOf(w: World, u: WEntity): number[] {
  if (!u.unit || u.ap < 1 || !u.alive || !u.onField) return [];
  const out: number[] = [];
  for (const t of w.entities) {
    const a = actionOn(u, t);
    if (!a) continue;
    const d = footDist(u, t);
    const [lo, hi] = a.op === 'ability' && a.id === 'jam' ? [0, 1] : [u.unit.MinRangeCells, u.unit.RangeCells];
    if (d >= lo && d <= hi) out.push(t.id);
  }
  return out;
}

/** Клетки, куда можно высадить юнита def (приближение спеки §4.1; ядро проверяет само). */
export function deployCells(w: World, unitId: string): Pt[] {
  const u = w.rules.Units.find((x) => x.ID === unitId);
  if (!u) return [];
  const { GridW, GridH, BuildZone: z, ParadropEdgeCells } = w.rules;
  const out: Pt[] = [];
  for (let y = 0; y < GridH; y++) {
    for (let x = 0; x < GridW; x++) {
      const edge = Math.min(x, y, GridW - 1 - x, GridH - 1 - y);
      if (u.Paradrop ? edge >= ParadropEdgeCells : x >= z.X && x < z.X + z.W && y >= z.Y && y < z.Y + z.H) continue;
      if (w.water[y * GridW + x] === true !== (u.Class === CLASS.SEA)) continue;
      const c = cellInfo(w, u.Class, x, y, 0);
      if (c.ok && !c.occupied) out.push({ x, y });
    }
  }
  return out;
}
