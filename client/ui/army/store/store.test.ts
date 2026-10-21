// Слайс армии и лаборатории: оптимистичные команды с откатом, housing, нехватка ресурсов, гейты лаборатории, отмена, ускорение,
// ленивое обновление по finish_at. «Сервер» — в памяти (test-kit), числа — из реального контента.
import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../core/api';
import { buildings, catalog, setup, NOW_S } from '../test-kit';
import { noticeText } from './view-model';
import { createArmyT } from '../i18n';
import { strings } from '../test-kit';

const tRu = createArmyT('ru', strings.ru);
const unit = (r: Awaited<ReturnType<typeof setup>>, id: string) => r.ctl.viewModel().train.units.find((u) => u.id === id)!;

describe('контент', () => {
  it('юниты, производители и узлы читаются из yaml', () => {
    expect(catalog.unit('u_brawler')).toMatchObject({ housing: 4, trainS: 45, pool: 'unit', producer: 'b_barracks', cost: { fuel: 110 } });
    expect(catalog.unit('v_humvee')).toMatchObject({ pool: 'garage', producer: 'b_factory' });
    expect(catalog.unit('c_medkit')).toMatchObject({ kind: 'spell', pool: 'spell', producer: 'b_support_bay' });
    expect(catalog.unit('u_paratrooper')?.group).toBe(4);
    expect(catalog.levelOf('u_rifleman', ['r_rifle_1'])).toBe(2);
    expect(catalog.levelOf('u_rifleman', ['r_rifle_1', 'r_rifle_2'])).toBe(4);
    expect(catalog.levelOf('u_sniper', [])).toBe(1);
  });

  it('цена ускорения — ceil(h^exp × factor) из building_costs (159 с → 1 гем, 6 ч → 17)', () => {
    expect(catalog.speedupGems(159)).toBe(1);
    expect(catalog.speedupGems(6 * 3600)).toBe(17);
    expect(catalog.speedupGems(0)).toBe(0);
  });
});

describe('загрузка и состояние', () => {
  it('refresh подтягивает армию и лабораторию; пулы и очереди попадают в view-model', async () => {
    const r = await setup();
    const vm = r.ctl.viewModel();
    expect(vm.loaded).toBe(true);
    expect(vm.pools.map((p) => [p.id, p.used, p.capacity])).toEqual([['unit', 28, 40], ['garage', 0, 20], ['spell', 0, 10]]);
    expect(vm.stock.map((s) => [s.id, s.count])).toEqual([['u_rifleman', 12], ['u_sniper', 2]]);
    expect(vm.queues[0]).toMatchObject({ buildingUid: 'br1', remainingS: 135 });
    expect(vm.queues[0]!.orders[0]).toMatchObject({ state: 'training', unitLeftS: 45, remaining: 3 });
  });

  it('лаборатория ещё не подключена на сервере (404) — экран армии работает, лаборатория помечена загруженной', async () => {
    const r = await setup();
    r.server.api.fetchLab = () => Promise.reject(new ApiError('not_found', 404, 'not found'));
    await r.ctl.actions.refresh();
    expect(r.ctl.viewModel().notice).toBeNull();
    expect(r.ctl.viewModel().lab.loaded).toBe(true);
  });
});

