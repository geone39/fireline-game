// Источники sim.wasm для Node (тесты, скрипты): файлы на диске вместо <script>/fetch.
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { join } from 'node:path';
import type { SimSources } from './loader';

export function nodeSources(dir: string): SimSources {
  return {
    async loadRuntime() {
      createRequire(import.meta.url)(join(dir, 'wasm_exec.js'));
    },
    async instantiate(imports) {
      return (await WebAssembly.instantiate(readFileSync(join(dir, 'sim.wasm')), imports)).instance;
    },
  };
}
