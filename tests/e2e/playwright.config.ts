import { defineConfig, devices } from '@playwright/test';

// Против поднятого compose (deploy/docker-compose.yml): клиент :5173 (vite проксирует /v1 на сервер).
// Переопределение: E2E_BASE_URL, E2E_API_URL. Тесты ходят в API только для проверок и подготовки.
const baseURL = process.env.E2E_BASE_URL ?? 'http://localhost:5173';

export default defineConfig({
  testDir: './specs',
  outputDir: './test-results',
  timeout: 90_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: [['list'], ['html', { open: 'never', outputFolder: './playwright-report' }]],
  use: {
    baseURL,
    viewport: { width: 1280, height: 800 },
    locale: 'ru-RU',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'], viewport: { width: 1280, height: 800 } } }],
});
