// Интеграция редактора базы в шелл: createApp (реальные ApiClient + слайс) против мока сервера (fetch)
// + мок Renderer 0.2 (ввод select/hover/drag → редактор, render(WorldState)) + компоненты ui/base как дерево VNode.
import { h } from 'preact';
import { describe, expect, it } from 'vitest';
import { BaseEditorView } from '../ui/base/components';
import { baseState, click, expand, find, findAll, mockRenderer, startItems, textOf } from '../core/base/test-kit';
import type { BaseItem, BaseStateDto } from '../core/base';
import { createBrowserAdapter, memoryKV } from '../platform/browser';
import { createApp, type Me } from './app';
import { loadContent } from './content';
import { createAppT } from './i18n';

const content = loadContent();
const DEVICE = 'dev_0123456789abcdef0123';
const NOW = Date.parse('2026-01-01T00:00:00Z');
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const apiError = (status: number, code: string, details?: unknown) => json(status, { error: { code, message: `srv:${code}`, details } });
const project = (x: number, y: number) => ({ x: x * 10, y: y * 5 });

interface Call {
  method: string;
  path: string;
  body: unknown;
}

/** Мини-сервер базы: хранит раскладку и кошелёк; fail(call) может подменить ответ на ошибку. */
function world(fail: (c: Call) => Response | undefined = () => undefined) {
  const calls: Call[] = [];
  let items: BaseItem[] = startItems();
  let version = 1;
  let cash = 500;
  const state = (): BaseStateDto => baseState(items, { version, server_time: '2026-01-01T00:00:00.000Z' });
  const me = (): Me => ({
    profile: { id: 'p1', nickname: 'Guest', hq_level: 1, trophies: 0, league: 'bronze', shield_until: null, created_at: '2026-01-01T00:00:00Z' },
    wallets: { cash, fuel: 200, alloy: 0, gems: 0 },
    builders: [{ id: 'b1', busy_until: null }],
  });
  const fetchFn = ((url: string, init: RequestInit = {}) => {
    const c: Call = { method: init.method ?? 'GET', path: url, body: init.body ? JSON.parse(init.body as string) : undefined };
    calls.push(c);
    const f = fail(c);
    if (f) return Promise.resolve(f);
    switch (url) {
      case '/v1/auth/guest':
        return Promise.resolve(json(200, { player_id: 'p1', device_id: DEVICE, tokens: { access_token: 'a', refresh_token: 'r' } }));
      case '/v1/me':
        return Promise.resolve(json(200, me()));
      case '/v1/base':
        return Promise.resolve(json(200, state()));
      case '/v1/base/commands':
        for (const cmd of c.body as { op: string; args: Record<string, unknown> }[]) {
          if (cmd.op === 'build') {
            items = [...items, { uid: cmd.args.uid as string, type: cmd.args.type as string, level: 1, x: cmd.args.x as number, y: cmd.args.y as number, rot: 0 }];
            cash -= 20;
          }
        }
        version++;
        return Promise.resolve(json(200, state()));
      case '/v1/base/layout':
        items = (c.body as { layout: { items: BaseItem[] } }).layout.items;
        version++;
        return Promise.resolve(json(200, { ...state(), unchanged: false }));
    }
    return Promise.resolve(apiError(404, 'not_found'));
  }) as unknown as typeof fetch;
  return { calls, fetchFn, items: () => items, cash: () => cash };
}

async function setup(w = world()) {
  const platform = createBrowserAdapter({ storage: memoryKV(), newDeviceId: () => DEVICE });
  let n = 0;
  const app = createApp({ platform, baseUrl: '', content, fetch: w.fetchFn, sleep: () => Promise.resolve(), newKey: () => 'k', now: () => NOW, newUid: () => `n${++n}` });
  await app.start();
  const r = mockRenderer();
  app.base.bindRenderer(r);
  const t = (lang: 'ru' | 'en') => createAppT(lang, content.names[lang]);
  const view = (lang: 'ru' | 'en' = 'en') => expand(h(BaseEditorView, { vm: app.base.viewModel(), actions: app.base.actions, t: t(lang), project }));
  /** Дождаться фоновых запросов (/v1/me после подтверждения). */
  const settle = () => new Promise((res) => setTimeout(res, 5));
  return { app, r, w, view, settle };
}

