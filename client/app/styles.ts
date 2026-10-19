// Раскладка слота редактора базы. Компоненты client/ui/base без стилей: тулбар, магазин и карточка живут в
// `[data-testid=base-editor]`, а SVG-подсветка рассчитывает на позиционирование относительно экрана —
// шелл занимает весь viewport, поэтому закрепляем её как fixed.
export const SHELL_CSS = `
html, body, #app { margin: 0; height: 100%; background: #111; }
.base-slot { position: absolute; inset: 0; pointer-events: none; color: #fff; font: 14px/1.3 system-ui, sans-serif; }
.base-slot [data-testid=base-editor] {
  position: absolute; left: 0; right: 0; bottom: 0; pointer-events: auto;
  padding: 8px 10px; background: rgba(0,0,0,.6); max-height: 28vh; overflow: auto;
}
/* Магазин — компактная лента, чтобы не закрывать базу (иначе здания под панелью недоступны для тапа). */
.base-slot [data-testid=shop] h3 { margin: 0 0 4px; font-size: 14px; }
.base-slot [data-testid=shop] ul { display: flex; gap: 6px 14px; margin: 0; padding: 0; list-style: none; flex-wrap: nowrap; overflow-x: auto; }
.base-slot [data-testid=shop] li { flex: 0 0 auto; white-space: nowrap; }
.base-slot [data-testid=card] h3 { margin: 0 0 4px; }
.base-slot [data-testid=card] p { margin: 2px 0; }
.base-slot svg[data-testid=overlay] { position: fixed; inset: 0; }
.base-slot [data-testid=notice] { color: #ffb4b4; }
.base-slot button { font: inherit; margin: 2px; }
.app-center { display: flex; flex-direction: column; gap: 12px; align-items: center; justify-content: center;
  height: 100%; color: #fff; font: 16px/1.4 system-ui, sans-serif; text-align: center; padding: 0 16px; }
`;
