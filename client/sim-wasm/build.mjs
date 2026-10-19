// Сборка WASM-ядра для клиента: `go build` из sim/wasm -> client/public/sim/{sim.wasm,wasm_exec.js} (не коммитятся),
// плюс актуальная копия типов sim/wasm/api.d.ts -> client/sim-wasm/api.d.ts (коммитится).
// Запуск: `npm run sim:build` (из client/). `--check` — только сверить копию api.d.ts, ничего не собирая.
/* global process, console */
import { spawnSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const wasmDir = resolve(here, '../../sim/wasm');
const outDir = resolve(here, '../public/sim');
const typesSrc = join(wasmDir, 'api.d.ts');
const typesDst = join(here, 'api.d.ts');

function findGo() {
  if (process.env.GO) return process.env.GO;
  const probe = spawnSync('go', ['version'], { encoding: 'utf8' });
  if (probe.status === 0) return 'go';
  for (const p of ['C:/Program Files/Go/bin/go.exe', '/usr/local/go/bin/go']) if (existsSync(p)) return p;
  throw new Error('go не найден: поставьте Go или задайте переменную GO');
}

if (process.argv.includes('--check')) {
  if (readFileSync(typesSrc, 'utf8') !== readFileSync(typesDst, 'utf8')) {
    console.error('client/sim-wasm/api.d.ts отстаёт от sim/wasm/api.d.ts: запустите npm run sim:build');
    process.exit(1);
  }
  console.log('api.d.ts актуален');
  process.exit(0);
}

const go = findGo();
mkdirSync(outDir, { recursive: true });
const build = spawnSync(go, ['build', '-trimpath', '-ldflags=-s -w', '-o', join(outDir, 'sim.wasm'), '.'], {
  cwd: wasmDir,
  stdio: 'inherit',
  env: { ...process.env, GOOS: 'js', GOARCH: 'wasm' },
});
if (build.status !== 0) process.exit(build.status ?? 1);

const goroot = spawnSync(go, ['env', 'GOROOT'], { encoding: 'utf8' }).stdout.trim();
const runtime = [join(goroot, 'lib', 'wasm', 'wasm_exec.js'), join(goroot, 'misc', 'wasm', 'wasm_exec.js')].find(existsSync);
if (!runtime) throw new Error(`wasm_exec.js не найден в ${goroot}`);
copyFileSync(runtime, join(outDir, 'wasm_exec.js'));
copyFileSync(typesSrc, typesDst);
console.log(`sim.wasm: ${statSync(join(outDir, 'sim.wasm')).size} байт -> ${outDir}`);
