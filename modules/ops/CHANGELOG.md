# CHANGELOG `ops`

## Unreleased
- contract-change: корневой `Makefile` теперь в OWNS `ops` (OQ-S4, OQ-O1, OQ-E3); реализация — OPS-3.
- feat: `deploy/docker-compose.yml` (postgres 16 + server + client dev), `deploy/server.Dockerfile`, `deploy/client-dev.Dockerfile`, `deploy/ENV.md`
- feat(ci): jobs `server` (go vet, gofmt, `REQUIRE_DOCKER=1 go test ./...`) и `client` (npm ci, lint, typecheck, test, build) с кэшем; job `check` без изменений
- модуль заведён
