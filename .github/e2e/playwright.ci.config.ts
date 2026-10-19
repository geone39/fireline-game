// CI-обёртка над tests/e2e/playwright.config.ts (владелец ops): флаги Chromium для раннера без GPU.
// На 2-vCPU раннере WebGL-канвас (pixi) рендерится программно; без этих флагов page.screenshot зависает.
import path from 'node:path';
import { defineConfig } from '../../tests/e2e/node_modules/@playwright/test';
import base from '../../tests/e2e/playwright.config';

export default defineConfig({
  ...base,
  testDir: path.resolve(__dirname, '../../tests/e2e/specs'),
  outputDir: path.resolve(__dirname, '../../tests/e2e/test-results'),
  reporter: [['list'], ['html', { open: 'never', outputFolder: path.resolve(__dirname, '../../tests/e2e/playwright-report') }]],
  use: {
    ...base.use,
    channel: 'chromium',
    launchOptions: { args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader', '--ignore-gpu-blocklist'] },
  },
});