describe('переполнение лагеря (housing)', () => {
  it('заказ, превышающий free, заблокирован целиком; подсказан максимум; запрос не уходит', async () => {
    const r = await setup();
    r.ctl.actions.setCount('u_brawler', 4); // need 16 > free 12
    expect(unit(r, 'u_brawler')).toMatchObject({ blocked: 'housing_full', maxCount: 3, freeHousing: 12 });
    await r.ctl.actions.train('u_brawler');
    expect(r.server.calls.filter((c) => c.name === 'train')).toHaveLength(0);
    r.ctl.actions.setMax('u_brawler');
    expect(unit(r, 'u_brawler')).toMatchObject({ count: 3, blocked: null });
  });

  it('заказ по максимуму проходит, место занято (used = capacity), следующий заказ заблокирован', async () => {
    const r = await setup();
    r.ctl.actions.selectBuilding('br2');
    r.ctl.actions.setCount('u_brawler', 3);
    await r.ctl.actions.train('u_brawler');
    const pool = r.ctl.viewModel().pools[0]!;
    expect(pool).toMatchObject({ used: 40, free: 0, full: true });
    expect(r.server.wallet.fuel).toBe(2000 - 330);
    expect(unit(r, 'u_rifleman')).toMatchObject({ blocked: 'housing_full', maxCount: 0 });
  });

  it('оптимистично: пока запрос в полёте, место уже занято и заказ в очереди; на отказе housing_full — откат и понятный текст', async () => {
    const r = await setup();
    r.ctl.actions.selectBuilding('br2');
    r.ctl.actions.setCount('u_brawler', 3);
    // клиент считает, что место есть, а сервер уже занят другим устройством → housing_full
    r.server.failNext('train', new ApiError('conflict', 409, 'housing', { reason: 'housing_full', pool: 'unit', capacity: 40, used: 38, free: 2, need: 12, max_count: 0 }));
    const release = r.server.hold();
    const p = r.ctl.actions.train('u_brawler');
    const vm = r.ctl.viewModel();
    expect(vm.pools[0]!.used).toBe(40);
    expect(vm.queues.find((q) => q.buildingUid === 'br2')?.orders[0]).toMatchObject({ remaining: 3 });
    expect(vm.busy).toBe(true);
    release();
    await p;
    const after = r.ctl.viewModel();
    expect(after.queues.find((q) => q.buildingUid === 'br2')).toBeUndefined();
    expect(after.pools[0]!.used).toBe(28); // откат + свежее состояние сервера
    expect(after.notice).toMatchObject({ code: 'conflict', reason: 'housing_full' });
    expect(noticeText(after.notice!, tRu)).toBe('Не хватает места в армии. Можно не больше 0');
    expect(r.settled).toBe(1);
  });

  it('место освобождается отменой заказа; освобождённое место снова доступно для заказа', async () => {
    const r = await setup();
    r.ctl.actions.setCount('u_brawler', 3);
    expect(unit(r, 'u_brawler').blocked).toBeNull();
    r.ctl.actions.selectBuilding('br2');
    await r.ctl.actions.train('u_brawler'); // used 40
    expect(r.ctl.viewModel().pools[0]!.free).toBe(0);
    await r.ctl.actions.cancelOrder('o1');
    expect(r.ctl.viewModel().pools[0]).toMatchObject({ used: 28, free: 12, full: false });
  });

  it('перк лаборатории показывается у пула и в списке перков', async () => {
    const army = { ...(await setup()).server.army, perks: { army_housing_pct: 5, raid_loot_pct: 0, scan_reveals_traps: false } };
    army.pools = { ...army.pools, unit: { capacity: 42, used: 40, free: 2, base: 40, perk_pct: 5 } };
    const r = await setup({ army });
    const vm = r.ctl.viewModel();
    expect(vm.pools[0]).toMatchObject({ capacity: 42, perkPct: 5 });
    expect(vm.perks.map((p) => p.key)).toEqual(['army_housing_pct']);
  });
});

describe('нехватка ресурсов', () => {
  it('предпроверка по кошельку: нехватка fuel блокирует кнопку и называет сколько нужно/есть', async () => {
    const r = await setup({ wallet: { fuel: 100, gems: 5 } });
    r.ctl.actions.setCount('u_brawler', 1);
    expect(unit(r, 'u_brawler')).toMatchObject({ blocked: 'no_funds', short: { fuel: { need: 110, have: 100 } }, maxCount: 0 });
    r.ctl.actions.setCount('u_rifleman', 4);
    expect(unit(r, 'u_rifleman')).toMatchObject({ blocked: null, maxCount: 4 });
  });

  it('сервер отказал insufficient_resources (кошелёк на клиенте устарел) — откат и текст с недостающим', async () => {
    const r = await setup({ noWallet: true, wallet: { fuel: 10 } });
    r.ctl.actions.setCount('u_rifleman', 2);
    await r.ctl.actions.train('u_rifleman');
    const vm = r.ctl.viewModel();
    expect(vm.notice).toMatchObject({ code: 'insufficient_resources' });
    expect(noticeText(vm.notice!, tRu)).toBe('Не хватает ресурсов. Не хватает: 50 Топливо');
    expect(vm.queues.find((q) => q.buildingUid === 'br1')!.orders).toHaveLength(1);
  });

  it('оптимистичная трата учитывается в предпроверке следующих заказов, пока запрос в полёте', async () => {
    const r = await setup({ wallet: { fuel: 110, gems: 5 } });
    const release = r.server.hold();
    r.ctl.actions.setCount('u_brawler', 1);
    const p = r.ctl.actions.train('u_brawler');
    expect(unit(r, 'u_brawler').blocked).toBe('no_funds'); // fuel 110 − 110 = 0
    release();
    await p;
  });
});

