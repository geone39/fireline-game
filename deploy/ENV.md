# Переменные окружения

Секреты в репозиторий не коммитятся. Значения по умолчанию в `docker-compose.yml` — только для локальной разработки; для переопределения используйте переменные shell или файл `deploy/.env` (игнорируется через `deploy/.gitignore`).

## Сервер (`server/cmd/api`)

| Переменная | Обязат. | По умолчанию | Описание |
|---|---|---|---|
| `DATABASE_URL` | да | — | DSN PostgreSQL (pgx), напр. `postgres://user:pass@host:5432/db?sslmode=disable` |
| `JWT_SECRET` | да | — | Ключ подписи JWT, минимум 32 байта (`auth.MinSecretLen`) |
| `LISTEN_ADDR` | нет | `:8080` | Адрес HTTP-сервера |
| `MIGRATE_ON_START` | нет | `true` | Применять goose-миграции при старте (`true`/иное = выкл.) |
| `CONTENT_DIR` | нет | `../content` | Каталог с `*.yaml` и `schema/*.json`; в образе — `/content` |
| `CONTENT_BUNDLE_URL` | нет | пусто | URL бандла контента, отдаётся клиентам в манифесте |
| `RATE_IP_RPS` | нет | `20` | Лимит запросов/с на IP (burst = 2×) |
| `RATE_USER_RPS` | нет | `10` | Лимит запросов/с на пользователя (burst = 2×) |

## Тесты сервера

| Переменная | Описание |
|---|---|
| `REQUIRE_DOCKER` | `1` — тесты с testcontainers падают, если Docker недоступен (иначе пропускаются). В CI включено. |

## docker compose (`deploy/docker-compose.yml`)

| Переменная | По умолчанию | Описание |
|---|---|---|
| `POSTGRES_PASSWORD` | `fireline-dev` | Пароль БД (пользователь/БД: `fireline`) |
| `JWT_SECRET` | dev-значение | См. выше |
| `RATE_IP_RPS`, `RATE_USER_RPS` | `20`, `10` | См. выше |
| `POSTGRES_PORT` | `5432` | Порт PG на хосте |
| `SERVER_PORT` | `8080` | Порт сервера на хосте |
| `CLIENT_PORT` | `5173` | Порт Vite dev на хосте |
| `FIRELINE_API_PROXY` | `http://server:8080` | Передаётся клиенту, см. ниже |
| `VITE_API_URL`, `VITE_CONTENT_VERSION` | пусто | Передаются клиенту, см. ниже |

Контейнер `client` монтирует `../client` → `/app`, а также `../content` → `/content` и `../art` → `/art` (read-only): клиент импортирует `../../content/*` и `../../art/*` относительно `/app`. Healthcheck клиента — `wget http://127.0.0.1:5173/`; `server` — `/healthz`, `postgres` — `pg_isready`. Запуск: `docker compose -f deploy/docker-compose.yml up --build`, приложение на `http://localhost:${CLIENT_PORT}`.

### sim.wasm в compose (OPS-6)

Одноразовый сервис `sim-wasm` (`deploy/sim-wasm.Dockerfile`, golang:1.26-alpine) при каждом `compose up` собирает `sim/wasm` → `client/public/sim/{sim.wasm,wasm_exec.js}` (bind-mount, в git не попадает) и завершается; `client` стартует после его успеха (`service_completed_successfully`). Vite раздаёт `public/` как есть, перезапуск не нужен; после правки `sim/` достаточно `docker compose up -d` (пересборка ~3 с благодаря именованным томам `go_cache`/`go_mod`). Вручную `node client/sim-wasm/build.mjs` больше не нужен. Если Go-сборка падает, `client` не стартует — смотрите `docker compose logs sim-wasm`.

## Клиент (`client/`, Vite)

| Переменная | По умолчанию | Описание |
|---|---|---|
| `FIRELINE_API_PROXY` | `http://server:8080` в контейнере (`/.dockerenv`), иначе `http://localhost:8080` | Куда vite dev проксирует `/v1` (только dev-сервер, в бандл не попадает) |
| `VITE_API_URL` | пусто | База API; пусто = тот же origin (через прокси) |
| `VITE_CONTENT_VERSION` | пусто | Хеш контента для заголовка `X-Content-Version`; пусто = сервер пропускает проверку |

Секретов `VITE_*` быть не должно: они попадают в клиентский бандл.
