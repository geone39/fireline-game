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

// Бой: цвета и тайминги анимаций (параметры отрисовки, не игровой баланс).
export const BATTLE_MOVE_STEP_MS = 120;
export const BATTLE_SHOT_MS = 220;
export const BATTLE_HIT_MS = 160;
export const BATTLE_EXPLOSION_MS = 450;
export const BATTLE_SMOKE_MS = 900;
export const BATTLE_SPAWN_MS = 250;
export const BATTLE_SPELL_MS = 500;
export const BATTLE_TICK_MS = 250;
export const BATTLE_HEAL_MS = 350;
export const BATTLE_TRAP_MS = 350;
export const BATTLE_END_MS = 400;
export const BATTLE_MOVE_COLOR = 0x4aa3ff;
export const BATTLE_MOVE_ALPHA = 0.35;
export const BATTLE_RANGE_COLOR = 0xffb347;
export const BATTLE_RANGE_ALPHA = 0.22;
export const BATTLE_TARGET_COLOR = 0xff4d4d;
export const BATTLE_ATTACKER_COLOR = 0x4a90d9;
export const BATTLE_DEFENDER_COLOR = 0xb5533c;
export const BATTLE_TRAP_COLOR = 0xe0c040;
export const BATTLE_HP_BG = 0x222222;
export const BATTLE_HP_ATTACKER = 0x5fd35f;
export const BATTLE_HP_DEFENDER = 0xe05a4a;
export const BATTLE_AP_FULL = 0xffd23f;
export const BATTLE_AP_EMPTY = 0x555555;
export const BATTLE_HP_BAR_WIDTH = 28;
export const BATTLE_HP_BAR_HEIGHT = 4;