describe('здания и гейты обучения', () => {
  it('locked: снайпер требует казарму 3 — у L2 заблокирован с описанием', async () => {
    const r = await setup();
    expect(unit(r, 'u_sniper')).toMatchObject({ blocked: 'locked', needBuilding: { type: 'b_barracks', level: 3 } });
    expect(unit(r, 'u_brawler')).toMatchObject({ blocked: null });
  });

  it('без построенных производителей панель обучения пуста', async () => {
    const r = await setup({ buildings: [{ uid: 'hq1', type: 'b_hq', level: 1 }] });
    expect(r.ctl.viewModel().train).toMatchObject({ buildings: [], selectedUid: null, units: [] });
  });

  it('кратность group: десант только по 4', async () => {
    const r = await setup({ buildings: buildings([{ uid: 'br3', type: 'b_barracks', level: 8 }, { uid: 'hq2', type: 'b_hq', level: 8 }]) });
    r.ctl.actions.selectBuilding('br3');
    expect(unit(r, 'u_paratrooper')).toMatchObject({ count: 4, group: 4 });
    r.ctl.actions.setCount('u_paratrooper', 5);
    expect(unit(r, 'u_paratrooper').blocked).toBe('bad_count');
  });

  it('очередь здания заполнена (queue.max_orders_per_building) — заказ заблокирован', async () => {
    const army = (await setup()).server.army;
    const orders = Array.from({ length: catalog.queueMaxOrders }, (_, i) => ({ order_id: `q${i}`, unit_id: 'u_rifleman', count: 1, remaining: 1, unit_finish_at: NOW_S + 12 * (i + 1) }));
    army.queues = [{ building_id: 'br2', orders, finish_at: NOW_S + 12 * orders.length }];
    army.pools.unit = { capacity: 40, used: 28 + orders.length, free: 12 - orders.length };
    const r = await setup({ army });
    r.ctl.actions.selectBuilding('br2');
    expect(r.ctl.viewModel().train.queueFull).toBe(true);
    expect(unit(r, 'u_rifleman').blocked).toBe('queue_full');
  });
});

describe('отмена и ускорение очереди', () => {
  it('отмена заказа: оптимистично снимает заказ и освобождает место; на отказе — откат', async () => {
    const r = await setup();
    r.server.failNext('cancel', new ApiError('not_found', 404, 'order not found'));
    const release = r.server.hold();
    const p = r.ctl.actions.cancelOrder('o1');
    const mid = r.ctl.viewModel();
    expect(mid.queues).toHaveLength(0);
    expect(mid.pools[0]).toMatchObject({ used: 16, free: 24 });
    release();
    await p;
    const vm = r.ctl.viewModel();
    expect(vm.notice?.code).toBe('not_found');
    expect(vm.queues).toHaveLength(1);
    expect(vm.pools[0]!.used).toBe(28);
  });

  it('отмена подтверждается вторым шагом; успех идёт с Idempotency-Key', async () => {
    const r = await setup();
    r.ctl.actions.askCancel('order:o1');
    expect(r.ctl.viewModel().queues[0]!.orders[0]!.confirming).toBe(true);
    await r.ctl.actions.cancelOrder('o1');
    expect(r.ctl.viewModel().queues).toHaveLength(0);
    expect(r.ctl.store.getState().ui.confirm).toBeNull();
    expect(r.server.calls.find((c) => c.name === 'cancel')).toMatchObject({ body: { order_id: 'o1' }, key: 'k1' });
  });

  it('ускорение: цена из контента, юниты сразу в запас, очередь пуста; gems списаны сервером', async () => {
    const r = await setup();
    const q = r.ctl.viewModel().queues[0]!;
    expect(q.speedupGems).toBe(catalog.speedupGems(135));
    expect(q.speedupGems).toBe(1);
    await r.ctl.actions.speedupQueue('br1');
    const vm = r.ctl.viewModel();
    expect(vm.queues).toHaveLength(0);
    expect(vm.stock.find((s) => s.id === 'u_brawler')?.count).toBe(3);
    expect(vm.pools[0]!.used).toBe(28); // housing не меняется
    expect(r.server.wallet.gems).toBe(49);
  });

  it('не хватает gems — кнопка заблокирована и объясняет; отказ сервера откатывает ускорение', async () => {
    const r = await setup({ wallet: { fuel: 1000, gems: 0 } });
    expect(r.ctl.viewModel().queues[0]).toMatchObject({ speedupBlocked: 'no_gems', gemsShort: { need: 1, have: 0 } });
    const r2 = await setup({ wallet: { fuel: 1000, gems: 0 }, noWallet: true });
    await r2.ctl.actions.speedupQueue('br1');
    expect(r2.ctl.viewModel().notice?.code).toBe('insufficient_resources');
    expect(r2.ctl.viewModel().queues).toHaveLength(1);
    expect(r2.ctl.viewModel().stock.find((s) => s.id === 'u_brawler')).toBeUndefined();
  });
});

