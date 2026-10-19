// QA-1, негативный: нехватка ресурсов даёт понятный отказ без частичных изменений.
import { expect, test } from '@playwright/test';
import { calibrate, num, openGame, pageApi, placeBuilding, reloadGame, shot, wallet, type Api } from '../helpers';

/** Потратить деньги через API на дешёвые (1 с) постройки, пока не останется < цены забора (60). 20 проволок (800) + 11 заборов (660). */
async function drainCash(api: Api): Promise<void> {
  const plan: { type: string; x: number; y: number }[] = [];
  for (let i = 0; i < 20; i++) plan.push({ type: 'd_wire', x: 5 + i, y: 5 });
  for (let i = 0; i < 11; i++) plan.push({ type: 'd_fence', x: 5 + i, y: 7 });
  for (let i = 0; i < plan.length; i += 2) {
    const batch = plan.slice(i, i + 2).map((p) => ({ op: 'build', args: { uid: `drain-${i}-${p.x}`, type: p.type, x: p.x, y: p.y, rot: 0 } }));
    await expect.poll(async () => (await api.base()).builders.busy, { timeout: 15_000 }).toBe(0);
    const r = await api.commands(batch);
    expect(r.status, JSON.stringify(r.body)).toBe(200);
  }
  await expect.poll(async () => (await api.base()).builders.busy, { timeout: 15_000 }).toBe(0);
}

test('не хватает денег: понятный отказ, состояние не меняется', async ({ page }, info) => {
  test.setTimeout(180_000);
  await openGame(page);
  const api = await pageApi(page);
  await drainCash(api);
  await reloadGame(page);
  const project = await calibrate(page);

  const before = await api.base();
  const cashBefore = (await api.me()).wallets.cash!;
  expect(cashBefore).toBe(40);
  expect(await num(wallet(page, 'cash'))).toBe(40);

  await placeBuilding(page, project, 'd_fence', 30, 16);
  const notice = page.getByTestId('notice');
  await expect(notice).toBeVisible();
  await expect(notice).toHaveText('Не хватает ресурсов');
  await shot(page, info, '07-insufficient-resources');

  // без частичных изменений: деньги, версия, раскладка, таймеры, строители — как до попытки
  await expect(wallet(page, 'cash')).toHaveText('40');
  const after = await api.base();
  expect(after.version).toBe(before.version);
  expect(after.layout.items).toEqual(before.layout.items);
  expect(after.timers).toEqual(before.timers);
  expect(after.builders).toEqual(before.builders);
  expect((await api.me()).wallets).toMatchObject({ cash: 40, fuel: 600 });

  await reloadGame(page);
  await expect(wallet(page, 'cash')).toHaveText('40');
  expect((await api.base()).layout.items).toEqual(before.layout.items);
});
