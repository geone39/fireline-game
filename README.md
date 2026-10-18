# Fireline (рабочее кодовое имя)

Браузерная стратегия в духе «Линия огня» + Clash of Clans: строишь базу, обучаешь армию, нападаешь на чужие базы (асинхронно) и на ботов в сюжете. Название, сеттинг и графика — **свои** (см. `docs/variants/`, ждут вашего выбора).

## С чего начать
| Вам нужно | Читать |
|---|---|
| Понять продукт и принятые решения | `docs/00-vision-and-decisions.md` |
| Понять устройство системы | `docs/01-architecture.md`, `docs/02-modules.md` |
| Выбрать название/сеттинг/графику | `docs/variants/README.md` |
| Взять задачу как агент/разработчик | `AGENTS.md`, затем `modules/<ваш модуль>/AGENT.md` |
| Баланс и экономика | `docs/design/economy.md`, `python3 tools/econ_sim.py` |
| Данные игры | `content/*.yaml` (`python3 tools/validate_content.py`) |

## Быстрый старт
```
make check        # валидация контента + экономика + go test
python3 tools/econ_sim.py
```
Структура: `content/` данные · `sim/` детерминированное боевое ядро (Go→WASM) · `server/` API · `client/` браузерный клиент · `modules/` брифы и контракты по доменам · `docs/` проектная документация · `tools/` инструменты.
