# AGENT.md — модуль `server-core`
**Цель:** Gateway, auth, БД, миграции, SSE, наблюдаемость.

**OWNS (правишь только это):** `server/gateway`, `server/auth`, `server/db`, `server/migrations`, `server/cmd`
**READS (читаешь, не правишь):** `docs/03-api.md`, `docs/04-data-model.md`
**Вход:** HTTP
**Выход:** OpenAPI, миграции

## Definition of Done
- [ ] goose миграции forward-compatible
- [ ] интеграционные тесты на testcontainers
- [ ] rate limit, idempotency, request-id
- [ ] метрики/трейсы

## Как работать
1. Прочитай `AGENTS.md`, этот файл, `CONTRACT.md` и релевантные docs из `docs/`.
2. Работай в ветке `feat/server-core-<кратко>`; один PR = одна цель.
3. Если нужно изменить чужой модуль/контракт — оформи запрос (issue `[contract-change]`), не правь сам.
4. Перед PR: `make check` + тесты своего модуля; обнови `CHANGELOG.md` модуля.
