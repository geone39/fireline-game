# CHANGELOG `design`

## Unreleased
- docs(design): DESIGN-2b — battle-sim.md и raids-pvp.md приведены к решениям владельца по OQ-7 (лимит 5 мин реального времени `battle.time_limit_s` вне sim, `max_turns` — предохранитель, ap:1/2, без LOS, строгий реплей, скрытые мины не в клиентском Snapshot, без автохода); CONTENT-REQ и поля Result обновлены; OQ-7.1/7.5–7.9 закрыты
- docs(design): DESIGN-2 — `docs/design/battle-sim.md` переписан под пошаговый бой на клетках (ADR-003): фазы, очки действий, ИИ обороны, высадка, стены/ловушки, заклинания/герои/корабли, результат/звёзды/LootBasis, детерминизм и реплей, примеры, поля Command/Result для sim 0.2, CONTENT-REQ; уточнены OQ-7.1…7.9
- docs(design): согласованы troops/defences/heroes/raids-pvp с пошаговой моделью
- модуль заведён
