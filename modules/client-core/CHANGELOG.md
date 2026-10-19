# CHANGELOG `client-core`

## Unreleased
- feat: store (Zustand-стиль) с командной очередью: оптимистичное применение, откат при отказе сервера, повтор пакета с тем же Idempotency-Key при сетевой ошибке, отбрасывание команд старше 5 мин
- feat: API-клиент (Bearer, Idempotency-Key, X-Content-Version, single-flight refresh с ротацией, content_outdated, ретраи)
- feat: PlatformAdapter, browser.ts; vk.ts/telegram.ts — заглушки с TODO правил платформ
- модуль заведён
- OWNS: `client/core/base/**` отдан модулю `base` (устранено пересечение)
