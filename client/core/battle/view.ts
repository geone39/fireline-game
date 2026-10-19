// Мир боя -> BattleWorldState для рендера (Renderer 0.3). Рендер ничего не решает: подсветка считается здесь.
import type { BattleEntity, BattleKind, BattleWorldState } from '../../engine/battle-types';
import { CLASS, KIND } from './rules';
import { deployCells, moveCells, targetsOf } from './highlight';
import { isTrap, type WEntity, type World } from './world';

export interface UiSelection {
  selectedId: number | null;
  /** Выбран юнит из запаса для высадки (id контента). */
  armedDeploy: string | null;
}

function kindOf(e: WEntity): BattleKind {
  if (isTrap(e)) return 'trap';
  if (e.bld?.Untargetable) return 'decor';
  switch (e.kind) {
    case KIND.defence: return 'defence';
    case KIND.wall: return 'wall';
    case KIND.vehicle: return 'vehicle';
    case KIND.ship: return 'ship';
    case KIND.hero: return 'hero';
    case KIND.troop:
    case KIND.decoy: return 'troop';
    default: return 'building';
  }
}

function toEntity(e: WEntity): BattleEntity {
  const unit = e.unit !== undefined;
  const out: BattleEntity = {
    id: e.id, side: e.owner, type: e.contentId, kind: kindOf(e), x: e.x, y: e.y, hp: e.hp, maxHp: e.maxHp,
  };
  if (!unit) {
    out.w = e.w;
    out.h = e.h;
    out.revealed = e.revealed;
  } else {
    out.layer = e.cls === CLASS.AIR ? 'air' : e.cls === CLASS.SEA ? 'sea' : 'ground';
    if (e.owner === 'attacker') {
      out.ap = e.ap;
      out.maxAp = e.maxAp;
    }
  }
  return out;
}

export function toRenderState(w: World, ui: UiSelection): BattleWorldState {
  const entities = w.entities.filter((e) => e.alive && e.onField).map(toEntity);
  const water = (w.rules.Maps.find((m) => m.ID === w.map)?.Water ?? []).map((r) => ({ x: r.X, y: r.Y, w: r.W, h: r.H }));
  const state: BattleWorldState = { gridSize: w.rules.GridW, water, entities, selectedId: ui.selectedId };
  if (w.phase === 'deploy' && ui.armedDeploy) {
    state.highlight = { moves: deployCells(w, ui.armedDeploy) };
  } else if (w.phase === 'player' && ui.selectedId) {
    const u = w.entities[ui.selectedId - 1];
    if (u && u.owner === 'attacker' && u.alive && u.onField && u.unit) {
      const targets = targetsOf(w, u);
      const ranged = u.ap > 0 && (u.unit.RangeCells > 0 || u.unit.Heal > 0);
      state.highlight = {
        moves: moveCells(w, u),
        ...(ranged ? { range: { x: u.x, y: u.y, min: u.unit.MinRangeCells, max: u.unit.RangeCells } } : {}),
        targets,
      };
    }
  }
  return state;
}
