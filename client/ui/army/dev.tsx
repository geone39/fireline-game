// Dev-страница экранов армии и лаборатории на «сервере» в памяти (fake-server.ts): http://localhost:5173/ui/army/dev.html
// Параметры: ?lang=ru|en, ?scene=housing|poor|lab|research|speedup (готовые состояния для скриншотов). Кнопки сверху двигают серверные часы и меняют кошелёк, чтобы пройти сценарии вручную.
import { render } from 'preact';
import artManifest from '../../../art/manifest.json';
import armyYaml from '../../../content/army.yaml?raw';
import costsYaml from '../../../content/building_costs.yaml?raw';
import enArmy from '../../../content/strings/en/army.yaml?raw';
import enNames from '../../../content/strings/en/names.yaml?raw';
import researchYaml from '../../../content/research.yaml?raw';
import ruArmy from '../../../content/strings/ru/army.yaml?raw';
import ruNames from '../../../content/strings/ru/names.yaml?raw';
import troopsYaml from '../../../content/troops.yaml?raw';
import { parseStrings } from '../../core/base/catalog';
import { ArmyLabApp } from './app';
import { parseArmyCatalog } from './content';
import { createFakeServer } from './fake-server';
import { createArmyT, type Lang } from './i18n';
import { useArmyViewModel } from './hook';
import { makeIconResolver } from './icons';
import { createArmyController } from './store/controller';
import type { BuildingRef } from './types';

const atlasUrls = import.meta.glob('../../../art/atlases/*.png', { eager: true, query: '?url', import: 'default' });
const icon = makeIconResolver(artManifest, (img) => (atlasUrls[`../../../art/${img}`] as string | undefined) ?? img);

const catalog = parseArmyCatalog(troopsYaml, researchYaml, armyYaml, costsYaml);
const lang: Lang = new URLSearchParams(location.search).get('lang') === 'en' ? 'en' : 'ru';
const strings = lang === 'en' ? { ...parseStrings(enNames), ...parseStrings(enArmy) } : { ...parseStrings(ruNames), ...parseStrings(ruArmy) };
const t = createArmyT(lang, strings);

let offsetMs = 0;
const clock = (): number => Date.now() + offsetMs;
const server = createFakeServer(catalog, { clock });
const buildings: BuildingRef[] = [
  { uid: 'hq1', type: 'b_hq', level: 3 },
  { uid: 'br1', type: 'b_barracks', level: 2 },
  { uid: 'br2', type: 'b_barracks', level: 2 },
  { uid: 'lab1', type: 'b_laboratory', level: 2 },
];
const ctl = createArmyController({ api: server.api, catalog, buildings: () => buildings, wallet: () => ({ ...server.wallet }), now: clock });

/** Готовые сцены для скриншотов и ручной проверки. */
async function scene(name: string | null): Promise<void> {
  await ctl.actions.refresh();
  const a = ctl.actions;
  switch (name) {
    case 'housing':
      a.setCount('u_brawler', 4); // нужно 16 мест, свободно 12
      break;
    case 'poor':
      server.wallet.fuel = 100;
      a.setCount('u_brawler', 1);
      break;
    case 'lab':
      a.selectTab('lab');
      break;
    case 'research':
      a.selectTab('lab');
      await a.research('r_rifle_1');
      offsetMs += 30 * 60_000;
      break;
    case 'speedup':
      a.selectBuilding('br2');
      a.setCount('u_brawler', 3);
      await a.train('u_brawler');
      break;
    default:
      break;
  }
}
void scene(new URLSearchParams(location.search).get('scene'));

function App() {
  const vm = useArmyViewModel(ctl);
  return (
    <>
      <div class="dev-bar">
        <button type="button" onClick={() => (offsetMs += 60_000)}>+1 мин</button>
        <button type="button" onClick={() => (offsetMs += 3_600_000)}>+1 ч</button>
        <button type="button" onClick={() => (server.wallet.fuel = 100)}>Мало fuel</button>
        <button type="button" onClick={() => (server.wallet.gems = 0)}>0 гемов</button>
        <button type="button" onClick={() => Object.assign(server.wallet, { fuel: 5000, gems: 50 })}>Богатый</button>
      </div>
      <ArmyLabApp vm={vm} actions={ctl.actions} t={t} icon={icon} />
    </>
  );
}

render(<App />, document.getElementById('root')!);
