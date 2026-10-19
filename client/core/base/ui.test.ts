// Тесты UI-слоя: view-model, привязка к Renderer 0.2 (мок) и компоненты client/ui/base (без DOM: дерево VNode).
// Лежат здесь, а не в ui/base, потому что vitest.include/tsconfig.include (корневые файлы client/) покрывают core/.
import { h } from 'preact';
import { describe, expect, it } from 'vitest';
import { BaseEditorView, BuildingCard, PlacementOverlay, ShopPanel } from '../../ui/base/components';
import { UI_KEYS, createT, formatDuration } from '../../ui/base/i18n';
import { createBaseController } from './controller';
import { toWorld } from './types';
import { baseState, catalog, click, expand, find, findAll, grid, mockApi, mockRenderer, names, pricing, startItems, textOf } from './test-kit';

const NOW = Date.parse('2026-01-01T00:00:00Z');

function setup(extra = {}) {
  const initial = baseState(startItems(), extra);
  const api = mockApi(initial);
  let n = 0;
  const ctl = createBaseController({ api, catalog, pricing, grid, newUid: () => `n${++n}`, now: () => NOW });
  ctl.model.store.reset(toWorld(initial));
  const r = mockRenderer();
  const off = ctl.bindRenderer(r);
  return { api, ctl, r, off };
}
const tEn = createT('en', names.en);
const tRu = createT('ru', names.ru);
const project = (x: number, y: number) => ({ x: x * 10, y: y * 5 });
const view = (ctl: ReturnType<typeof setup>['ctl'], t = tEn) =>
  expand(h(BaseEditorView, { vm: ctl.viewModel(), actions: ctl.actions, t, project }));

describe('привязка к Renderer 0.2', () => {
  it('первый render отдаёт items/water/footprints из контента; select выбирает здание', () => {
    const { r, ctl } = setup();
    const w = r.renders[0]!;
    expect(w.gridSize).toBe(grid.size);
    expect(w.items).toHaveLength(startItems().length);
    expect(w.footprints?.b_hq).toEqual([4, 4]);
    r.emit({ type: 'select', uid: 'cm1', x: 14, y: 20 });
    expect(r.renders.at(-1)?.selectedUid).toBe('cm1');
    expect(ctl.viewModel().card?.uid).toBe('cm1');
    r.emit({ type: 'select', uid: null, x: 1, y: 1 });
    expect(ctl.viewModel().card).toBeNull();
  });

  it('drag-drop: рендер видит перетаскиваемое здание на новом месте, подсветка зелёная/красная', () => {
    const { r, ctl } = setup();
    ctl.actions.toggleEdit();
    r.emit({ type: 'select', uid: 'cm1', x: 14, y: 20 });
    r.emit({ type: 'dragStart', uid: 'cm1', x: 14, y: 20 });
    r.emit({ type: 'dragMove', uid: 'cm1', x: 14, y: 26 });
    expect(r.renders.at(-1)?.items?.find((i) => i.uid === 'cm1')).toMatchObject({ x: 14, y: 26 });
    expect(ctl.viewModel().overlay.tiles.every((t) => t.ok)).toBe(true);
    r.emit({ type: 'dragMove', uid: 'cm1', x: 20, y: 20 });
    const bad = ctl.viewModel().overlay;
    expect(bad.reason).toBe('overlap');
    expect(bad.tiles.some((t) => !t.ok)).toBe(true);
    r.emit({ type: 'dragEnd', uid: 'cm1', x: 14, y: 26 });
    expect(ctl.editor.dirty()).toBe(true);
    ctl.actions.undo();
    expect(ctl.editor.dirty()).toBe(false);
    expect(r.renders.at(-1)?.items?.find((i) => i.uid === 'cm1')).toMatchObject({ x: 14, y: 20 });
  });

  it('вне режима редактирования drag игнорируется; отписка отключает рендер', () => {
    const { r, ctl, off } = setup();
    r.emit({ type: 'select', uid: 'cm1', x: 14, y: 20 });
    r.emit({ type: 'dragStart', uid: 'cm1', x: 14, y: 20 });
    expect(ctl.editor.store.getState().drag).toBeNull();
    off();
    const n = r.renders.length;
    r.emit({ type: 'select', uid: 'hq', x: 20, y: 20 });
    expect(r.renders).toHaveLength(n);
  });

  it('при постановке тап по клетке двигает призрак (виден рендеру), а не выбирает', () => {
    const { r, ctl } = setup();
    ctl.actions.pickBuilding('b_vault');
    r.emit({ type: 'select', uid: null, x: 30, y: 30 });
    const ghost = r.renders.at(-1)?.items?.find((i) => i.uid === '__ghost__');
    expect(ghost).toMatchObject({ type: 'b_vault', x: 30, y: 30 });
    expect(ctl.viewModel().placing?.ok).toBe(true);
  });
});

