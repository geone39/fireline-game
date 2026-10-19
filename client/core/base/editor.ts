// Режим редактирования базы: выбор, постановка нового здания, перетаскивание с undo/redo.
// Перестановки живут в черновике (overrides поверх view) и уходят одним PUT /v1/base/layout по «Сохранить».
// ASSUMPTION: undo/redo покрывает только черновик перестановок/поворотов; build/remove/upgrade/... — команды,
// они применяются сразу (оптимистично, с откатом сервером) и до сохранения не отменяются.
import { ApiError } from '../api';
import { createStore, type Store } from '../store';
import type { Catalog } from './catalog';
import type { BaseModel } from './model';
import { validatePlacement, type Placement, type PlacementCtx } from './validate';
import type { BaseItem, Grid } from './types';

export interface Pose {
  x: number;
  y: number;
  rot: number;
}
export type Overrides = Readonly<Record<string, Pose>>;

export interface EditorState {
  editing: boolean;
  selectedUid: string | null;
  /** Новое здание ещё не построено: призрак на сетке, ждёт подтверждения. */
  placing: (Pose & { type: string }) | null;
  /** Перетаскивание: смещение указателя внутри здания и текущая поза. */
  drag: { uid: string; offX: number; offY: number; pose: Pose } | null;
  hover: { x: number; y: number } | null;
  overrides: Overrides;
  undoStack: readonly Overrides[];
  redoStack: readonly Overrides[];
  saving: boolean;
  /** uid здания, для которого показано подтверждение сноса. */
  askRemove: string | null;
  /** Код последней ошибки сохранения / причина отказа клиентской проверки. */
  notice: string | null;
}

export const GHOST_UID = '__ghost__';

export interface BaseEditor {
  store: Store<EditorState>;
  setEditing(on: boolean): boolean;
  select(uid: string | null): void;
  askRemove(uid: string | null): void;
  startPlacing(type: string): void;
  cancelPlacing(): void;
  movePlacing(x: number, y: number): void;
  confirmPlacing(): Promise<string | null>;
  rotate(): void;
  dragStart(uid: string, x: number, y: number): void;
  dragMove(x: number, y: number): void;
  dragEnd(x: number, y: number): void;
  undo(): void;
  redo(): void;
  save(): Promise<boolean>;
  cancel(): void;
  setHover(tile: { x: number; y: number } | null): void;
  /** Раскладка с учётом черновика (и перетаскивания) — то, что рисуется и что уйдёт в PUT. */
  effectiveItems(): BaseItem[];
  dirty(): boolean;
  /** Подсветка допустимости для призрака или перетаскиваемого/повёрнутого здания; null, если нечего проверять. */
  placement(): Placement | null;
}

export interface EditorOptions {
  model: BaseModel;
  catalog: Catalog;
  grid: Grid;
}

const INITIAL: EditorState = {
  editing: false,
  selectedUid: null,
  placing: null,
  drag: null,
  hover: null,
  overrides: {},
  undoStack: [],
  redoStack: [],
  saving: false,
  askRemove: null,
  notice: null,
};

export function applyOverrides(items: readonly BaseItem[], overrides: Overrides, drag: EditorState['drag']): BaseItem[] {
  return items.map((i) => {
    const p = drag && drag.uid === i.uid ? drag.pose : overrides[i.uid];
    return p ? { ...i, x: p.x, y: p.y, rot: p.rot } : i;
  });
}

