# CI: расход минут GitHub Actions и что делать при блокировке (OPS-8, #136)

## Как устроен CI
Файл `.github/workflows/ci.yml`. Триггеры: `pull_request`, `push` только в `main`, `workflow_dispatch` (ручной, гоняет всё). Push в ветку с открытым PR отдельно не запускается. Новый пуш в PR отменяет устаревший запуск (`concurrency`).

Job `changes` (≈10 с, биллинг 1 мин) через `dorny/paths-filter` определяет затронутые области; остальные jobs пропускаются (статус *skipped*, это не провал). На push в main и workflow_dispatch всё `true`.

| job | когда запускается на PR | ≈ мин |
|---|---|---|
| `changes` | всегда | 1 |
| `check` | всегда; docs-only (docs/**, backlog/**, `*.md`, `.gitattributes`) → только `make content`, иначе `make check` (включает server и client) | 1 / 3 |
| `e2e` | client/**, server/**, content/**, deploy/**, tests/**, sim/**, .github/e2e/**, ci.yml, Makefile | 4–5 |
| `wasm` | sim/**, client/sim-wasm/**, deploy/**, ci.yml, Makefile | 1 |
| `simbattle` | tools/simbattle/**, sim/**, content/**, server/svc/raid/**, ci.yml, Makefile | 1 |

Отдельные jobs `server` и `client` удалены: `make check` уже содержит обе цели (были дублем). У всех jobs `timeout-minutes: 15`.
`simbattle-nightly` — раз в неделю (пн 03:17 UTC) и по запросу.

Минуты округляются вверх на каждый job, поэтому лишние мелкие jobs тоже стоят.

## Как проверить остаток минут
- Владелец аккаунта: GitHub → Settings → Billing and plans → Plans and usage (Actions: использовано/включено, лимит трат).
- CLI (нужен scope `user`): `gh api /users/<login>/settings/billing/actions` (поля `total_minutes_used`, `included_minutes`).
- Минуты по запускам: `gh run view <id> --json jobs` (startedAt/completedAt).
Бесплатный тариф: 2000 мин/мес для приватных репозиториев (ubuntu x1); публичные репозитории бесплатны.

## Что делать при блокировке
Симптом: «The job was not started because recent account payments have failed or your spending limit needs to be increased».
1. Решение за владельцем: поднять Spending limit / исправить способ оплаты (Billing), либо сделать репозиторий публичным, либо ждать сброса месячного лимита (1-е число).
2. До восстановления: прогонять проверки локально (`docs/runbooks/dev-checks.md`, `make check`, при изменениях sim — `make wasm`; e2e — `make e2e`) и указывать результат в PR.
3. Не плодить пуши в PR и ручных запусков; тяжёлые прогоны (`simbattle-matrix`, e2e) — локально.
4. После восстановления пере-запустить: `gh run rerun <id>` или `gh workflow run ci`.
