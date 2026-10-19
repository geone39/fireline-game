// Загрузчик sim.wasm (Go wasm_exec.js). Артефакты собирает `npm run sim:build` в client/public/sim/.
// Источники (скрипт рантайма и сам wasm) подменяемы: в браузере — <script> + fetch, в тестах — файлы на диске.
import type { FirelineSim } from './api';

export interface SimSources {
  /** Выполнить wasm_exec.js: после него в globalThis есть класс `Go`. */
  loadRuntime(): Promise<void>;
  /** Скомпилировать и инстанцировать sim.wasm с importObject рантайма. */
  instantiate(imports: WebAssembly.Imports): Promise<WebAssembly.Instance>;
}

interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

/** Таймаут ожидания `fireline_sim_ready`, мс (защита от зависшего рантайма). */
export const LOAD_TIMEOUT_MS = 30_000;

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const s = document.createElement('script');
    s.src = src;
    s.onload = () => resolve();
    s.onerror = () => reject(new Error(`не удалось загрузить ${src}`));
    document.head.appendChild(s);
  });
}

/** Браузерные источники: `<base>sim/wasm_exec.js` и `<base>sim/sim.wasm` (base = import.meta.env.BASE_URL). */
export function browserSources(base = '/'): SimSources {
  const prefix = base.endsWith('/') ? base : `${base}/`;
  return {
    loadRuntime: () => loadScript(`${prefix}sim/wasm_exec.js`),
    async instantiate(imports) {
      const res = await fetch(`${prefix}sim/sim.wasm`);
      if (!res.ok) throw new Error(`sim.wasm: HTTP ${res.status}`);
      if (typeof WebAssembly.instantiateStreaming === 'function' && res.headers.get('content-type')?.includes('application/wasm')) {
        return (await WebAssembly.instantiateStreaming(res, imports)).instance;
      }
      return (await WebAssembly.instantiate(await res.arrayBuffer(), imports)).instance;
    },
  };
}

let cached: Promise<FirelineSim> | undefined;

/**
 * Загрузить ядро и вернуть глобальный `fireline_sim`. Повторные вызовы возвращают тот же промис (wasm грузится один раз);
 * после ошибки следующая попытка начнётся заново.
 */
export function loadFirelineSim(src: SimSources = browserSources()): Promise<FirelineSim> {
  if (globalThis.fireline_sim) return Promise.resolve(globalThis.fireline_sim);
  cached ??= start(src).catch((e: unknown) => {
    cached = undefined;
    throw e;
  });
  return cached;
}

/** Только для тестов: забыть загруженное ядро. */
export function resetSimLoader(): void {
  cached = undefined;
  globalThis.fireline_sim = undefined;
  globalThis.fireline_sim_ready = undefined;
}

async function start(src: SimSources): Promise<FirelineSim> {
  await src.loadRuntime();
  const Go = (globalThis as unknown as { Go?: new () => GoRuntime }).Go;
  if (!Go) throw new Error('wasm_exec.js не определил Go');
  const go = new Go();
  const ready = new Promise<FirelineSim>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('sim.wasm не ответил вовремя')), LOAD_TIMEOUT_MS);
    globalThis.fireline_sim_ready = () => {
      clearTimeout(timer);
      const api = globalThis.fireline_sim;
      if (api) resolve(api);
      else reject(new Error('sim.wasm не определил fireline_sim'));
    };
  });
  const instance = await src.instantiate(go.importObject);
  // run() завершится только при выходе Go-программы; ядро живёт, пока открыта страница.
  go.run(instance).catch((e: unknown) => {
    console.error('sim.wasm завершился с ошибкой', e);
  });
  return ready;
}
