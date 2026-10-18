# AGENT.md — модуль `economy`
**Цель:** Темп прогресса, цены, лут, лимиты анти-абьюза, леджер.

**OWNS (правишь только это):** `content/economy.yaml`, `tools/econ_sim.py`, `docs/design/economy.md`, `server/svc/economy`
**READS (читаешь, не правишь):** `content/*`
**Вход:** Цели темпа (design), метрики (ops)
**Выход:** Таблицы цен/времени, econ_report.json, Ledger

## Definition of Done
- [ ] econ_sim: казуал ≈ цели ±20%, активный ≤ 1.35× быстрее
- [ ] каждый PR баланса прикладывает вывод econ_sim
- [ ] Ledger идемпотентен (idem_key) и append-only

## Как работать
1. Прочитай `AGENTS.md`, этот файл, `CONTRACT.md` и релевантные docs из `docs/`.
2. Работай в ветке `feat/economy-<кратко>`; один PR = одна цель.
3. Если нужно изменить чужой модуль/контракт — оформи запрос (issue `[contract-change]`), не правь сам.
4. Перед PR: `make check` + тесты своего модуля; обнови `CHANGELOG.md` модуля.