describe('view-model', () => {
  it('магазин: цены, лимиты, блокировки по штабу и билдерам', () => {
    const { ctl } = setup();
    const shop = ctl.viewModel().shop;
    const by = (id: string) => shop.find((e) => e.type === id)!;
    expect(by('b_vault').blocked).toBeNull();
    expect(by('b_vault').cost).toEqual(pricing.cost('b_vault', 1));
    expect(by('b_radar').blocked).toBe('locked');
    expect(by('x_flag').blocked).toBe('no_price');
    expect(shop.some((e) => e.type === 'b_hq' || e.type === 'o_rock')).toBe(false);
    const busy = setup({ builders: { total: 1, busy: 1 }, timers: [{ id: 't', kind: 'upgrade', target_id: 'cm1', finish_at: '2026-01-01T01:00:00Z', to_level: 2 }] });
    expect(busy.ctl.viewModel().shop.find((e) => e.type === 'b_vault')?.blocked).toBe('no_builder');
  });

  it('радиусы обороны: зона огня и мёртвая зона из defences.yaml', () => {
    const { ctl } = setup();
    ctl.actions.select('mg1');
    const radii = ctl.viewModel().overlay.radii;
    expect(radii.map((r) => r.kind)).toEqual(['range']);
    const sn = setup({ layout: { items: [...startItems(), { uid: 'sn', type: 'd_sniper_tower', level: 1, x: 30, y: 30, rot: 0 }] } });
    sn.ctl.actions.select('sn');
    expect(sn.ctl.viewModel().overlay.radii.map((r) => r.kind)).toEqual(['range', 'dead']);
  });

  it('карточка: таймер по серверному времени, цена ускорения из building_costs.yaml', () => {
    const { ctl } = setup({
      timers: [{ id: 't', kind: 'upgrade', target_id: 'cm1', finish_at: '2026-01-01T02:00:00Z', to_level: 2 }],
      builders: { total: 2, busy: 1 },
    });
    ctl.actions.select('cm1');
    const c = ctl.viewModel().card!;
    expect(c.timer).toMatchObject({ kind: 'upgrade', toLevel: 2, remainingS: 7200 });
    expect(c.speedupGems).toBe(pricing.speedupGems(7200));
    expect(c.upgrade.blocked).toBe('busy');
  });
});

