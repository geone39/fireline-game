# CHANGELOG `client-core`

## Unreleased
- contract-change: OWNS += `client/app/**` (bootstrap, шелл, HUD ресурсов, вход гостя; решение владельца 2026-10-19); экраны `client/ui/<module>` остаются у своих модулей, `client/app` их только подключает
- feat: store (Zustand-стиль) с командной очередью: оптимистичное применение, откат при отказе сервера, повтор пакета с тем же Idempotency-Key при сетевой ошибке, отбрасывание команд старше 5 мин
- feat: API-клиент (Bearer, Idempotency-Key, X-Content-Version, single-flight refresh с ротацией, content_outdated, ретраи)
- feat: PlatformAdapter, browser.ts; vk.ts/telegram.ts — заглушки с TODO правил платформ
- модуль заведён
- OWNS: `client/core/base/**` отдан модулю `base` (устранено пересечение)
