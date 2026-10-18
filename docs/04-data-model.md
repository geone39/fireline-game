# 04. Модель данных (PostgreSQL 16)
```sql
players(id uuid pk, created_at, auth_provider text, ext_id text, nickname text, hq_level int, trophies int, league text, shield_until timestamptz, last_seen timestamptz, UNIQUE(auth_provider,ext_id));
wallets(player_id fk, resource text, amount bigint, PRIMARY KEY(player_id,resource));
ledger(id bigserial pk, player_id fk, resource text, delta bigint, reason text, ref text, idem_key text UNIQUE, created_at);   -- append-only, wallets = SUM(ledger) (проверка по ночной сверке)
base_state(player_id pk, version int, layout jsonb, updated_at);                       -- текущая раскладка
base_events(id bigserial, player_id, op text, args jsonb, applied_at);                 -- командный журнал
timers(id uuid pk, player_id, kind text, target_id text, finish_at timestamptz, payload jsonb);  -- стройка, обучение, исследование
army(player_id, unit_id text, level int, count int, PRIMARY KEY(player_id,unit_id,level));
defence_snapshots(id uuid pk, player_id, base_version int, hq_level int, layout jsonb, garrison jsonb, created_at);  -- иммутабельны
raids(id uuid pk, attacker_id, defender_id nullable, bot_base_id nullable, snapshot_id fk, seed bigint, content_version text, state text, started_at, expires_at, finished_at, commands jsonb, result jsonb, verified bool, suspicious bool);
loot_ledger(raid_id, resource, taken bigint);
campaign_progress(player_id, stage_id text, stars int, first_clear_claimed bool, PRIMARY KEY(player_id,stage_id));
energy(player_id pk, value int, updated_at);
research(player_id, node_id text, state text, finish_at, PRIMARY KEY(player_id,node_id));
notifications(id, player_id, kind, payload jsonb, created_at, read_at);
feature_flags(key pk, value jsonb);
analytics_events(id bigserial, player_id, name text, props jsonb, ts timestamptz);  -- партиции по дню
```
Индексы: `players(trophies) WHERE shield_until < now()` (partial не допускает now(); используем столбец `shielded bool` с джобом снятия), `raids(defender_id, finished_at desc)`, `timers(finish_at)`.
Размер: ~25 КБ на игрока (layout+army); реплей ≈ 5–20 КБ gz в object storage.
