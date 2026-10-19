// «Теневое» состояние боя на клиенте. Ядро sim по WASM отдаёт только события и Result (состояния мира нет:
// OQ-C4b-1), поэтому клиент ведёт копию мира сам: стартовая раскладка из Snapshot (ids по каноническому порядку
// спеки §2.4) + события Apply. Копия нужна ТОЛЬКО для отрисовки и подсветки; допустимость команд решает ядро.
import type { Event, Snapshot } from '../../sim-wasm/api';
import { BLOCK, CLASS, KIND, scaledHp, type BuildingDef, type SimRules, type UnitDef } from './rules';

export type Owner = 'attacker' | 'defender';
export type Phase = 'deploy' | 'player' | 'over';

export interface WEntity {
  id: number;
  owner: Owner;
  contentId: string;
  level: number;
  kind: number;
  /** sim.UnitClass (для построек — INF, слой не используется). */
  cls: number;
  x: number;
  y: number;
  w: number;
  h: number;
  hp: number;
  maxHp: number;
  ap: number;
  maxAp: number;
  alive: boolean;
  /** Занимает клетки сетки (false: в бункере / на борту / ещё не вышел). */
  onField: boolean;
  revealed: boolean;
  unit?: UnitDef;
  bld?: BuildingDef;
}

export interface World {
  rules: SimRules;
  phase: Phase;
  turn: number;
  entities: WEntity[]; // индекс = id - 1
  water: boolean[];
  /** Id карты (Snapshot.Map) — для прямоугольников воды в рендере. */
  map: string;
}

export const EV = {
  moved: 1, attacked: 2, killed: 3, destroyed: 4, trap: 5, spell: 6, tick: 7, healed: 8, released: 9, turnEnded: 10,
  ended: 11, spawned: 12,
} as const;

export function rulesOf(s: Snapshot): SimRules {
  return s.Rules as unknown as SimRules;
}

export const isTrap = (e: WEntity): boolean => e.bld !== undefined && (e.bld.Hidden || e.bld.Trap !== 0);

/** Клиентский вид Snapshot: скрытые ловушки без `Revealed` не попадают в него (OQ-7.8). */
export function clientView(s: Snapshot): Snapshot {
  const hidden = new Set(rulesOf(s).Buildings.filter((b) => b.Hidden).map((b) => b.ID));
  return { ...s, Buildings: (s.Buildings ?? []).filter((b) => b.Revealed || !hidden.has(b.ID)) };
}

export function createWorld(s: Snapshot): World {
  const rules = rulesOf(s);
  const water = new Array<boolean>(rules.GridW * rules.GridH).fill(false);
  for (const r of rules.Maps.find((m) => m.ID === s.Map)?.Water ?? []) {
    for (let y = r.Y; y < r.Y + r.H; y++) for (let x = r.X; x < r.X + r.W; x++) water[y * rules.GridW + x] = true;
  }
  const bld = new Map(rules.Buildings.map((b) => [b.ID, b]));
  const unit = new Map(rules.Units.map((u) => [u.ID, u]));
  const ents: WEntity[] = [];
  const order = [...(s.Buildings ?? [])].sort(
    (a, b) => a.Pos.Y - b.Pos.Y || a.Pos.X - b.Pos.X || (a.ID < b.ID ? -1 : a.ID > b.ID ? 1 : 0) || a.Level - b.Level,
  );
  for (const pb of order) {
    const d = bld.get(pb.ID);
    if (!d) continue;
    const maxHp = scaledHp(d.HP, d.HPGrowthPermille, pb.Level);
    ents.push({
      id: ents.length + 1, owner: 'defender', contentId: d.ID, level: pb.Level, kind: d.Kind, cls: CLASS.INF,
      x: pb.Pos.X, y: pb.Pos.Y, w: d.W, h: d.H, hp: maxHp, maxHp, ap: 0, maxAp: 0, alive: true, onField: true,
      revealed: pb.Revealed, bld: d,
    });
  }
  // Гарнизон: бункеры по id берут юнитов по порядку списка (спека §7.2), юниты внутри до события Released.
  let gi = 0;
  const garrison = s.Garrison ?? [];
  for (const b of [...ents]) {
    for (let k = 0; k < (b.bld?.Garrison ?? 0) && gi < garrison.length; k++, gi++) {
      const g = garrison[gi];
      const u = g && unit.get(g.ID);
      if (g && u) ents.push(makeUnit(ents.length + 1, 'defender', u, g.Level, 0, 0, false));
    }
  }
  return { rules, phase: 'deploy', turn: 0, entities: ents, water, map: s.Map };
}

function makeUnit(id: number, owner: Owner, u: UnitDef, level: number, x: number, y: number, onField: boolean): WEntity {
  const maxHp = scaledHp(u.HP, u.HPGrowthPermille, level);
  const kind = u.Hero ? KIND.hero : u.Class === CLASS.VEH ? KIND.vehicle : u.Class === CLASS.SEA ? KIND.ship : KIND.troop;
  return {
    id, owner, contentId: u.ID, level, kind, cls: u.Class, x, y, w: 1, h: 1, hp: maxHp, maxHp, ap: u.AP, maxAp: u.AP,
    alive: true, onField, revealed: false, unit: u,
  };
}

