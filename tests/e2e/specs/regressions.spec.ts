// Регрессионные тесты исправленных багов. Ожидания — по состоянию API/HUD (expect.poll / auto-retry), без пауз.
import { expect, test } from '@playwright/test';
import { calibrate, openGame, pageApi, placeBuilding } from '../helpers';

// geone39/fireline#60 (исправлен в #63): клиент не обновлял базу по окончании таймера стройки/улучшения.
test('#60: после окончания стройки и улучшения клиент сам обновляет базу (без перезагрузки)', async ({ page }) => {
  await openGame(page);
  const api = await pageApi(page);
  const project = await calibrate(page);
  const wireAt = project(24, 14);

  await test.step('стройка проволоки (1 с): строитель освобождается, «Улучшить» доступна', async () => {
    await placeBuilding(page, project, 'd_wire', 24, 14);
    const wire = (await api.base()).layout.items.find((i) => i.type === 'd_wire')!;
    await expect(page.locator(`[data-testid=timer-${wire.uid}]`)).toHaveCount(0, { timeout: 15_000 }); // таймер ушёл из HUD сам
    await expect(page.getByTestId('hud-builders')).toContainText('2/2', { timeout: 15_000 });
    expect((await api.base()).timers).toEqual([]);
    if (!(await page.getByTestId('card').isVisible())) await page.mouse.click(wireAt.x, wireAt.y);
    await expect(page.getByTestId('card-upgrade')).toBeEnabled({ timeout: 15_000 });
  });

  await test.step('улучшение (2 с): по окончании строитель свободен, уровень 2 в карточке', async () => {
    await page.getByTestId('card-upgrade').click();
    // промежуточный таймер (2 с) не проверяем — он может закончиться раньше опроса; ждём итог
    await expect
      .poll(async () => (await api.base()).layout.items.find((i) => i.type === 'd_wire')?.level, { timeout: 15_000 })
      .toBe(2);
    await expect.poll(async () => (await api.base()).timers.length, { timeout: 15_000 }).toBe(0);
    await expect(page.getByTestId('hud-builders')).toContainText('2/2', { timeout: 15_000 });
    await expect(page.getByTestId('card-level')).toContainText('2/', { timeout: 15_000 });
  });

  await test.step('сохранение раскладки сразу после улучшения: без layout_mismatch', async () => {
    await page.getByTestId('toggle-edit').click();
    const from = project(24, 14);
    const to = project(28, 14);
    await page.mouse.click(from.x, from.y);
    await expect(page.getByTestId('card')).toBeVisible();
    await page.mouse.move(from.x, from.y);
    await page.mouse.down();
    await page.mouse.move((from.x + to.x) / 2, (from.y + to.y) / 2, { steps: 5 });
    await page.mouse.move(to.x, to.y, { steps: 5 });
    await page.mouse.up();
    await expect(page.getByTestId('save')).toBeEnabled();
    const v0 = (await api.base()).version;
    await page.getByTestId('save').click();
    await expect(page.getByTestId('save')).toBeDisabled();
    await expect.poll(async () => (await api.base()).version).toBeGreaterThan(v0);
    const wire = (await api.base()).layout.items.find((i) => i.type === 'd_wire')!;
    expect(wire).toMatchObject({ x: 28, y: 14, level: 2 });
  });
});
