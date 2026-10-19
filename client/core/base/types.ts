// Типы слайса базы. Формы API — docs/03-api.md, раздел Base. Игровых чисел здесь нет: всё из content/*.yaml.
import type { BaseItem } from '../../engine/base-types';

export type { BaseItem };

export interface BaseTimer {
  id: string;
  kind: 'build' | 'upgrade' | (string & {});
  target_id: string;
  finish_at: string;
  to_level: number;
}

/** Ответ GET /v1/base, POST /v1/base/commands, PUT /v1/base/layout. */
export interface BaseStateDto {
  version: number;
  layout: { items: BaseItem[] };
  timers?: BaseTimer[];
  builders?: { total: number; busy: number };
  collectors?: Record<string, number>;
  server_time?: string;
  content_version?: string;
  unchanged?: boolean;
}

/** Мир, над которым работает store (подтверждённое состояние + оптимистичные команды). */
export interface BaseWorld {
  version: number;
  items: readonly BaseItem[];
  timers: readonly BaseTimer[];
  builders: { total: number; busy: number };
  collectors: Readonly<Record<string, number>>;
  /** server_time последнего ответа (ISO) — для расчёта оставшегося времени таймеров. */
  serverTime: string | null;
}

export const EMPTY_WORLD: BaseWorld = {
  version: 0,
  items: [],
  timers: [],
  builders: { total: 0, busy: 0 },
  collectors: {},
  serverTime: null,
};

export function toWorld(s: BaseStateDto): BaseWorld {
  const timers = s.timers ?? [];
  return {
    version: s.version,
    items: s.layout.items,
    timers,
    builders: s.builders ?? { total: 0, busy: timers.length },
    collectors: s.collectors ?? {},
    serverTime: s.server_time ?? null,
  };
}

export type Kind = 'core' | 'building' | 'passive' | 'active' | 'decor' | 'obstacle';

export interface LimitStep {
  at: number;
  max: number;
}

export interface Def {
  id: string;
  kind: Kind;
  nameKey: string;
  w: number;
  h: number;
  unlockHq: number;
  requiresWater: boolean;
  /** null — лимита по штабу нет. */
  limits: readonly LimitStep[] | null;
  /** Потолок уровня из buildings.yaml (у обороны его нет — см. Pricing.maxLevel). */
  levels: number | null;
  range: number | null;
  minRange: number | null;
  auraR: number | null;
  clearCost: Readonly<Record<string, number>>;
  reward: Readonly<Record<string, number>>;
  /** Ресурс, который здание производит (сборщик). */
  produces: string | null;
}

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface Grid {
  size: number;
  water: readonly Rect[];
}

export interface BaseCommandDto {
  op: string;
  args: Record<string, unknown>;
}

/** Подмножество API-клиента, нужное слайсу (мокается в тестах). */
export interface BaseApi {
  fetchBase(): Promise<BaseStateDto>;
  sendCommands(commands: BaseCommandDto[], idempotencyKey: string): Promise<BaseStateDto>;
  saveLayout(items: readonly BaseItem[]): Promise<BaseStateDto>;
}