describe('редактор базы в шелле', () => {
  it('постройка: магазин → тап по клетке → подтверждение; оптимистично в рендере, команда на сервер, HUD обновился', async () => {
    const { app, r, w, view, settle } = await setup();
    expect(app.store.getState().me?.wallets.cash).toBe(500);
    click(find(view(), 'shop-b_cash_mine')?.children.flatMap((c) => (typeof c === 'string' ? [] : [c])).find((e) => e.tag === 'button'));
    expect(app.base.viewModel().placing?.type).toBe('b_cash_mine');
    r.emit({ type: 'select', uid: null, x: 30, y: 30 }); // тап по пустой клетке двигает призрак
    expect(app.base.editor.store.getState().placing).toMatchObject({ x: 30, y: 30 });
    expect(app.base.viewModel().placing?.ok).toBe(true);

    const sent = app.base.editor.confirmPlacing();
    // до ответа сервера здание уже в мире рендера
    expect(r.renders.at(-1)?.items?.some((i) => i.type === 'b_cash_mine' && i.x === 30 && i.y === 30)).toBe(true);
    await sent;
    await settle();
    const post = w.calls.find((c) => c.path === '/v1/base/commands');
    expect(post?.body).toMatchObject([{ op: 'build', args: { type: 'b_cash_mine', x: 30, y: 30 } }]);
    expect(app.store.getState().me?.wallets.cash).toBe(480);
    expect(w.items().some((i) => i.type === 'b_cash_mine')).toBe(true);
  });

  it('улучшение: тап по зданию → карточка → «Улучшить» шлёт upgrade с целевым уровнем', async () => {
    const { app, r, w, view } = await setup();
    r.emit({ type: 'select', uid: 'cm1', x: 14, y: 20 });
    expect(r.renders.at(-1)?.selectedUid).toBe('cm1');
    const card = find(view(), 'card');
    expect(textOf(card!)).toContain('Cash Mine');
    click(find(view(), 'card-upgrade'));
    await new Promise((res) => setTimeout(res, 5));
    expect(w.calls.find((c) => c.path === '/v1/base/commands')?.body).toMatchObject([{ op: 'upgrade', args: { uid: 'cm1', to: 2 } }]);
    expect(app.base.viewModel().card?.uid).toBe('cm1');
  });

  it('перестановка: режим правки → drag-drop → PUT /v1/base/layout с новой позицией; undo до сохранения', async () => {
    const { app, r, w, view } = await setup();
    click(find(view(), 'toggle-edit'));
    expect(app.base.viewModel().toolbar.editing).toBe(true);
    r.emit({ type: 'select', uid: 'cm1', x: 14, y: 20 });
    r.emit({ type: 'dragStart', uid: 'cm1', x: 14, y: 20 });
    r.emit({ type: 'dragMove', uid: 'cm1', x: 14, y: 26 });
    r.emit({ type: 'dragEnd', uid: 'cm1', x: 14, y: 26 });
    expect(r.renders.at(-1)?.items?.find((i) => i.uid === 'cm1')).toMatchObject({ x: 14, y: 26 });
    expect(w.calls.some((c) => c.path === '/v1/base/layout')).toBe(false); // без «Сохранить» ничего не уходит
    click(find(view(), 'undo'));
    expect(r.renders.at(-1)?.items?.find((i) => i.uid === 'cm1')).toMatchObject({ x: 14, y: 20 });
    click(find(view(), 'redo'));

    click(find(view(), 'save'));
    await new Promise((res) => setTimeout(res, 5));
    const put = w.calls.find((c) => c.path === '/v1/base/layout');
    expect(put?.method).toBe('PUT');
    expect((put?.body as { layout: { items: BaseItem[] } }).layout.items.find((i) => i.uid === 'cm1')).toMatchObject({ x: 14, y: 26 });
    expect(app.base.editor.dirty()).toBe(false);
    expect(app.base.model.store.getState().confirmed.items.find((i) => i.uid === 'cm1')).toMatchObject({ y: 26 });
  });

  it('insufficient_resources: откат оптимистичной постройки и понятный текст (ru/en)', async () => {
    const { app, r, view, settle } = await setup(world((c) => (c.path === '/v1/base/commands' ? apiError(422, 'insufficient_resources', { reason: 'cash' }) : undefined)));
    app.base.actions.pickBuilding('b_cash_mine');
    r.emit({ type: 'select', uid: null, x: 30, y: 30 });
    await app.base.editor.confirmPlacing();
    await settle();
    expect(r.renders.at(-1)?.items?.some((i) => i.x === 30 && i.y === 30)).toBe(false); // откат
    expect(textOf(find(view('ru'), 'notice')!)).toBe('Не хватает ресурсов');
    expect(textOf(find(view('en'), 'notice')!)).toBe('Not enough resources');
  });

  it('conflict и not_found: сообщения на языке игрока, а не сырой текст сервера', async () => {
    const { app, view } = await setup(world((c) => (c.path === '/v1/base/commands' ? apiError(409, 'conflict', { reason: 'no_builder' }) : undefined)));
    await app.base.model.commands.upgrade('cm1');
    expect(textOf(find(view('ru'), 'notice')!)).toBe('Конфликт: действие недоступно');

    const nf = await setup(world((c) => (c.path === '/v1/base/commands' ? apiError(404, 'not_found') : undefined)));
    await nf.app.base.model.commands.collect();
    expect(textOf(find(nf.view('ru'), 'notice')!)).toBe('База не найдена или объект уже удалён');
    expect(textOf(find(nf.view('en'), 'notice')!)).toBe('Base or object not found');
  });

  it('отказ сохранения раскладки (invalid) показывается, черновик остаётся для правки', async () => {
    const { app, r, view } = await setup(world((c) => (c.path === '/v1/base/layout' ? apiError(400, 'invalid', { reason: 'sealed' }) : undefined)));
    app.base.actions.toggleEdit();
    r.emit({ type: 'select', uid: 'cm1', x: 14, y: 20 });
    r.emit({ type: 'dragStart', uid: 'cm1', x: 14, y: 20 });
    r.emit({ type: 'dragEnd', uid: 'cm1', x: 14, y: 26 });
    await app.base.editor.save();
    expect(textOf(find(view('ru'), 'notice')!)).toBe('Сервер отклонил раскладку');
    expect(app.base.editor.dirty()).toBe(true);
  });

  it('следующая команда убирает прошлый отказ', async () => {
    let reject = true;
    const { app, view } = await setup(world((c) => (c.path === '/v1/base/commands' && reject ? apiError(422, 'insufficient_resources') : undefined)));
    await app.base.model.commands.collect();
    expect(findAll(view(), 'notice')).toHaveLength(1);
    reject = false;
    await app.base.model.commands.collect();
    expect(findAll(view(), 'notice')).toHaveLength(0);
  });

  it('режим просмотра/редактирования: переключатель; сборка доступна, пока нечего собирать — кнопка выключена', async () => {
    const { app, view } = await setup();
    const tb = (): ReturnType<typeof find> => find(view(), 'toggle-edit');
    expect(tb()?.props['aria-pressed']).toBe(false);
    expect(find(view(), 'edit-tools')).toBeUndefined();
    click(tb());
    expect(tb()?.props['aria-pressed']).toBe(true);
    expect(find(view(), 'edit-tools')).toBeDefined();
    expect(find(view(), 'collect')?.props.disabled).toBe(true);
    click(tb());
    expect(app.base.viewModel().toolbar.editing).toBe(false);
  });
});
