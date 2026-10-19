# AGENT.md — модуль `client-engine`
**Цель:** Рендер и анимации (PixiJS).

**OWNS (правишь только это):** `client/engine/**`, корневые файлы сборки `client/` (`package.json`, `vite.config.*`, `tsconfig*.json`, `index.html`, lock-файл)
**READS (читаешь, не правишь):** `art/manifest.json`
**Вход:** World state из core/sim
**Выход:** Canvas, события ввода

## Definition of Done
- [ ] 60 fps на референсном устройстве (Moto G-class 2021)
- [ ] draw calls <150
- [ ] атласы с manifest-хешами
- [ ] интерфейс Renderer мокается в тестах

## Как работать
1. Прочитай `AGENTS.md`, этот файл, `CONTRACT.md` и релевантные docs из `docs/`.
2. Работай в ветке `feat/client-engine-<кратко>`; один PR = одна цель.
3. Если нужно изменить чужой модуль/контракт — оформи запрос (issue `[contract-change]`), не правь сам.
4. Перед PR: `make check` + тесты своего модуля; обнови `CHANGELOG.md` модуля.
