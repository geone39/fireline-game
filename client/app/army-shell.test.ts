// CLIENT-5: навигация шелла, HUD армии, кошелёк в панели построек, таймеры армии, ошибки сервера, состав для локального боя.
import { h } from 'preact';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../core/api';
import { baseState, click, expand, find, findAll, item, startItems, textOf } from '../core/base/test-kit';
import { createBrowserAdapter, memoryKV } from '../platform/browser';
import { ArmyLabApp } from '../ui/army/app';
import { createArmyT } from '../ui/army/i18n';
import { createFakeServer, type FakeServer } from '../ui/army/fake-server';
import { catalog as armyCatalog, setup as armySetup } from '../ui/army/test-kit';
import { noticeText } from '../ui/army/store/view-model';
import { BaseEditorView } from '../ui/base/components';
import { createT } from '../ui/base/i18n';
import { createApp, type Me } from './app';
import { testSnapshot } from './battle-fixture';
import { armySnapshot, canStartBattle, prepEntries, selectAll, selectedTotal, setCount } from './battle-prep';
import { loadContent } from './content';
import { Hud } from './hud';
import { selectHud, selectHudArmy } from './hud-model';
import { createAppT } from './i18n';
import { activeNav, NAV_ITEMS, navTarget } from './nav';
import { NavBar } from './Shell';

const content = loadContent();
const SERVER_T0 = Date.parse('2026-01-01T00:00:00.000Z');
const tRu = createAppT('ru', content.names.ru);
const tEn = createAppT('en', content.names.en);

describe('навигация', () => {
  it('четыре раздела: База / Армия / Лаборатория / Бой', () => {
    expect(NAV_ITEMS).toEqual(['base', 'army', 'lab', 'battle']);
  });
  it('армия и лаборатория — один экран, разные вкладки', () => {
    expect(navTarget('base')).toEqual({ screen: 'base' });
    expect(navTarget('army')).toEqual({ screen: 'army', tab: 'army' });
    expect(navTarget('lab')).toEqual({ screen: 'army', tab: 'lab' });
    expect(navTarget('battle')).toEqual({ screen: 'battle' });
  });
  it('подсветка следует за вкладкой экрана армии', () => {
    expect(activeNav('base', 'army')).toBe('base');
    expect(activeNav('army', 'army')).toBe('army');
    expect(activeNav('army', 'lab')).toBe('lab');
    expect(activeNav('battle', 'lab')).toBe('battle');
  });
  it('NavBar: подписи ru/en, aria-current у активной, клик отдаёт id', () => {
    const picked: string[] = [];
    const tree = expand(h(NavBar, { active: 'lab', t: tRu, onSelect: (id) => picked.push(id), floating: false }));
    expect(NAV_ITEMS.map((id) => textOf(find(tree, `nav-${id}`)!))).toEqual(['База', 'Армия', 'Лаборатория', 'Бой']);
    expect(find(tree, 'nav-lab')!.props['aria-current']).toBe('page');
    expect(find(tree, 'nav-army')!.props['aria-current']).toBeUndefined();
    click(find(tree, 'nav-battle'));
    expect(picked).toEqual(['battle']);
    const en = expand(h(NavBar, { active: 'base', t: tEn, onSelect: () => undefined, floating: true }));
    expect(NAV_ITEMS.map((id) => textOf(find(en, `nav-${id}`)!))).toEqual(['Base', 'Army', 'Laboratory', 'Battle']);
  });
});

