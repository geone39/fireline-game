# AGENT.md — модуль `design`
**Цель:** Требования и пакеты вариантов; ничего не кодирует.

**OWNS (правишь только это):** `docs/design/**`, `docs/variants/**`, `docs/open-questions.md`
**READS (читаешь, не правишь):** всё
**Вход:** Решения владельца продукта (docs/variants/DECISIONS.md)
**Выход:** User stories с acceptance, ТЗ на арт/звук, тексты (ru/en) в content/strings/

## Definition of Done
- [ ] у каждой фичи есть acceptance-критерии и пример экрана
- [ ] нет противоречий с content/*.yaml (проверка вручную + review economy)
- [ ] вопросы вынесены в open-questions.md

## Как работать
1. Прочитай `AGENTS.md`, этот файл, `CONTRACT.md` и релевантные docs из `docs/`.
2. Работай в ветке `feat/design-<кратко>`; один PR = одна цель.
3. Если нужно изменить чужой модуль/контракт — оформи запрос (issue `[contract-change]`), не правь сам.
4. Перед PR: `make check` + тесты своего модуля; обнови `CHANGELOG.md` модуля.
