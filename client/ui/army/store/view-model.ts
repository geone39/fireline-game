// Селекторы: состояние слайса + контент + здания + кошелёк → готовая к показу модель экранов армии и лаборатории.
// Всё без хуков и DOM, поэтому тестируется напрямую. Предпроверки (housing, кошелёк, гейты) — подсказки; решает сервер.
import type { ArmyCatalog, ResearchNode, UnitDef, UnitKind } from '../content';
import type { BuildingRef, World } from '../types';
import { formatCost, type T } from '../i18n';

export type Tab = 'army' | 'lab';

export interface UiState {
  tab: Tab;
  /** uid выбранного здания-производителя; null — первое доступное. */
  buildingUid: string | null;
  /** Введённое количество по юнитам; нет записи — минимальный заказ (group). */
  counts: Readonly<Record<string, number>>;
  /** Ключ действия, ожидающего подтверждения отмены: `order:<id>` или `research`. */
  confirm: string | null;
}

export const INITIAL_UI: UiState = { tab: 'army', buildingUid: null, counts: {}, confirm: null };

/** Отказ сервера или сети в виде, пригодном для показа. */
export interface Notice {
  code: string;
  reason: string | null;
  message: string;
  details: Readonly<Record<string, unknown>>;
}

export type Wallet = Readonly<Record<string, number>>;

export interface PoolVM {
  id: string;
  labelKey: string;
  used: number;
  capacity: number;
  free: number;
  perkPct: number;
  /** Занятость 0…100 для полосы. */
  percent: number;
  full: boolean;
}

export interface PerkVM {
  key: string;
  labelKey: string;
  descKey: string;
  value: number | boolean;
}

export interface StockVM {
  id: string;
  nameKey: string;
  kind: UnitKind;
  count: number;
  level: number;
  inRaid: number;
  queued: number;
}

export interface OrderVM {
  orderId: string;
  unitId: string;
  nameKey: string;
  count: number;
  remaining: number;
  state: 'training' | 'waiting';
  /** Секунд до готовности юнита в работе (только у головы). */
  unitLeftS: number | null;
  unitPercent: number;
  confirming: boolean;
}

export interface QueueVM {
  buildingUid: string;
  nameKey: string;
  /** Порядковый номер среди зданий того же типа (1…) и сколько их всего: «Казарма #2». */
  ordinal: number;
  sameType: number;
  level: number | null;
  orders: OrderVM[];
  remainingS: number;
  percent: number;
  speedupGems: number;
  speedupBlocked: 'no_gems' | null;
  gemsShort: { need: number; have: number } | null;
}

export type TrainBlock = 'locked' | 'queue_full' | 'bad_count' | 'housing_full' | 'no_funds';

export interface TrainUnitVM {
  id: string;
  nameKey: string;
  kind: UnitKind;
  level: number;
  housing: number;
  group: number;
  trainS: number;
  unitCost: Readonly<Record<string, number>>;
  count: number;
  totalCost: Record<string, number>;
  totalTimeS: number;
  blocked: TrainBlock | null;
  /** Чего не хватает до открытия (locked). */
  needHq: number | null;
  needBuilding: { type: string; level: number } | null;
  /** Максимум, который поместится и по карману (кратно group), ≥ 0. */
  maxCount: number;
  freeHousing: number | null;
  short: Record<string, { need: number; have: number }> | null;
}

export interface TrainBuildingVM {
  uid: string;
  type: string;
  nameKey: string;
  ordinal: number;
  sameType: number;
  level: number;
  selected: boolean;
  orders: number;
}

export interface TrainVM {
  buildings: TrainBuildingVM[];
  selectedUid: string | null;
  units: TrainUnitVM[];
  queueFull: boolean;
}

export type NodeStatus = 'done' | 'researching' | 'soon' | 'lab_level' | 'prerequisite' | 'locked_hq' | 'available';

export interface LabNodeVM {
  id: string;
  nameKey: string;
  itemId: string | null;
  perkKey: string | null;
  fromLevel: number | null;
  toLevel: number;
  fuel: number;
  hours: number;
  status: NodeStatus;
  blocked: 'lab_busy' | 'no_funds' | 'no_lab' | null;
  needLab: number | null;
  /** Текущий уровень лаборатории (для подсказки «нужна лаборатория N, сейчас M»). */
  haveLab: number;
  needHq: number | null;
  prereqNameKey: string | null;
  short: Record<string, { need: number; have: number }> | null;
}

