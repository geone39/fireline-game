// Иконки-заглушки юнитов и заклинаний из art/manifest.json (атлас units/spells). Шелл передаёт resolver в компоненты;
// без него (тесты, нет атласа) показывается буквенная плашка.
export interface IconSpec {
  url: string;
  x: number;
  y: number;
  w: number;
  h: number;
  atlasW: number;
  atlasH: number;
}

export type IconResolver = (id: string) => IconSpec | null;

interface ManifestShape {
  atlases: Record<string, { image: string; size: [number, number] }>;
  sprites: { id: string; atlas: string; frames: { x: number; y: number; w: number; h: number }[] }[];
}

/** atlasUrl: относительный путь образа из манифеста (`atlases/units.png`) → URL, который отдаёт сборка. */
export function makeIconResolver(manifest: unknown, atlasUrl: (image: string) => string): IconResolver {
  const m = manifest as ManifestShape;
  const byId = new Map(m.sprites.map((s) => [s.id, s]));
  return (id) => {
    const s = byId.get(id);
    const f = s?.frames[0];
    const a = s ? m.atlases[s.atlas] : undefined;
    if (!s || !f || !a) return null;
    return { url: atlasUrl(a.image), x: f.x, y: f.y, w: f.w, h: f.h, atlasW: a.size[0], atlasH: a.size[1] };
  };
}