describe('ленивое обновление по finish_at', () => {
  it('юниты дозревают локально по unit_finish_at; следующий стартует в момент окончания предыдущего', async () => {
    const r = await setup();
    r.clock.advance(46);
    const vm = r.ctl.viewModel();
    expect(vm.stock.find((s) => s.id === 'u_brawler')?.count).toBe(1);
    expect(vm.queues[0]!.orders[0]).toMatchObject({ remaining: 2, unitLeftS: 44 });
    r.clock.advance(200);
    const done = r.ctl.viewModel();
    expect(done.stock.find((s) => s.id === 'u_brawler')?.count).toBe(3);
    expect(done.queues).toHaveLength(0);
  });

  it('заказ за заказом: второй заказ стартует, когда закончил первый', async () => {
    const army = (await setup()).server.army;
    army.queues[0]!.orders.push({ order_id: 'o2', unit_id: 'u_rifleman', count: 2, remaining: 2, unit_finish_at: NOW_S + 135 + 12 });
    army.queues[0]!.finish_at = NOW_S + 135 + 24;
    const r = await setup({ army });
    r.clock.advance(135 + 13);
    const vm = r.ctl.viewModel();
    expect(vm.stock.find((s) => s.id === 'u_brawler')?.count).toBe(3);
    expect(vm.stock.find((s) => s.id === 'u_rifleman')?.count).toBe(13);
    expect(vm.queues[0]!.orders.map((o) => [o.orderId, o.remaining])).toEqual([['o2', 1]]);
  });

  it('checkDue ходит за состоянием только когда что-то дозрело и не чаще минимального интервала', async () => {
    const r = await setup();
    const fetches = () => r.server.calls.filter((c) => c.name === 'fetchArmy').length;
    const before = fetches();
    expect(r.ctl.checkDue()).toBe(false);
    r.clock.advance(46);
    expect(r.ctl.checkDue()).toBe(true);
    await Promise.resolve();
    await new Promise((res) => setTimeout(res, 0));
    expect(fetches()).toBe(before + 1);
    expect(r.ctl.checkDue()).toBe(false); // интервал
  });
});