export type LabGroupKind = UnitKind | 'perk';

export interface LabGroupVM {
  kind: LabGroupKind;
  nodes: LabNodeVM[];
}

export interface ActiveResearchVM {
  nodeId: string;
  nameKey: string;
  remainingS: number;
  percent: number;
  speedupGems: number;
  speedupBlocked: 'no_gems' | null;
  gemsShort: { need: number; have: number } | null;
  confirming: boolean;
}

export interface LabViewModel {
  loaded: boolean;
  hasLab: boolean;
  labLevel: number | null;
  active: ActiveResearchVM | null;
  groups: LabGroupVM[];
  perks: PerkVM[];
}

export interface ArmyViewModel {
  tab: Tab;
  loaded: boolean;
  /** Идут команды или фоновое обновление. */
  busy: boolean;
  notice: Notice | null;
  pools: PoolVM[];
  perks: PerkVM[];
  stock: StockVM[];
  queues: QueueVM[];
  train: TrainVM;
  lab: LabViewModel;
}

export interface VmInput {
  /** Оптимистичный мир, уже «дозревший» до nowS. */
  world: World;
  ui: UiState;
  notice: Notice | null;
  busy: boolean;
  catalog: ArmyCatalog;
  buildings: readonly BuildingRef[];
  /** Кошелёк за вычетом ещё не подтверждённых трат; undefined — предпроверка по ресурсам отключена. */
  wallet: Wallet | undefined;
  nowS: number;
}

const buildingNameKey = (type: string): string => `b.${type.replace(/^b_/, '')}`;
const clamp = (n: number, lo: number, hi: number): number => Math.min(Math.max(n, lo), hi);
/** Порядковый номер здания среди зданий того же типа (по порядку в списке) и их число. */
function ordinalOf(buildings: readonly BuildingRef[], uid: string, type: string): { ordinal: number; sameType: number } {
  const same = buildings.filter((b) => b.type === type);
  return { ordinal: Math.max(1, same.findIndex((b) => b.uid === uid) + 1), sameType: Math.max(1, same.length) };
}
const percentOf = (done: number, total: number): number => (total <= 0 ? 100 : clamp(Math.floor((done * 100) / total), 0, 100));

function shortfall(cost: Readonly<Record<string, number>>, wallet: Wallet | undefined): Record<string, { need: number; have: number }> | null {
  if (!wallet) return null;
  const out: Record<string, { need: number; have: number }> = {};
  for (const [res, need] of Object.entries(cost)) {
    const have = wallet[res] ?? 0;
    if (need > have) out[res] = { need, have };
  }
  return Object.keys(out).length > 0 ? out : null;
}

function perksOf(perks: Readonly<Record<string, number | boolean>>): PerkVM[] {
  return Object.entries(perks)
    .filter(([, v]) => v !== 0 && v !== false)
    .map(([key, value]) => ({ key, labelKey: `perk.${key}`, descKey: `perk.${key}.desc`, value }));
}

function selectPools(w: World, catalog: ArmyCatalog): PoolVM[] {
  const ids = [...catalog.pools, ...Object.keys(w.army.pools).filter((p) => !catalog.pools.includes(p))];
  return ids
    .filter((id) => w.army.pools[id] !== undefined)
    .map((id) => {
      const p = w.army.pools[id]!;
      return {
        id,
        labelKey: `army.pool.${id}`,
        used: p.used,
        capacity: p.capacity,
        free: p.free,
        perkPct: p.perk_pct ?? 0,
        percent: percentOf(p.used, p.capacity),
        full: p.capacity > 0 && p.free <= 0,
      };
    });
}