export interface ApplyCtx {
  /** Команда, породившая события (для Moved при высадке нужен id контента нового юнита). */
  command: { op: string; id?: string; level?: number };
}

/** Применить события одной команды к миру (чистая функция). Неизвестные сущности пропускаются. */
export function applyEvents(w: World, events: readonly Event[], ctx: ApplyCtx): World {
  const ents = w.entities.map((e) => ({ ...e }));
  const next: World = { ...w, entities: ents };
  const get = (id: number): WEntity | undefined => (id > 0 ? ents[id - 1] : undefined);
  const unitDefs = new Map(w.rules.Units.map((u) => [u.ID, u]));
  for (const ev of events) {
    const actor = get(ev.Actor);
    const target = get(ev.Target);
    switch (ev.Kind) {
      case EV.moved:
        if (!actor && ev.Actor === ents.length + 1 && ctx.command.op === 'deploy') {
          const u = ctx.command.id ? unitDefs.get(ctx.command.id) : undefined;
          if (u) ents.push(makeUnit(ev.Actor, 'attacker', u, ctx.command.level ?? 1, ev.Cell.X, ev.Cell.Y, true));
        } else if (actor && target) {
          actor.onField = false; // погрузка на борт
        } else if (actor) {
          actor.x = ev.Cell.X;
          actor.y = ev.Cell.Y;
          actor.onField = true;
        }
        break;
      case EV.attacked:
        if (target) target.hp = Math.max(0, target.hp - ev.Value);
        break;
      case EV.killed:
      case EV.destroyed:
        if (target) Object.assign(target, { alive: false, hp: 0, onField: false });
        break;
      case EV.trap:
        if (actor) Object.assign(actor, { alive: false, hp: 0, onField: false });
        break;
      case EV.healed:
        if (target) target.hp = Math.min(target.maxHp, target.hp + ev.Value);
        break;
      case EV.released:
        // Actor выходит на клетку (из бункера или из подбитого перевозчика — тогда ap = 0, спека §5.4)
        if (actor) {
          Object.assign(actor, { x: ev.Cell.X, y: ev.Cell.Y, onField: true });
          if (target && !target.alive && target.owner === 'attacker') actor.ap = 0;
        }
        break;
      case EV.spawned: {
        const d = actor?.unit?.Ability.DecoyUnit;
        const u = d ? unitDefs.get(d) : undefined;
        if (u && ev.Target === ents.length + 1) {
          const e = makeUnit(ev.Target, 'attacker', u, actor?.level ?? 1, ev.Cell.X, ev.Cell.Y, true);
          e.kind = KIND.troop;
          ents.push(e);
        }
        break;
      }
      case EV.turnEnded:
        next.turn = ev.Value + 1;
        for (const e of ents) if (e.owner === 'attacker' && e.alive) e.ap = e.maxAp;
        break;
      case EV.ended:
        next.phase = 'over';
        break;
    }
  }
  return next;
}

export function covers(e: WEntity, x: number, y: number): boolean {
  return x >= e.x && x < e.x + e.w && y >= e.y && y < e.y + e.h;
}

/** Клетка занята/непроходима для класса cls (UI-копия cellState ядра; решает всё равно ядро). */
export function cellInfo(w: World, cls: number, x: number, y: number, self: number): { ok: boolean; occupied: boolean } {
  const { GridW, GridH } = w.rules;
  if (x < 0 || y < 0 || x >= GridW || y >= GridH) return { ok: false, occupied: false };
  const isWater = w.water[y * GridW + x] === true;
  if (cls === CLASS.SEA) {
    if (!isWater) return { ok: false, occupied: false };
  } else if (cls !== CLASS.AIR) {
    if (isWater) return { ok: false, occupied: false };
    for (const e of w.entities) {
      if (!e.bld || !e.alive || !covers(e, x, y)) continue;
      if (e.bld.Block === BLOCK.all || (e.bld.Block === BLOCK.vehicles && cls === CLASS.VEH)) return { ok: false, occupied: false };
    }
  }
  const air = cls === CLASS.AIR;
  for (const e of w.entities) {
    if (e.id === self || !e.unit || !e.alive || !e.onField || !covers(e, x, y)) continue;
    if ((e.cls === CLASS.AIR) === air) return { ok: true, occupied: true };
  }
  return { ok: true, occupied: false };
}

/** Чебышёв между футпринтами (docs/design/battle-sim.md §2.2). */
export function footDist(a: WEntity, b: WEntity): number {
  const gap = (a0: number, aw: number, b0: number, bw: number): number => Math.max(0, b0 - (a0 + aw - 1), a0 - (b0 + bw - 1));
  return Math.max(gap(a.x, a.w, b.x, b.w), gap(a.y, a.h, b.y, b.h));
}
