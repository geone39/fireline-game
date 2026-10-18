# 01. Архитектура

## 1. Слои и компоненты
```
┌──────────────────────────── Browser (PWA) ────────────────────────────┐
│ client/ui (Preact)      screens: Base, Raid, Campaign, Shop, Lab, Social│
│ client/engine (PixiJS)  renderer: iso tilemap, sprites, FX, camera      │
│ client/core (TS)        state store, command queue, offline cache, sync  │
│ client/sim-wasm         sim.wasm (Go) – battle prediction/replay playback│
│ client/platform         PlatformAdapter: browser | vk | telegram         │
└──────────────┬──────────────────────────────────────────────────────────┘
               │ HTTPS JSON (REST) + SSE (notifications)   [WebSocket не нужен в R1]
┌──────────────▼──────────────── server (Go, один бинарь, модульный монолит) ┐
│ gateway: auth, rate-limit, idempotency, request-id, content-version check │
│ svc/profile   игрок, прогресс, сессии                                     │
│ svc/base      раскладка базы, стройка, очереди, сбор ресурсов (в «ленивом │
│               пересчёте» по времени)                                      │
│ svc/army      обучение, лагеря, исследования                              │
│ svc/raid      подбор цели, старт боя, приём реплея, проверка, награды     │
│ svc/campaign  этапы, энергия, первые награды                              │
│ svc/economy   кошельки (ledger), лимиты, лут, магазин                     │
│ svc/league    кубки, лиги, сезоны, щит                                    │
│ svc/notify    уведомления «вас атаковали»; журнал защит                   │
│ svc/shop      платежи платформ (позже), премиум                           │
│ jobs/         воркеры: проверка реплеев, сезоны, очистка щитов, аналитика │
│ sim (library) та же Go-библиотека, что в WASM                             │
└──────────────┬────────────────────────────────────────────────────────────┘
               │ pgx
        PostgreSQL 16   (+ Redis только для rate-limit/matchmaking-кэша, опц.)
        Object storage: реплеи (gz), content bundles, ассеты → CDN
```
**Модульный монолит:** сервисы — пакеты с интерфейсами; общаются только через интерфейсы пакета, не через чужие таблицы. Выделение в микросервисы возможно позже по границам (`svc/*`).

## 2. Ключевые принципы
1. **Command sourcing для базы:** клиент не присылает «состояние», а *команды* (`BuildStart`, `Upgrade`, `Move`, `TrainQueue`, `Collect`). Сервер валидирует по контенту и ресурсам, пишет в `base_events` и применяет к `base_state`. Идемпотентность: заголовок `Idempotency-Key`.
2. **Время — серверное.** Таймеры (стройки, обучение, производство) хранятся как `finish_at`. Состояние «сейчас» вычисляется лениво при запросе (`lazy settle`), без cron на каждого игрока.
3. **Бой = (снимок защиты, армия, seed, журнал команд).** Сервер выдаёт `raid_ticket` (снимок + seed + версия контента), клиент играет локально, присылает журнал; сервер перезапускает `sim.Run` и сравнивает `Result`. Расхождение → результат сервера, флаг подозрения.
4. **Защита ИИ-управляема сервером/sim** — защитника нет онлайн; его герой/гарнизон — часть снимка.
5. **Версионирование контента:** `content_version` в каждом тикете/бою. Старые реплеи проигрываются старым bundle.
6. **Платформы как плагины:** `PlatformAdapter` (идентификация, платёж, шаринг, хранение, события жизненного цикла). Сервер принимает `auth_provider` ∈ {guest, google?, vk, telegram}; в R1 — `guest` + email/OAuth опционально.

## 3. Поток рейда (sequence)
```
Client                         Server(raid)                      Sim
  │ POST /raids/search            │ pick target (see §4)           │
  │──────────────────────────────▶│ lock target 90s (soft), charge search cost
  │◀── target preview (fog: no traps visible; radar optional)
  │ POST /raids/start {target, army, spells, hero}
  │──────────────────────────────▶│ validate army available; reserve units (deduct)
  │◀── ticket {raid_id, snapshot, seed, content_version, expires}
  │  play locally (sim.wasm), commands logged
  │ POST /raids/{id}/finish {commands[], clientResult}
  │──────────────────────────────▶│ enqueue verify job  ────────▶ sim.Run(snapshot,commands,seed)
  │                               │ compare, compute stars/loot/trophies (economy+league)
  │◀── result {stars, loot, trophies, replay_id}  (sync wait ≤2s, else poll)
  │                               │ write defence log for victim, set shield, notify (SSE/next login)
```
Таймаут тикета: если `finish` не пришёл → армия считается потерянной, 0 звёзд (anti-quit-abuse).

