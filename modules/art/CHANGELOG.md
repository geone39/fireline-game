# CHANGELOG `art`

## Unreleased
- модуль заведён
- OWNS: явно включён `art/tools/` (скрипты генерации заглушек)
- ART-0: `art/manifest.json` + атласы-заглушки (`art/atlases/`) для всех сущностей content (юниты, техника, корабли, спеллы, герои, здания, оборона, декор, препятствия); генератор `art/tools/gen_placeholders.py`, проверка `art/tools/check_manifest.py`; `art/LICENSES.md`
- ART-1 (#125, спайк): `art/STYLE.md` (стиль A01-lite, палитра P01+P03, размеры, слои, анимация, атласы, лимиты, юр. чистота); `art/spike/` — два пути рендера тестовых ассетов `b_hq`, `b_cash_mine`, `u_rifleman` (Node-процедурный 4 напр. и Blender 8 напр.) из общих `models.json`+`palette.json`, атласы, манифесты, `preview.html`, `COMPARISON.md` с рекомендацией; `art/tools/merge_spike.py`. `art/manifest.json` и контракт не менялись (запрос расширения — OQ-ART1-1)
