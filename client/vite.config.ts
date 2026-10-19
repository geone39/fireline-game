import { defineConfig } from 'vitest/config';
import preact from '@preact/preset-vite';

export default defineConfig({
  plugins: [preact()],
  // art/ и content/ лежат рядом с client/ в репозитории и читаются сборкой напрямую.
  server: { fs: { allow: ['..'] } },
  test: {
    environment: 'node',
    include: ['engine/**/*.test.ts', 'core/**/*.test.ts', 'platform/**/*.test.ts'],
  },
});
