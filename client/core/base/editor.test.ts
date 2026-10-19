import { describe, expect, it } from 'vitest';
import { ApiError } from '../api';
import { createBaseEditor } from './editor';
import { createBaseModel } from './model';
import { toWorld } from './types';
import { baseState, catalog, grid, mockApi, pricing, startItems } from './test-kit';

function setup() {
  const initial = baseState(startItems());
  const api = mockApi(initial);
  let n = 0;
  const model = createBaseModel({ api, catalog, pricing, newUid: () => `n${++n}` });
  model.store.reset(toWorld(initial));
  const editor = createBaseEditor({ model, catalog, grid });
  editor.setEditing(true);
  editor.select('cm1');
  return { api, model, editor };
}

const pos = (e: ReturnType<typeof setup>['editor'], uid: string) => {
  const i = e.effectiveItems().find((x) => x.uid === uid)!;
  return [i.x, i.y];
};

describe('редактор: drag-drop с черновиком', () => {
  it('drag по допустимой клетке двигает здание в черновике, но не в сторе', () => {
    const { editor, model } = setup();
    editor.dragStart('cm1', 15, 21); // схватили за клетку (+1,+1) от якоря (14,20)
    editor.dragMove(18, 24);
    expect(pos(editor, 'cm1')).toEqual([17, 23]); // живое перетаскивание учитывает смещение захвата
    expect(editor.placement()?.ok).toBe(true);
    editor.dragEnd(18, 24);
    expect(pos(editor, 'cm1')).toEqual([17, 23]);
    expect(editor.dirty()).toBe(true);
    expect(model.store.getState().view.items.find((i) => i.uid === 'cm1')).toMatchObject({ x: 14, y: 20 });
  });

  it('drop на занятую клетку/воду/за карту возвращает здание на место', () => {
    const { editor } = setup();
    editor.dragStart('cm1', 14, 20);
    editor.dragMove(20, 20);
    expect(editor.placement()).toMatchObject({ ok: false, reason: 'overlap' });
    editor.dragEnd(20, 20);
    expect(pos(editor, 'cm1')).toEqual([14, 20]);
    expect(editor.dirty()).toBe(false);
    expect(editor.store.getState().notice).toBe('overlap');
    editor.dragStart('cm1', 14, 20);
    editor.dragEnd(-5, 3);
    expect(editor.store.getState().notice).toBe('out_of_bounds');
  });

  it('drag стартует только в режиме редактирования и только на выбранном', () => {
    const { editor } = setup();
    editor.dragStart('hq', 20, 20);
    expect(editor.store.getState().drag).toBeNull();
    editor.cancel();
    editor.setEditing(false);
    editor.dragStart('cm1', 14, 20);
    expect(editor.store.getState().drag).toBeNull();
  });

  it('undo/redo по шагам; новое действие сбрасывает redo', () => {
    const { editor } = setup();
    const move = (uid: string, x: number, y: number) => {
      editor.select(uid);
      const it = editor.effectiveItems().find((i) => i.uid === uid)!;
      editor.dragStart(uid, it.x, it.y);
      editor.dragEnd(x, y);
    };
    move('cm1', 14, 24);
    move('mg1', 26, 26);
    expect([pos(editor, 'cm1'), pos(editor, 'mg1')]).toEqual([[14, 24], [26, 26]]);
    editor.undo();
    expect(pos(editor, 'mg1')).toEqual([26, 20]);
    expect(pos(editor, 'cm1')).toEqual([14, 24]);
    editor.undo();
    expect(editor.dirty()).toBe(false);
    expect(editor.store.getState().undoStack).toHaveLength(0);
    editor.redo();
    expect(pos(editor, 'cm1')).toEqual([14, 24]);
    editor.redo();
    expect(pos(editor, 'mg1')).toEqual([26, 26]);
    editor.undo();
    move('mg1', 30, 30); // новое действие
    expect(editor.store.getState().redoStack).toHaveLength(0);
    editor.undo();
    editor.undo();
    editor.undo(); // лишний undo — безопасен
    expect(editor.dirty()).toBe(false);
  });

  it('возврат на исходную клетку убирает override', () => {
    const { editor } = setup();
    editor.dragStart('cm1', 14, 20);
    editor.dragEnd(14, 24);
    editor.dragStart('cm1', 14, 24);
    editor.dragEnd(14, 20);
    expect(editor.dirty()).toBe(false);
  });

  it('выйти из режима при несохранённых изменениях нельзя; cancel сбрасывает', () => {
    const { editor } = setup();
    editor.dragStart('cm1', 14, 20);
    editor.dragEnd(14, 24);
    expect(editor.setEditing(false)).toBe(false);
    expect(editor.store.getState().editing).toBe(true);
    editor.cancel();
    expect(editor.setEditing(false)).toBe(true);
  });

  it('save: PUT /v1/base/layout с полной раскладкой, затем черновик очищается и версия растёт', async () => {
    const { editor, api, model } = setup();
    editor.dragStart('cm1', 14, 20);
    editor.dragEnd(14, 24);
    expect(await editor.save()).toBe(true);
    expect(api.calls.saved).toHaveLength(1);
    expect(api.calls.saved[0]?.find((i) => i.uid === 'cm1')).toMatchObject({ x: 14, y: 24 });
    expect(api.calls.saved[0]).toHaveLength(startItems().length);
    expect(editor.dirty()).toBe(false);
    expect(model.store.getState().confirmed.version).toBe(2);
    expect(model.store.getState().view.items.find((i) => i.uid === 'cm1')).toMatchObject({ y: 24 });
  });

  it('save: отказ сервера оставляет черновик и показывает код', async () => {
    const { editor, api } = setup();
    api.onSave = () => Promise.reject(new ApiError('invalid', 400, 'sealed'));
    editor.dragStart('cm1', 14, 20);
    editor.dragEnd(14, 24);
    expect(await editor.save()).toBe(false);
    expect(editor.dirty()).toBe(true);
    expect(editor.store.getState().notice).toBe('invalid');
    expect(editor.store.getState().saving).toBe(false);
  });

  it('save: сначала досылает очередь команд; офлайн — не сохраняет', async () => {
    const { editor, api, model } = setup();
    editor.dragStart('cm1', 14, 20);
    editor.dragEnd(14, 24);
    api.onSend = () => Promise.reject(new ApiError('network', 0, 'down'));
    model.store.enqueue({ op: 'remove', args: { uid: 'rock' } });
    expect(await editor.save()).toBe(false);
    expect(api.calls.saved).toHaveLength(0);
    expect(editor.store.getState().notice).toBe('offline');
  });
});

