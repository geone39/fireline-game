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

async function snapshot(api: Api) {
  const [base, me] = [await api.base(), await api.me()];
  return { version: base.version, layout: base.layout.items, timers: base.timers, builders: base.builders, wallets: me.wallets };
}

test('не хватает денег: понятный отказ, состояние не меняется', async ({ page }, info) => {
  test.setTimeout(180_000);
  await openGame(page);
  const api = await pageApi(page);
  await drainCash(api);
  await reloadGame(page);
  const project = await calibrate(page);

  const before = await snapshot(api);
  expect(before.wallets.cash).toBe(40);
  expect(await num(wallet(page, 'cash'))).toBe(40);

  await test.step('сервер: build d_fence за 60 при 40 → 422 insufficient_resources, без изменений (независимо от UI)', async () => {
    const r = await api.commands([{ op: 'build', args: { uid: 'neg-fence', type: 'd_fence', x: 30, y: 16, rot: 0 } }]);
    expect(r.status, JSON.stringify(r.body)).toBe(422);
    expect(r.body.error?.code).toBe('insufficient_resources');
    expect(await snapshot(api)).toEqual(before);
  });

  await test.step('UI: кнопка постройки либо заблокирована с подсказкой (UX #62), либо активна и отказ показывает сервер', async () => {
    const card = page.getByTestId('shop-d_fence');
    const button = card.locator('button');
    await expect(button).toBeVisible();
    // клиент мог ещё не получить кошелёк для предпроверки: дожидаемся, пока состояние кнопки установится (disabled или нет, но не меняется)
    const blocked = await card.getAttribute('data-blocked');
    if (blocked !== null || (await button.isDisabled())) {
      await expect(button).toBeDisabled();
      await expect(card).toHaveAttribute('data-blocked', 'no_funds');
      await expect(page.getByTestId('shop-d_fence-block')).toContainText(/(Нужно|Need)\s+60.*(есть|have)\s+40/);
      await shot(page, info, '07-insufficient-resources');
    } else {
      await placeBuilding(page, project, 'd_fence', 30, 16);
      const notice = page.getByTestId('notice');
      await expect(notice).toBeVisible();
      await expect(notice).toHaveText('Не хватает ресурсов');
      await shot(page, info, '07-insufficient-resources');
    }
  });

  // без частичных изменений: деньги, версия, раскладка, таймеры, строители — как до попытки
  await expect(wallet(page, 'cash')).toHaveText('40');
  expect(await snapshot(api)).toEqual(before);

  await reloadGame(page);
  await expect(wallet(page, 'cash')).toHaveText('40');
  expect((await snapshot(api)).layout).toEqual(before.layout);
});
