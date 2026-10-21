// Типы слайса армии и лаборатории. Формы API — docs/design/army-lab.md §8 и server/svc/army/state.go.
// Игровых чисел здесь нет: всё из content/*.yaml (см. content.ts).

export interface CountDto {
  id: string;
  count: number;
  level: number;
}

export interface SlotDto {
  id: string;
  count: number;
}

export interface PoolDto {
  capacity: number;
  used: number;
  free: number;
  base?: number;
  perk_pct?: number;
}

export interface OrderDto {
  order_id: string;
  unit_id: string;
  count: number;
  remaining: number;
  /** Unix-секунды: когда будет готов юнит, который обучается сейчас. */
  unit_finish_at: number;
}

export interface QueueDto {
  building_id: string;
  orders: OrderDto[];
  /** Unix-секунды: когда будет готова вся очередь. */
  finish_at: number;
}

export interface DeployedDto {
  raid_id: string;
  units: SlotDto[];
  spells: SlotDto[];
}

/** Ответ GET /v1/army, POST /v1/army/train|cancel|speedup. */
export interface ArmyStateDto {
  stock: CountDto[];
  spells: CountDto[];
  deployed?: DeployedDto[];
  pools: Record<string, PoolDto>;
  queues: QueueDto[];
  perks?: Record<string, number | boolean>;
  /** RFC3339. */
  server_time?: string;
  content_version?: string;
}

export interface LabActiveDto {
  node_id: string;
  /** Unix-секунды (допускается и RFC3339 — нормализуется в toSec). */
  started_at: number | string;
  finish_at: number | string;
  paid?: Record<string, number> | number;
}

export interface LabNodeDto {
  id: string;
  available: boolean;
  reason?: string;
  cost?: Record<string, number>;
  hours?: number;
}

/**
 * Ответ GET /v1/lab и POST /v1/lab/research|cancel|speedup (ASSUMPTION: army-lab.md §8, эндпоинты предлагает SERVER-5).
 * Сервер может вернуть только часть полей: недостающее UI считает из контента.
 */
export interface LabStateDto {
  lab_level?: number | null;
  active: LabActiveDto | null;
  done: string[];
  nodes?: LabNodeDto[];
  server_time?: string;
}

export interface TrainRequest {
  building_id: string;
  unit_id: string;
  count: number;
}

/** Подмножество API-клиента, нужное слайсу (в тестах мокается). Все мутации получают Idempotency-Key. */
export interface ArmyApi {
  fetchArmy(): Promise<ArmyStateDto>;
  train(req: TrainRequest, idempotencyKey: string): Promise<ArmyStateDto>;
  cancel(req: { order_id: string; count?: number }, idempotencyKey: string): Promise<ArmyStateDto>;
  speedup(req: { building_id: string }, idempotencyKey: string): Promise<ArmyStateDto>;
  fetchLab(): Promise<LabStateDto>;
  research(req: { node_id: string }, idempotencyKey: string): Promise<LabStateDto>;
  cancelResearch(idempotencyKey: string): Promise<LabStateDto>;
  speedupResearch(idempotencyKey: string): Promise<LabStateDto>;
}

/** Здание игрока (подмножество BaseItem): шелл отдаёт список из слайса базы. */
export interface BuildingRef {
  uid: string;
  type: string;
  level: number;
}

// --- Мир слайса ---

export interface ArmyWorld {
  stock: readonly CountDto[];
  spells: readonly CountDto[];
  deployed: readonly DeployedDto[];
  pools: Readonly<Record<string, PoolDto>>;
  queues: readonly QueueDto[];
  perks: Readonly<Record<string, number | boolean>>;
}

export interface LabWorld {
  labLevel: number | null;
  active: { nodeId: string; startedAt: number; finishAt: number; paid?: Record<string, number> | number } | null;
  done: readonly string[];
  /** Подсказки сервера по доступности узлов (если отдал). */
  serverNodes: readonly LabNodeDto[];
}

export interface World {
  army: ArmyWorld;
  lab: LabWorld;
  /** server_time последнего ответа, секунды (null — ответов ещё не было). */
  serverTimeS: number | null;
  armyLoaded: boolean;
  labLoaded: boolean;
}

export const EMPTY_WORLD: World = {
  army: { stock: [], spells: [], deployed: [], pools: {}, queues: [], perks: {} },
  lab: { labLevel: null, active: null, done: [], serverNodes: [] },
  serverTimeS: null,
  armyLoaded: false,
  labLoaded: false,
};

/** Unix-секунды из числа или RFC3339-строки. */
export function toSec(v: number | string | undefined | null): number | null {
  if (v === undefined || v === null) return null;
  if (typeof v === 'number') return Number.isFinite(v) ? Math.trunc(v) : null;
  const ms = Date.parse(v);
  return Number.isNaN(ms) ? null : Math.floor(ms / 1000);
}

export function armyFromDto(s: ArmyStateDto): ArmyWorld {
  return {
    stock: s.stock ?? [],
    spells: s.spells ?? [],
    deployed: s.deployed ?? [],
    pools: s.pools ?? {},
    queues: s.queues ?? [],
    perks: s.perks ?? {},
  };
}

export function labFromDto(s: LabStateDto, prev: LabWorld): LabWorld {
  const a = s.active;
  const startedAt = a ? toSec(a.started_at) : null;
  const finishAt = a ? toSec(a.finish_at) : null;
  return {
    labLevel: s.lab_level ?? prev.labLevel,
    active:
      a && finishAt !== null
        ? { nodeId: a.node_id, startedAt: startedAt ?? finishAt, finishAt, paid: a.paid }
        : null,
    done: s.done ?? [],
    serverNodes: s.nodes ?? [],
  };
}
