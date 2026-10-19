# CHANGELOG `ops`

## Unreleased
- feat(ci): job `e2e` (compose up --wait, Playwright с кэшем браузеров, артефакты и логи compose при падении) и job `wasm` (`make wasm`: сборка sim.wasm, паритет G01..G20 через node, размер в лог и summary, артефакт) (OPS-5; закрывает запросы qa и OQ-SIM10(2))
- feat(ci): `concurrency` — устаревшие запуски PR/веток отменяются, на main нет
- feat(make): цели `wasm`, `e2e`, `e2e-down` (не входят в `check`)
- feat(compose): client монтирует `../content:/content:ro` и `../art:/art:ro`, healthcheck клиента, `FIRELINE_API_PROXY`/`VITE_API_URL`/`VITE_CONTENT_VERSION` передаются в контейнер; `ENV.md` описывает VITE_* (OPS-4, закрывает OQ-A4)
- feat(make): цели `server` (vet, gofmt, test), `client` (npm ci/lint/typecheck/test/build) в `check`; `econ` теперь с `--check` (OPS-3, закрывает OQ-O1/OQ-S4/OQ-E3)
- refactor(ci): jobs `server`/`client` вызывают `make server`/`make client`; job `check` ставит Go (по `server/go.mod`) и Node, `REQUIRE_DOCKER=1`
- docs: `docs/runbooks/dev-checks.md` — проверки для Windows без make
- contract-change: корневой `Makefile` теперь в OWNS `ops` (OQ-S4, OQ-O1, OQ-E3); реализация — OPS-3.
- feat: `deploy/docker-compose.yml` (postgres 16 + server + client dev), `deploy/server.Dockerfile`, `deploy/client-dev.Dockerfile`, `deploy/ENV.md`
- feat(ci): jobs `server` (go vet, gofmt, `REQUIRE_DOCKER=1 go test ./...`) и `client` (npm ci, lint, typecheck, test, build) с кэшем; job `check` без изменений
- модуль заведён
