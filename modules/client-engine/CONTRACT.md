# CONTRACT — `client-engine`

interface Renderer { loadAtlas(m); render(world); on(input) ; setCamera(); play(fx) }.

_Изменения контракта — отдельным PR с меткой `contract-change`; версия контракта указывается ниже._

**Версия контракта:** 0.2 (аддитивно к 0.1; `contract-change`)

Изменения 0.2 (всё необязательное, потребители 0.1 не ломаются):
- `WorldState` + `items`, `water`, `footprints`, `selectedUid` (рендер ничего не решает: данные приходят из store/контента).
- `InputEvent` + `select {uid|null,x,y}`, `hover {tile|null}`, `dragStart/dragMove/dragEnd {uid,x,y}`. Drag начинается только на выбранном (`selectedUid`) здании; иначе жест двигает камеру. `zoom.delta` = изменение `camera.zoom`.
- `setCamera` ограничивает камеру краями карты и допустимым зумом.
- `PixiRenderer` (не часть интерфейса): `init(host)`, `centerCamera(zoom)`, опция `resolveAssetUrl`.
