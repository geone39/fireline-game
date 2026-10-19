// Данные поля боя для рендера (Renderer 0.3). Рендер ничего не решает про правила: допустимые ходы, дальность и цели
// приходят снаружи (core строит их из Snapshot/Events ядра sim и контента). Координаты — клетки боевой сетки (sim.Cell).

export interface BattleCell {
  x: number;
  y: number;
}

export type BattleSide = 'attacker' | 'defender';

/** Класс сущности (sim.EntityKind + декор/препятствие/ловушка). */
export type BattleKind =
  | 'building'
  | 'defence'
  | 'wall'
  | 'troop'
  | 'vehicle'
  | 'ship'
  | 'hero'
  | 'decor'
  | 'trap';

/** Слой, в котором стоит сущность; воздушные рисуются поверх всего. */
export type BattleLayer = 'ground' | 'air' | 'sea';

export interface BattleEntity {
  /** sim.EntityID (>0). */
  id: number;
  side: BattleSide;
  /** Id контента и спрайта в art/manifest.json (b_hq, d_wall, u_rifleman, ...). */
  type: string;
  kind: BattleKind;
  layer?: BattleLayer;
  /** Левая верхняя клетка. */
  x: number;
  y: number;
  /** Размер футпринта в клетках (по умолчанию 1×1). */
  w?: number;
  h?: number;
  hp: number;
  maxHp: number;
  /** Очки действий юнита атакующего: остаток и максимум (маркеры над юнитом). */
  ap?: number;
  maxAp?: number;
  /** Скрытая ловушка (мина/пружина): рисуется только если открыта радаром/сработала. */
  revealed?: boolean;
}

/** Подсветка клеток. Всё — данные от core; рендер только рисует. */
export interface BattleHighlight {
  /** Клетки, куда выбранный юнит может сходить. */
  moves?: readonly BattleCell[];
  /** Дальность атаки по Чебышёву вокруг клетки (min..max включительно). */
  range?: { x: number; y: number; min: number; max: number };
  /** Id сущностей, которые можно атаковать/лечить выбранным юнитом. */
  targets?: readonly number[];
}

export interface BattleWorldState {
  gridSize: number;
  /** Прямоугольники воды карты (content/base_grid.yaml). */
  water?: readonly { x: number; y: number; w: number; h: number }[];
  entities: readonly BattleEntity[];
  /** Выбранный свой юнит. */
  selectedId?: number | null;
  highlight?: BattleHighlight;
}

/** Событие ядра sim (sim.Event). Kind: 1 moved, 2 attacked, 3 killed, 4 destroyed, 5 trap_triggered, 6 spell_cast,
 *  7 effect_tick, 8 healed, 9 released, 10 turn_ended, 11 battle_ended. Форма совпадает с Event из sim/wasm/api.d.ts. */
export interface BattleEvent {
  Kind: number;
  Actor: number;
  Target: number;
  Cell: { X: number; Y: number };
  Value: number;
}

export const EVENT = {
  moved: 1,
  attacked: 2,
  killed: 3,
  destroyed: 4,
  trapTriggered: 5,
  spellCast: 6,
  effectTick: 7,
  healed: 8,
  released: 9,
  turnEnded: 10,
  battleEnded: 11,
} as const;

export function footprintSize(e: BattleEntity): { w: number; h: number } {
  return { w: e.w ?? 1, h: e.h ?? 1 };
}
