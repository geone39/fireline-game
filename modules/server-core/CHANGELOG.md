# CHANGELOG `server-core`

## Unreleased
- SERVER-3: `base.Mount` подключён под `/v1/base` (`GET /base`, `POST /base/commands`, `PUT /base/layout`) внутри авторизованной группы: Bearer, Idempotency-Key, rate limit, `X-Content-Version`; `server_time`/`content_version` и формат ошибок общие. `X-Content-Version` теперь ставится middleware на всех авторизованных ответах.
- migrations: `00003_defence_snapshots` (OQ-B7) — `defence_snapshots` по docs/04-data-model.md, `UNIQUE(player_id, base_version)`, иммутабельна (триггер на UPDATE). `SnapshotSink` реализован через `db.Store.InsertSnapshot` в транзакции `PUT /v1/base/layout`.
- auth: `Service.Provision` — хук в той же транзакции, что и upsert гостя; `cmd/api` подключает `BaseModule.Provision` = стартовая `base_state` + `Ledger.GrantStartTx` (OQ-E4). Ошибка откатывает создание игрока (атомарно); повторный логин идемпотентен и «долечивает» ранее созданных гостей.
- `GET /v1/me`: `builders` теперь из base (по слоту `{id:"bN", busy_until, target_id}`).
- `cmd/api` читает `building_costs.yaml` (`LoadPriceBook`): без файла (ECON-4) сервер не стартует. Интеграционные тесты используют фикстуру PriceBook и от файла не зависят.
- ASSUMPTION: стартовая раскладка — только HQ L1 по центру сетки (в content её нет, OQ-S6); `InitBase` не имеет tx-варианта, поэтому тот же INSERT выполняется в `Provision`.
- contract-change (отдельный коммит, на утверждение владельцем): формы ответов `/v1/base*` в `docs/03-api.md`.
- SERVER-2: `POST /v1/auth/refresh` (ротация: старый jti revoked атомарно, повтор отозванного/просроченного/неизвестного -> `unauthorized`); закрывает OQ-S1/S2 по реализации.
- gateway: карта статусов по docs/03-api.md (`invalid` всегда 400, в т.ч. неверный метод и слишком большое тело); `X-Content-Version` проверяется и на GET; `X-Content-Version` в заголовке каждого ответа.
- migrations: `00002_collector_pools` (OQ-E1) — аддитивная, `collector_pools(player_id, resource, amount, updated_at)`, PK(player_id, resource), CASCADE по игроку.
- `GET /v1/me`: поле `builders` (пока пустой список; данные придут из base, OQ-S3).
- наблюдаемость: `GET /metrics` (Prometheus): запросы/латентность по route-паттерну, in-flight, rate-limit, idempotent replays, refresh, пул БД, Go/process.
- ASSUMPTION: `/metrics` без auth на публичном порту (закрывать на прокси); форма элемента `builders` — `{id, busy_until, target_id}`; повтор отозванного refresh не отзывает остальные токены игрока.
- не сделано: logout/revoke, трейсы (OTel), SSE, OpenAPI.
- contract-change 0.2: в `docs/03-api.md` добавлены `unauthorized`/`not_found`, `X-Content-Version`, `POST /v1/auth/refresh`; реализация — SERVER-2.
- SERVER-1: каркас Go-сервера `server/` (go.mod, chi, pgx/v5, goose, zerolog), `cmd/api`.
- gateway: request-id, access log, recover, rate limit (IP + пользователь), Idempotency-Key для мутаций, проверка `X-Content-Version`, единый формат ошибок, `server_time` и `content_version` во всех ответах.
- auth: `POST /v1/auth/guest`, JWT HS256 (access 15 мин, refresh 30 д, refresh хранится в `refresh_tokens`).
- `GET /v1/me`, `GET /v1/content/manifest` (версия = хэш content/*.yaml|json), `/healthz`, `/readyz`.
- migrations: `00001_init` — players, wallets, ledger (append-only), base_state, base_events, timers + refresh_tokens, idempotency_keys.
- интеграционные тесты на testcontainers (PostgreSQL 16).
- не сделано: refresh-ручка, метрики/трейсы (Prometheus/OTel), SSE, OpenAPI — см. docs/open-questions.md (OQ-S1..S5).