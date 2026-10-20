# Локальные проверки без `make` (Windows / PowerShell)

`make check` = `content` + `econ` + `sim` + `server` + `client` (см. корневой `Makefile`; CI вызывает те же цели). Если `make` нет, выполняйте команды напрямую из корня репозитория.

```powershell
# content
python tools/validate_content.py

# econ
python tools/econ_sim.py --check | Out-Null

# sim
Push-Location sim; go test ./...; Pop-Location

# server (REQUIRE_DOCKER=1 — как в CI; нужен запущенный Docker, иначе тесты БД пропускаются)
Push-Location server
go vet ./...
$out = gofmt -l .; if ($out) { "gofmt needed:"; $out; throw "gofmt" }
$env:REQUIRE_DOCKER = "1"; go test ./...
Remove-Item Env:REQUIRE_DOCKER
Pop-Location

# client
Push-Location client
npm ci; npm run lint; npm run typecheck; npm test; npm run build
Pop-Location
```

Заметки:
- Нужны Python 3 с `pyyaml`, `jsonschema` (`pip install pyyaml jsonschema`), Go (версия из `server/go.mod`), Node 22.
- В PowerShell 5.1 нет `&&`; после каждой команды проверяйте `$LASTEXITCODE` или запускайте блоки по отдельности.
- Команды должны совпадать с целями `Makefile`; при изменении цели обновляйте этот файл.

## WASM и e2e (отдельные CI-jobs, не входят в `make check`)

```powershell
# wasm: сборка и паритет G01..G20 (нужны Go и Node)
powershell -File sim\wasm\build.ps1
Push-Location sim; go test -count=1 -v -run TestWasmParity ./wasm/wire/; Pop-Location   # не должно быть SKIP

# e2e: Docker + Node 22
docker compose -f deploy/docker-compose.yml up -d --build --wait
Push-Location tests\e2e; npm ci; npx playwright install chromium; npx playwright test; Pop-Location
docker compose -f deploy/docker-compose.yml down -v
```

## simbattle (боты-атаки, QA-2; отдельный CI-job, не входит в `make check`)

```powershell
# нужен Go в PATH (C:\Program Files\Go\bin); отдельный go.mod в tools/simbattle
go -C tools/simbattle vet ./...
$out = gofmt -l tools/simbattle; if ($out) { "gofmt needed:"; $out; throw "gofmt" }
go -C tools/simbattle test -short ./...      # ≈25 с — то же, что `make simbattle` и job `simbattle` на PR
go -C tools/simbattle test ./...             # полный прогон тестов, ≈50 с (`make simbattle-full`)
go -C tools/simbattle run . -seeds 6 -levels research   # матрица win-rate → tools/simbattle/out/ (`make simbattle-matrix`)
```

- Job `simbattle` в `ci.yml` гоняет `make simbattle` (vet, gofmt, `test -short`) на каждый push/PR.
- Nightly `.github/workflows/simbattle-nightly.yml` (cron 03:17 UTC и ручной запуск: Actions → simbattle-nightly → Run workflow, входы `seeds`, `levels`) делает полную матрицу, кладёт `report.md`/`report.json` в артефакт `simbattle-report` (30 дней) и печатает таблицу в job summary. Ничего не коммитит и не падает из-за win-rate (он информативный до M3); падает только при ошибке самого прогона.

Цели `make wasm`, `make e2e`, `make e2e-down` делают то же. При падении e2e в CI артефакт `e2e-failure` содержит отчёт, trace, скриншоты и `compose.log`.

## Экран боя в docker compose (OPS-6)

`docker compose -f deploy/docker-compose.yml up --build` сам собирает `sim.wasm` и `wasm_exec.js` (сервис `sim-wasm`, вывод в `client/public/sim/`), ручной `node client/sim-wasm/build.mjs` не нужен. Экран боя: `http://localhost:5173/?screen=battle`. После правок `sim/` — `docker compose -f deploy/docker-compose.yml up -d` (пересборка ~3 с, кэш Go в томах). Нет экрана боя/клиент не стартует — `docker compose logs sim-wasm`. Подробнее: `deploy/ENV.md`.
