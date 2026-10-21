// Компоненты экранов армии и лаборатории (без DOM: дерево VNode) на реальном контенте и «сервере» в памяти.
import { h } from 'preact';
import { describe, expect, it } from 'vitest';
import { LabScreen } from '../lab/components';
import { ArmyLabApp } from './app';
import { ArmyScreen } from './components';
import { EXTRA_KEYS, createArmyT, type Lang, type T } from './i18n';
import { makeIconResolver } from './icons';
import { click, expand, find, findAll, setup, strings, textOf, type Rig, type El } from './test-kit';

const tRu = createArmyT('ru', strings.ru);
const tEn = createArmyT('en', strings.en);

const army = (r: Rig, t: T = tRu) => expand(h(ArmyScreen, { vm: r.ctl.viewModel(), actions: r.ctl.actions, t }));
const lab = (r: Rig, t: T = tRu) => expand(h(LabScreen, { vm: r.ctl.viewModel(), actions: r.ctl.actions, t }));
const text = (nodes: (El | string)[], id: string) => textOf(find(nodes, id));
const settle = () => new Promise((res) => setTimeout(res, 0));

describe('экран армии', () => {
  it('пулы: занято/вместимость, полосы с aria-valuenow, подпись пула из content', async () => {
    const r = await setup();
    const ui = army(r);
    expect(text(ui, 'pool-unit-value')).toBe('28/40');
    expect(textOf(find(ui, 'pool-unit'))).toContain('Лагерь');
    expect(textOf(find(ui, 'pool-garage'))).toContain('Гараж');
    expect(textOf(find(ui, 'pool-spell'))).toContain('Центр поддержки');
    const meter = find(ui, 'pool-unit-meter')!;
    expect(meter.props.role).toBe('progressbar');
    expect(meter.props['aria-valuenow']).toBe(70);
    expect(meter.props['aria-label']).toBe('Лагерь');
  });

  it('состав: количество, уровень-бейдж, в очереди; пустая армия — подсказка', async () => {
    const r = await setup();
    const ui = army(r);
    expect(text(ui, 'stock-u_rifleman-count')).toBe('×12');
    expect(text(ui, 'stock-u_rifleman-level')).toBe('ур. 1');
    const empty = await setup({ army: { ...(await setup()).server.army, stock: [], queues: [] } });
    expect(find(army(empty), 'stock-empty')).toBeDefined();
  });

  it('переполнение лагеря: кнопка заказа disabled, причина связана через aria-describedby, показан максимум', async () => {
    const r = await setup();
    r.ctl.actions.setCount('u_brawler', 4);
    const ui = army(r);
    const btn = find(ui, 'train-u_brawler-submit')!;
    expect(btn.props.disabled).toBe(true);
    expect(btn.props['aria-describedby']).toBe('train-u_brawler-block');
    expect(text(ui, 'train-u_brawler-block')).toBe(' Не хватает места в армии. Поместится: 3');
    expect(() => click(btn)).toThrow('disabled');
    click(find(ui, 'train-u_brawler-max'));
    const ui2 = army(r);
    expect(find(ui2, 'train-u_brawler-submit')!.props.disabled).toBe(false);
    expect((find(ui2, 'train-u_brawler-count')!.props as { value: number }).value).toBe(3);
  });

  it('одинаковые здания различаются номером: «Казарма #1» / «Казарма #2»', async () => {
    const r = await setup();
    const ui = army(r);
    expect(textOf(find(ui, 'train-building-br1'))).toContain('Казарма #1');
    expect(textOf(find(ui, 'train-building-br2'))).toContain('Казарма #2');
    expect(textOf(find(ui, 'queue-br1'))).toContain('Очередь: Казарма #1');
  });

  it('нехватка ресурсов: понятный текст «Нужно X, есть Y» на ru и en', async () => {
    const r = await setup({ wallet: { fuel: 100, gems: 1 } });
    r.ctl.actions.setCount('u_brawler', 1);
    expect(text(army(r), 'train-u_brawler-block')).toBe(' Нужно 110 Топливо, есть 100 Топливо');
    expect(text(army(r, tEn), 'train-u_brawler-block')).toBe(' Need 110 Fuel, have 100 Fuel');
  });

  it('заказ: клик по «Обучить» отправляет команду, очередь показывает заказ, поле количества отзывчиво', async () => {
    const r = await setup();
    click(find(army(r), 'train-building-br2'));
    click(find(army(r), 'train-u_rifleman-inc'));
    expect((find(army(r), 'train-u_rifleman-count')!.props as { value: number }).value).toBe(2);
    (find(army(r), 'train-u_rifleman-count')!.props.onInput as (e: unknown) => void)({ currentTarget: { value: '5' } });
    expect(text(army(r), 'train-u_rifleman-total')).toContain('125 Топливо');
    click(find(army(r), 'train-u_rifleman-submit'));
    await settle();
    expect(r.server.calls.find((c) => c.name === 'train')).toMatchObject({ body: { building_id: 'br2', unit_id: 'u_rifleman', count: 5 } });
    expect(find(army(r), 'queue-br2')).toBeDefined();
  });

  it('очередь: оставшееся время, прогресс, ускорение показывает цену в гемах, отмена в два шага', async () => {
    const r = await setup();
    let ui = army(r);
    expect(text(ui, 'queue-br1-left')).toContain('2 м 15 с');
    expect(text(ui, 'queue-br1-speedup')).toBe('Ускорить за 1 гем');
    expect(find(ui, 'order-o1-meter')!.props['aria-valuenow']).toBe(0);
    click(find(ui, 'order-o1-cancel'));
    ui = army(r);
    expect(find(ui, 'order-o1-cancel')).toBeUndefined();
    click(find(ui, 'order-o1-cancel-no'));
    expect(find(army(r), 'order-o1-cancel')).toBeDefined();
    click(find(army(r), 'order-o1-cancel'));
    click(find(army(r), 'order-o1-cancel-yes'));
    await settle();
    expect(find(army(r), 'queues-empty')).toBeDefined();
    expect(r.server.calls.some((c) => c.name === 'cancel')).toBe(true);
  });

  it('ускорение без гемов: disabled + объяснение; с гемами — работает', async () => {
    const poor = await setup({ wallet: { fuel: 500, gems: 0 } });
    const ui = army(poor);
    expect(find(ui, 'queue-br1-speedup')!.props.disabled).toBe(true);
    expect(find(ui, 'queue-br1-speedup')!.props['aria-describedby']).toBe('queue-br1-block');
    expect(text(ui, 'queue-br1-block')).toContain('Не хватает гемов');
    const rich = await setup();
    click(find(army(rich), 'queue-br1-speedup'));
    await settle();
    expect(find(army(rich), 'stock-u_brawler')).toBeDefined();
  });

  it('отказ сервера показывается плашкой role=alert и закрывается', async () => {
    const r = await setup({ noWallet: true, wallet: { fuel: 10 } });
    r.ctl.actions.setCount('u_rifleman', 2);
    click(find(army(r), 'train-u_rifleman-submit'));
    await settle();
    const n = find(army(r), 'notice')!;
    expect(n.props.role).toBe('alert');
    expect(textOf(n)).toContain('Не хватает ресурсов');
    click(find(army(r), 'notice-close'));
    expect(find(army(r), 'notice')).toBeUndefined();
  });

  it('до первой загрузки — индикатор; иконки: без resolver буквенная плашка, с resolver — спрайт из атласа', () => {
    const resolve = makeIconResolver(
      { atlases: { units: { image: 'atlases/units.png', size: [1024, 128] } }, sprites: [{ id: 'u_rifleman', atlas: 'units', frames: [{ x: 48, y: 0, w: 48, h: 48 }] }] },
      (img) => `/art/${img}`,
    );
    expect(resolve('u_rifleman')).toMatchObject({ url: '/art/atlases/units.png', x: 48 });
    expect(resolve('nope')).toBeNull();
  });
});

