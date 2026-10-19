# 02. Модули и границы (для параллельной работы агентов)

Правило зависимостей: стрелка = «может импортировать/читать контракт». Обратных стрелок нет.
```
content ──▶ sim ──▶ server/svc/raid
   │         └────▶ client/sim-wasm
   ├──────▶ economy-tools (tools/econ_sim.py)
   └──────▶ server/svc/*  ,  client/*
design (docs/) — источник требований для всех, кода не содержит
```
| Модуль | Владеет (OWNS) | Публичный контракт | Не трогает |
|---|---|---|---|
| `design` | `docs/design/**`, `docs/variants/**` | требования (user stories + acceptance) | код |
| `content` | `content/**` | JSON-схемы `content/schema/**`, bundle manifest | sim/server код |
| `economy` | `content/economy.yaml`, `tools/econ_sim.py`, `server/svc/economy` | `economy.yaml` схема + интерфейс `Ledger` | боевую логику |
| `sim` | `sim/**` | `sim.Run(Snapshot,[]Command,seed) Result` + golden replays | сеть, БД |
| `base` | `server/svc/base`, `client/ui/base`, редактор | `BaseCommand` список, `BaseState` | sim |
| `army` | `server/svc/army`, `client/ui/army` | `TrainCommand`, `ArmyState` | sim |
| `raid` | `server/svc/raid`, `client/ui/raid` | API `/raids/*`, `RaidTicket` | экономику (зовёт `Ledger`) |
| `campaign` | `content/campaign/**`, `server/svc/campaign`, UI | формат раскладки бота | sim |
| `server-core` | `server/gateway`, `server/db`, миграции | OpenAPI `docs/03-api.md` | игровая логика |
| `client-engine` | `client/engine/**` | `Renderer` интерфейс, ассет-манифест | сервер |
| `client-core` | `client/core/**`, `client/platform/**`, `client/app/**` (шелл, HUD, bootstrap) | `PlatformAdapter`, store | рендер |
| `art` | `art/**` (брифы, атласы, шрифты) | манифест ассетов, спеки размеров | код |
| `audio` | `audio/**` | манифест звуков | код |
| `qa` | `tests/e2e/**`, `tools/simbattle` | отчёты | продакшн-код |
| `ops` | `deploy/**`, `.github/**` | пайплайны, dashboards | игровую логику |

## Как поставить задачу агенту
Шаблон issue: `[module] цель · контекст (файлы) · контракт (что нельзя ломать) · acceptance · как проверить`. Пример: `[sim] flamer cone targeting · sim/ · Result не меняет схему · golden G07 · go test ./... && make check`.

## Контракты-границы (где они лежат)
- Данные → `content/schema/*.json` (jsonschema 2020-12) + версия `schema:` в каждом YAML.
- API → `docs/03-api.md` (затем генерируется `openapi.yaml`, источник правды).
- Sim → `sim/types.go` (Go) и `client/sim-wasm/api.d.ts` (генерируется).
- Платформа → `client/platform/PlatformAdapter.ts`.
- Экономика → `content/economy.yaml` + `Ledger` интерфейс: `Apply(ctx, playerID, []Delta, reason, idemKey) error` (все изменения ресурсов только через ledger, запись неизменяема).
