# CHANGELOG `content`

## Unreleased
- модуль заведён
- OWNS: `content/economy.yaml` исключён (принадлежит `economy`, устранено пересечение)
- feat(content): CONTENT-2 — `content/strings/{ru,en}/names.yaml` (85 `name_key`, нейтральные плейсхолдеры, D-10)
- data(content): добавлены `name_key` для `o_rock`/`o_wreck`, всех узлов `research.yaml`, глав `ch2..ch4` (аддитивно, `schema: 1` не меняется — миграция не требуется; `title_placeholder` оставлен)
- feat(tools): `validate_content.py` проверяет наличие `name_key` у каждого id, их уникальность, полноту ru/en и отсутствие «сирот» в strings
