# Git-процесс
- Trunk-based: короткие ветки от `main`, squash-merge, линейная история. Теги релизов `vX.Y.Z`, `content-vN` (версия данных идёт отдельно от кода).
- CODEOWNERS закрепляет модуль за ревьюером (см. `.github/CODEOWNERS`). Контентные PR (баланс) требуют ревью `economy` + `design`.
- Изменение формата данных → миграция + повышение `schema:` в YAML + запись в `docs/adr/`.
- Семантика сообщений: `feat(sim): add flamer cone targeting`, `data(content): rebalance u_sniper`.
- Релиз: tag → CI собирает `sim.wasm`, клиент, сервер-образ, `content.bundle.json` (хеш в манифесте) → staging → smoke → prod.
- Откат: прежний тег + прежняя версия `content.bundle` (сервер хранит последние 5; бои привязаны к своей версии данных).
