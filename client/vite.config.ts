import { existsSync } from 'node:fs';
import { defineConfig } from 'vitest/config';
import preact from '@preact/preset-vite';

// API для dev: запросы /v1 проксируются на сервер (клиент ходит на тот же origin, CORS не нужен).
// В контейнере compose сервер доступен как server:8080, на хосте — localhost:8080; переопределение: FIRELINE_API_PROXY.
const apiTarget = process.env.FIRELINE_API_PROXY ?? (existsSync('/.dockerenv') ? 'http://server:8080' : 'http://localhost:8080');

export default defineConfig({
  plugins: [preact()],
  // art/ и content/ лежат рядом с client/ в репозитории и читаются сборкой напрямую.
  server: { fs: { allow: ['..'] }, proxy: { '/v1': { target: apiTarget, changeOrigin: true } } },
  test: {
    environment: 'node',
    include: ['engine/**/*.test.ts', 'core/**/*.test.ts', 'platform/**/*.test.ts', 'sim-wasm/**/*.test.ts', 'app/**/*.test.{ts,tsx}'],
  },
});
