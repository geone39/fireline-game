# tests/e2e — сквозные тесты (Playwright)

Модуль `qa`. Тесты ходят в настоящий стек (postgres + server + client) через браузер Chromium.

| Спека | Что проверяет |
|---|---|
| `specs/base.spec.ts` | новый гость → стартовая база, 1500/600 → стройка (списание, таймер) → улучшение → перестановка и сохранение → перезагрузка: тот же игрок, состояние сохранилось |
| `specs/insufficient.spec.ts` | нехватка денег: «Не хватает ресурсов», без частичных изменений (кошелёк, версия базы, раскладка, таймеры) |
| `specs/regressions.spec.ts` | регрессии исправленных багов. #60: после окончания стройки/улучшения клиент сам обновляет базу (строитель свободен, «Улучшить» доступна, сохранение раскладки без `layout_mismatch`) — без перезагрузки |

Новый известный баг → тест с `test.fail(true, 'issue')` в `regressions.spec.ts`; после фикса снять `test.fail` (иначе тест «неожиданно пройдёт» и упадёт).

Скриншоты ключевых экранов: `screenshots/*.png` (перезаписываются каждым прогоном; копия — в `test-results/`).

## Запуск локально (Windows / PowerShell)

Нужны Docker и Node 22+ (в `PATH`: `C:\Program Files\nodejs`).

```powershell
# 1. стек (из корня репозитория)
docker compose -f deploy/docker-compose.yml up -d --build --wait

# 2. тесты
cd tests\e2e
npm ci
npx playwright install chromium     # один раз
npx playwright test                  # или: npm test

# 3. отчёт и остановка
npx playwright show-report
cd ..\..; docker compose -f deploy/docker-compose.yml down
```

Каждый тест создаёт нового гостя (чистый контекст браузера = новый `device_id`), поэтому старые данные в БД не мешают
и сбрасывать том не обязательно.

Переменные: `E2E_BASE_URL` (по умолчанию `http://localhost:5173`, клиент; vite проксирует `/v1`), `E2E_API_URL`
(по умолчанию = `E2E_BASE_URL`; для прямого обращения к серверу — `http://localhost:8080`). Если `CLIENT_PORT`/`SERVER_PORT`
в compose изменены — задайте соответствующий `E2E_BASE_URL`.

Полезно: `npx playwright test --headed`, `--debug`, `npx playwright show-trace test-results/<папка>/trace.zip`.

## Запуск в CI (план, `.github/**` принадлежит ops)

Шаги job `e2e` (запрос к ops — `REQUESTS.md`):

```yaml
- run: docker compose -f deploy/docker-compose.yml up -d --build --wait
- uses: actions/setup-node@v4
  with: {node-version: 22, cache: npm, cache-dependency-path: tests/e2e/package-lock.json}
- run: npm ci && npx playwright install --with-deps chromium
  working-directory: tests/e2e
- run: npx playwright test
  working-directory: tests/e2e
- if: always()
  uses: actions/upload-artifact@v4
  with: {name: e2e-report, path: "tests/e2e/playwright-report\ntests/e2e/test-results\ntests/e2e/screenshots"}
- if: always()
  run: docker compose -f deploy/docker-compose.yml logs && docker compose -f deploy/docker-compose.yml down -v
```

## Как устроено

- **Время.** Реальных ожиданий мало: проволока строится 1 с, улучшение 2 с (`content/building_costs.yaml`); долгие
  стройки (пулемётное гнездо — 10 мин) проверяются по наличию таймера в HUD и в `GET /v1/base`, без ожидания завершения.
  Тестового хука ускорения времени нет (запрос к ops/server-core — `REQUESTS.md`).
- **Канвас.** Здания ставятся и двигаются мышью по канвасу. Проекция «тайл → страница» калибруется по полигону подсветки
  призрака (`helpers.ts: calibrate`), поэтому тесты не зависят от размера окна/камеры. Допущение: призрак без наведения
  стоит в клетке `floor(grid.w / 2)` = 22 (`content/base_grid.yaml`).
- **API.** `helpers.ts: Api` — только для проверок состояния и подготовки (потратить деньги), не для самих сценариев.
  Токен берётся из `localStorage` страницы.
- **Ожидания.** Фиксированных пауз нет: ждём состояние через auto-retry `expect(...)` по HUD и `expect.poll` по `GET /v1/base`. Клиент сам обновляет базу по `finish_at` (#60 исправлен), поэтому основной сценарий идёт без `reloadGame()`; перезагрузка остаётся только в финальной проверке сохранности.
