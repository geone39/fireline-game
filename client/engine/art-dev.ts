// Dev-страница настоящих атласов (CLIENT-6): `npm run dev` -> /engine/art-dev.html?view=base|battle|perf&backend=blender|proc|none.
// Штаб, шахта и стрелок из art/spike в клетках базы и боя; стрелок — со всеми анимациями в 8 направлениях
// (Blender) или 4 (процедурный путь). Режим perf: 150 построек, замер draw calls и fps. Не часть app.
import artManifest from '../../art/manifest.json';
import blenderManifest from '../../art/spike/blender/out/manifest.json';
import procManifest from '../../art/spike/procedural/out/manifest.json';
import baseGridYaml from '../../content/base_grid.yaml?raw';
import buildingsYaml from '../../content/buildings.yaml?raw';
import defencesYaml from '../../content/defences.yaml?raw';
import type { BaseItem } from './base-types';
import { mockBaseState } from './base-source';
import { EVENT, type BattleEntity, type BattleEvent, type BattleWorldState } from './battle-types';
import { centerOn } from './camera';
import { parseBaseGrid, parseFootprints } from './content';
import { PixiRenderer } from './pixi-renderer';
import type { WorldState } from './renderer';

type Backend = 'blender' | 'proc' | 'none';
type View = 'base' | 'battle' | 'perf';

// Счётчик draw calls: подмена методов WebGL до создания контекста PixiJS.
const draws = { n: 0 };
for (const proto of [WebGL2RenderingContext.prototype, WebGLRenderingContext.prototype] as unknown as Record<string, unknown>[]) {
  for (const name of ['drawElements', 'drawArrays', 'drawElementsInstanced', 'drawArraysInstanced']) {
    const orig = proto[name] as ((...a: unknown[]) => void) | undefined;
    if (!orig) continue;
    proto[name] = function (this: unknown, ...a: unknown[]) {
      draws.n++;
      return orig.apply(this, a);
    };
  }
}

const imageUrls = import.meta.glob('../../art/**/*.{png,webp}', { eager: true, query: '?url', import: 'default' });
const fetched: string[] = [];
const resolveUrl = (path: string): string => {
  fetched.push(path);
  return (imageUrls[`../../art/${path}`] as string | undefined) ?? path;
};

interface Manifest {
  schema: number;
  atlases: Record<string, unknown>;
  sprites: { id: string }[];
  _anims?: Record<string, unknown>;
}

/** Заглушки ART-0 + подмена b_hq, b_cash_mine, u_rifleman выбранным путём спайка (как art/tools/merge_spike.py). */
function compose(backend: Backend): Manifest {
  const base = artManifest as unknown as Manifest;
  if (backend === 'none') return base;
  const spike = (backend === 'blender' ? blenderManifest : procManifest) as unknown as Manifest;
  const ids = new Set(spike.sprites.map((s) => s.id));
  return {
    ...base,
    atlases: { ...base.atlases, ...spike.atlases },
    sprites: [...base.sprites.filter((s) => !ids.has(s.id)), ...spike.sprites],
    _anims: spike._anims,
  };
}

// Направления art: 0 вниз, далее по часовой стрелке на экране. Вектор в тайлах (dx, dy).
const DIR: [number, number][] = [
  [1, 1], [0, 1], [-1, 1], [-1, 0], [-1, -1], [0, -1], [1, -1], [1, 0],
];
const CENTER = { x: 20, y: 20 };
const RING = 7;
const WALL_RING = 3;

