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
| POST | /v1/army/cancel, /v1/army/speedup | `{order_id, count?}` — отмена с возвратом; `{building_id}` — ускорение очереди здания за gems |
| GET | /v1/lab | лаборатория: узлы, активное исследование |
| POST | /v1/lab/research | начать исследование `{node_id}` |
| POST | /v1/lab/cancel, /v1/lab/speedup | `{}` — отмена (возврат 50%) / ускорение за gems |
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

## Базa: `/v1/base*` (формы ответов, SERVER-3)
`BaseState` (ответ `GET /v1/base`, `POST /v1/base/commands`, `PUT /v1/base/layout`) + `server_time`, `content_version`:
`{version, layout:{items:[{uid,type,level,x,y,rot}]}, timers:[{id,kind,target_id,finish_at,to_level}], builders:{total,busy}, collectors:{<resource>:<accrued>}, replayed?, results?:[{op,uid,replayed}]}`; `PUT /v1/base/layout` добавляет `unchanged`.
`POST /v1/base/commands`: тело — массив `[{op,args}]` или `{commands:[...]}`; поля команды (`uid`, `type`, `x`, `y`, `rot`, `to`) допустимы и на верхнем уровне. Пакет атомарен; нужен `Idempotency-Key`.
`PUT /v1/base/layout`: `{layout:{items:[...]}}` или `{items:[...]}`; меняются только позиции/повороты существующих объектов; при изменении версия растёт и пишется иммутабельный defence snapshot.
Ошибки базы: `details.reason` (код правила), `details.uid?`, `details.index?` (номер команды); `invalid` 400, `not_found` 404 (нет базы), `conflict` 409 (uid занят, все билдеры заняты), `insufficient_resources` 422.
`GET /v1/me`: `builders:[{id,busy_until,target_id?}]` — по элементу на слот билдера.

## Армия и лаборатория: `/v1/army*`, `/v1/lab*` (формы ответов, SERVER-5)
`ArmyState` (ответ `GET /v1/army` и всех `POST /v1/army/*`) + `server_time`, `content_version`: `{stock:[{id,count,level}], spells:[{id,count,level}], deployed:[{raid_id,units:[Slot],spells:[Slot]}], pools:{<unit|garage|spell>:{capacity,used,free,base,perk_pct}}, queues:[{building_id,finish_at,orders:[{order_id,unit_id,count,remaining,unit_finish_at}]}], perks:{<key>:int}}`; `finish_at`/`unit_finish_at` — unix-секунды; готовые юниты попадают в `stock` лениво при любом чтении.
`POST /v1/army/train`: `{building_id, unit_id, count}`; `POST /v1/army/cancel`: `{order_id, count?}`; `POST /v1/army/speedup`: `{building_id}`. Нужен `Idempotency-Key`; повтор ключа отдаёт сохранённый ответ (`Idempotent-Replay: true`).
Туториал (`content/army.yaml tutorial.free_finish_first_order`): первый успешно поставленный в очередь заказ армии игрока завершается бесплатно один раз — ответ этого `POST /v1/army/train` уже содержит юнитов в `stock` и пустую очередь; `start_army` автоматически не выдаётся (`start_army_auto_grant: false`).
`LabState` (ответ `GET /v1/lab` и `POST /v1/lab/*`): `{lab_level (0: лаборатории нет), active:{node_id,started_at,finish_at,paid}|null, done:[node_id], nodes:[{id,available,reason?,cost,hours,to_level?}], perks}`.
Ошибки (`details.reason` — код правила): `invalid` 400 (`bad_request`, `wrong_building`, `not_built`, `not_multiple_of_group`, `bad_count`, `empty_queue`, `no_node`, `already_done`, `coming_soon`); `not_found` 404 (`no_building`, `no_order`, нет базы); `conflict` 409 (`housing_full` + `max_count`, `queue_full`, `locked` + `need`, `lab_busy`, `lab_level`, `prerequisite`, `no_research`); `insufficient_resources` 422 (`details` перечисляет нехватку).
`GET /v1/me` дополнительно: `army:{pools:{<pool>:{used,capacity}}}`, `lab:{level, active:{node_id,finish_at}|null}`.

## Примеры
`POST /v1/raids/start` → `{raid_id, expires_at, seed, content_version, engine_version, snapshot:{...}, rules:{max_turns, time_limit_s, deploy_zone:"outside_build_zone"}}` (`snapshot` — клиентский вид **без скрытых ловушек**, OQ-7.8; `time_limit_s` и `max_turns` берутся из `content/battle.yaml`; по `expires_at = now + time_limit_s` сервер обрывает бой)
`POST /v1/raids/{id}/finish` ← `{commands:[{turn:0,op:"deploy",id:"u_rifleman",level:1,to:{x:1,y:10}}, {turn:1,op:"move",unit:3,path:[{x:2,y:10},{x:3,y:10}]}, {turn:1,op:"attack",unit:3,target:1}, {turn:1,op:"end_turn"}], result_hash:"..."}` → `{stars:2, destruction:63, end_reason:"retreat", loot:{cash:1840,fuel:910}, trophies:+27, replay_id:"..."}`
Команды: `op` ∈ `deploy|load|start|move|attack|ability|cast|end_turn|retreat`; поля `turn, unit, target, id, level, to{x,y}, path[{x,y}]` (см. `modules/sim/CONTRACT.md` 0.2). Клетки — целые координаты сетки. Сервер пересчитывает бой по полному снимку; если у клиентского журнала есть отклонённые команды (`rejected`) или нет завершения — клиентский результат отвергается, засчитывается пересчёт (OQ-7.7). Истёкший `time_limit_s` засчитывается как `retreat` по сыгранным ходам.
