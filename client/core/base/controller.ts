// Контроллер редактора базы: склеивает слайс (model), черновик (editor), селекторы и Renderer 0.2.
// Это точка, которую шелл создаёт один раз и отдаёт UI (см. docs/open-questions.md, OQ-B10).
import type { Renderer } from '../../engine/renderer';
import type { Catalog, Pricing } from './catalog';
import { createBaseEditor, type BaseEditor } from './editor';
import { createBaseModel, type BaseModel, type BaseModelOptions } from './model';
import type { Grid } from './types';
import { buildWorldState, selectViewModel, serverClock, type BaseViewModel } from './view-model';

export interface BaseActions {
  toggleEdit(): void;
  select(uid: string | null): void;
  pickBuilding(type: string): void;
  confirmPlacing(): void;
  cancelPlacing(): void;
  rotate(): void;
  upgrade(uid: string): void;
  speedup(uid: string): void;
  askRemove(uid: string | null): void;
  remove(uid: string): void;
  clearObstacle(uid: string): void;
  collect(): void;
  undo(): void;
  redo(): void;
  save(): void;
  cancel(): void;
}

export interface BaseController {
  model: BaseModel;
  editor: BaseEditor;
  actions: BaseActions;
  viewModel(): BaseViewModel;
  /** Подписка на любые изменения модели/редактора (для хука UI). */
  subscribe(l: () => void): () => void;
  /** Подключить рендер: события ввода → редактор, изменения → render(). Возвращает отписку. */
  bindRenderer(r: Renderer): () => void;
}

export interface ControllerOptions extends Omit<BaseModelOptions, 'catalog'> {
  catalog: Catalog;
  pricing?: Pricing;
  grid: Grid;
  now?: () => number;
}

export function createBaseController(o: ControllerOptions): BaseController {
  const now = o.now ?? Date.now;
  const model = createBaseModel(o);
  const editor = createBaseEditor({ model, catalog: o.catalog, grid: o.grid });

  // Момент получения последнего server_time: от него идёт отсчёт оставшегося времени таймеров.
  let receivedAt = now();
  let lastServerTime = model.store.getState().confirmed.serverTime;
  model.store.subscribe((s) => {
    if (s.confirmed.serverTime !== lastServerTime) {
      lastServerTime = s.confirmed.serverTime;
      receivedAt = now();
    }
  });

  const viewModel = (): BaseViewModel => {
    const gs = model.store.getState();
    const es = editor.store.getState();
    return selectViewModel({
      world: gs.view,
      items: editor.effectiveItems(),
      editor: es,
      catalog: o.catalog,
      pricing: o.pricing,
      serverNowMs: serverClock(gs.confirmed, receivedAt, now()),
      placement: editor.placement(),
      rejection: gs.lastRejection ? { code: gs.lastRejection.code, message: gs.lastRejection.message } : null,
    });
  };

  const subscribe = (l: () => void): (() => void) => {
    const a = model.store.subscribe(l);
    const b = editor.store.subscribe(l);
    return () => {
      a();
      b();
    };
  };

  const c = model.commands;
  const actions: BaseActions = {
    toggleEdit: () => void editor.setEditing(!editor.store.getState().editing),
    select: (uid) => editor.select(uid),
    pickBuilding: (t) => editor.startPlacing(t),
    confirmPlacing: () => void editor.confirmPlacing(),
    cancelPlacing: () => editor.cancelPlacing(),
    rotate: () => editor.rotate(),
    upgrade: (uid) => void c.upgrade(uid),
    speedup: (uid) => void c.speedup(uid),
    askRemove: (uid) => editor.askRemove(uid),
    remove: (uid) => {
      editor.select(null);
      void c.remove(uid);
    },
    clearObstacle: (uid) => {
      editor.select(null);
      void c.clearObstacle(uid);
    },
    collect: () => void c.collect(),
    undo: () => editor.undo(),
    redo: () => editor.redo(),
    save: () => void editor.save(),
    cancel: () => editor.cancel(),
  };

  function bindRenderer(r: Renderer): () => void {
    const draw = (): void =>
      r.render(buildWorldState({ items: editor.effectiveItems(), editor: editor.store.getState(), catalog: o.catalog, grid: o.grid }));
    const off = r.on((e) => {
      const s = editor.store.getState();
      switch (e.type) {
        case 'select':
          if (s.placing) editor.movePlacing(e.x, e.y);
          else editor.select(e.uid);
          break;
        case 'hover':
          editor.setHover(e.tile);
          break;
        case 'dragStart':
          editor.dragStart(e.uid, e.x, e.y);
          break;
        case 'dragMove':
          editor.dragMove(e.x, e.y);
          break;
        case 'dragEnd':
          editor.dragEnd(e.x, e.y);
          break;
        default:
          break; // pan/zoom/tileTap обрабатывает сам рендер
      }
    });
    const unsub = subscribe(draw);
    draw();
    return () => {
      off();
      unsub();
    };
  }

  return { model, editor, actions, viewModel, subscribe, bindRenderer };
}
