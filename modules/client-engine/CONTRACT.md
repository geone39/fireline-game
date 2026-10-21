# CONTRACT — `client-engine`

interface Renderer { loadAtlas(m); render(world); on(input) ; setCamera(); play(fx) }.

_Изменения контракта — отдельным PR с меткой `contract-change`; версия контракта указывается ниже._

**Версия контракта:** 0.3 (аддитивно к 0.2 и 0.1; `contract-change`)

Изменения 0.2 (всё необязательное, потребители 0.1 не ломаются):
- `WorldState` + `items`, `water`, `footprints`, `selectedUid` (рендер ничего не решает: данные приходят из store/контента).
- `InputEvent` + `select {uid|null,x,y}`, `hover {tile|null}`, `dragStart/dragMove/dragEnd {uid,x,y}`. Drag начинается только на выбранном (`selectedUid`) здании; иначе жест двигает камеру. `zoom.delta` = изменение `camera.zoom`.
- `setCamera` ограничивает камеру краями карты и допустимым зумом.
- `PixiRenderer` (не часть интерфейса): `init(host)`, `centerCamera(zoom)`, опция `resolveAssetUrl`.

Изменения 0.3 (CLIENT-4a, рендер боя; всё аддитивно, потребители 0.2 не ломаются):
- Новый интерфейс `BattleRenderer extends Renderer`: `setBattle(state: BattleWorldState)`, `playEvents(events: BattleEvent[])`, `skipEvents()`, `setSkipAnimations(on)`. `PixiRenderer` реализует оба. `render(world)` возвращает рендер на базу, `setBattle` — на бой; камера общая и сохраняется.
- `BattleWorldState {gridSize, water?, entities[], selectedId?, highlight?}`. `BattleEntity {id (sim.EntityID), side, type (id контента/спрайта), kind, layer?, x,y,w?,h?, hp,maxHp, ap?,maxAp?, revealed?}`. `highlight {moves[], range{x,y,min,max}, targets[]}`: рендер ничего не вычисляет по правилам — допустимые ходы/цели приходят от core; дальность рисуется по Чебышёву (min..max). Скрытые ловушки (`kind:'trap'`) без `revealed` не рисуются и не выбираются.
- `BattleEvent` = `Event` из `sim/wasm/api.d.ts` (`Kind 1..11, Actor, Target, Cell, Value`). Семантика воспроизведения: `Moved` (Value = пройденная стоимость; `Target≠0` — погрузка), `Attacked` (Value = урон, засчитывается в момент попадания), `Killed/Destroyed` (взрыв + дым), `TrapTriggered`, `SpellCast`, `EffectTick` (Value = урон), `Healed` (Value = лечение), `Released` (появление Target), `TurnEnded/BattleEnded`.
- Порядок вызовов: `playEvents(events)`, затем `setBattle(after)` — авторитетное состояние «после» применяется, когда очередь проиграна (до этого показывается состояние «до», изменённое событиями). Обратный порядок допустим: события доигрываются поверх уже обновлённого состояния, недостающие сущности пропускаются. Всегда после очереди показывается ровно последнее `setBattle`, поэтому анимация не может разойтись с ядром. `skipEvents()` доигрывает мгновенно; `setSkipAnimations(true)` — постоянный режим без анимаций.
- `InputEvent` + `battleSelect {id}` (тап по своему юниту), `battleTarget {id,x,y}` (тап по сущности обороны, либо по своей из `highlight.targets`, например лечение), `battleCell {x,y}` (тап по прочей клетке), `battleEventsDone` (очередь проиграна/пропущена). `pan`/`zoom`/`hover` как раньше; drag в бою отсутствует.
- Приоритет тапа под пальцем: воздух > юниты > постройки/стены > открытые ловушки; декор и ловушки — не цели (`battleCell`).
- Отрисовка: заглушки из `art/manifest.json` по `type` (при отсутствии спрайта — фигура-заглушка), полоска hp (у юнитов всегда, у построек после урона), маркеры AP над своими юнитами, подсветка ходов/дальности/целей/выбора, эффекты выстрела, попадания, взрыва, дыма, появления, заклинания, ловушки, лечения.
- Dev-страница: `client/engine/battle-dev.html` (`npm run dev` -> `/engine/battle-dev.html`).

Изменения 0.3.1 (CLIENT-6; интерфейс `Renderer`/`BattleRenderer` не меняется, расширяется только чтение манифеста `art`, аддитивно):
- `loadAtlas(manifest)` регистрирует атласы и грузит только нужные текущему экрану (база/бой) по категории; остальные подгружаются при переходе, ненужные выгружаются. WebP (поле `webp` или `image` с `.webp`) с запасным PNG; атлас, не загрузившийся ни по одному адресу, не бросает ошибку — остаются заглушки.
- Необязательные поля манифеста: `sprites[].anims`, `anchors.top`, `atlases.*.webp|category`, служебный `_anims` (см. OQ-CLIENT6-1). Спрайты без них рисуются как раньше (кадр 0, якорь `ground`).
- Анимация и направления: юниты — `idle/walk/attack/die`, 8 (или 4/1) направлений по вектору движения/цели в изометрии; здания — 1 направление. События боя: `Moved` → walk, `Attacked` → attack в сторону цели, `Killed` → die. Нет клипа или кадра — прежняя заглушка/кадр 0.
- Не часть интерфейса: у `PixiRenderer` добавлены `framesRendered`, `fps`, `loadedAtlases` (dev-замеры). Dev-страница: `client/engine/art-dev.html`.