## 4. Подбор цели (matchmaking)
`candidates = bases WHERE trophies BETWEEN t±window AND shield_until < now AND NOT recently_attacked_by_me AND daily_raids_received < limit AND not self AND not owner_active_in_last_5min`. Скоринг: близость HQ-уровня, «lootable stock» (>0), случайный дроп. Кэш индекса (Redis sorted-set по trophies) или таблица+индекс PG на R1 (до ~50k MAU достаточно). Поиск стоит `search_cost_cash`. Боты-«наполнители» (из `campaign` шаблонов) подставляются, если кандидатов <3.

## 5. Хранение баз и защиты
Раскладка (`base_layout`): список `{id, level, x, y, rot}`. При изменении раскладки создаётся **defence snapshot** (immutable, версия). Атакуют всегда последнюю сохранённую версию. Изменения раскладки заблокированы на время активного рейда против вас (если рейд идёт — применятся после).
Редактор базы обязан валидировать: границы сетки, пересечения, обязательные проходы (нет «полной замкнутости» без разрушаемых стен), лимиты по HQ, `requires_water`.

## 6. Anti-abuse
Щит после рейда (8/12/16 ч по потерянным звёздам); лимит 3 рейда на одну базу/сутки; кулдаун пары 24 ч; защита 20% запасов; max потеря 35%/сутки; запрет рейда по «брошенным» (>30 дней) кроме как с лутом ×0.1; ограничение смены раскладки в «горячий» момент; детектор: >N побед против одного слабого игрока; верификация реплеев 100%.

## 7. Клиент
- **Рендер:** изометрическая сетка 44×44 тайла; PixiJS v8, спрайт-атласы (TexturePacker-формат), culling, пул частиц. 60 fps цель на среднем смартфоне 2021 г.
- **Состояние:** единый store (Zustand-стиль) + «командная очередь» с оптимистичным применением и откатом при ошибке сервера.
- **Оффлайн:** просмотр базы из кэша, стройки в очереди помечаются «ожидает синхронизации» (не более 5 мин).
- **Ассеты:** манифест с хешами, ленивая подгрузка по главам; first-load < 3 МБ, полный < 40 МБ; CDN кэш `immutable`.
- **Платформы:** `PlatformAdapter` { `id()`, `signedAuth()`, `purchase(sku)`, `share(payload)`, `storage`, `lifecycle` , `adsPolicy` }. Реализации: `browser.ts` (R1), `vk.ts`, `telegram.ts` (P2, правила сторов — в `docs/platform-notes.md`).
- **i18n:** все строки `name_key`; ru/en с первого дня.
- **Доступность:** режим для дальтоников, масштаб UI, замедление боя ×0.5 (только просмотр реплея).

## 8. Сервер: технологии и нефункционалка
Go 1.24, `net/http` + chi, pgx/v5, goose миграции, sqlc для запросов, zerolog, OpenTelemetry, Prometheus, Sentry. Конфиг через env. Docker, Helm/compose. Целевые SLO: p95 API < 200 мс; верификация боя < 1.5 с; доступность 99.5%; RPO 5 мин (WAL), RTO 1 ч. Нагрузка R1: 20k DAU, ~5 рейдов/DAU/день → ~1.2 рейда/с пик ×5; один 4-vCPU инстанс + PG.
Безопасность: HTTPS, JWT (access 15 мин, refresh 30 д, ротация), подписи платформ, rate-limit per IP+user, валидация размеров, параметризованные запросы, секреты в менеджере, GDPR-удаление аккаунта.

## 9. Окружения и релизы
`dev` (docker compose: pg + server + client dev) → `staging` (автодеплой main) → `prod` (тег). Фичефлаги в БД (`feature_flags`). Миграции вперёд-совместимые (expand/contract).

## 10. Наблюдаемость и аналитика
События (`analytics_events`): `session_start`, `build_start`, `raid_start/finish`, `campaign_finish`, `purchase`, `tutorial_step`. Дашборды: D1/D7/D30, ARPDAU, воронка тутор., win-rate по составам, **экономический баланс** (приход/расход по ресурсам, медиана времени на уровень HQ — сравнение с `docs/econ_report.json`).

## 11. Тестовая стратегия
- sim: unit + property (детерминизм: 1000 случайных боёв дважды = одинаковый хеш), golden-реплеи (`sim/testdata/replays/*.json`), фазз команд.
- balance: `simbattle` (боты-атаки по эталонным базам, win-rate-матрица; цель — ни один состав не > 65% и не < 30% против эталонов своего HQ).
- server: интеграция с testcontainers PG; контрактные тесты по OpenAPI.
- client: vitest, Playwright e2e (стройка, рейд, покупка), визуальные скриншот-тесты ключевых экранов.
- нагрузочные: k6 на `/raids/*`.
