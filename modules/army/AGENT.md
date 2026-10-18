# AGENT.md — модуль `army`
**Цель:** Армия: обучение, лагеря, лаборатория.

**OWNS (правишь только это):** `server/svc/army`, `client/ui/army`, `client/ui/lab`
**READS (читаешь, не правишь):** `content/troops.yaml`, `content/research.yaml`
**Вход:** TrainCommand, ResearchCommand
**Выход:** ArmyState

## Definition of Done
- [ ] очереди по зданиям, лимит housing
- [ ] смена уровней через исследования
- [ ] тесты на переполнение лагеря

## Как работать
1. Прочитай `AGENTS.md`, этот файл, `CONTRACT.md` и релевантные docs из `docs/`.
2. Работай в ветке `feat/army-<кратко>`; один PR = одна цель.
3. Если нужно изменить чужой модуль/контракт — оформи запрос (issue `[contract-change]`), не правь сам.
4. Перед PR: `make check` + тесты своего модуля; обнови `CHANGELOG.md` модуля.
