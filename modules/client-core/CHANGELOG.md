# CHANGELOG `client-core`

## Unreleased
- fix(app): #60 — клиент синхронизирует базу по таймерам: один отложенный таймер на ближайший `finish_at` (с поправкой на `server_time`) → `GET /v1/base` + `/v1/me`; перевзвод после каждого подтверждённого состояния и смены фазы, отмена в `dispose()`. Строитель освобождается, уровень и «Улучшить» обновляются без перезагрузки; сохранение раскладки больше не шлёт устаревший `level`. Тесты: `client/app/timer-sync.test.ts` (истёк таймер, смещение часов ±1 ч, возврат вкладки, dispose). Запрос к qa — OQ-A7
- feat(app): CLIENT-3b — редактор базы проверен end-to-end и закреплён интеграционными тестами (`client/app/editor.test.ts`: createApp + мок сервера + мок Renderer 0.2 + компоненты ui/base): постройка, улучшение, drag-drop → PUT /v1/base/layout, undo/redo, переключатель просмотр/правка
- feat(app): отказы сервера (insufficient_resources, conflict, not_found, invalid, …) — понятный текст ru/en (`reason.*` в `client/app/i18n.ts`, новые строки — OQ-A1), показ 6 с и автоскрытие
- fix(core): `store` сбрасывает `lastRejection` при новой команде (раньше «Не хватает ресурсов» висело до перезагрузки)
- fix(app): панель редактора ниже 28vh, магазин — горизонтальная лента: раньше панель закрывала базу, и здание под ней нельзя было выбрать тапом
- feat(app): CLIENT-3 — `client/app`: bootstrap и шелл (Preact), вход гостя (`/v1/auth/guest`, device_id и токены в `PlatformAdapter.storage`, refresh, повторный вход при потере сессии), загрузка `/v1/me` и `/v1/base`, обработка `content_outdated`/ошибок сети; экран базы = PixiRenderer + HUD + слот редактора `client/ui/base` (подключён по OQ-B10)
- feat(app): HUD — кошелёк cash/fuel/alloy/gems из `/v1/me` (обновляется после команд базы), свободные билдеры, накопленное в сборщиках, таймеры построек; ru/en, названия через name_key (новые строки — OQ-A1)
- fix(core): `ApiClient.refresh` распаковывает ответ сервера `{player_id, tokens}` (раньше сохранялась вся обёртка)
- chore(build): vite proxy `/v1` → сервер (`FIRELINE_API_PROXY`, в контейнере `server:8080`), `index.html` → `/app/main.tsx`, `tsconfig`/vitest include `app`, `@types/node` (корневые файлы client-engine — минимальная правка, см. PR)
- contract-change: OWNS += `client/app/**` (bootstrap, шелл, HUD ресурсов, вход гостя; решение владельца 2026-10-19); экраны `client/ui/<module>` остаются у своих модулей, `client/app` их только подключает
- feat: store (Zustand-стиль) с командной очередью: оптимистичное применение, откат при отказе сервера, повтор пакета с тем же Idempotency-Key при сетевой ошибке, отбрасывание команд старше 5 мин
- feat: API-клиент (Bearer, Idempotency-Key, X-Content-Version, single-flight refresh с ротацией, content_outdated, ретраи)
- feat: PlatformAdapter, browser.ts; vk.ts/telegram.ts — заглушки с TODO правил платформ
- модуль заведён
- OWNS: `client/core/base/**` отдан модулю `base` (устранено пересечение)
