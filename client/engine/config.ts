// Параметры рендера (не игровой баланс). Размер сетки и карты берутся из content/base_grid.yaml (см. content.ts).
export const TILE_WIDTH = 64;
export const TILE_HEIGHT = 32;
export const BACKGROUND_COLOR = 0x111111;
export const TILE_FILL = 0x3b5b3b;
export const TILE_FILL_ALT = 0x456845;
export const TILE_STROKE = 0x1e2e1e;
export const WATER_FILL = 0x2b5f8a;
export const WATER_FILL_ALT = 0x31699a;
export const HOVER_COLOR = 0xffffff;
export const HOVER_ALPHA = 0.35;
export const SELECT_COLOR = 0xffd23f;
export const FALLBACK_BUILDING_FILL = 0x8a5a44;

export const MIN_ZOOM = 0.4;
export const MAX_ZOOM = 2.5;
export const WHEEL_ZOOM_STEP = 1.1;
/** Сдвиг указателя (px), после которого касание считается жестом, а не тапом. */
export const TAP_SLOP_PX = 6;
/** Запас вокруг видимой области для culling (px мира). */
export const CULL_MARGIN_PX = 32;