export function createBaseEditor(o: EditorOptions): BaseEditor {
  const store = createStore<EditorState>({ ...INITIAL });
  const ctx: PlacementCtx = { catalog: o.catalog, grid: o.grid };
  const items = (): readonly BaseItem[] => o.model.store.getState().view.items;
  const set = (p: Partial<EditorState>): void => store.setState(p);
  const effective = (): BaseItem[] => {
    const s = store.getState();
    return applyOverrides(items(), s.overrides, s.drag);
  };
  const base = (uid: string): BaseItem | undefined => items().find((i) => i.uid === uid);

  /** Записать новую позу в черновик (с undo). Поза, совпавшая с серверной, убирает override. */
  function commitPose(uid: string, pose: Pose): void {
    const s = store.getState();
    const orig = base(uid);
    if (!orig) return;
    const next: Record<string, Pose> = { ...s.overrides };
    if (pose.x === orig.x && pose.y === orig.y && pose.rot === orig.rot) delete next[uid];
    else next[uid] = pose;
    const cur = s.overrides[uid] ?? { x: orig.x, y: orig.y, rot: orig.rot };
    if (cur.x === pose.x && cur.y === pose.y && cur.rot === pose.rot) return;
    set({ overrides: next, undoStack: [...s.undoStack, s.overrides], redoStack: [], notice: null });
  }

  const poseOf = (i: BaseItem): Pose => ({ x: i.x, y: i.y, rot: i.rot });

  function checkMove(uid: string, pose: Pose): Placement | null {
    const it = effective().find((i) => i.uid === uid);
    return it ? validatePlacement(ctx, effective(), { uid, type: it.type, ...pose }, 'move') : null;
  }

  const self: BaseEditor = {
    store,
    effectiveItems: effective,
    dirty: () => Object.keys(store.getState().overrides).length > 0,

    setEditing(on) {
      if (!on && self.dirty()) return false; // выйти можно через save() или cancel()
      set({ ...INITIAL, editing: on, selectedUid: store.getState().selectedUid });
      return true;
    },
    select(uid) {
      if (store.getState().drag) return;
      set({ selectedUid: uid, placing: null, askRemove: null, notice: null });
    },
    askRemove: (uid) => set({ askRemove: uid }),
    startPlacing(type) {
      const def = o.catalog.get(type);
      if (!def) return;
      const hover = store.getState().hover;
      const mid = Math.floor(o.grid.size / 2);
      set({ placing: { type, x: hover?.x ?? mid, y: hover?.y ?? mid, rot: 0 }, selectedUid: null, notice: null });
    },
    cancelPlacing: () => set({ placing: null, notice: null }),
    movePlacing(x, y) {
      const p = store.getState().placing;
      if (p) set({ placing: { ...p, x, y } });
    },
    async confirmPlacing() {
      const p = store.getState().placing;
      const pl = self.placement();
      if (!p || !pl) return null;
      if (!pl.ok) {
        set({ notice: pl.reason });
        return null;
      }
      set({ placing: null, notice: null });
      const uid = await o.model.commands.build(p.type, p.x, p.y, p.rot);
      set({ selectedUid: uid });
      return uid;
    },
    rotate() {
      const s = store.getState();
      if (s.placing) {
        const d = o.catalog.get(s.placing.type);
        if (d && d.w !== d.h) set({ placing: { ...s.placing, rot: (s.placing.rot + 1) % 4 } });
        return;
      }
      if (!s.editing || !s.selectedUid) return;
      const it = effective().find((i) => i.uid === s.selectedUid);
      const d = it && o.catalog.get(it.type);
      if (!it || !d || d.w === d.h) return;
      const pose = { x: it.x, y: it.y, rot: (it.rot + 1) % 4 };
      const pl = checkMove(it.uid, pose);
      if (pl?.ok) commitPose(it.uid, pose);
      else set({ notice: pl?.reason ?? null });
    },

    dragStart(uid, x, y) {
      const s = store.getState();
      const it = effective().find((i) => i.uid === uid);
      if (!s.editing || !it || s.placing || s.selectedUid !== uid) return;
      const d = o.catalog.get(it.type);
      if (!d || d.kind === 'obstacle') return;
      set({ drag: { uid, offX: x - it.x, offY: y - it.y, pose: poseOf(it) }, notice: null });
    },
    dragMove(x, y) {
      const d = store.getState().drag;
      if (d) set({ drag: { ...d, pose: { ...d.pose, x: x - d.offX, y: y - d.offY } } });
    },
    dragEnd(x, y) {
      const d = store.getState().drag;
      if (!d) return;
      const pose = { ...d.pose, x: x - d.offX, y: y - d.offY };
      set({ drag: null });
      const pl = checkMove(d.uid, pose);
      if (pl?.ok) commitPose(d.uid, pose);
      else set({ notice: pl?.reason ?? null }); // здание возвращается на прежнее место
    },

    undo() {
      const s = store.getState();
      const prev = s.undoStack[s.undoStack.length - 1];
      if (!prev) return;
      set({ overrides: prev, undoStack: s.undoStack.slice(0, -1), redoStack: [...s.redoStack, s.overrides], notice: null });
    },
    redo() {
      const s = store.getState();
      const next = s.redoStack[s.redoStack.length - 1];
      if (!next) return;
      set({ overrides: next, redoStack: s.redoStack.slice(0, -1), undoStack: [...s.undoStack, s.overrides], notice: null });
    },
    cancel: () => set({ overrides: {}, undoStack: [], redoStack: [], drag: null, placing: null, notice: null }),

    async save() {
      if (!self.dirty() || store.getState().saving) return false;
      set({ saving: true, notice: null });
      try {
        await o.model.store.flush();
        if (o.model.store.getState().pending.length > 0) {
          set({ saving: false, notice: 'offline' });
          return false;
        }
        const state = await o.model.api.saveLayout(effective());
        o.model.applyServerState(state);
        set({ overrides: {}, undoStack: [], redoStack: [], saving: false });
        return true;
      } catch (e) {
        set({ saving: false, notice: e instanceof ApiError ? e.code : 'internal' });
        return false;
      }
    },

    setHover(tile) {
      const s = store.getState();
      set({ hover: tile });
      if (s.placing && tile) set({ placing: { ...s.placing, x: tile.x, y: tile.y } });
    },

    placement() {
      const s = store.getState();
      if (s.placing) {
        return validatePlacement(ctx, effective(), { uid: GHOST_UID, ...s.placing }, 'build');
      }
      if (s.drag) return checkMove(s.drag.uid, s.drag.pose);
      return null;
    },
  };
  return self;
}
