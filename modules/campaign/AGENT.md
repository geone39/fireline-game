# AGENT.md — модуль `campaign`
**Цель:** Сюжетные этапы против ботов.

**OWNS (правишь только это):** `content/campaign/**`, `server/svc/campaign`, `client/ui/campaign`
**READS (читаешь, не правишь):** `sim`, `content/campaign.yaml`
**Вход:** Раскладки ботов
**Выход:** Прогресс, награды

## Definition of Done
- [ ] каждая раскладка проходит layout_lint (штаб достижим)
- [ ] энергия и награды как в economy.yaml
- [ ] первичные награды выдаются один раз

## Как работать
1. Прочитай `AGENTS.md`, этот файл, `CONTRACT.md` и релевантные docs из `docs/`.
2. Работай в ветке `feat/campaign-<кратко>`; один PR = одна цель.
3. Если нужно изменить чужой модуль/контракт — оформи запрос (issue `[contract-change]`), не правь сам.
4. Перед PR: `make check` + тесты своего модуля; обнови `CHANGELOG.md` модуля.
