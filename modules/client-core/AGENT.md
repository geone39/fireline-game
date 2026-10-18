# AGENT.md — модуль `client-core`
**Цель:** Стейт, команда→сервер, PlatformAdapter, оффлайн, WASM-интеграция.

**OWNS (правишь только это):** `client/core/**`, `client/platform/**`, `client/sim-wasm/**`
**READS (читаешь, не правишь):** `docs/03-api.md`
**Вход:** API
**Выход:** Store, адаптеры

## Definition of Done
- [ ] команды оптимистичны с откатом
- [ ] адаптер browser.ts реализован; vk/telegram — заглушки с TODO правил
- [ ] юнит-тесты стора

## Как работать
1. Прочитай `AGENTS.md`, этот файл, `CONTRACT.md` и релевантные docs из `docs/`.
2. Работай в ветке `feat/client-core-<кратко>`; один PR = одна цель.
3. Если нужно изменить чужой модуль/контракт — оформи запрос (issue `[contract-change]`), не правь сам.
4. Перед PR: `make check` + тесты своего модуля; обнови `CHANGELOG.md` модуля.
