// QA-1: сквозной сценарий базы M1. Окружение: docker compose (postgres + server + client), см. README.
import { expect, test } from '@playwright/test';
import { calibrate, num, openGame, pageApi, placeBuilding, reloadGame, shot, wallet } from '../helpers';

test('новый гость: стартовая база → стройка → улучшение → перестановка → перезагрузка', async ({ page }, info) => {
    await openGame(page);
  const api = await pageApi(page);
  let project = await calibrate(page);
  const me0 = await api.me();
  const playerId = me0.profile.id;

  await test.step('стартовая база и ресурсы 1500/600', async () => {
    expect(await num(wallet(page, 'cash'))).toBe(1500);
    expect(await num(wallet(page, 'fuel'))).toBe(600);
    await expect(page.getByTestId('hud-builders')).toContainText('2/2');
    expect((await api.base()).layout.items.map((i) => i.type)).toEqual(['b_hq']);
    await shot(page, info, '01-start-base');
  });

  let nestUid = '';
  await test.step('построить пулемётное гнездо: ресурсы списаны, идёт таймер стройки', async () => {
    await placeBuilding(page, project, 'd_mg_nest', 26, 16);
    await expect(wallet(page, 'cash')).toHaveText('900');
    await expect(page.locator('[data-testid^=timer-]')).toHaveCount(1);
    await expect(page.getByTestId('hud-builders')).toContainText('1/2');
    const base = await api.base();
    const nest = base.layout.items.find((i) => i.type === 'd_mg_nest')!;
    expect(nest).toMatchObject({ x: 26, y: 16, level: 1 });
    nestUid = nest.uid;
    expect(base.timers).toEqual([expect.objectContaining({ kind: 'build', target_id: nestUid })]);
    expect((await api.me()).wallets.cash).toBe(900);
    await shot(page, info, '02-building-timer');
  });

  let wireUid = '';
  await test.step('построить проволоку (1 с), дождаться готовности и улучшить', async () => {
    await placeBuilding(page, project, 'd_wire', 24, 14);
    await expect(wallet(page, 'cash')).toHaveText('860');
    wireUid = (await api.base()).layout.items.find((i) => i.type === 'd_wire')!.uid;
    // стройка проволоки — 1 с; сервер пересчитывает таймеры лениво, HUD обновляется по запросам/тикам клиента
    await expect.poll(async () => (await api.base()).timers.some((t) => t.target_id === wireUid), { timeout: 15_000 }).toBe(false);
    // BUG geone39/fireline#60 (specs/known-bugs.spec.ts): без перезагрузки завершённая стройка остаётся «Уже строится».
    // Обходим перезагрузкой, чтобы проверить остальной сценарий.
    await reloadGame(page);
    project = await calibrate(page);
    const at = project(24, 14);
    await page.mouse.click(at.x, at.y);
    await expect(page.getByTestId('card')).toBeVisible();
    await expect(page.getByTestId('card-level')).toContainText('1/');
    await shot(page, info, '03-card');
    await page.getByTestId('card-upgrade').click();
    await expect(page.getByTestId('card-timer')).toContainText('Улучшается');
    const base = await api.base();
    expect(base.timers).toEqual(expect.arrayContaining([expect.objectContaining({ kind: 'upgrade', target_id: wireUid, to_level: 2 })]));
    expect((await api.me()).wallets.cash).toBeLessThan(860);
    await shot(page, info, '04-upgrade');
  });

  await test.step('перестановка и сохранение раскладки', async () => {
    // улучшение проволоки (2 с) завершится на сервере; клиент о нём не узнает без перезагрузки (#60) — иначе PUT layout
    // уйдёт с устаревшим level и получит layout_mismatch
    await expect.poll(async () => (await api.base()).timers.some((t) => t.target_id === wireUid), { timeout: 15_000 }).toBe(false);
    await reloadGame(page);
    project = await calibrate(page);
    await page.getByTestId('toggle-edit').click();
    const from = project(26, 16);
    const to = project(30, 16);
    // перетаскивание работает только у выбранного здания в режиме редактирования: сначала тап, затем драг
    await page.mouse.click(from.x, from.y);
    await expect(page.getByTestId('card')).toBeVisible();
    await page.mouse.move(from.x, from.y);
    await page.mouse.down();
    await page.mouse.move((from.x + to.x) / 2, (from.y + to.y) / 2, { steps: 5 });
    await page.mouse.move(to.x, to.y, { steps: 5 });
    await page.mouse.up();
    await expect(page.getByTestId('save')).toBeEnabled();
    await shot(page, info, '05-edit-dragged');
    const v0 = (await api.base()).version;
    await page.getByTestId('save').click();
    await expect(page.getByTestId('save')).toBeDisabled();
    const base = await api.base();
    expect(base.version).toBeGreaterThan(v0);
    expect(base.layout.items.find((i) => i.uid === nestUid)).toMatchObject({ x: 30, y: 16 });
    await page.getByTestId('toggle-edit').click();
  });

  await test.step('перезагрузка: тот же игрок, раскладка и ресурсы сохранились', async () => {
    const before = await api.base();
    const cash = (await api.me()).wallets.cash!;
    await reloadGame(page);
    const api2 = await pageApi(page);
    const me = await api2.me();
    expect(me.profile.id).toBe(playerId);
    expect(me.wallets.cash).toBe(cash);
    const after = await api2.base();
    expect(after.layout.items).toEqual(before.layout.items);
    expect(after.timers.map((t) => t.target_id).sort()).toEqual(before.timers.map((t) => t.target_id).sort());
    await expect(wallet(page, 'cash')).toHaveText(cash.toLocaleString('en-US'));
    await expect(page.locator(`[data-testid=timer-${nestUid}]`)).toBeVisible();
    await shot(page, info, '06-after-reload');
  });
});
