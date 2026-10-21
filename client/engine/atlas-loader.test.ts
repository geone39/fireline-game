import { describe, expect, it, vi } from 'vitest';
import type { ArtAtlasEntry } from './atlas';
import { AtlasLoader, atlasCandidates, screenWants, type AtlasLoaderDeps } from './atlas-loader';

const atlases: Record<string, ArtAtlasEntry> = {
  buildings: { image: 'atlases/buildings.png', data: 'b.json', size: [1, 1] },
  units: { image: 'atlases/units.png', webp: 'atlases/units.webp', data: 'u.json', size: [1, 1] },
  spike_blender_heroes: { image: 'spike/heroes.png', data: 'h.json', size: [1, 1] },
  odd: { image: 'x/odd.png', category: 'vehicles', data: 'o.json', size: [1, 1] },
};

function setup(over: Partial<AtlasLoaderDeps<string>> = {}) {
  const loads: string[] = [];
  const unloads: string[] = [];
  const dropped: string[] = [];
  const deps: AtlasLoaderDeps<string> = {
    resolve: (p) => `/${p}`,
    load: async (url) => {
      loads.push(url);
      return `tex:${url}`;
    },
    unload: (_t, url) => void unloads.push(url),
    supportsWebp: () => true,
    onUnload: (n) => void dropped.push(n),
    ...over,
  };
  const loader = new AtlasLoader<string>(deps);
  loader.setManifest(atlases);
  return { loader, loads, unloads, dropped };
}

describe('atlasCandidates', () => {
  it('WebP первым, PNG запасным; без поддержки — только PNG', () => {
    expect(atlasCandidates(atlases.units as ArtAtlasEntry, true)).toEqual(['atlases/units.webp', 'atlases/units.png']);
    expect(atlasCandidates(atlases.units as ArtAtlasEntry, false)).toEqual(['atlases/units.png']);
    expect(atlasCandidates(atlases.buildings as ArtAtlasEntry, true)).toEqual(['atlases/buildings.png']);
  });
  it('image с расширением .webp получает PNG-запас', () => {
    const e = { image: 'a/x.webp' };
    expect(atlasCandidates(e, true)).toEqual(['a/x.webp', 'a/x.png']);
    expect(atlasCandidates(e, false)).toEqual(['a/x.png', 'a/x.webp']);
  });
});

describe('screenWants', () => {
  it('база — свои категории, бой — все', () => {
    expect(screenWants('base', 'buildings')).toBe(true);
    expect(screenWants('base', 'units')).toBe(false);
    expect(screenWants('battle', 'units')).toBe(true);
    expect(screenWants('battle', 'vehicles')).toBe(true);
  });
});

describe('AtlasLoader', () => {
  it('грузит только атласы экрана (по категории, в т.ч. из суффикса имени)', async () => {
    const { loader, loads } = setup();
    const fresh = await loader.acquire('base');
    expect(fresh).toEqual(['buildings']);
    expect(loads).toEqual(['/atlases/buildings.png']);
    await loader.acquire('battle');
    expect(loader.loadedNames.sort()).toEqual(['buildings', 'odd', 'spike_blender_heroes', 'units']);
    expect(loads).toContain('/atlases/units.webp');
  });

  it('повторный acquire не грузит заново; параллельные запросы делят одну загрузку', async () => {
    const { loader, loads } = setup();
    await Promise.all([loader.acquire('base'), loader.acquire('base')]);
    await loader.acquire('base');
    expect(loads).toHaveLength(1);
  });

  it('release выгружает атласы без держателей и сбрасывает кэш кадров', async () => {
    const { loader, unloads, dropped } = setup();
    await loader.acquire('base');
    await loader.acquire('battle');
    expect(loader.release('battle')).toEqual(expect.arrayContaining(['units', 'odd', 'spike_blender_heroes']));
    expect(loader.has('units')).toBe(false);
    expect(loader.has('buildings')).toBe(true); // держит база
    expect(dropped).toContain('units');
    expect(unloads).toContain('/atlases/units.webp');
    expect(loader.release('base')).toEqual(['buildings']);
    expect(loader.loadedNames).toEqual([]);
  });

  it('WebP не загрузился — берётся PNG', async () => {
    const load = vi.fn(async (url: string) => {
      if (url.endsWith('.webp')) throw new Error('decode');
      return `tex:${url}`;
    });
    const { loader } = setup({ load });
    await loader.acquire('battle');
    expect(loader.get('units')).toBe('tex:/atlases/units.png');
    expect(load.mock.calls.map((c) => c[0])).toEqual(expect.arrayContaining(['/atlases/units.webp', '/atlases/units.png']));
  });

  it('без поддержки WebP браузером сразу PNG', async () => {
    const { loader, loads } = setup({ supportsWebp: () => false });
    await loader.acquire('battle');
    expect(loads).not.toContain('/atlases/units.webp');
    expect(loads).toContain('/atlases/units.png');
  });

  it('ни один адрес не загрузился: onError, остальные атласы работают, acquire не падает', async () => {
    const onError = vi.fn();
    const load = async (url: string) => {
      if (url.includes('buildings')) throw new Error('404');
      return `tex:${url}`;
    };
    const { loader } = setup({ load, onError });
    const fresh = await loader.acquire('battle');
    expect(onError).toHaveBeenCalledTimes(1);
    expect(onError.mock.calls[0]?.[0]).toBe('buildings');
    expect(loader.has('buildings')).toBe(false);
    expect(fresh).toContain('units');
  });

  it('экран ушёл, пока грузилось: текстура сразу освобождается', async () => {
    let finish: (v: string) => void = () => undefined;
    const { loader, unloads } = setup({ load: () => new Promise<string>((r) => (finish = r)) });
    const p = loader.acquire('base');
    await Promise.resolve();
    await Promise.resolve();
    loader.release('base');
    finish('late');
    expect(await p).toEqual([]);
    expect(loader.has('buildings')).toBe(false);
    expect(unloads).toHaveLength(1);
  });

  it('новый манифест выгружает старое', async () => {
    const { loader, unloads } = setup();
    await loader.acquire('base');
    loader.setManifest({});
    expect(loader.loadedNames).toEqual([]);
    expect(unloads).toHaveLength(1);
  });

  it('смена манифеста во время загрузки: новая загрузка не делит устаревший результат', async () => {
    const resolvers: ((v: string) => void)[] = [];
    const { loader } = setup({ load: () => new Promise<string>((r) => resolvers.push(r)) });
    const first = loader.acquire('base');
    await Promise.resolve();
    await Promise.resolve();
    loader.setManifest(atlases);
    const second = loader.acquire('base');
    await Promise.resolve();
    await Promise.resolve();
    expect(resolvers).toHaveLength(2); // вторая загрузка своя, а не результат первой
    resolvers.forEach((r, i) => r(`tex${i}`));
    expect(await first).toEqual([]);
    expect(await second).toEqual(['buildings']);
    expect(loader.get('buildings')).toBe('tex1');
  });
});
