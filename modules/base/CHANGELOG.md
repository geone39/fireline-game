# CHANGELOG `base`

## Unreleased
- feat: `LoadGrid`/`ParseGrid` читают размер сетки и воду из content/base_grid.yaml (OQ-B1/B5); тесты каталога не закрепляют отсутствие size/лимитов у decor, obstacles и ресурсных зданий (BASE-2)
- feat: server/svc/base — сетка, каталог из content/*.yaml, валидация build/move и раскладки (границы, пересечения, лимиты по HQ, requires_water, запрет полной замкнутости HQ без разрушаемых стен), идемпотентность по uid, типы остальных BaseCommand (BASE-1)
- модуль заведён
