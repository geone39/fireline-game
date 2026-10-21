// Общие помощники e2e: API-клиент для проверок/подготовки и доступ к канвасу базы по тайловым координатам.
import { expect, type Locator, type Page, type TestInfo } from '@playwright/test';
import { randomUUID } from 'node:crypto';
import { readFileSync } from 'node:fs';

export interface BaseItem {
  uid: string;
  type: string;
  level: number;
  x: number;
  y: number;
  rot: number;
}
export interface BaseState {
  version: number;
  layout: { items: BaseItem[] };
  timers: { id: string; kind: string; target_id: string; finish_at: string; to_level?: number }[];
  builders: { total: number; busy: number };
  error?: { code: string; message: string; details?: { reason?: string } };
}
export interface Me {
  profile: { id: string; nickname: string; hq_level: number };
  wallets: Record<string, number>;
  builders: { id: string; busy_until: string | null }[];
  /** SERVER-5 (docs/03-api.md): на main до слияния #133 полей может не быть. */
  army?: { pools: Record<string, { used: number; capacity: number }> };
  lab?: { level: number; active: { node_id: string; finish_at: number } | null };
}

const API = process.env.E2E_API_URL ?? process.env.E2E_BASE_URL ?? 'http://localhost:5173';

export class Api {
  constructor(public token: string) {}

  private async call<T>(method: string, path: string, body?: unknown): Promise<{ status: number; body: T }> {
    const r = await fetch(`${API}${path}`, {
      method,
      headers: { 'content-type': 'application/json', authorization: `Bearer ${this.token}`, 'Idempotency-Key': randomUUID() },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    return { status: r.status, body: (await r.json()) as T };
  }

  me = async (): Promise<Me> => (await this.call<Me>('GET', '/v1/me')).body;
  base = async (): Promise<BaseState> => (await this.call<BaseState>('GET', '/v1/base')).body;
  commands = (cmds: { op: string; args: Record<string, unknown> }[]) => this.call<BaseState>('POST', '/v1/base/commands', cmds);
}

/** Токен текущей сессии страницы (клиент хранит его в localStorage, ключ оканчивается на fireline.tokens). */
export async function pageApi(page: Page): Promise<Api> {
  const raw = await page.evaluate(() => {
    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i) ?? '';
      if (k.endsWith('fireline.tokens')) return localStorage.getItem(k);
    }
    return null;
  });
  expect(raw, 'токены гостя в localStorage').not.toBeNull();
  return new Api((JSON.parse(raw as string) as { access_token: string }).access_token);
}

export const wallet = (page: Page, res: string): Locator => page.locator(`[data-testid=wallet-${res}] strong`);
export const num = async (loc: Locator): Promise<number> => Number((await loc.innerText()).replace(/[^\d]/g, ''));

/** Открыть игру новым гостем (чистый контекст Playwright = пустой localStorage) и дождаться HUD с данными. */
export async function openGame(page: Page): Promise<void> {
  await page.goto('/?lang=ru');
  await expect(page.getByTestId('hud-profile')).toBeVisible();
  await expect(wallet(page, 'cash')).toBeVisible();
}

/** Клетка призрака по умолчанию (floor(grid.w/2), content/base_grid.yaml: 44 → 22), пока указатель не над канвасом. */
const GHOST_DEFAULT = 22;

export type Project = (tx: number, ty: number) => { x: number; y: number };

/**
 * Калибровка проекции «тайл → страница»: открываем призрак здания (указатель ещё не двигали), берём вершины
 * его полигона подсветки (project(x,y), (x+1,y), (x,y+1)) и разворачиваем в аффинное отображение. Призрак закрываем.
 */
export async function calibrate(page: Page): Promise<Project> {
  await page.locator('[data-testid=shop] button:enabled').first().click(); // любое доступное здание: нужен только призрак
  await expect(page.getByTestId('overlay-tile').first()).toBeVisible();
  const b = await page.evaluate(() => {
    const tile = document.querySelector('[data-testid=overlay-tile]') as SVGPolygonElement;
    const pts = (tile.getAttribute('points') ?? '').split(' ').map((p) => p.split(',').map(Number));
    const r = (document.querySelector('[data-testid=base-screen]') as HTMLElement).getBoundingClientRect(); // координаты полигонов — от base-screen
    return { pts, left: r.left, top: r.top };
  });
  await page.getByTestId('placement-cancel').click();
  type P = [number, number];
  const [v0, v1, , v3] = b.pts as [P, P, P, P];
  return (tx, ty) => ({
    x: b.left + v0[0] + (tx - GHOST_DEFAULT) * (v1[0] - v0[0]) + (ty - GHOST_DEFAULT) * (v3[0] - v0[0]),
    y: b.top + v0[1] + (tx - GHOST_DEFAULT) * (v1[1] - v0[1]) + (ty - GHOST_DEFAULT) * (v3[1] - v0[1]),
  });
}

/** Скриншот ключевого экрана: в outputPath теста и в tests/e2e/screenshots (для вложения в PR). */
export async function shot(page: Page, info: TestInfo, name: string): Promise<void> {
  await page.screenshot({ path: info.outputPath(`${name}.png`) });
  await page.screenshot({ path: `screenshots/${name}.png` });
}

/** Поставить здание через UI: выбрать в панели, навести/кликнуть по клетке, подтвердить. */
export async function placeBuilding(page: Page, project: Project, type: string, x: number, y: number): Promise<void> {
  // после стройки клиент оставляет открытой карточку нового здания — закрываем, чтобы вернуться к панели построек
  if (await page.getByTestId('card').isVisible()) await page.getByTestId('card-close').click();
  await page.getByTestId(`shop-${type}`).locator('button').click();
  const at = project(x, y);
  await page.mouse.move(at.x, at.y);
  await page.mouse.click(at.x, at.y);
  await expect(page.getByTestId('placement-state')).toHaveAttribute('data-ok', 'true');
  await page.getByTestId('placement-confirm').click();
}

/** Перезагрузить страницу и дождаться данных игрока (токены берутся из localStorage → тот же игрок). */
export async function reloadGame(page: Page): Promise<void> {
  await page.reload();
  await expect(page.getByTestId('hud-profile')).toBeVisible();
  await expect(wallet(page, 'cash')).toBeVisible();
}

const CONTENT = new URL('../../content/', import.meta.url);

/** Типы зданий стартовой раскладки из content/start_layout.yaml (если файла/ключей нет — null: сервер ещё строит базу сам). */
export function startLayoutTypes(): string[] | null {
  try {
    const text = readFileSync(new URL('start_layout.yaml', CONTENT), 'utf8');
    const types = [...text.matchAll(/^\s*-\s*\{[^}]*type:\s*([\w]+)/gm)].map((m) => m[1]!);
    return types.length > 0 ? types : null;
  } catch {
    return null;
  }
}

/** Одноразовый грант кошелька нового гостя из content/economy.yaml (start_grant: cash, fuel). */
export function startGrant(): { cash: number; fuel: number } {
  const text = readFileSync(new URL('economy.yaml', CONTENT), 'utf8');
  const start = text.search(/^start_grant:/m);
  const block = start < 0 ? '' : text.slice(start, text.indexOf('\n#', start) < 0 ? undefined : text.indexOf('\n#', start));
  const get = (k: string): number => Number(new RegExp('^\\s+' + k + ':\\s*(\\d+)', 'm').exec(block)?.[1]);
  return { cash: get('cash'), fuel: get('fuel') };
}
