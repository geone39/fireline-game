// HUD: селектор и компоненты (дерево VNode без DOM, как в core/base/ui.test.ts).
import { h } from 'preact';
import { describe, expect, it } from 'vitest';
import { toWorld } from '../core/base';
import { baseState, expand, find, findAll, startItems, textOf } from '../core/base/test-kit';
import type { Me } from './app';
import { loadContent } from './content';
import { Hud } from './hud';
import { selectHud, WALLET_ORDER } from './hud-model';
import { createAppT } from './i18n';

const content = loadContent();
const NOW = Date.parse('2026-01-01T00:00:00Z');
const me: Me = {
  profile: { id: 'p', nickname: 'Guest', hq_level: 1, trophies: 3, league: 'bronze', shield_until: null, created_at: '' },
  wallets: { cash: 1500, fuel: 20, gems: 5 },
  builders: [
    { id: 'b1', busy_until: '2026-01-01T00:10:00Z', target_id: 'cm1' },
    { id: 'b2', busy_until: null },
  ],
};
const world = toWorld(
  baseState(startItems(), {
    builders: { total: 2, busy: 1 },
    collectors: { cash: 120, fuel: 0 },
    timers: [{ id: 't1', kind: 'upgrade', target_id: 'cm1', finish_at: '2026-01-01T01:02:05Z', to_level: 2 }],
  }),
);
const vm = () => selectHud({ me, world, catalog: content.catalog, serverNowMs: NOW });
const mineKey = content.catalog.get('b_cash_mine')!.nameKey;

describe('selectHud', () => {
  it('кошелёк: фиксированный порядок, отсутствующие валюты = 0', () => {
    expect(vm().wallet.map((w) => w.res)).toEqual([...WALLET_ORDER]);
    expect(vm().wallet.map((w) => w.amount)).toEqual([1500, 20, 0, 5]);
  });
  it('билдеры, сборщики (только > 0) и таймеры', () => {
    const v = vm();
    expect(v.builders).toEqual({ total: 2, free: 1 });
    expect(v.collectors).toEqual([{ res: 'cash', amount: 120 }]);
    expect(v.timers).toEqual([{ id: 't1', uid: 'cm1', nameKey: mineKey, kind: 'upgrade', toLevel: 2, remainingS: 3725 }]);
  });
  it('без /v1/me: нули и билдеры из базы', () => {
    const v = selectHud({ me: null, world, catalog: content.catalog, serverNowMs: NOW });
    expect(v.profile).toBeNull();
    expect(v.wallet.every((w) => w.amount === 0)).toBe(true);
    expect(v.builders.total).toBe(2);
  });
});

describe('<Hud>', () => {
  it('ru: названия ресурсов, билдеры, таймер с name_key', () => {
    const t = createAppT('ru', content.names.ru);
    const tree = expand(h(Hud, { vm: vm(), t }));
    expect(textOf(find(tree, 'wallet-cash')!)).toBe('Деньги: 1,500');
    expect(textOf(find(tree, 'wallet-gems')!)).toContain('Гемы');
    expect(textOf(find(tree, 'hud-builders')!)).toContain('Строители: 1/2 свободно');
    expect(textOf(find(tree, 'collector-cash')!)).toContain('120');
    const timer = textOf(find(tree, 'timer-cm1')!);
    expect(timer).toContain(content.names.ru[mineKey]!);
    expect(timer).toContain('1 ч 2 м');
  });
  it('en: другая локаль; пустые сборщики/таймеры — подписи', () => {
    const t = createAppT('en', content.names.en);
    const empty = selectHud({ me, world: toWorld(baseState(startItems())), catalog: content.catalog, serverNowMs: NOW });
    const tree = expand(h(Hud, { vm: empty, t }));
    expect(textOf(find(tree, 'wallet-cash')!)).toContain('Cash');
    expect(textOf(find(tree, 'hud-collectors')!)).toContain('Nothing to collect');
    expect(findAll(tree, 'hud-timers-empty')).toHaveLength(1);
  });
});
