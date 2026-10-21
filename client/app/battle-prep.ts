// Подготовка локального боя: состав из реальной армии игрока (ArmyState) вместо тестового Snapshot.
// Чистые функции, без DOM. Серверных рейдов ещё нет (raids/start), поэтому Army собираем на клиенте по контракту sim:
// ArmySlot { ID, Level, Count }. ASSUMPTION: это тренировочный бой — юниты не списываются со склада и потери не применяются
// (escrow и потери — на стороне сервера, army-lab.md §6.2–6.3); уровень слота = уровень юнита из ArmyState (лаборатория);
// защитник и Rules — фикстура G24 (настоящий Snapshot от сервера: OQ-C4b-2); героя в армии игрока пока нет — Hero пустой.
import type { ArmySlot, Snapshot } from '../sim-wasm/api';
import type { StockVM } from '../ui/army/store/view-model';

export type Unsupported = 'ship_no_water' | 'not_in_rules';

export interface PrepEntry {
  id: string;
  nameKey: string;
  kind: StockVM['kind'];
  level: number;
  /** Сколько есть в запасе (без отданных в рейд и без стоящих в очереди). */
  available: number;
  /** Не может участвовать в этом бою (null — может). */
  unsupported: Unsupported | null;
}

/** Выбор игрока: id → сколько взять. */
export type Selection = Readonly<Record<string, number>>;

function ruleIds(table: unknown): Set<string> {
  const out = new Set<string>();
  if (table && typeof table === 'object') {
    for (const v of Object.values(table as Record<string, unknown>)) {
      const id = (v as { ID?: unknown } | null)?.ID;
      if (typeof id === 'string') out.add(id);
    }
  }
  return out;
}

/** Строки выбора состава из запаса игрока для данного Snapshot (цели). Пустой запас и нулевые строки отбрасываются. */
export function prepEntries(stock: readonly StockVM[], target: Snapshot): PrepEntry[] {
  const units = ruleIds(target.Rules.Units);
  const spells = ruleIds(target.Rules.Spells);
  return stock
    .filter((s) => s.count > 0)
    .map((s) => ({
      id: s.id,
      nameKey: s.nameKey,
      kind: s.kind,
      level: s.level,
      available: s.count,
      // Корабли применимы только против карты с водой (army-lab.md §6.1 п.4); у фикстуры воды нет. ASSUMPTION.
      unsupported: s.kind === 'ship' ? ('ship_no_water' as const) : (s.kind === 'spell' ? spells : units).has(s.id) ? null : ('not_in_rules' as const),
    }));
}

/** Пресет «Вся армия»: всё из запаса, что применимо. */
export function selectAll(entries: readonly PrepEntry[]): Record<string, number> {
  return Object.fromEntries(entries.filter((e) => e.unsupported === null).map((e) => [e.id, e.available]));
}

/** Ввод игрока → допустимое количество: целое от 0 до запаса, для неприменимых 0. */
export function setCount(sel: Selection, e: PrepEntry, n: number): Record<string, number> {
  const v = e.unsupported !== null || !Number.isFinite(n) ? 0 : Math.min(e.available, Math.max(0, Math.trunc(n)));
  return { ...sel, [e.id]: v };
}

export function selectedTotal(entries: readonly PrepEntry[], sel: Selection): number {
  return entries.filter((e) => e.kind !== 'spell' && e.unsupported === null).reduce((n, e) => n + Math.min(sel[e.id] ?? 0, e.available), 0);
}

/** Можно начинать: выбран хотя бы один боевой юнит (бой без юнитов Start не пройдёт: battle-sim.md §4.1). */
export const canStartBattle = (entries: readonly PrepEntry[], sel: Selection): boolean => selectedTotal(entries, sel) > 0;

/** Snapshot локального боя: защитник/Rules из `target`, Army/Spells — из выбора игрока. */
export function armySnapshot(target: Snapshot, entries: readonly PrepEntry[], sel: Selection): Snapshot {
  const slots = (spell: boolean): ArmySlot[] =>
    entries
      .filter((e) => (e.kind === 'spell') === spell && e.unsupported === null)
      .map((e) => ({ ID: e.id, Level: e.level, Count: Math.min(sel[e.id] ?? 0, e.available) }))
      .filter((s) => s.Count > 0);
  return { ...target, BaseID: 'local-army', Army: slots(false), Spells: slots(true), Hero: '', HeroLevel: 0 };
}