describe('компоненты', () => {
  it('ShopPanel: названия по name_key (ru/en), заблокированные кнопки, выбор типа', () => {
    const { ctl } = setup();
    const picks: string[] = [];
    const tree = expand(h(ShopPanel, { entries: ctl.viewModel().shop, t: tEn, onPick: (x: string) => picks.push(x) }));
    expect(textOf(find(tree, 'shop-b_vault')!)).toContain(names.en['b.vault']);
    expect(textOf(find(expand(h(ShopPanel, { entries: ctl.viewModel().shop, t: tRu, onPick: () => undefined })), 'shop-b_vault')!)).toContain(names.ru['b.vault']);
    expect(textOf(find(tree, 'shop-b_radar-block')!)).toContain('HQ 6');
    const btn = find(tree, 'shop-b_vault')!.children.find((c) => typeof c !== 'string' && c.tag === 'button');
    click(btn as never);
    expect(picks).toEqual(['b_vault']);
    expect(() => click(find(tree, 'shop-b_radar')!.children.find((c) => typeof c !== 'string' && c.tag === 'button') as never)).toThrow('disabled');
  });

  it('BuildingCard: уровень, улучшить, снести в два шага', () => {
    const { ctl, api } = setup();
    ctl.actions.select('cm1');
    let tree = view(ctl);
    expect(textOf(find(tree, 'card-level')!)).toBe(`${tEn('ui.level')} 1/10`);
    click(find(tree, 'card-upgrade'));
    expect(api.calls.sent[0]?.commands[0]).toEqual({ op: 'upgrade', args: { uid: 'cm1', to: 2 } });
    click(find(view(ctl), 'card-remove'));
    tree = view(ctl);
    expect(find(tree, 'card-remove')).toBeUndefined();
    click(find(tree, 'card-remove-no'));
    expect(find(view(ctl), 'card-remove')).toBeDefined();
  });

  it('карточка препятствия: «очистить» с ценой и наградой, без улучшения и сноса', () => {
    const { ctl, api } = setup();
    ctl.actions.select('rock');
    const tree = view(ctl);
    expect(find(tree, 'card-upgrade')).toBeUndefined();
    expect(find(tree, 'card-remove')).toBeUndefined();
    expect(textOf(find(tree, 'card-clear')!)).toContain('200');
    expect(textOf(find(tree, 'card-reward')!)).toContain('1');
    click(find(tree, 'card-clear'));
    expect(api.calls.sent[0]?.commands[0]).toEqual({ op: 'clear_obstacle', args: { uid: 'rock' } });
  });

  it('Toolbar: undo/redo/сохранить доступны по состоянию редактора', async () => {
    const { ctl, r, api } = setup();
    let tree = view(ctl);
    expect(find(tree, 'undo')).toBeUndefined();
    click(find(tree, 'toggle-edit'));
    tree = view(ctl);
    expect(find(tree, 'undo')?.props.disabled).toBe(true);
    expect(find(tree, 'save')?.props.disabled).toBe(true);
    r.emit({ type: 'select', uid: 'cm1', x: 14, y: 20 });
    r.emit({ type: 'dragStart', uid: 'cm1', x: 14, y: 20 });
    r.emit({ type: 'dragEnd', uid: 'cm1', x: 14, y: 24 });
    tree = view(ctl);
    expect(find(tree, 'undo')?.props.disabled).toBe(false);
    expect(find(tree, 'toggle-edit')?.props.disabled).toBe(true); // нельзя выйти с несохранённым
    click(find(tree, 'undo'));
    expect(find(view(ctl), 'redo')?.props.disabled).toBe(false);
    click(find(view(ctl), 'redo'));
    click(find(view(ctl), 'save'));
    await new Promise((res) => setTimeout(res, 0));
    expect(api.calls.saved).toHaveLength(1);
    expect(ctl.editor.dirty()).toBe(false);
  });

  it('PlacementBar: красный отказ с причиной, зелёное подтверждение строит здание', async () => {
    const { ctl, api, r } = setup();
    ctl.actions.pickBuilding('b_vault');
    r.emit({ type: 'select', uid: null, x: 20, y: 20 });
    let tree = view(ctl);
    expect(find(tree, 'placement-confirm')?.props.disabled).toBe(true);
    expect(textOf(find(tree, 'placement-state')!)).toBe(tEn('reason.overlap'));
    expect(findAll(tree, 'overlay-tile').some((t) => t.props['data-ok'] === false)).toBe(true);
    r.emit({ type: 'select', uid: null, x: 30, y: 30 });
    tree = view(ctl);
    expect(findAll(tree, 'overlay-tile').every((t) => t.props['data-ok'] === true)).toBe(true);
    click(find(tree, 'placement-confirm'));
    await new Promise((res) => setTimeout(res, 0));
    expect(api.calls.sent[0]?.commands[0]?.op).toBe('build');
  });

  it('PlacementOverlay: клетки (зелёные/красные) и радиусы проецируются', () => {
    const tree = expand(
      h(PlacementOverlay, {
        overlay: { tiles: [{ x: 1, y: 1, ok: true }, { x: 2, y: 1, ok: false }], radii: [{ cx: 5, cy: 5, r: 3, kind: 'range' }, { cx: 5, cy: 5, r: 1, kind: 'dead' }], reason: 'overlap' },
        project,
      }),
    );
    const tiles = findAll(tree, 'overlay-tile');
    expect(tiles.map((t) => t.props.fill)).toEqual(['#2fbf71', '#e5484d']);
    expect(String(tiles[0]?.props.points)).toBe('10.0,5.0 20.0,5.0 20.0,10.0 10.0,10.0');
    expect(String(find(tree, 'overlay-radius-range')?.props.points).split(' ')).toHaveLength(48);
    expect(find(tree, 'overlay-radius-dead')?.props['stroke-dasharray']).toBeDefined();
  });

  it('отказ сервера показывается пользователем (Notice) на выбранном языке', async () => {
    const { ctl, api } = setup();
    const { ApiError } = await import('../api');
    api.onSend = () => Promise.reject(new ApiError('insufficient_resources', 422, 'no cash'));
    ctl.actions.select('cm1');
    await ctl.model.commands.upgrade('cm1');
    expect(textOf(find(view(ctl), 'notice')!)).toBe(tEn('reason.insufficient_resources'));
    expect(textOf(find(view(ctl, tRu), 'notice')!)).toBe(tRu('reason.insufficient_resources'));
  });

  it('BuildingCard напрямую: прогресс стройки', () => {
    const { ctl } = setup({ timers: [{ id: 't', kind: 'build', target_id: 'cm1', finish_at: '2026-01-01T00:01:05Z', to_level: 1 }] });
    ctl.actions.select('cm1');
    const noop = () => undefined;
    const tree = expand(
      h(BuildingCard, { card: ctl.viewModel().card!, t: tEn, onUpgrade: noop, onSpeedup: noop, onAskRemove: noop, onRemove: noop, onClear: noop, onRotate: noop, onClose: noop }),
    );
    expect(textOf(find(tree, 'card-timer')!)).toContain('1 m 5 s');
  });
});

describe('i18n', () => {
  it('ключи ru и en совпадают, форматирование длительности', () => {
    expect([...UI_KEYS.ru].sort()).toEqual([...UI_KEYS.en].sort());
    expect(formatDuration(3725, tEn)).toBe('1 h 2 m');
    expect(formatDuration(0, tRu)).toBe('0 с');
    expect(tEn('b.hq')).toBe(names.en['b.hq']);
    expect(tEn('unknown.key')).toBe('unknown.key');
  });
});