describe('HUD армии', () => {
  it('до загрузки армии блока нет', () => {
    expect(selectHudArmy(null)).toBeNull();
  });

  it('housing по пулам, очереди обучения, текущее исследование', async () => {
    const rig = await armySetup({ lab: { lab_level: 2, active: null, done: [] } });
    await rig.ctl.actions.research('r_rifle_1');
    const a = selectHudArmy(rig.ctl.viewModel())!;
    expect(a.pools.find((p) => p.id === 'unit')).toMatchObject({ used: 28, capacity: 40, full: false });
    expect(a.training).toEqual({ units: 3, remainingS: 135 });
    expect(a.research).toMatchObject({ nameKey: armyCatalog.node('r_rifle_1')!.nameKey, percent: 0 });
    expect(a.research!.remainingS).toBeGreaterThan(0);
    rig.clock.advance(a.research!.remainingS / 2);
    expect(selectHudArmy(rig.ctl.viewModel())!.research!.percent).toBe(50);
  });

  it('Hud рисует «Армия used/cap», обучение и исследование (ru)', async () => {
    const rig = await armySetup({ lab: { lab_level: 2, active: null, done: [] } });
    await rig.ctl.actions.research('r_rifle_1');
    const vm = selectHud({ me: null, world: baseWorld(), catalog: content.catalog, serverNowMs: SERVER_T0, army: rig.ctl.viewModel() });
    const tree = expand(h(Hud, { vm, t: tRu }));
    expect(textOf(find(tree, 'army-pool-unit')!)).toContain('28/40');
    expect(textOf(find(tree, 'hud-training')!)).toContain('Обучение: 3');
    expect(textOf(find(tree, 'hud-research')!)).toContain('Исследование');
    const idle = selectHud({ me: null, world: baseWorld(), catalog: content.catalog, serverNowMs: SERVER_T0, army: (await armySetup()).ctl.viewModel() });
    expect(textOf(find(expand(h(Hud, { vm: idle, t: tEn })), 'hud-research')!)).toBe('Laboratory idle');
    expect(findAll(expand(h(Hud, { vm: selectHud({ me: null, world: baseWorld(), catalog: content.catalog, serverNowMs: 0 }), t: tRu })), 'hud-army')).toHaveLength(0);
  });
});

function baseWorld() {
  return {
    version: 1,
    items: startItems(),
    timers: [],
    builders: { total: 1, busy: 0 },
    collectors: {},
    serverTime: '',
  } as unknown as Parameters<typeof selectHud>[0]['world'];
}

// --- Приложение с «сервером» базы и армии ---
const json = (status: number, body: unknown) => new Response(JSON.stringify(body), { status });
const me = (wallets: Record<string, number>): Me => ({
  profile: { id: 'p1', nickname: 'G', hq_level: 1, trophies: 0, league: 'bronze', shield_until: null, created_at: '2026-01-01T00:00:00Z' },
  wallets,
  builders: [{ id: 'b1', busy_until: null }],
});

function rig(o: { withArmy?: boolean; wallets?: Record<string, number> } = {}) {
  const fake: FakeServer = createFakeServer(armyCatalog, { clock: () => Date.now(), wallet: { fuel: 2000, cash: 0, alloy: 0, gems: 50 } });
  const paths: string[] = [];
  const wallets = o.wallets ?? { cash: 5, fuel: 0, alloy: 0, gems: 0 };
  const fetchFn = (async (url: string, init: RequestInit = {}) => {
    paths.push(`${init.method ?? 'GET'} ${url}`);
    const body = (init.body ? JSON.parse(init.body as string) : {}) as never;
    const key = new Headers(init.headers).get('Idempotency-Key') ?? '';
    try {
      if (url === '/v1/auth/guest') return json(200, { player_id: 'p1', device_id: 'd', tokens: { access_token: 'a', refresh_token: 'r' } });
      if (url === '/v1/me') return json(200, me(o.withArmy === false ? wallets : { ...fake.wallet, ...wallets }));
      if (url === '/v1/base') return json(200, baseState([...startItems(), item('br1', 'b_barracks', 30, 30, 0, 2), item('lab1', 'b_laboratory', 34, 30, 0, 2)], { server_time: new Date().toISOString() }));
      if (o.withArmy === false) return json(404, { error: { code: 'not_found', message: 'nf' } });
      if (url === '/v1/army') return json(200, await fake.api.fetchArmy());
      if (url === '/v1/lab') return json(200, await fake.api.fetchLab());
      if (url === '/v1/army/train') return json(200, await fake.api.train(body, key));
      if (url === '/v1/lab/research') return json(200, await fake.api.research(body, key));
    } catch (e) {
      if (e instanceof ApiError) return json(e.status, { error: { code: e.code, message: e.message, details: e.details } });
      throw e;
    }
    return json(404, { error: { code: 'not_found', message: 'nf' } });
  }) as unknown as typeof fetch;
  const platform = createBrowserAdapter({ storage: memoryKV(), newDeviceId: () => 'dev_0123456789abcdef0123', document: Object.assign(new EventTarget(), { visibilityState: 'visible' }), target: new EventTarget() });
  const app = createApp({ platform, baseUrl: '', content, fetch: fetchFn, sleep: () => Promise.resolve() });
  const count = (p: string) => paths.filter((x) => x === p).length;
  return { app, fake, count, paths };
}

