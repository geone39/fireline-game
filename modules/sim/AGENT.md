# AGENT.md — модуль `sim`
**Цель:** Детерминированное боевое ядро.

**OWNS (правишь только это):** `sim/**`
**READS (читаешь, не правишь):** `content/**`, `docs/design/battle-sim.md`
**Вход:** Snapshot, []Command, seed
**Выход:** Result (звёзды, %, StateHash, LootBasis)

## Definition of Done
- [ ] go test ./... зелёный
- [ ] property: Run дважды = одинаковый Result на 1000 случайных боёв
- [ ] golden-реплеи G01..G26 не изменились без bump версии движка
- [ ] нет float/time/rand/map-iter в логике (lint-скрипт)
- [ ] бенчмарк ≥5000 тиков/с

## Как работать
1. Прочитай `AGENTS.md`, этот файл, `CONTRACT.md` и релевантные docs из `docs/`.
2. Работай в ветке `feat/sim-<кратко>`; один PR = одна цель.
3. Если нужно изменить чужой модуль/контракт — оформи запрос (issue `[contract-change]`), не правь сам.
4. Перед PR: `make check` + тесты своего модуля; обнови `CHANGELOG.md` модуля.