function battleScene(size: number, footprints: Record<string, readonly [number, number]>): BattleWorldState {
  const fp = (t: string) => ({ w: footprints[t]?.[0] ?? 1, h: footprints[t]?.[1] ?? 1 });
  const entities: BattleEntity[] = [
    { id: 1, side: 'attacker', type: 'u_rifleman', kind: 'troop', x: CENTER.x, y: CENTER.y, hp: 90, maxHp: 90, ap: 2, maxAp: 2 },
    { id: 2, side: 'defender', type: 'b_hq', kind: 'building', x: 5, y: 5, ...fp('b_hq'), hp: 800, maxHp: 800 },
    { id: 3, side: 'defender', type: 'b_cash_mine', kind: 'building', x: 31, y: 5, ...fp('b_cash_mine'), hp: 500, maxHp: 500 },
    { id: 4, side: 'defender', type: 'b_cash_mine', kind: 'building', x: 5, y: 31, ...fp('b_cash_mine'), hp: 500, maxHp: 500 },
  ];
  DIR.forEach(([dx, dy], d) => {
    entities.push({
      id: 11 + d, side: 'defender', type: 'd_wall', kind: 'wall',
      x: CENTER.x + dx * WALL_RING, y: CENTER.y + dy * WALL_RING, hp: 900, maxHp: 900,
    });
    entities.push({
      id: 21 + d, side: 'attacker', type: 'u_rifleman', kind: 'troop',
      x: CENTER.x + dx * RING, y: CENTER.y + dy * RING, hp: 90, maxHp: 90,
    });
  });
  return { gridSize: size, entities };
}

const ev = (Kind: number, Actor: number, Target: number, x: number, y: number, Value = 0): BattleEvent => ({
  Kind, Actor, Target, Cell: { X: x, Y: y }, Value,
});

function perfItems(footprints: Record<string, readonly [number, number]>, sprites: Set<string>): BaseItem[] {
  const types = Object.keys(footprints).filter(
    (t) => sprites.has(t) && (footprints[t]?.[0] ?? 9) <= 3 && (footprints[t]?.[1] ?? 9) <= 3 && !t.startsWith('x_'),
  );
  const items: BaseItem[] = [];
  const slot = 3;
  const per = Math.floor(44 / slot);
  for (let i = 0; i < 150; i++) {
    const type = types[(i * 7) % types.length] as string;
    items.push({ uid: `p${i}`, type, level: 1, x: (i % per) * slot, y: Math.floor(i / per) * slot, rot: 0 });
  }
  return items;
}

