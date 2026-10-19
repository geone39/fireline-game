// Чистые селекторы: слайс + редактор → то, что рисуют компоненты и рендер. Без побочных эффектов.
import type { WorldState } from '../../engine/renderer';
import type { Catalog, Pricing } from './catalog';
import { GHOST_UID, type EditorState } from './editor';
import { footprintSize, hqLevel, type PlacementReason } from './validate';
import type { BaseItem, BaseTimer, BaseWorld, Def, Grid, Kind } from './types';

export type ShopBlock = 'locked' | 'limit' | 'no_builder' | 'no_price';

export interface ShopEntry {
  type: string;
  nameKey: string;
  kind: Kind;
  cost: Record<string, number>;
  count: number;
  /** null — без лимита. */
  limit: number | null;
  blocked: ShopBlock | null;
  unlockHq: number;
}

export type ActionBlock = 'busy' | 'max_level' | 'no_builder' | 'no_price';

export interface CardVM {
  uid: string;
  type: string;
  nameKey: string;
  kind: Kind;
  level: number;
  maxLevel: number;
  timer: { kind: string; toLevel: number; remainingS: number } | null;
  upgrade: { cost: Record<string, number>; blocked: ActionBlock | null };
  speedupGems: number | null;
  canRemove: boolean;
  clear: { cost: Record<string, number>; reward: Record<string, number> } | null;
  canRotate: boolean;
  confirmRemove: boolean;
}

export interface Radius {
  cx: number;
  cy: number;
  r: number;
  /** range — зона огня (зелёная); dead — мёртвая зона (красная); aura — радиус ауры. */
  kind: 'range' | 'dead' | 'aura';
}

export interface OverlayVM {
  tiles: { x: number; y: number; ok: boolean }[];
  radii: Radius[];
  reason: PlacementReason | null;
}

export interface BaseViewModel {
  shop: ShopEntry[];
  card: CardVM | null;
  toolbar: {
    editing: boolean;
    canUndo: boolean;
    canRedo: boolean;
    dirty: boolean;
    saving: boolean;
    hasCollectable: boolean;
    builders: { total: number; busy: number };
  };
  placing: { type: string; nameKey: string; ok: boolean; reason: PlacementReason | null; canRotate: boolean } | null;
  overlay: OverlayVM;
  notice: string | null;
  rejection: { code: string; message: string } | null;
}

export interface ViewInput {
  world: BaseWorld;
  /** раскладка с черновиком и перетаскиванием */
  items: readonly BaseItem[];
  editor: EditorState;
  catalog: Catalog;
  pricing?: Pricing;
  /** текущее время сервера, мс (Date.parse(server_time) + прошло с ответа) */
  serverNowMs: number;
  placement: { ok: boolean; reason: PlacementReason | null; tiles: { x: number; y: number; ok: boolean }[] } | null;
  rejection: { code: string; message: string } | null;
}

export function timerOf(world: BaseWorld, uid: string): BaseTimer | undefined {
  return world.timers.find((t) => t.target_id === uid);
}

export function remainingSeconds(t: BaseTimer, serverNowMs: number): number {
  return Math.max(0, Math.ceil((Date.parse(t.finish_at) - serverNowMs) / 1000));
}

function radiiOf(d: Def, it: BaseItem): Radius[] {
  const s = footprintSize(d.w, d.h, it.rot);
  const cx = it.x + s.w / 2;
  const cy = it.y + s.h / 2;
  const out: Radius[] = [];
  if (d.range) out.push({ cx, cy, r: d.range, kind: 'range' });
  if (d.minRange) out.push({ cx, cy, r: d.minRange, kind: 'dead' });
  if (d.auraR) out.push({ cx, cy, r: d.auraR, kind: 'aura' });
  return out;
}

