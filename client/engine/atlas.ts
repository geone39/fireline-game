import type { AtlasManifest } from './renderer';

// Форма art/manifest.json (schema 1). Заглушки ART-0; финальный арт сохранит формат. Всё сверх контракта art 0.1
// (fps, anims, anchors.top, atlases.*.webp/category, _anims) необязательно и читается аддитивно (OQ-CLIENT6-*).
export interface ArtFrame {
  name: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

/** Клип анимации в манифесте (предложение OQ-ART1-1). `frames` — плоский список (направление-мажорно) или по направлениям. */
export interface SpriteAnimSpec {
  fps?: number;
  loop?: boolean;
  /** 1, 4 или 8. Нумерация: 0 = лицом к камере, далее по часовой стрелке на экране. */
  dirs?: number;
  frames: readonly (string | readonly string[])[];
}

/** Служебные метаданные спайка ART-1: `_anims[id][anim] = {fps, frames, loop}` при кадрах `{id}_{anim}_d{dir}_{n}`. */
export type LegacyAnimMeta = Record<string, { fps?: number; frames?: number; loop?: boolean }>;

export interface ArtSprite {
  id: string;
  atlas: string;
  frames: ArtFrame[];
  /** Якорь «ground»: точка кадра (px), совпадающая с центром футпринта на земле. `top` — верх спрайта (для полосок hp). */
  anchors: { ground: { x: number; y: number }; top?: { x: number; y: number } };
  fps?: number;
  anims?: Record<string, SpriteAnimSpec>;
}

export interface ArtAtlasEntry {
  image: string;
  data: string;
  size: [number, number];
  /** Необязательно: WebP-версия того же атласа (предпочитается при поддержке; PNG — запасной). */
  webp?: string;
  /** Необязательно: категория (buildings, units, ...). Иначе — последний сегмент имени атласа (`spike_blender_units` -> units). */
  category?: string;
}

export interface ArtManifest {
  schema: number;
  atlases: Record<string, ArtAtlasEntry>;
  sprites: ArtSprite[];
  _anims?: Record<string, LegacyAnimMeta>;
}

export function parseArtManifest(m: AtlasManifest): ArtManifest {
  if (m.schema !== 1 || typeof m.atlases !== 'object' || !Array.isArray(m.sprites)) {
    throw new Error('art manifest: unsupported format');
  }
  return m as unknown as ArtManifest;
}

/** Индекс спрайтов по id типа (b_hq, d_wall, ...). */
export function indexSprites(m: ArtManifest): Map<string, ArtSprite> {
  const out = new Map<string, ArtSprite>();
  for (const s of m.sprites) if (s.frames.length > 0) out.set(s.id, s);
  return out;
}

/** Категория атласа. */
export function atlasCategory(name: string, entry: Pick<ArtAtlasEntry, 'category'>): string {
  if (entry.category) return entry.category;
  const i = name.lastIndexOf('_');
  return i < 0 ? name : name.slice(i + 1);
}
