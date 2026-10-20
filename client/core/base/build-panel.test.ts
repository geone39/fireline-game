// Issue #62: панель построек — нехватка ресурсов, ускорение за гемы, компоновка (не перекрывает ввод канваса).
import { h } from 'preact';
import { describe, expect, it } from 'vitest';
import { BaseEditorView, BuildingCard, ShopPanel } from '../../ui/base/components';
import { createT, formatGems, UI_KEYS } from '../../ui/base/i18n';
import { createBaseController } from './controller';
import { shortfallOf } from './view-model';
import { toWorld } from './types';
import { baseState, catalog, click, expand, find, grid, mockApi, names, pricing, startItems, textOf } from './test-kit';

const NOW = Date.parse('2026-01-01T00:00:00Z');
const tEn = createT('en', names.en);
const tRu = createT('ru', names.ru);
const project = (x: number, y: number) => ({ x, y });

function setup(wallet: Record<string, number> | undefined, extra = {}) {
  const initial = baseState(startItems(), extra);
  const ctl = createBaseController({ api: mockApi(initial), catalog, pricing, grid, newUid: () => 'n1', now: () => NOW, wallet: () => wallet });
  ctl.model.store.reset(toWorld(initial));
  return ctl;
}
const view = (ctl: ReturnType<typeof setup>, t = tEn) => expand(h(BaseEditorView, { vm: ctl.viewModel(), actions: ctl.actions, t, project }));
const shopBtn = (tree: ReturnType<typeof view>, type: string) => find(tree, `shop-${type}`)!.children.find((c) => typeof c !== 'string' && c.tag === 'button');
const vaultCash = pricing.cost('b_vault', 1).cash!;

describe('панель построек: не хватает ресурсов', () => {
  it('shortfallOf: сравнивает с кошельком; без кошелька или при достатке — null', () => {
    expect(shortfallOf({ cash: 100, alloy: 5 }, { cash: 40, alloy: 5 })).toEqual({ cash: { need: 100, have: 40 } });
    expect(shortfallOf({ cash: 100 }, {})).toEqual({ cash: { need: 100, have: 0 } });
    expect(shortfallOf({ cash: 100 }, { cash: 100 })).toBeNull();
    expect(shortfallOf({ cash: 100 }, undefined)).toBeNull();
  });

  it('здание не по карману: disabled, aria-describedby → «нужно X, есть Y», title', () => {
    const ctl = setup({ cash: vaultCash - 1, fuel: 0, alloy: 0, gems: 0 });
    const e = ctl.viewModel().shop.find((s) => s.type === 'b_vault')!;
    expect(e.blocked).toBe('no_funds');
    expect(e.short).toEqual({ cash: { need: vaultCash, have: vaultCash - 1 } });
    const tree = view(ctl);
    const btn = shopBtn(tree, 'b_vault')!;
    expect(typeof btn !== 'string' && btn.props.disabled).toBe(true);
    expect(typeof btn !== 'string' && btn.props['aria-describedby']).toBe('shop-b_vault-block');
    expect(find(tree, 'shop-b_vault')!.props['data-blocked']).toBe('no_funds');
    const hint = `Need ${vaultCash} ${tEn('res.cash')}, have ${vaultCash - 1} ${tEn('res.cash')}`;
    expect(textOf(find(tree, 'shop-b_vault-block')!).trim()).toBe(hint);
    expect(typeof btn !== 'string' && btn.props.title).toBe(hint);
    expect(() => click(btn as never)).toThrow('disabled');
    expect(textOf(find(view(ctl, tRu), 'shop-b_vault-block')!)).toContain('Нужно');
  });

  it('хватает — кнопка активна; кошелёк неизвестен — предпроверки нет (решает сервер)', () => {
    const rich = setup({ cash: vaultCash, gems: 0 });
    expect(rich.viewModel().shop.find((s) => s.type === 'b_vault')!.blocked).toBeNull();
    const unknown = setup(undefined);
    expect(unknown.viewModel().shop.find((s) => s.type === 'b_vault')!.blocked).toBeNull();
  });

  it('приоритет причин: штаб и лимит важнее нехватки денег', () => {
    const ctl = setup({ cash: 0 });
    expect(ctl.viewModel().shop.find((s) => s.type === 'b_radar')!.blocked).toBe('locked');
  });

  it('улучшение в карточке тоже подсвечивает нехватку', () => {
    const ctl = setup({ cash: 0 });
    ctl.actions.select('cm1');
    const tree = view(ctl);
    expect(find(tree, 'card-upgrade')!.props.disabled).toBe(true);
    expect(textOf(find(tree, 'card-upgrade-block')!)).toContain('Need');
  });
});

const timer = { id: 't', kind: 'upgrade', target_id: 'cm1', finish_at: '2026-01-01T02:00:00Z', to_level: 2 };
const gemsNeeded = pricing.speedupGems(7200);

