# AGENT.md — модуль `content`
**Цель:** Каталог данных и схемы; сборка bundle.

**OWNS (правишь только это):** `content/**`, `tools/validate_content.py`, `tools/contentc (план)`
**READS (читаешь, не правишь):** `docs/design/**`
**Вход:** Требования design, числа от economy/balance
**Выход:** content/*.yaml, schema/*.json, bundle manifest {version, sha256, files}

## Definition of Done
- [ ] python3 tools/validate_content.py = 0 ошибок
- [ ] у каждого id есть name_key и запись в strings/ru,en
- [ ] смена schema: сопровождается миграцией

## Как работать
1. Прочитай `AGENTS.md`, этот файл, `CONTRACT.md` и релевантные docs из `docs/`.
2. Работай в ветке `feat/content-<кратко>`; один PR = одна цель.
3. Если нужно изменить чужой модуль/контракт — оформи запрос (issue `[contract-change]`), не правь сам.
4. Перед PR: `make check` + тесты своего модуля; обнови `CHANGELOG.md` модуля.
