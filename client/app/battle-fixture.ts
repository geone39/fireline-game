// ТЕСТОВЫЙ бой без сервера: Snapshot — копия из golden-фикстуры sim (battle-snapshot.json = G24.snapshot; клиентский
// контейнер не видит ../sim, поэтому копия лежит рядом, тест battle-model.test.ts следит за расхождением) (G24: оборона со стенами, проволокой,
// джаммером, миномётом, штабом; Rules встроены в файл) + свой набор армии/заклинаний/героя. Числа тут — фикстура ядра,
// НЕ баланс. Настоящий Snapshot (с Rules из content/*.yaml) придёт от сервера: OQ-C4b-2.
import g24 from './battle-snapshot.json';
import type { Snapshot } from '../sim-wasm/api';

export const TEST_SEED = '1';

export function testSnapshot(): Snapshot {
  const base = g24 as unknown as Snapshot;
  return {
    ...base,
    BaseID: 'local-test',
    Army: [
      { ID: 'u_rifleman', Level: 1, Count: 4 },
      { ID: 'u_brawler', Level: 1, Count: 2 },
      { ID: 'v_tank', Level: 1, Count: 2 },
      { ID: 'u_medic', Level: 1, Count: 1 },
    ],
    Spells: [
      { ID: 'c_airstrike', Level: 1, Count: 1 },
      { ID: 'c_medkit', Level: 1, Count: 1 },
    ],
    Hero: 'h_cover',
    HeroLevel: 1,
  };
}