async function main(): Promise<void> {
  const q = new URLSearchParams(location.search);
  let view = (q.get('view') ?? 'base') as View;
  let backend = (q.get('backend') ?? 'blender') as Backend;
  const host = document.getElementById('app') as HTMLElement;
  const hud = document.getElementById('hud') as HTMLElement;
  const grid = parseBaseGrid(baseGridYaml);
  const footprints = parseFootprints(buildingsYaml, defencesYaml);
  const renderer = new PixiRenderer({ resolveAssetUrl: resolveUrl });
  await renderer.init(host);
  let battle = battleScene(grid.size, footprints);
  let loadedBackend: Backend | null = null;

  const markButtons = () => {
    document.querySelectorAll<HTMLButtonElement>('[data-view]').forEach((b) => b.classList.toggle('on', b.dataset.view === view));
    document.querySelectorAll<HTMLButtonElement>('[data-backend]').forEach((b) => b.classList.toggle('on', b.dataset.backend === backend));
    (document.getElementById('battle-bar') as HTMLElement).style.display = view === 'battle' ? '' : 'none';
  };

  const show = async (): Promise<void> => {
    markButtons();
    const size = { width: host.clientWidth, height: host.clientHeight };
    if (view === 'battle') {
      renderer.skipEvents();
      battle = battleScene(grid.size, footprints);
      renderer.setBattle(battle);
      renderer.setCamera(centerOn(CENTER.x, CENTER.y, 1.6, size));
    } else {
      const sprites = new Set((compose(backend).sprites as { id: string }[]).map((s) => s.id));
      const items = view === 'perf' ? perfItems(footprints, sprites) : mockBaseState().layout.items;
      const world: WorldState = { gridSize: grid.size, water: grid.maps[grid.defaultMap] ?? [], footprints, items, selectedUid: null };
      renderer.render(world);
      if (view === 'perf') renderer.setCamera(centerOn(20, 20, 0.6, size));
      else renderer.setCamera(centerOn(20, 22, 1.4, size));
    }
    // Манифест грузится один раз на путь; переключение база/бой подгружает и освобождает атласы лениво (по экранам).
    if (loadedBackend !== backend) {
      loadedBackend = backend;
      await renderer.loadAtlas(compose(backend) as unknown as Record<string, unknown>);
    }
  };

  document.querySelectorAll<HTMLButtonElement>('[data-view]').forEach((b) =>
    b.addEventListener('click', () => {
      view = b.dataset.view as View;
      void show();
    }),
  );
  document.querySelectorAll<HTMLButtonElement>('[data-backend]').forEach((b) =>
    b.addEventListener('click', () => {
      backend = b.dataset.backend as Backend;
      void show();
    }),
  );

  const play = (events: BattleEvent[], after: BattleWorldState): void => {
    renderer.playEvents(events);
    battle = after;
    renderer.setBattle(battle);
  };
  document.getElementById('walk')?.addEventListener('click', () => {
    const events: BattleEvent[] = [];
    const moved = new Map<number, { x: number; y: number }>();
    DIR.forEach(([dx, dy], d) => {
      const e = battle.entities.find((x) => x.id === 21 + d);
      if (!e) return;
      const to = { x: e.x + dx * 2, y: e.y + dy * 2 };
      moved.set(e.id, to);
      events.push(ev(EVENT.moved, e.id, 0, to.x, to.y, 2));
    });
    play(events, { ...battle, entities: battle.entities.map((e) => (moved.has(e.id) ? { ...e, ...moved.get(e.id) } : e)) });
  });
  document.getElementById('attack')?.addEventListener('click', () => {
    play(DIR.map(([dx, dy], d) => ev(EVENT.attacked, 1, 11 + d, CENTER.x + dx * WALL_RING, CENTER.y + dy * WALL_RING, 1)), battle);
  });
  document.getElementById('die')?.addEventListener('click', () => {
    const events = DIR.map(([dx, dy], d) => {
      const e = battle.entities.find((x) => x.id === 21 + d);
      return ev(EVENT.killed, 1, 21 + d, e?.x ?? CENTER.x + dx * RING, e?.y ?? CENTER.y + dy * RING);
    });
    play(events, { ...battle, entities: battle.entities.filter((e) => e.id < 21 || e.id > 28) });
  });
  document.getElementById('reset')?.addEventListener('click', () => {
    renderer.skipEvents();
    battle = battleScene(grid.size, footprints);
    renderer.setBattle(battle);
  });

  // HUD: fps и draw calls за кадр (среднее за ~1 с).
  let lastFrames = 0;
  let lastDraws = 0;
  let lastAt = performance.now();
  let drawsPerFrame = 0;
  const stats = { drawsPerFrame: 0, fps: 0, maxDrawsPerFrame: 0 };
  setInterval(() => {
    const now = performance.now();
    const frames = renderer.framesRendered - lastFrames;
    if (frames > 0) {
      drawsPerFrame = (draws.n - lastDraws) / frames;
      stats.fps = (frames * 1000) / (now - lastAt);
      stats.drawsPerFrame = drawsPerFrame;
      stats.maxDrawsPerFrame = Math.max(stats.maxDrawsPerFrame, drawsPerFrame);
    }
    lastFrames = renderer.framesRendered;
    lastDraws = draws.n;
    lastAt = now;
    hud.textContent = [
      `view=${view} backend=${backend}`,
      `fps ${stats.fps.toFixed(1)} (ticker ${renderer.fps.toFixed(1)}) · draw calls/кадр ${drawsPerFrame.toFixed(1)}`,
      `атласы: ${renderer.loadedAtlases.join(', ') || '—'}`,
      `запрошено: ${[...new Set(fetched)].join(', ')}`,
    ].join('\n');
  }, 1000);

  (window as unknown as { __art?: unknown }).__art = { renderer, stats, draws };
  await show();
}

void main();