describe('редактор: постановка нового здания', () => {
  it('призрак подсвечивается, подтверждение возможно только на допустимой клетке', async () => {
    const { editor, api } = setup();
    editor.startPlacing('b_vault');
    editor.movePlacing(20, 20);
    expect(editor.placement()).toMatchObject({ ok: false, reason: 'overlap' });
    expect(await editor.confirmPlacing()).toBeNull();
    expect(api.calls.sent).toHaveLength(0);
    editor.setHover({ x: 30, y: 30 }); // курсор ведёт призрак
    expect(editor.placement()?.ok).toBe(true);
    const uid = await editor.confirmPlacing();
    expect(uid).toBe('n1');
    expect(api.calls.sent[0]?.commands[0]).toEqual({ op: 'build', args: { uid: 'n1', type: 'b_vault', x: 30, y: 30, rot: 0 } });
    expect(editor.store.getState().placing).toBeNull();
    expect(editor.store.getState().selectedUid).toBe('n1');
  });

  it('rotate: у квадратных футпринтов — no-op', () => {
    const { editor } = setup();
    editor.startPlacing('b_vault');
    editor.rotate();
    expect(editor.store.getState().placing?.rot).toBe(0);
  });

  it('удаление: ask → remove', () => {
    const { editor } = setup();
    editor.askRemove('cm1');
    expect(editor.store.getState().askRemove).toBe('cm1');
    editor.select('hq');
    expect(editor.store.getState().askRemove).toBeNull();
  });
});