describe('ускорение за гемы', () => {
  it('гемов меньше N — кнопка неактивна, рядом «нужно N, есть M»', () => {
    const ctl = setup({ cash: 999999, gems: gemsNeeded - 1 }, { timers: [timer], builders: { total: 2, busy: 1 } });
    ctl.actions.select('cm1');
    const tree = view(ctl);
    const btn = find(tree, 'card-speedup')!;
    expect(btn.props.disabled).toBe(true);
    expect(btn.props['aria-describedby']).toBe('card-speedup-block');
    expect(textOf(find(tree, 'card-speedup-block')!).trim()).toBe(`Need ${formatGems(gemsNeeded, tEn)}, have ${formatGems(gemsNeeded - 1, tEn)}`);
  });

  it('гемов хватает или кошелёк неизвестен — активна', () => {
    for (const wallet of [{ gems: gemsNeeded }, undefined]) {
      const ctl = setup(wallet, { timers: [timer], builders: { total: 2, busy: 1 } });
      ctl.actions.select('cm1');
      const tree = view(ctl);
      expect(find(tree, 'card-speedup')!.props.disabled).toBe(false);
      expect(find(tree, 'card-speedup-block')).toBeUndefined();
    }
  });

  it('склонение: 1 гем, 2 гема, 5 гемов, 11 гемов, 21 гем, 22 гема; en: 1 gem, 2 gems', () => {
    const ru = [1, 2, 4, 5, 11, 12, 21, 22, 25, 111].map((n) => formatGems(n, tRu));
    expect(ru).toEqual(['1 гем', '2 гема', '4 гема', '5 гемов', '11 гемов', '12 гемов', '21 гем', '22 гема', '25 гемов', '111 гемов']);
    expect([1, 2, 0].map((n) => formatGems(n, tEn))).toEqual(['1 gem', '2 gems', '0 gems']);
  });

  it('текст кнопки использует правильную форму; ключи ru/en совпадают', () => {
    const ctl = setup({ gems: 99 }, { timers: [timer], builders: { total: 2, busy: 1 } });
    ctl.actions.select('cm1');
    const c = ctl.viewModel().card!;
    const noop = () => undefined;
    const render = (speedupGems: number) =>
      textOf(
        find(
          expand(h(BuildingCard, { card: { ...c, speedupGems, speedupAffordable: true }, t: tRu, onUpgrade: noop, onSpeedup: noop, onAskRemove: noop, onRemove: noop, onClear: noop, onRotate: noop, onClose: noop })),
          'card-speedup',
        )!,
      );
    expect(render(1)).toBe('Ускорить · 1 гем');
    expect(render(3)).toBe('Ускорить · 3 гема');
    expect(render(7)).toBe('Ускорить · 7 гемов');
    expect([...UI_KEYS.ru].sort()).toEqual([...UI_KEYS.en].sort());
  });
});

describe('компоновка: панель не перекрывает ввод канваса', () => {
  it('корень редактора пропускает клики, каждая панель включает их сама', () => {
    const tree = view(setup({ cash: 1 }));
    expect((find(tree, 'base-editor')!.props.style as Record<string, unknown>).pointerEvents).toBe('none');
    for (const id of ['toolbar', 'shop-panel']) expect((find(tree, id)!.props.style as Record<string, unknown>).pointerEvents).toBe('auto');
  });

  it('панель сворачивается: список зданий исчезает, состояние сохраняется при входе в редактирование', () => {
    const ctl = setup({ cash: 1 });
    let tree = view(ctl);
    expect(find(tree, 'shop-b_vault')).toBeDefined();
    expect(find(tree, 'shop-toggle')!.props['aria-expanded']).toBe(true);
    click(find(tree, 'shop-toggle'));
    tree = view(ctl);
    expect(find(tree, 'shop-b_vault')).toBeUndefined();
    expect(find(tree, 'shop-toggle')!.props['aria-expanded']).toBe(false);
    ctl.actions.toggleEdit();
    expect(ctl.viewModel().shopOpen).toBe(false);
    click(find(view(ctl), 'shop-toggle'));
    expect(find(view(ctl), 'shop-b_vault')).toBeDefined();
  });

  it('кнопка сворачивания вне [data-testid=shop]: e2e берут первую кнопку внутри него как здание', () => {
    const shop = find(view(setup({ cash: 1000 })), 'shop')!;
    const buttons: string[] = [];
    const visit = (n: unknown): void => {
      if (typeof n === 'string') return;
      const e = n as { tag: string; props: Record<string, unknown>; children: unknown[] };
      if (e.tag === 'button') buttons.push(String(e.props['data-testid'] ?? ''));
      e.children.forEach(visit);
    };
    visit(shop);
    expect(buttons).not.toContain('shop-toggle');
    expect(buttons.length).toBeGreaterThan(0);
  });

  it('ShopPanel без onToggle (старые потребители) рисует список как раньше', () => {
    const ctl = setup(undefined);
    const tree = expand(h(ShopPanel, { entries: ctl.viewModel().shop, t: tEn, onPick: () => undefined }));
    expect(find(tree, 'shop-b_vault')).toBeDefined();
    expect(find(tree, 'shop-toggle')).toBeUndefined();
  });
});
