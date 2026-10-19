// Dev-страница рендера боя (не часть app): `npm run dev` -> /engine/battle-dev.html.
// Тестовый BattleWorldState и серия событий из battle-fixture.ts; ввод пишется в #log.
import artManifest from '../../art/manifest.json';
import baseGridYaml from '../../content/base_grid.yaml?raw';
import buildingsYaml from '../../content/buildings.yaml?raw';
import defencesYaml from '../../content/defences.yaml?raw';
import { sampleAfter, sampleBattle, sampleEvents } from './battle-fixture';
import { centerOn } from './camera';
import { parseBaseGrid, parseFootprints } from './content';
import { PixiRenderer } from './pixi-renderer';

const atlasUrls = import.meta.glob('../../art/atlases/*.png', { eager: true, query: '?url', import: 'default' });
const atlasUrl = (path: string): string => (atlasUrls[`../../art/${path}`] as string | undefined) ?? path;

async function main(): Promise<void> {
  const host = document.getElementById('app') as HTMLElement;
  const log = document.getElementById('log') as HTMLElement;
  const grid = parseBaseGrid(baseGridYaml);
  const footprints = parseFootprints(buildingsYaml, defencesYaml);
  const mapId = new URLSearchParams(location.search).get('map') ?? grid.defaultMap;
  const fresh = () => sampleBattle(grid.size, footprints, grid.maps[mapId] ?? []);
  const renderer = new PixiRenderer({ resolveAssetUrl: atlasUrl });
  await renderer.init(host);
  let state = fresh();
  renderer.setBattle(state);
  renderer.setCamera(centerOn(15, 22, 1, { width: host.clientWidth, height: host.clientHeight }));
  if (!new URLSearchParams(location.search).has('noart')) await renderer.loadAtlas(artManifest as Record<string, unknown>);
  renderer.on((e) => {
    if (e.type.startsWith('battle')) log.textContent = JSON.stringify(e);
    if (e.type === 'battleSelect') {
      state = { ...state, selectedId: e.id };
      renderer.setBattle(state);
    }
  });
  document.getElementById('play')?.addEventListener('click', () => {
    renderer.playEvents(sampleEvents());
    state = sampleAfter(state);
    renderer.setBattle(state); // авторитетное «после»: применится, когда очередь проиграна
  });
  document.getElementById('skip')?.addEventListener('click', () => renderer.skipEvents());
  document.getElementById('reset')?.addEventListener('click', () => {
    state = fresh();
    renderer.skipEvents();
    renderer.setBattle(state);
  });
  (window as unknown as { __renderer?: PixiRenderer }).__renderer = renderer;
}

void main();