export function selectViewModel(i: ViewInput): BaseViewModel {
  const { world, catalog, pricing, editor } = i;
  const hq = hqLevel(catalog, world.items);
  const builderFree = world.builders.busy < world.builders.total;

  const shop: ShopEntry[] = catalog
    .all()
    .filter((d) => d.kind !== 'core' && d.kind !== 'obstacle')
    .map((d) => {
      const count = world.items.filter((it) => it.type === d.id).length;
      const limit = catalog.maxCount(d, hq);
      let blocked: ShopBlock | null = null;
      if (hq < d.unlockHq) blocked = 'locked';
      else if (limit !== null && count >= limit) blocked = 'limit';
      else if (!pricing?.has(d.id)) blocked = 'no_price';
      else if (!builderFree) blocked = 'no_builder';
      return {
        type: d.id,
        nameKey: d.nameKey,
        kind: d.kind,
        cost: pricing?.cost(d.id, 1) ?? {},
        count,
        limit,
        blocked,
        unlockHq: d.unlockHq,
      };
    });

  let card: CardVM | null = null;
  const sel = editor.selectedUid ? i.items.find((it) => it.uid === editor.selectedUid) : undefined;
  const sd = sel && catalog.get(sel.type);
  if (sel && sd) {
    const t = timerOf(world, sel.uid);
    const maxLevel = pricing?.maxLevel(sd) ?? sd.levels ?? 1;
    const remainingS = t ? remainingSeconds(t, i.serverNowMs) : 0;
    let blocked: ActionBlock | null = null;
    if (!pricing?.has(sd.id)) blocked = 'no_price';
    else if (t) blocked = 'busy';
    else if (sel.level >= maxLevel) blocked = 'max_level';
    else if (!builderFree) blocked = 'no_builder';
    card = {
      uid: sel.uid,
      type: sel.type,
      nameKey: sd.nameKey,
      kind: sd.kind,
      level: sel.level,
      maxLevel,
      timer: t ? { kind: t.kind, toLevel: t.to_level, remainingS } : null,
      upgrade: { cost: pricing?.cost(sd.id, sel.level + 1) ?? {}, blocked },
      speedupGems: t && pricing ? pricing.speedupGems(remainingS) : null,
      canRemove: sd.kind !== 'core' && sd.kind !== 'obstacle',
      clear: sd.kind === 'obstacle' ? { cost: { ...sd.clearCost }, reward: { ...sd.reward } } : null,
      canRotate: editor.editing && sd.kind !== 'obstacle' && sd.w !== sd.h,
      confirmRemove: editor.askRemove === sel.uid,
    };
  }

  const overlay: OverlayVM = { tiles: [], radii: [], reason: null };
  if (i.placement) {
    overlay.tiles = i.placement.tiles;
    overlay.reason = i.placement.reason;
  }
  // Радиусы: у призрака — его собственные, иначе у выбранного/перетаскиваемого здания.
  const focus: BaseItem | undefined = editor.placing
    ? { ...editor.placing, uid: GHOST_UID, level: 1 }
    : sel;
  const fd = focus && catalog.get(focus.type);
  if (focus && fd) overlay.radii = radiiOf(fd, focus);

  const pd = editor.placing ? catalog.get(editor.placing.type) : undefined;
  return {
    shop,
    card,
    toolbar: {
      editing: editor.editing,
      canUndo: editor.undoStack.length > 0,
      canRedo: editor.redoStack.length > 0,
      dirty: Object.keys(editor.overrides).length > 0,
      saving: editor.saving,
      hasCollectable: Object.values(world.collectors).some((v) => v > 0),
      builders: world.builders,
    },
    placing:
      editor.placing && pd
        ? {
            type: editor.placing.type,
            nameKey: pd.nameKey,
            ok: i.placement?.ok ?? false,
            reason: i.placement?.reason ?? null,
            canRotate: pd.w !== pd.h,
          }
        : null,
    overlay,
    notice: editor.notice,
    rejection: i.rejection,
  };
}

/** Состояние для Renderer 0.2: данные приходят снаружи, рендер ничего не решает. */
export function buildWorldState(i: { items: readonly BaseItem[]; editor: EditorState; catalog: Catalog; grid: Grid }): WorldState {
  const items: BaseItem[] = [...i.items];
  const p = i.editor.placing;
  if (p) items.push({ uid: GHOST_UID, type: p.type, level: 1, x: p.x, y: p.y, rot: p.rot });
  const hl = p ?? i.editor.drag?.pose ?? i.editor.hover;
  return {
    gridSize: i.grid.size,
    items,
    water: i.grid.water,
    footprints: i.catalog.footprints(),
    selectedUid: p ? GHOST_UID : i.editor.selectedUid,
    ...(hl ? { highlight: { x: hl.x, y: hl.y } } : {}),
  };
}

/** Текущее время сервера по server_time ответа и локальным часам. */
export function serverClock(world: BaseWorld, receivedAtMs: number, nowMs: number): number {
  const base = world.serverTime ? Date.parse(world.serverTime) : receivedAtMs;
  return base + (nowMs - receivedAtMs);
}
