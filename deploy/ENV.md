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

## Клиент

Клиент (`client/`) в R1 не читает переменных окружения.