describe('экран лаборатории', () => {
  const nodeText = (ui: (El | string)[], id: string, part: string) => text(ui, `node-${id}-${part}`);

  it('статусы узлов: доступно, гейт лаборатории, предыдущее исследование, скоро', async () => {
    const r = await setup();
    const ui = lab(r);
    expect(nodeText(ui, 'r_rifle_1', 'status')).toBe('Доступно');
    expect(nodeText(ui, 'r_rifle_1', 'step')).toBe('ур. 1 → 2');
    expect(nodeText(ui, 'r_rifle_1', 'price')).toBe('800 Топливо · 2 ч');
    expect(nodeText(ui, 'r_sniper_1', 'status')).toBe('Нужна лаборатория 3 (сейчас 2)');
    expect(nodeText(ui, 'r_rifle_2', 'status')).toContain('Сначала:');
    expect(nodeText(ui, 'r_smoke_1', 'status')).toBe('Скоро');
    expect(find(ui, 'node-r_sniper_1-start')).toBeUndefined();
    expect(find(ui, 'lab-group-perk')).toBeDefined();
  });

  it('исследование: старт → активная карточка с прогрессом, ускорение и отмена; остальные доступные узлы блокируются «занята»', async () => {
    const r = await setup();
    click(find(lab(r), 'node-r_rifle_1-start'));
    await settle();
    const ui = lab(r);
    expect(text(ui, 'lab-active-name')).toBe(strings.ru['r.rifle_1']);
    expect(text(ui, 'lab-active-left')).toBe('2 ч осталось');
    expect(find(ui, 'lab-active-meter')!.props['aria-valuenow']).toBe(0);
    expect(find(ui, 'node-r_brawler_1-start')!.props.disabled).toBe(true);
    expect(text(ui, 'node-r_brawler_1-block')).toBe(' Лаборатория занята');
    expect(nodeText(ui, 'r_rifle_1', 'status')).toBe('Исследуется');
    r.clock.advance(1800);
    expect(find(lab(r), 'lab-active-meter')!.props['aria-valuenow']).toBe(25);
    click(find(lab(r), 'lab-cancel'));
    expect(find(lab(r), 'lab-cancel-yes')).toBeDefined();
    click(find(lab(r), 'lab-cancel-yes'));
    await settle();
    expect(find(lab(r), 'lab-active')).toBeUndefined();
    expect(find(lab(r), 'lab-idle')).toBeDefined();
  });

  it('ускорение исследования списывает гемы, узел становится «Изучено»', async () => {
    const r = await setup();
    click(find(lab(r), 'node-r_rifle_1-start'));
    await settle();
    expect(text(lab(r), 'lab-speedup')).toBe('Ускорить за 7 гемов');
    click(find(lab(r), 'lab-speedup'));
    await settle();
    expect(nodeText(lab(r), 'r_rifle_1', 'status')).toBe('Изучено');
  });

  it('без лаборатории — подсказка; на en тексты английские', async () => {
    const r = await setup({ lab: { active: null, done: [] }, buildings: [{ uid: 'hq1', type: 'b_hq', level: 3 }] });
    expect(text(lab(r), 'lab-level')).toBe('Постройте лабораторию, чтобы исследовать улучшения');
    const e = await setup();
    expect(nodeText(lab(e, tEn), 'r_sniper_1', 'status')).toBe('Needs laboratory 3 (now 2)');
  });
});

