# Открытые вопросы

- **OQ-1** Модель боя: дискретный realtime (рекомендуется, D-03) или ходовой планировщик (ADR-003). Блокирует старт sim-этапа M2. Владелец: product-owner.
- **OQ-2** Выбор сеттинга/названия/стиля (docs/variants/DECISIONS.md). Не блокирует инженерию.
- **OQ-3** Монетизация R1: оставить только внутриигровой тест-магазин? Нужны ли реклама/награды за видео?
- **OQ-4** Аутентификация в браузере R1: только гость + привязка e-mail/Google? (нужно для переноса прогресса)
- **OQ-5** Юр.: политика данных, возрастной рейтинг, проверка товарных знаков выбранного названия.
- **OQ-6** Хостинг: свой VPS/облако? Влияет на ops (docs/01 §9).

- **OQ-S1** (server-core, contract-change запрос к docs/03-api.md) Коды ошибок: добавить `unauthorized` (HTTP 401) и `not_found` (404), сейчас в списке их нет. ASSUMPTION: сервер уже отдаёт именно их. Также: HTTP 409 для `content_outdated`, 429 для `rate_limited`; заголовок клиента `X-Content-Version` для проверки версии контента.
- **OQ-S2** (server-core) Нет эндпоинта `POST /v1/auth/refresh` (ротация refresh-токена, docs/01 §8) и logout/revoke. Токены и таблица `refresh_tokens` готовы; нужен контракт на ручку. ASSUMPTION: `{refresh_token}` -> новая пара, старый jti помечается revoked.
- **OQ-S3** (server-core) `GET /v1/me` пока отдаёт profile + wallets; `builders`, `shield`, `league` (детали) ждут модулей base/economy. Стартовые кошельки гостя не выдаются (нужны значения из content/economy.yaml, решает economy). `players.hq_level DEFAULT 1` — ASSUMPTION.
- **OQ-S4** (server-core -> orchestrator) Корневой `Makefile` не в OWNS server-core: нужна цель `server` (`cd server && go vet ./... && go test ./...`) в `check`, и Docker в CI для testcontainers (`REQUIRE_DOCKER=1`, иначе тесты БД пропускаются).
- **OQ-S5** (server-core) Guest `device_id` — клиентский секрет (16-64 символа `[A-Za-z0-9_-]`); при отсутствии сервер генерирует и возвращает. Утеря = потеря гостевого аккаунта до `auth/link`.