# AGENT.md — модуль `raid`
**Цель:** PvP-рейды: подбор, тикет, приём реплея, верификация, лут, кубки, щит.

**OWNS (правишь только это):** `server/svc/raid`, `server/svc/league`, `server/jobs/verify`, `client/ui/raid`
**READS (читаешь, не правишь):** `sim`, `content/economy.yaml`
**Вход:** RaidStart/Finish
**Выход:** Result, DefenceLog, Notification

## Definition of Done
- [ ] подбор не возвращает щитованных/в кулдауне
- [ ] верификация 100% реплеев, расхождение логируется
- [ ] анти-абьюз правила из economy.yaml покрыты тестами
- [ ] p95 finish <2 с

## Как работать
1. Прочитай `AGENTS.md`, этот файл, `CONTRACT.md` и релевантные docs из `docs/`.
2. Работай в ветке `feat/raid-<кратко>`; один PR = одна цель.
3. Если нужно изменить чужой модуль/контракт — оформи запрос (issue `[contract-change]`), не правь сам.
4. Перед PR: `make check` + тесты своего модуля; обнови `CHANGELOG.md` модуля.
