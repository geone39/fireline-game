// Общие помощники тестов: реальный контент из content/*.yaml, «сервер» в памяти по правилам army-lab.md (фикстура F-ARMY-1),
// и мини-«рендер» VNode без DOM. Файл не входит в публичную поверхность (index.ts его не экспортирует).
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import type { VNode } from 'preact';
import { parseStrings } from '../../core/base/catalog';
import { parseArmyCatalog, type ArmyCatalog } from './content';
import { createArmyController, type ArmyController } from './store/controller';
import { NOW_MS, createFakeServer, fixtureArmy, fixtureLab, type FakeServer, type Wallet } from './fake-server';
import type { ArmyStateDto, BuildingRef, LabStateDto } from './types';

export { NOW_MS, fixtureArmy, fixtureLab, type FakeServer, type Wallet };

const root = resolve(__dirname, '../../..');
const read = (p: string): string => readFileSync(resolve(root, p), 'utf8');

export const catalog: ArmyCatalog = parseArmyCatalog(
  read('content/troops.yaml'),
  read('content/research.yaml'),
  read('content/army.yaml'),
  read('content/building_costs.yaml'),
);

export const strings = {
  ru: { ...parseStrings(read('content/strings/ru/names.yaml')), ...parseStrings(read('content/strings/ru/army.yaml')) },
  en: { ...parseStrings(read('content/strings/en/names.yaml')), ...parseStrings(read('content/strings/en/army.yaml')) },
};

export const NOW_S = NOW_MS / 1000;

/** Часы теста: двигаются вручную. */
export function makeClock(): { now: () => number; advance: (s: number) => void } {
  let t = NOW_MS;
  return { now: () => t, advance: (s) => void (t += s * 1000) };
}

export const buildings = (extra: BuildingRef[] = []): BuildingRef[] => [
  { uid: 'hq1', type: 'b_hq', level: 3 },
  { uid: 'br1', type: 'b_barracks', level: 2 },
  { uid: 'br2', type: 'b_barracks', level: 2 },
  { uid: 'lab1', type: 'b_laboratory', level: 3 },
  ...extra,
];

export interface Rig {
  server: FakeServer;
  ctl: ArmyController;
  clock: ReturnType<typeof makeClock>;
  settled: number;
}

export async function setup(o: { army?: ArmyStateDto; lab?: LabStateDto; wallet?: Wallet; buildings?: BuildingRef[]; noWallet?: boolean } = {}): Promise<Rig> {
  const clock = makeClock();
  const server = createFakeServer(catalog, { army: o.army, lab: o.lab, wallet: o.wallet, clock: clock.now });
  const rig = { server, clock, settled: 0 } as Rig;
  rig.ctl = createArmyController({
    api: server.api,
    catalog,
    buildings: () => o.buildings ?? buildings(),
    wallet: o.noWallet ? undefined : () => ({ ...server.wallet }),
    now: clock.now,
    newKey: (() => {
      let n = 0;
      return () => `k${++n}`;
    })(),
    onSettled: () => void rig.settled++,
  });
  await rig.ctl.actions.refresh();
  return rig;
}

// --- Мини-«рендер» VNode без DOM (как в core/base/test-kit.ts) ---
export interface El {
  tag: string;
  props: Record<string, unknown>;
  children: (El | string)[];
}

export function expand(node: unknown): (El | string)[] {
  if (node === null || node === undefined || typeof node === 'boolean') return [];
  if (typeof node === 'string' || typeof node === 'number') return [String(node)];
  if (Array.isArray(node)) return node.flatMap(expand);
  const v = node as VNode<Record<string, unknown>>;
  if (typeof v.type === 'function') return expand((v.type as (p: Record<string, unknown>) => unknown)({ ...v.props }));
  const { children, ...props } = v.props;
  return [{ tag: String(v.type), props, children: expand(children) }];
}

export function findAll(nodes: (El | string)[], testid: string): El[] {
  const out: El[] = [];
  const walk = (ns: (El | string)[]): void => {
    for (const n of ns) {
      if (typeof n === 'string') continue;
      if (n.props['data-testid'] === testid) out.push(n);
      walk(n.children);
    }
  };
  walk(nodes);
  return out;
}
export const find = (nodes: (El | string)[], testid: string): El | undefined => findAll(nodes, testid)[0];
export const textOf = (n: El | string | undefined): string => (n === undefined ? '' : typeof n === 'string' ? n : n.children.map(textOf).join(''));

export function click(e: El | undefined): void {
  if (!e) throw new Error('element not found');
  if (e.props.disabled) throw new Error('button is disabled');
  (e.props.onClick as () => void)();
}