function selectStock(w: World, catalog: ArmyCatalog): StockVM[] {
  const inRaid = new Map<string, number>();
  for (const d of w.army.deployed) for (const s of [...d.units, ...d.spells]) inRaid.set(s.id, (inRaid.get(s.id) ?? 0) + s.count);
  const queued = new Map<string, number>();
  for (const q of w.army.queues) for (const o of q.orders) queued.set(o.unit_id, (queued.get(o.unit_id) ?? 0) + o.remaining);
  const have = new Map<string, { count: number; level: number }>();
  for (const c of [...w.army.stock, ...w.army.spells]) have.set(c.id, { count: c.count, level: c.level });
  const ids = new Set<string>([...have.keys(), ...inRaid.keys()]);
  return catalog.unitOrder
    .filter((id) => ids.has(id))
    .map((id) => {
      const u = catalog.unit(id)!;
      return {
        id,
        nameKey: u.nameKey,
        kind: u.kind,
        count: have.get(id)?.count ?? 0,
        level: have.get(id)?.level ?? catalog.levelOf(id, w.lab.done),
        inRaid: inRaid.get(id) ?? 0,
        queued: queued.get(id) ?? 0,
      };
    });
}

function producersOf(buildings: readonly BuildingRef[], catalog: ArmyCatalog): BuildingRef[] {
  return buildings.filter((b) => catalog.producers.includes(b.type));
}

function selectQueues(i: VmInput): QueueVM[] {
  const { world: w, catalog, nowS, wallet } = i;
  return w.army.queues.map((q) => {
    const b = i.buildings.find((x) => x.uid === q.building_id);
    const remainingS = Math.max(0, q.finish_at - nowS);
    const totalS = q.orders.reduce((s, o) => s + o.remaining * (catalog.unit(o.unit_id)?.trainS ?? 0), 0);
    const gems = catalog.speedupGems(remainingS);
    const have = wallet?.gems;
    return {
      buildingUid: q.building_id,
      nameKey: buildingNameKey(b?.type ?? q.building_id.split('#')[0] ?? q.building_id),
      ...(b ? ordinalOf(i.buildings, b.uid, b.type) : { ordinal: 1, sameType: 1 }),
      level: b?.level ?? null,
      orders: q.orders.map((o, idx) => {
        const train = catalog.unit(o.unit_id)?.trainS ?? 0;
        const left = Math.max(0, o.unit_finish_at - nowS);
        return {
          orderId: o.order_id,
          unitId: o.unit_id,
          nameKey: catalog.unit(o.unit_id)?.nameKey ?? o.unit_id,
          count: o.count,
          remaining: o.remaining,
          state: idx === 0 ? 'training' : 'waiting',
          unitLeftS: idx === 0 ? left : null,
          unitPercent: idx === 0 ? percentOf(train - left, train) : 0,
          confirming: i.ui.confirm === `order:${o.order_id}`,
        };
      }),
      remainingS,
      percent: percentOf(totalS - remainingS, totalS),
      speedupGems: gems,
      speedupBlocked: have !== undefined && have < gems ? 'no_gems' : null,
      gemsShort: have !== undefined && have < gems ? { need: gems, have } : null,
    };
  });
}

function unlockState(u: UnitDef, hq: number, b: BuildingRef | undefined): { needHq: number | null; needBuilding: { type: string; level: number } | null } {
  const needHq = u.unlockHq > hq ? u.unlockHq : null;
  const lvl = u.unlockBuilding[u.producer];
  const needBuilding = lvl !== undefined && lvl > (b?.level ?? 0) ? { type: u.producer, level: lvl } : null;
  return { needHq, needBuilding };
}

function roundDown(n: number, step: number): number {
  return Math.max(0, Math.floor(n / step) * step);
}

