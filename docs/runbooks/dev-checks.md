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

Цели `make wasm`, `make e2e`, `make e2e-down` делают то же. При падении e2e в CI артефакт `e2e-failure` содержит отчёт, trace, скриншоты и `compose.log`.
