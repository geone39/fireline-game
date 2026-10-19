import { defineConfig } from 'vitest/config';
import preact from '@preact/preset-vite';

export default defineConfig({
  plugins: [preact()],
  test: {
    environment: 'node',
    include: ['engine/**/*.test.ts', 'core/**/*.test.ts', 'platform/**/*.test.ts'],
  },
});
