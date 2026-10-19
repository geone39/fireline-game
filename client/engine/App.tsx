import { useEffect, useRef } from 'preact/hooks';
import artManifest from '../../art/manifest.json';
import baseGridYaml from '../../content/base_grid.yaml?raw';
import buildingsYaml from '../../content/buildings.yaml?raw';
import defencesYaml from '../../content/defences.yaml?raw';
import { mockBaseSource, type BaseSource } from './base-source';
import { parseBaseGrid, parseFootprints } from './content';
import { PixiRenderer } from './pixi-renderer';
import type { WorldState } from './renderer';

const atlasUrls = import.meta.glob('../../art/atlases/*.png', { eager: true, query: '?url', import: 'default' });

function atlasUrl(path: string): string {
  return (atlasUrls[`../../art/${path}`] as string | undefined) ?? path;
}

/** Dev-хост рендера базы. Источник состояния подставляется снаружи; по умолчанию — тестовая база. */
export function App({ source = mockBaseSource }: { source?: BaseSource }) {
  const host = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = host.current;
    if (!el) return;
    const renderer = new PixiRenderer({ resolveAssetUrl: atlasUrl });
    let disposed = false;
    void (async () => {
      await renderer.init(el);
      const grid = parseBaseGrid(baseGridYaml);
      const mapId = new URLSearchParams(location.search).get('map') ?? grid.defaultMap;
      const footprints = parseFootprints(buildingsYaml, defencesYaml);
      const base = await source.load();
      if (disposed) {
        renderer.destroy();
        return;
      }
      let world: WorldState = {
        gridSize: grid.size,
        water: grid.maps[mapId] ?? [],
        footprints,
        items: base.layout.items,
        selectedUid: null,
      };
      renderer.render(world);
      renderer.centerCamera(1);
      renderer.on((e) => {
        if (e.type === 'select') {
          world = { ...world, selectedUid: e.uid };
          renderer.render(world);
        }
      });
      await renderer.loadAtlas(artManifest as Record<string, unknown>);
      (window as unknown as { __renderer?: PixiRenderer }).__renderer = renderer;
    })();
    return () => {
      disposed = true;
    };
  }, [source]);
  return <div ref={host} style={{ width: '100%', height: '100%' }} />;
}
