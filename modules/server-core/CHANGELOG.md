# CHANGELOG `server-core`

## Unreleased
- SERVER-1: каркас Go-сервера `server/` (go.mod, chi, pgx/v5, goose, zerolog), `cmd/api`.
- gateway: request-id, access log, recover, rate limit (IP + пользователь), Idempotency-Key для мутаций, проверка `X-Content-Version`, единый формат ошибок, `server_time` и `content_version` во всех ответах.
- auth: `POST /v1/auth/guest`, JWT HS256 (access 15 мин, refresh 30 д, refresh хранится в `refresh_tokens`).
- `GET /v1/me`, `GET /v1/content/manifest` (версия = хэш content/*.yaml|json), `/healthz`, `/readyz`.
- migrations: `00001_init` — players, wallets, ledger (append-only), base_state, base_events, timers + refresh_tokens, idempotency_keys.
- интеграционные тесты на testcontainers (PostgreSQL 16).
- не сделано: refresh-ручка, метрики/трейсы (Prometheus/OTel), SSE, OpenAPI — см. docs/open-questions.md (OQ-S1..S5).