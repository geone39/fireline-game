// Раскладка слота редактора базы. Компоненты client/ui/base без стилей: тулбар, магазин и карточка живут в
// `[data-testid=base-editor]`, а SVG-подсветка рассчитывает на позиционирование относительно экрана —
// шелл занимает весь viewport, поэтому закрепляем её как fixed.
export const SHELL_CSS = `
html, body, #app { margin: 0; height: 100%; background: #111; }
.base-slot { position: absolute; inset: 0; pointer-events: none; color: #fff; font: 14px/1.3 system-ui, sans-serif; }
.base-slot [data-testid=base-editor] {
  position: absolute; left: 0; right: 0; bottom: 0; pointer-events: auto;
  padding: 8px 10px; background: rgba(0,0,0,.6); max-height: 40vh; overflow: auto;
}
.base-slot svg[data-testid=overlay] { position: fixed; inset: 0; }
.base-slot [data-testid=notice] { color: #ffb4b4; }
.base-slot button { font: inherit; margin: 2px; }
.app-center { display: flex; flex-direction: column; gap: 12px; align-items: center; justify-content: center;
  height: 100%; color: #fff; font: 16px/1.4 system-ui, sans-serif; text-align: center; padding: 0 16px; }
`;