function selectTrain(i: VmInput, hq: number): TrainVM {
  const { world: w, catalog, ui, wallet } = i;
  const producers = producersOf(i.buildings, catalog);
  const selected = producers.find((b) => b.uid === ui.buildingUid) ?? producers[0];
  const ordersOf = (uid: string): number => w.army.queues.find((q) => q.building_id === uid)?.orders.length ?? 0;
  const buildings = producers.map((b) => ({
    uid: b.uid,
    type: b.type,
    nameKey: buildingNameKey(b.type),
    ...ordinalOf(producers, b.uid, b.type),
    level: b.level,
    selected: b.uid === selected?.uid,
    orders: ordersOf(b.uid),
  }));
  const queueFull = selected !== undefined && ordersOf(selected.uid) >= catalog.queueMaxOrders;
  const units: TrainUnitVM[] = (selected ? catalog.unitsOf(selected.type) : []).map((u) => {
    const pool = w.army.pools[u.pool];
    const count = ui.counts[u.id] ?? u.group;
    const totalCost: Record<string, number> = {};
    for (const [r, c] of Object.entries(u.cost)) totalCost[r] = c * count;
    const { needHq, needBuilding } = unlockState(u, hq, selected);
    const free = pool ? pool.free : null;
    const fitHousing = free === null ? Number.POSITIVE_INFINITY : roundDown(Math.floor(free / u.housing), u.group);
    let fitFunds = Number.POSITIVE_INFINITY;
    if (wallet) for (const [r, c] of Object.entries(u.cost)) if (c > 0) fitFunds = Math.min(fitFunds, Math.floor((wallet[r] ?? 0) / c));
    const fit = Math.min(fitHousing, fitFunds);
    const maxCount = Number.isFinite(fit) ? roundDown(fit, u.group) : 0;
    const short = count > 0 ? shortfall(totalCost, wallet) : null;
    let blocked: TrainBlock | null = null;
    if (needHq !== null || needBuilding !== null) blocked = 'locked';
    else if (queueFull) blocked = 'queue_full';
    else if (count < 1 || count % u.group !== 0) blocked = 'bad_count';
    else if (free !== null && count * u.housing > free) blocked = 'housing_full';
    else if (short) blocked = 'no_funds';
    return {
      id: u.id,
      nameKey: u.nameKey,
      kind: u.kind,
      level: catalog.levelOf(u.id, w.lab.done),
      housing: u.housing,
      group: u.group,
      trainS: u.trainS,
      unitCost: u.cost,
      count,
      totalCost,
      totalTimeS: count * u.trainS,
      blocked,
      needHq,
      needBuilding,
      maxCount,
      freeHousing: free,
      short,
    };
  });
  return { buildings, selectedUid: selected?.uid ?? null, units, queueFull };
}

