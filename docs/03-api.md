# 03. API v1 (REST JSON, `Authorization: Bearer`, `Idempotency-Key` для мутаций)
Ошибки: `{error:{code,message,details}}`; коды: `invalid`, `unauthorized`, `not_found`, `insufficient_resources`, `conflict`, `shielded`, `cooldown`, `content_outdated`, `rate_limited`, `internal`.
HTTP-статусы: `invalid` 400, `unauthorized` 401, `not_found` 404, `conflict` и `content_outdated` 409, `rate_limited` 429, `internal` 500; прочие бизнес-коды (`insufficient_resources`, `shielded`, `cooldown`) — 409 или 422 по решению модуля, но стабильно для кода.
Все ответы содержат `server_time` и `content_version`; клиент передаёт свою версию контента заголовком `X-Content-Version`; при несовпадении → `content_outdated` + URL bundle.

| Метод | Путь | Назначение |
|---|---|---|
| POST | /v1/auth/guest | создать/войти гостем → tokens |
| POST | /v1/auth/refresh | `{refresh_token}` → новая пара tokens; старый refresh (jti) помечается revoked (ротация; повторное использование отозванного → `unauthorized`) |
| POST | /v1/auth/link | привязать провайдера (vk/telegram/google) |
| GET | /v1/me | профиль, кошельки, билдеры, щит, лига |
| GET | /v1/base | текущая раскладка + таймеры (ленивый пересчёт) |
| POST | /v1/base/commands | пакет команд базы `[{op,args}]` (build, upgrade, move, remove, collect, speedup, clear_obstacle) |
| PUT | /v1/base/layout | сохранить раскладку (валидация) → новый defence snapshot |
| GET | /v1/army | армия, лагеря, очереди обучения |
| POST | /v1/army/train | поставить в очередь `{building_id, unit_id, count}` |
| POST | /v1/lab/research | начать исследование |
| POST | /v1/raids/search | найти цель (списывает стоимость поиска) |
| POST | /v1/raids/start | `{target_id, army[], spells[], hero}` → `RaidTicket` |
| POST | /v1/raids/{id}/finish | `{commands[], client_result_hash}` → результат |
| GET | /v1/raids/{id}/replay | журнал + snapshot для просмотра |
| GET | /v1/defence-log | кто и как атаковал вас, потери, реплеи |
| POST | /v1/campaign/{stage}/start / finish | аналогично raids, цель — бот-раскладка |
| GET | /v1/leagues/me, /v1/leaderboard | кубки, топ |
| POST | /v1/shop/purchase | покупка (premium-currency внутри игры) |
| GET | /v1/events/stream | SSE: `defended`, `building_done`, `training_done` |
| GET | /v1/content/manifest | текущая версия bundle + хеши |

## Примеры
`POST /v1/raids/start` → `{raid_id, expires_at, seed, content_version, snapshot:{...}, rules:{max_ticks:1800, deploy_zone:"outside_buildings+1"}}`
`POST /v1/raids/{id}/finish` ← `{commands:[{t:12,op:"deploy",id:"u_rifleman",x:3200,y:5100}], result_hash:"..."}` → `{stars:2, loot:{cash:1840,fuel:910}, trophies:+27, replay_id:"..."}`