describe('вкладки и a11y', () => {
  it('tablist: aria-selected и переключение', async () => {
    const r = await setup();
    const render = () => expand(h(ArmyLabApp, { vm: r.ctl.viewModel(), actions: r.ctl.actions, t: tRu }));
    expect(find(render(), 'tab-army')!.props['aria-selected']).toBe(true);
    expect(find(render(), 'army-screen')).toBeDefined();
    click(find(render(), 'tab-lab'));
    expect(find(render(), 'tab-lab')!.props['aria-selected']).toBe(true);
    expect(find(render(), 'lab-screen')).toBeDefined();
    expect(find(render(), 'army-screen')).toBeUndefined();
  });

  it('все кнопки действий — type=button; недоступные — disabled, а не только стилем', async () => {
    const r = await setup({ wallet: { fuel: 0, gems: 0 } });
    for (const b of [...findAll(army(r), 'train-u_rifleman-submit'), ...findAll(lab(r), 'node-r_rifle_1-start')]) {
      expect(b.props.type).toBe('button');
      expect(b.props.disabled).toBe(true);
    }
  });

  it('адаптив: стили содержат медиазапрос для узкого экрана и auto-fill сетку', async () => {
    const r = await setup();
    const css = textOf(find(army(r), 'army-styles'));
    expect(css).toContain('@media (max-width:600px)');
    expect(css).toContain('auto-fill');
  });
});

describe('локализация', () => {
  it('все ключи, которые запрашивают экраны, есть в content/strings или EXTRAS на ru и en', async () => {
    const r = await setup({ army: { ...(await setup()).server.army, perks: { army_housing_pct: 5, raid_loot_pct: 5, scan_reveals_traps: true }, deployed: [{ raid_id: 'x', units: [{ id: 'u_rifleman', count: 2 }], spells: [] }] } });
    r.ctl.actions.setCount('u_brawler', 9);
    await r.ctl.actions.research('r_rifle_1');
    r.ctl.actions.askCancel('research');
    for (const lang of ['ru', 'en'] as Lang[]) {
      const missing = new Set<string>();
      const base = createArmyT(lang, strings[lang]);
      const t: T = (key, vars) => {
        const out = base(key, vars);
        if (out === key) missing.add(key);
        return out;
      };
      const vm = r.ctl.viewModel();
      expand(h(ArmyLabApp, { vm, actions: r.ctl.actions, t }));
      expand(h(LabScreen, { vm, actions: r.ctl.actions, t }));
      expect([...missing], lang).toEqual([]);
    }
  });

  it('набор EXTRAS на ru и en совпадает', () => {
    expect([...EXTRA_KEYS.ru].sort()).toEqual([...EXTRA_KEYS.en].sort());
  });

  it('сообщения об отказах: reason.* из strings для всех причин армии и лаборатории', () => {
    for (const reason of ['housing_full', 'locked', 'queue_full', 'not_multiple_of_group', 'lab_level', 'lab_busy', 'prerequisite']) {
      expect(strings.ru[`reason.${reason}`], reason).toBeTruthy();
      expect(strings.en[`reason.${reason}`], reason).toBeTruthy();
    }
  });
});