function selectLab(i: VmInput, hq: number): LabViewModel {
  const { world: w, catalog, wallet, nowS } = i;
  const lab = w.lab;
  const lvFromBuildings = i.buildings.filter((b) => b.type === catalog.labBuilding).reduce<number | null>((m, b) => Math.max(m ?? 0, b.level), null);
  const labLevel = lab.labLevel ?? lvFromBuildings;
  const hasLab = labLevel !== null && labLevel > 0;
  const activeId = lab.active?.nodeId ?? null;

  const nodeVM = (n: ResearchNode): LabNodeVM => {
    const tier = catalog.tiers.get(n.tier) ?? { fuel: 0, hours: 0 };
    const server = lab.serverNodes.find((x) => x.id === n.id);
    const unit = n.item ? catalog.unit(n.item) : undefined;
    const sameItem = n.item ? catalog.nodes.filter((x) => x.item === n.item && x.toLevel < n.toLevel) : [];
    const prereqId = n.requires.find((r) => !lab.done.includes(r)) ?? sameItem.filter((x) => !lab.done.includes(x.id)).sort((a, b) => a.toLevel - b.toLevel)[0]?.id;
    const prereq = prereqId ? catalog.node(prereqId) : undefined;
    const fuelCost = { fuel: tier.fuel };
    const short = shortfall(fuelCost, wallet);
    let status: NodeStatus = 'available';
    if (lab.done.includes(n.id)) status = 'done';
    else if (n.id === activeId) status = 'researching';
    else if (n.soon) status = 'soon';
    else if (prereq) status = 'prerequisite';
    else if (n.lab > (labLevel ?? 0)) status = 'lab_level';
    else if (unit && unit.unlockHq > hq) status = 'locked_hq';
    else if (server && !server.available) {
      if (server.reason === 'prerequisite') status = 'prerequisite';
      else if (server.reason === 'lab_level') status = 'lab_level';
    }
    let blocked: LabNodeVM['blocked'] = null;
    if (status === 'available') {
      if (!hasLab) blocked = 'no_lab';
      else if (lab.active) blocked = 'lab_busy';
      else if (short) blocked = 'no_funds';
    }
    return {
      id: n.id,
      nameKey: n.nameKey,
      itemId: n.item,
      perkKey: n.perk?.key ?? null,
      fromLevel: n.item ? catalog.levelOf(n.item, lab.done) : null,
      toLevel: n.toLevel,
      fuel: tier.fuel,
      hours: tier.hours,
      status,
      blocked,
      needLab: status === 'lab_level' ? n.lab : null,
      haveLab: labLevel ?? 0,
      needHq: status === 'locked_hq' && unit ? unit.unlockHq : null,
      prereqNameKey: status === 'prerequisite' ? (prereq?.nameKey ?? null) : null,
      short: blocked === 'no_funds' ? short : null,
    };
  };

  const kinds: LabGroupKind[] = ['unit', 'vehicle', 'ship', 'spell', 'perk'];
  const groups: LabGroupVM[] = kinds
    .map((kind) => ({
      kind,
      nodes: catalog.nodes
        .filter((n) => (kind === 'perk' ? n.perk !== null : n.item !== null && catalog.unit(n.item)?.kind === kind))
        .map(nodeVM),
    }))
    .filter((g) => g.nodes.length > 0);

  let active: ActiveResearchVM | null = null;
  if (lab.active) {
    const n = catalog.node(lab.active.nodeId);
    const remainingS = Math.max(0, lab.active.finishAt - nowS);
    const gems = catalog.speedupGems(remainingS);
    const have = wallet?.gems;
    active = {
      nodeId: lab.active.nodeId,
      nameKey: n?.nameKey ?? lab.active.nodeId,
      remainingS,
      percent: percentOf(nowS - lab.active.startedAt, lab.active.finishAt - lab.active.startedAt),
      speedupGems: gems,
      speedupBlocked: have !== undefined && have < gems ? 'no_gems' : null,
      gemsShort: have !== undefined && have < gems ? { need: gems, have } : null,
      confirming: i.ui.confirm === 'research',
    };
  }
  return { loaded: w.labLoaded, hasLab, labLevel, active, groups, perks: perksOf(w.army.perks) };
}

export function selectViewModel(i: VmInput): ArmyViewModel {
  const hqLevels = i.buildings.filter((b) => b.type === 'b_hq').map((b) => b.level);
  const hq = hqLevels.length > 0 ? Math.max(...hqLevels) : Number.POSITIVE_INFINITY;
  return {
    tab: i.ui.tab,
    loaded: i.world.armyLoaded,
    busy: i.busy,
    notice: i.notice,
    pools: selectPools(i.world, i.catalog),
    perks: perksOf(i.world.army.perks),
    stock: selectStock(i.world, i.catalog),
    queues: selectQueues(i),
    train: selectTrain(i, hq),
    lab: selectLab(i, hq),
  };
}

/** Человекочитаемый текст отказа: причина из reason.* + уточнения (max_count, missing, need). */
export function noticeText(n: Notice, t: T): string {
  const reasonKey = n.reason ? `reason.${n.reason}` : `reason.${n.code}`;
  let text = t(reasonKey);
  if (text === reasonKey) text = t(`reason.${n.code}`);
  if (text === `reason.${n.code}`) text = n.message || text;
  const d = n.details;
  if (n.reason === 'housing_full' && typeof d.max_count === 'number') text += `. ${t('ui.army.max_count', { n: d.max_count })}`;
  const missing = d.missing;
  if (n.code === 'insufficient_resources' && missing && typeof missing === 'object') {
    text += `. ${t('ui.army.missing', { missing: formatCost(missing as Record<string, number>, t) })}`;
  }
  const need = d.need;
  if (n.reason === 'locked' && need && typeof need === 'object') {
    const parts = Object.entries(need as Record<string, number>).map(([k, v]) => (k === 'hq' ? t('ui.army.need_hq', { n: v }) : `${t(`b.${k}`)} ${v}`));
    text += `. ${t('ui.army.need', { need: parts.join(', ') })}`;
  }
  if (n.reason === 'lab_level' && typeof d.need === 'number') text += ` (${d.need})`;
  return text;
}
