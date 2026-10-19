import type { BaseItem } from './base-types';

/** Подмножество BaseState (docs/03-api.md, раздел Base), нужное рендеру. Остальные поля игнорируются. */
export interface BaseStateView {
  version: number;
  layout: { items: BaseItem[] };
}

/** Источник состояния базы. Рендер сам API не зовёт: реальный источник даёт client-core, в dev и тестах — мок. */
export interface BaseSource {
  load(): Promise<BaseStateView>;
}

/** Тестовая база для dev-сервера и тестов: подмножество типов из content/buildings.yaml и defences.yaml. */
export function mockBaseState(): BaseStateView {
  const it = (uid: string, type: string, x: number, y: number, rot = 0): BaseItem => ({
    uid,
    type,
    level: 1,
    x,
    y,
    rot,
  });
  const items: BaseItem[] = [
    it('hq', 'b_hq', 20, 20),
    it('cm1', 'b_cash_mine', 14, 20),
    it('cm2', 'b_cash_mine', 14, 24),
    it('fr1', 'b_fuel_refinery', 26, 20),
    it('vault', 'b_vault', 20, 14),
    it('tank', 'b_fuel_tank', 26, 14),
    it('barr', 'b_barracks', 20, 26),
    it('camp', 'b_camp', 25, 25),
    it('lab', 'b_laboratory', 14, 14),
    it('mg1', 'd_mg_nest', 18, 17),
    it('mg2', 'd_mg_nest', 24, 18),
    it('sn1', 'd_sniper_tower', 12, 18),
    it('rock', 'o_rock', 8, 8),
    it('wreck', 'o_wreck', 30, 30),
  ];
  for (let i = 0; i < 10; i++) items.push(it(`w${i}`, 'd_wall', 10 + i, 30));
  for (let i = 0; i < 10; i++) items.push(it(`f${i}`, 'd_fence', 10, 20 + i));
  return { version: 1, layout: { items } };
}

export const mockBaseSource: BaseSource = { load: () => Promise.resolve(mockBaseState()) };
