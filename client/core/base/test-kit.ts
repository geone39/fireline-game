// Общие фикстуры тестов слайса и UI: реальный контент + моки API и Renderer. В прод-сборку не входит.
import type { VNode } from 'preact';
import baseGridYaml from '../../../content/base_grid.yaml?raw';
import buildingsYaml from '../../../content/buildings.yaml?raw';
import costsYaml from '../../../content/building_costs.yaml?raw';
import defencesYaml from '../../../content/defences.yaml?raw';
import enNames from '../../../content/strings/en/names.yaml?raw';
import ruNames from '../../../content/strings/ru/names.yaml?raw';
import type { InputEvent, InputHandler, Renderer, WorldState } from '../../engine/renderer';
import { parseCatalog, parseGrid, parsePricing, parseStrings } from './catalog';
import type { BaseApi, BaseCommandDto, BaseItem, BaseStateDto } from './types';

export const catalog = parseCatalog(buildingsYaml, defencesYaml);
export const grid = parseGrid(baseGridYaml);
export const coastalGrid = parseGrid(baseGridYaml, 'coastal');
export const pricing = parsePricing(costsYaml);
export const names = { en: parseStrings(enNames), ru: parseStrings(ruNames) };

export const item = (uid: string, type: string, x: number, y: number, rot = 0, level = 1): BaseItem => ({ uid, type, level, x, y, rot });

export function baseState(items: BaseItem[], extra: Partial<BaseStateDto> = {}): BaseStateDto {
  return {
    version: 1,
    layout: { items },
    timers: [],
    builders: { total: 2, busy: 0 },
    collectors: {},
    server_time: '2026-01-01T00:00:00.000Z',
    ...extra,
  };
}

/** HQ 1-го уровня, рудник, две пулемётные точки... — всё в пределах сетки и без пересечений. */
export const startItems = (): BaseItem[] => [
  item('hq', 'b_hq', 20, 20),
  item('cm1', 'b_cash_mine', 14, 20),
  item('mg1', 'd_mg_nest', 26, 20),
  item('rock', 'o_rock', 8, 8),
];

export interface MockApi extends BaseApi {
  calls: { sent: { commands: BaseCommandDto[]; key: string }[]; saved: BaseItem[][] };
  /** Что вернёт следующий вызов sendCommands/saveLayout (функция от входа) либо бросит. */
  onSend: (commands: BaseCommandDto[], key: string) => Promise<BaseStateDto>;
  onSave: (items: readonly BaseItem[]) => Promise<BaseStateDto>;
}

export function mockApi(initial: BaseStateDto): MockApi {
  const api: MockApi = {
    calls: { sent: [], saved: [] },
    fetchBase: () => Promise.resolve(initial),
    sendCommands(commands, key) {
      api.calls.sent.push({ commands, key });
      return api.onSend(commands, key);
    },
    saveLayout(items) {
      api.calls.saved.push([...items]);
      return api.onSave(items);
    },
    onSend: () => Promise.resolve(initial),
    onSave: (items) => Promise.resolve({ ...initial, version: initial.version + 1, layout: { items: [...items] } }),
  };
  return api;
}

export interface MockRenderer extends Renderer {
  renders: WorldState[];
  emit(e: InputEvent): void;
}

export function mockRenderer(): MockRenderer {
  const handlers = new Set<InputHandler>();
  const r: MockRenderer = {
    renders: [],
    loadAtlas: () => Promise.resolve(),
    render: (w) => void r.renders.push(w),
    on(h) {
      handlers.add(h);
      return () => void handlers.delete(h);
    },
    setCamera: () => undefined,
    play: () => undefined,
    emit: (e) => handlers.forEach((h) => h(e)),
  };
  return r;
}

// --- Мини-«рендер» VNode без DOM: раскрывает функциональные компоненты в дерево элементов. ---
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
  if (typeof v.type === 'function') {
    return expand((v.type as (p: Record<string, unknown>) => unknown)({ ...v.props }));
  }
  const { children, ...props } = v.props;
  const el: El = { tag: String(v.type), props, children: expand(children) };
  return [el];
}

export function walk(nodes: (El | string)[], f: (e: El) => void): void {
  for (const n of nodes) {
    if (typeof n === 'string') continue;
    f(n);
    walk(n.children, f);
  }
}

export function findAll(nodes: (El | string)[], testid: string): El[] {
  const out: El[] = [];
  walk(nodes, (e) => {
    if (e.props['data-testid'] === testid) out.push(e);
  });
  return out;
}

export function find(nodes: (El | string)[], testid: string): El | undefined {
  return findAll(nodes, testid)[0];
}

export function textOf(n: El | string): string {
  return typeof n === 'string' ? n : n.children.map(textOf).join('');
}

export function click(e: El | undefined): void {
  if (!e) throw new Error('element not found');
  if (e.props.disabled) throw new Error('button is disabled');
  (e.props.onClick as () => void)();
}
