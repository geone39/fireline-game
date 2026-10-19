// Модель панели боя: чистые функции от BattleState (тестируются без DOM).
import type { BattleState } from '../core/battle';
import type { T } from '../ui/base/i18n';

/** sim.RejectCode -> ключ строки (battle.reject.<code>), неизвестные коды -> battle.reject.unknown. */
export function rejectKey(code: number): string {
  return code >= 1 && code <= 17 ? `battle.reject.${code}` : 'battle.reject.unknown';
}

export const END_REASON_KEYS = ['', 'full_destruction', 'no_units', 'turn_limit', 'retreat', 'log_end'] as const;

export interface PanelModel {
  phase: 'deploy' | 'player' | 'over';
  turn: number;
  maxTurns: number;
  /** Выбранный свой юнит. */
  unit: { id: number; type: string; hp: number; maxHp: number; ap: number; maxAp: number; canUnload: boolean; hasAbility: boolean } | null;
  canStart: boolean;
  deployed: number;
  stock: BattleState['stock'];
  armed: BattleState['armed'];
  reject: string | null;
  result: { stars: number; destruction: number; reason: string; turns: number } | null;
}

export function selectPanel(s: BattleState, t: T): PanelModel {
  const w = s.world;
  const sel = s.selectedId ? w.entities[s.selectedId - 1] : undefined;
  const deployed = w.entities.filter((e) => e.owner === 'attacker' && e.alive).length;
  const r = s.result;
  return {
    phase: w.phase,
    turn: w.turn,
    maxTurns: w.rules.MaxTurns,
    unit:
      sel && sel.unit && sel.owner === 'attacker' && sel.alive
        ? {
            id: sel.id, type: sel.contentId, hp: sel.hp, maxHp: sel.maxHp, ap: sel.ap, maxAp: sel.maxAp,
            canUnload: sel.unit.Carries > 0, hasAbility: sel.unit.Ability.Kind !== 0,
          }
        : null,
    canStart: w.phase === 'deploy' && deployed > 0,
    deployed,
    stock: s.stock,
    armed: s.armed,
    reject: s.lastReject ? t(rejectKey(s.lastReject.code)) : null,
    result: r
      ? { stars: r.Stars, destruction: r.Destruction, reason: t(`battle.end.${END_REASON_KEYS[r.EndReason] ?? 'unknown'}`), turns: r.TurnsUsed }
      : null,
  };
}

export const stars = (n: number): string => '★'.repeat(n) + '☆'.repeat(Math.max(0, 3 - n));