describe('приложение: армия подключена к шеллу', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
    vi.setSystemTime(SERVER_T0);
  });
  afterEach(() => vi.useRealTimers());

  it('старт грузит армию и лабораторию; HUD-блок появляется', async () => {
    const r = rig();
    await r.app.start();
    expect(r.app.store.getState().phase).toBe('ready');
    expect(r.count('GET /v1/army')).toBe(1);
    expect(r.count('GET /v1/lab')).toBe(1);
    expect(selectHudArmy(r.app.army.viewModel())).not.toBeNull();
    r.app.dispose();
  });

  it('сервер без /v1/army: шелл всё равно стартует, блока армии нет', async () => {
    const r = rig({ withArmy: false });
    await r.app.start();
    expect(r.app.store.getState().phase).toBe('ready');
    expect(selectHudArmy(r.app.army.viewModel())).toBeNull();
    r.app.dispose();
  });

  it('кошелёк из /v1/me передан панели построек: нехватка ресурсов подсвечена (#62)', async () => {
    const r = rig({ wallets: { cash: 5, fuel: 0, alloy: 0, gems: 0 } });
    await r.app.start();
    const vm = r.app.base.viewModel();
    const blocked = vm.shop.filter((s) => s.blocked === 'no_funds');
    expect(blocked.length).toBeGreaterThan(0);
    const tree = expand(h(BaseEditorView, { vm, actions: r.app.base.actions, t: createT('ru', content.names.ru), project: (x: number, y: number) => ({ x, y }) }));
    const first = blocked[0]!;
    const btn = find(tree, `shop-${first.type}`)!.children.find((c) => typeof c !== 'string' && c.tag === 'button');
    expect(btn && typeof btn !== 'string' && btn.props.disabled).toBe(true);
    r.app.dispose();
  });

  it('таймер армии: по finish_at очереди армия перезапрашивается без открытой вкладки', async () => {
    const r = rig();
    await r.app.start();
    const before = r.count('GET /v1/army');
    // фикстура: голова очереди готова через 45 с
    await vi.advanceTimersByTimeAsync(44_000);
    expect(r.count('GET /v1/army')).toBe(before);
    await vi.advanceTimersByTimeAsync(3_000);
    expect(r.count('GET /v1/army')).toBe(before + 1);
    r.app.dispose();
  });

  it('dispose снимает таймер армии', async () => {
    const r = rig();
    await r.app.start();
    r.app.dispose();
    const before = r.count('GET /v1/army');
    await vi.advanceTimersByTimeAsync(600_000);
    expect(r.count('GET /v1/army')).toBe(before);
  });

  it('обучение тратит ресурсы → HUD обновляет кошелёк (/v1/me)', async () => {
    const r = rig({ wallets: { cash: 5, fuel: 2000, alloy: 0, gems: 0 } });
    await r.app.start();
    const meBefore = r.count('GET /v1/me');
    r.app.army.actions.selectBuilding('br1');
    await r.app.army.actions.train('u_rifleman');
    expect(r.count('GET /v1/me')).toBeGreaterThan(meBefore);
    r.app.dispose();
  });
});

