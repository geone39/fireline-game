# AGENT.md — модуль `base`
**Цель:** База: раскладка, стройка, очереди, сбор, редактор и обороной симулятор-превью.

**OWNS (правишь только это):** `server/svc/base`, `client/ui/base`, `client/core/base`
**READS (читаешь, не правишь):** `content/buildings.yaml`, `content/defences.yaml`, `content/economy.yaml`
**Вход:** BaseCommand из клиента
**Выход:** BaseState, defence snapshot

## Definition of Done
- [ ] валидация размещения 100% серверная
- [ ] ленивый пересчёт таймеров покрыт тестами (время вперёд/назад)
- [ ] команды идемпотентны
- [ ] UI: drag-drop, подсветка радиусов, undo до сохранения

## Как работать
1. Прочитай `AGENTS.md`, этот файл, `CONTRACT.md` и релевантные docs из `docs/`.
2. Работай в ветке `feat/base-<кратко>`; один PR = одна цель.
3. Если нужно изменить чужой модуль/контракт — оформи запрос (issue `[contract-change]`), не правь сам.
4. Перед PR: `make check` + тесты своего модуля; обнови `CHANGELOG.md` модуля.