describe('лаборатория', () => {
  const node = (r: Awaited<ReturnType<typeof setup>>, id: string) => r.ctl.viewModel().lab.groups.flatMap((g) => g.nodes).find((n) => n.id === id)!;

  it('статусы: доступно / нужен уровень лаборатории / предыдущее исследование / скоро / перк', async () => {
    const r = await setup(); // лаборатория 2
    expect(node(r, 'r_rifle_1')).toMatchObject({ status: 'available', blocked: null, fromLevel: 1, toLevel: 2, fuel: 800, hours: 2 });
    expect(node(r, 'r_brawler_1')).toMatchObject({ status: 'available' }); // lab 2
    expect(node(r, 'r_sniper_1')).toMatchObject({ status: 'lab_level', needLab: 3, haveLab: 2 });
    expect(node(r, 'r_rifle_2')).toMatchObject({ status: 'prerequisite', prereqNameKey: 'r.rifle_1' });
    expect(node(r, 'r_smoke_1').status).toBe('soon');
    expect(node(r, 'r_camp_cap')).toMatchObject({ status: 'lab_level', perkKey: 'army_housing_pct' });
  });

  it('исследование с гейтом lab: сервер отказывает lab_level — откат, текст с требуемым уровнем', async () => {
    const r = await setup();
    await r.ctl.actions.research('r_sniper_1');
    const vm = r.ctl.viewModel();
    expect(vm.lab.active).toBeNull();
    expect(vm.notice).toMatchObject({ code: 'conflict', reason: 'lab_level' });
    expect(noticeText(vm.notice!, tRu)).toBe('Нужен более высокий уровень лаборатории (3)');
  });

  it('старт исследования: оптимистично активно, потом подтверждено; второе блокируется lab_busy', async () => {
    const r = await setup();
    const release = r.server.hold();
    const p = r.ctl.actions.research('r_rifle_1');
    expect(r.ctl.viewModel().lab.active).toMatchObject({ nodeId: 'r_rifle_1', remainingS: 2 * 3600, percent: 0 });
    expect(node(r, 'r_brawler_1')).toMatchObject({ status: 'available', blocked: 'lab_busy' });
    release();
    await p;
    expect(r.server.wallet.fuel).toBe(1200);
    expect(node(r, 'r_rifle_1').status).toBe('researching');
    r.clock.advance(3600);
    expect(r.ctl.viewModel().lab.active).toMatchObject({ percent: 50, remainingS: 3600 });
  });

  it('нехватка fuel для исследования блокирует кнопку с объяснением', async () => {
    const r = await setup({ wallet: { fuel: 500, gems: 5 } });
    expect(node(r, 'r_rifle_1')).toMatchObject({ blocked: 'no_funds', short: { fuel: { need: 800, have: 500 } } });
  });

  it('нет лаборатории — исследования недоступны', async () => {
    const r = await setup({ lab: { active: null, done: [] }, buildings: [{ uid: 'hq1', type: 'b_hq', level: 3 }] });
    expect(r.ctl.viewModel().lab.hasLab).toBe(false);
    expect(node(r, 'r_rifle_1')).toMatchObject({ status: 'lab_level', needLab: 1, haveLab: 0 });
  });

  it('отмена исследования: оптимистично свободна, на отказе возвращается', async () => {
    const r = await setup();
    await r.ctl.actions.research('r_rifle_1');
    r.server.failNext('cancelResearch', new ApiError('internal', 500, 'boom'));
    r.ctl.actions.askCancel('research');
    expect(r.ctl.viewModel().lab.active?.confirming).toBe(true);
    await r.ctl.actions.cancelResearch();
    expect(r.ctl.viewModel().lab.active).toMatchObject({ nodeId: 'r_rifle_1', confirming: false });
    expect(r.ctl.viewModel().notice?.code).toBe('internal');
    await r.ctl.actions.cancelResearch();
    expect(r.ctl.viewModel().lab.active).toBeNull();
    expect(node(r, 'r_rifle_1').status).toBe('available');
  });

  it('ускорение исследования: узел готов, уровень юнита растёт во всём запасе; цена из контента (2 ч → 7 гемов)', async () => {
    const r = await setup();
    await r.ctl.actions.research('r_rifle_1');
    expect(r.ctl.viewModel().lab.active?.speedupGems).toBe(catalog.speedupGems(2 * 3600));
    await r.ctl.actions.speedupResearch();
    const vm = r.ctl.viewModel();
    expect(vm.lab.active).toBeNull();
    expect(node(r, 'r_rifle_1').status).toBe('done');
    expect(vm.stock.find((s) => s.id === 'u_rifleman')?.level).toBe(2);
    expect(r.server.wallet.gems).toBe(50 - catalog.speedupGems(2 * 3600));
    expect(node(r, 'r_rifle_2')).toMatchObject({ status: 'lab_level', fromLevel: 2 }); // lab 3 нужна, у нас 2
  });

  it('исследование завершилось по таймеру — checkDue подтягивает лабораторию', async () => {
    const r = await setup();
    await r.ctl.actions.research('r_rifle_1');
    r.server.lab.done = ['r_rifle_1'];
    r.server.lab.active = null; // «сервер» дозрел сам
    r.clock.advance(2 * 3600 + 1);
    expect(r.ctl.checkDue()).toBe(true);
    await new Promise((res) => setTimeout(res, 0));
    expect(node(r, 'r_rifle_1').status).toBe('done');
  });

  it('команды идут по одной и в порядке постановки', async () => {
    const r = await setup();
    const a = r.ctl.actions.research('r_rifle_1');
    const b = r.ctl.actions.cancelResearch();
    await Promise.all([a, b]);
    expect(r.server.calls.filter((c) => c.name === 'research' || c.name === 'cancelResearch').map((c) => c.name)).toEqual(['research', 'cancelResearch']);
    expect(r.ctl.viewModel().lab.active).toBeNull();
  });
});
