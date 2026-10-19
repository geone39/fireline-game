import type { AtlasManifest } from './renderer';

// Форма art/manifest.json (schema 1). Заглушки ART-0; финальный арт сохранит формат.
export interface ArtFrame {
  name: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface ArtSprite {
  id: string;
  atlas: string;
  frames: ArtFrame[];
  /** Якорь «ground»: точка кадра (px), совпадающая с центром футпринта на земле. */
  anchors: { ground: { x: number; y: number } };
}

export interface ArtManifest {
  schema: number;
  atlases: Record<string, { image: string; data: string; size: [number, number] }>;
  sprites: ArtSprite[];
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
