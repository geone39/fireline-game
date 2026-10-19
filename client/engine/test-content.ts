import artManifestJson from '../../art/manifest.json';
import baseGridYaml from '../../content/base_grid.yaml?raw';
import buildingsYaml from '../../content/buildings.yaml?raw';
import defencesYaml from '../../content/defences.yaml?raw';
import { parseBaseGrid, parseFootprints } from './content';

// Реальные файлы контента и арта: тесты проверяют согласованность рендера с ними.
export const grid = parseBaseGrid(baseGridYaml);
export const footprints = parseFootprints(buildingsYaml, defencesYaml);
export const artManifest = artManifestJson as Record<string, unknown>;
