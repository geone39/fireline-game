// Известные баги: тесты помечены test.fail() — зелёные, пока баг жив; когда баг починят, тест станет «неожиданно
// прошедшим» и упадёт — тогда снимите test.fail() и перенесите проверку в основной сценарий.
import { expect, test } from '@playwright/test';
import { calibrate, openGame, placeBuilding } from '../helpers';

test('#60: после окончания стройки клиент освобождает строителя и даёт улучшить без перезагрузки', async ({ page }) => {
  test.fail(true, 'geone39/fireline#60: клиент не обновляет базу по окончании таймера');
  await openGame(page);
  const project = await calibrate(page);
  await placeBuilding(page, project, 'd_wire', 24, 14); // 1 с
  await expect(page.getByTestId('hud-timers')).toContainText('готово');
  await expect(page.getByTestId('hud-builders')).toContainText('2/2', { timeout: 10_000 });
  await expect(page.getByTestId('card-upgrade')).toBeEnabled({ timeout: 10_000 });
});