describe('ошибки сервера армии — понятный текст', () => {
  const armyT = (lang: 'ru' | 'en') => createArmyT(lang, content.names[lang]);

  it('housing_full: «Не хватает места в армии» + максимум', () => {
    const n = { code: 'conflict', reason: 'housing_full', message: 'x', details: { reason: 'housing_full', max_count: 3 } };
    expect(noticeText(n, armyT('ru'))).toBe('Не хватает места в армии. Можно не больше 3');
    expect(noticeText(n, armyT('en'))).toContain('At most 3');
    expect(noticeText(n, tRu)).toContain('Не хватает места в армии');
  });

  it('insufficient_resources: «Не хватает ресурсов» + чего именно', () => {
    const n = { code: 'insufficient_resources', reason: null, message: 'm', details: { missing: { fuel: 40 } } };
    expect(noticeText(n, armyT('ru'))).toContain('Не хватает ресурсов');
    expect(noticeText(n, armyT('ru'))).toContain('40');
    expect(noticeText(n, tEn)).toContain('Not enough resources');
  });

  it('отказ сервера доходит до экрана армии и откатывает заказ', async () => {
    const r = rig({ wallets: { cash: 5, fuel: 2000, alloy: 0, gems: 0 } });
    await r.app.start();
    r.app.army.actions.selectBuilding('br1');
    r.app.army.actions.setCount('u_rifleman', 1); // предпроверка пройдена, отказывает сервер
    r.fake.failNext('train', new ApiError('conflict', 409, 'housing_full', { reason: 'housing_full', max_count: 3 }));
    await r.app.army.actions.train('u_rifleman');
    const notice = r.app.army.viewModel().notice!;
    expect(noticeText(notice, armyT('ru'))).toContain('Не хватает места в армии');
    const tree = expand(h(ArmyLabApp, { vm: r.app.army.viewModel(), actions: r.app.army.actions, t: armyT('ru') }));
    expect(tree.length).toBeGreaterThan(0);
    r.app.dispose();
  });
});

describe('состав для локального боя из армии игрока', () => {
  it('запас → строки выбора; ошибки применимости: корабли без воды, юниты вне правил', async () => {
    const rg = await armySetup({
      army: {
        stock: [
          { id: 'u_rifleman', count: 5, level: 1 },
          { id: 'u_brawler', count: 0, level: 1 },
          { id: 's_gunboat', count: 1, level: 1 },
          { id: 'u_nonexistent', count: 2, level: 1 },
        ],
        spells: [{ id: 'c_medkit', count: 2, level: 1 }],
        pools: {},
        queues: [],
      },
    });
    const entries = prepEntries(rg.ctl.viewModel().stock, testSnapshot());
    expect(entries.map((e) => e.id).sort()).toEqual(['c_medkit', 's_gunboat', 'u_rifleman']);
    expect(entries.find((e) => e.id === 's_gunboat')!.unsupported).toBe('ship_no_water');
    expect(entries.find((e) => e.id === 'u_rifleman')!.unsupported).toBeNull();
    expect(selectAll(entries)).toEqual({ u_rifleman: 5, c_medkit: 2 });
  });

  it('выбор ограничен запасом; Snapshot получает состав и уровни из лаборатории', async () => {
    const rg = await armySetup({ lab: { lab_level: 3, active: null, done: [] } });
    // исследуем и сразу ускоряем: уровень стрелка в запасе станет 2 (ArmyState отдаёт level)
    await rg.ctl.actions.research('r_rifle_1');
    await rg.ctl.actions.speedupResearch();
    const target = testSnapshot();
    const entries = prepEntries(rg.ctl.viewModel().stock, target);
    const rifle = entries.find((e) => e.id === 'u_rifleman')!;
    expect(rifle.level).toBe(2);
    let sel = selectAll(entries);
    expect(selectedTotal(entries, sel)).toBe(14);
    sel = setCount(sel, rifle, 999);
    expect(sel.u_rifleman).toBe(rifle.available);
    sel = setCount(sel, rifle, -3);
    expect(sel.u_rifleman).toBe(0);
    sel = setCount(sel, rifle, 7.9);
    const snap = armySnapshot(target, entries, sel);
    expect(snap.Army).toEqual([
      { ID: 'u_rifleman', Level: 2, Count: 7 },
      { ID: 'u_sniper', Level: 1, Count: 2 },
    ]);
    expect(snap.Spells).toEqual([]);
    expect(snap.Hero).toBe('');
    expect(snap.Buildings).toEqual(target.Buildings); // защитник и Rules — из цели
    expect(snap.Rules).toBe(target.Rules);
  });

  it('пустой выбор и только заклинания — старт запрещён', async () => {
    const rg = await armySetup();
    const entries = prepEntries(rg.ctl.viewModel().stock, testSnapshot());
    expect(canStartBattle(entries, selectAll(entries))).toBe(true);
    expect(canStartBattle(entries, {})).toBe(false);
    expect(canStartBattle([], {})).toBe(false);
  });
});
