import type { ArtFrame, ArtSprite, LegacyAnimMeta, SpriteAnimSpec } from './atlas';
import { DEFAULT_ANIM_FPS } from './config';
import { toClipDir } from './direction';

/** Клип анимации: кадры по направлениям `frames[dir][i]` (имена кадров манифеста). */
export interface AnimClip {
  name: string;
  fps: number;
  loop: boolean;
  dirs: number;
  frames: string[][];
}

export type ClipSet = Map<string, AnimClip>;

/** Спрайт манифеста с индексами для рендера: кадры по имени и клипы. */
export interface SpriteArt {
  def: ArtSprite;
  frames: Map<string, ArtFrame>;
  clips: ClipSet;
}

/** Стандартные имена состояний (спека art). Остальные имена из манифеста тоже допустимы. */
export type AnimName = 'idle' | 'walk' | 'attack' | 'die' | (string & {});

const NAME_RE = /^(.+)_d(\d+)_(\d+)$/;

/**
 * Клипы спрайта. Источники по приоритету:
 *  1. `sprite.anims` (предложение OQ-ART1-1: `{name: {fps, loop, dirs, frames}}`);
 *  2. кадры вида `{id}_{anim}_d{dir}_{n}` (формат спайка ART-1), метаданные — из `meta` (`_anims[id]`);
 *  3. устаревший формат: несколько кадров и `fps > 0` — один зацикленный клип `idle` без направлений;
 *  4. иначе клипов нет — спрайт статичен (frames[0]).
 */
export function buildClips(def: ArtSprite, meta?: LegacyAnimMeta): ClipSet {
  const have = new Set(def.frames.map((f) => f.name));
  const clips: ClipSet = new Map();
  if (def.anims) {
    for (const [name, a] of Object.entries(def.anims)) {
      const clip = fromSpec(name, a, have, def.fps);
      if (clip) clips.set(name, clip);
    }
    if (clips.size > 0) return clips;
  }
  const prefix = `${def.id}_`;
  const grouped = new Map<string, Map<number, { n: number; name: string }[]>>();
  for (const f of def.frames) {
    if (!f.name.startsWith(prefix)) continue;
    const m = NAME_RE.exec(f.name.slice(prefix.length));
    if (!m) continue;
    const anim = m[1] as string;
    const dir = Number(m[2]);
    let byDir = grouped.get(anim);
    if (!byDir) grouped.set(anim, (byDir = new Map()));
    let list = byDir.get(dir);
    if (!list) byDir.set(dir, (list = []));
    list.push({ n: Number(m[3]), name: f.name });
  }
  for (const [anim, byDir] of grouped) {
    const dirs = Math.max(...byDir.keys()) + 1;
    const frames: string[][] = [];
    for (let d = 0; d < dirs; d++) {
      frames.push((byDir.get(d) ?? []).sort((a, b) => a.n - b.n).map((x) => x.name));
    }
    if (frames.some((l) => l.length === 0)) continue;
    const m = meta?.[anim];
    clips.set(anim, {
      name: anim,
      fps: m?.fps ?? def.fps ?? DEFAULT_ANIM_FPS,
      loop: m?.loop ?? anim !== 'die',
      dirs,
      frames,
    });
  }
  if (clips.size > 0) return clips;
  if (def.frames.length > 1 && (def.fps ?? 0) > 0) {
    clips.set('idle', {
      name: 'idle',
      fps: def.fps as number,
      loop: true,
      dirs: 1,
      frames: [def.frames.map((f) => f.name)],
    });
  }
  return clips;
}

function fromSpec(name: string, a: SpriteAnimSpec, have: Set<string>, spriteFps?: number): AnimClip | undefined {
  const raw = a.frames as readonly (string | readonly string[])[];
  const perDir: string[][] =
    raw.length > 0 && Array.isArray(raw[0])
      ? (raw as readonly (readonly string[])[]).map((l) => [...l])
      : splitDirs(raw as readonly string[], a.dirs ?? 1);
  const frames = perDir.map((l) => l.filter((n) => have.has(n)));
  if (frames.length === 0 || frames.some((l) => l.length === 0)) return undefined;
  return { name, fps: a.fps ?? spriteFps ?? DEFAULT_ANIM_FPS, loop: a.loop ?? name !== 'die', dirs: frames.length, frames };
}

/** Плоский список кадров -> по направлениям (направление-мажорно: `dirs` блоков равной длины). */
function splitDirs(flat: readonly string[], dirs: number): string[][] {
  const d = Math.max(1, dirs);
  const per = Math.floor(flat.length / d);
  const out: string[][] = [];
  for (let i = 0; i < d; i++) out.push(flat.slice(i * per, (i + 1) * per));
  return out;
}

export function buildSpriteArt(def: ArtSprite, meta?: LegacyAnimMeta): SpriteArt {
  return { def, frames: new Map(def.frames.map((f) => [f.name, f])), clips: buildClips(def, meta) };
}

/** Индекс кадра клипа по времени (мс с начала): зацикленный или с остановкой на последнем. */
export function frameIndex(count: number, fps: number, loop: boolean, elapsedMs: number): number {
  if (count <= 1 || fps <= 0 || elapsedMs <= 0) return 0;
  const i = Math.floor((elapsedMs * fps) / 1000);
  return loop ? i % count : Math.min(count - 1, i);
}

/** Длительность одного прохода клипа, мс. */
export function clipDuration(clip: AnimClip, dir = 0): number {
  const n = clip.frames[dir]?.length ?? 0;
  return clip.fps > 0 ? Math.round((n * 1000) / clip.fps) : 0;
}

/** Нужный клип или запасной: запрошенное состояние -> idle -> первый клип. */
export function resolveClip(clips: ClipSet, want: AnimName): AnimClip | undefined {
  return clips.get(want) ?? clips.get('idle') ?? clips.values().next().value;
}

/** Имя кадра клипа в момент `elapsedMs` для сектора направления `facing` (0..7). */
export function clipFrame(clip: AnimClip, facing: number, elapsedMs: number): string | undefined {
  const list = clip.frames[toClipDir(facing, clip.dirs)] ?? clip.frames[0];
  if (!list || list.length === 0) return undefined;
  return list[frameIndex(list.length, clip.fps, clip.loop, elapsedMs)];
}

/**
 * Состояние воспроизведения одной сущности. Время идёт только через `advance`. Смена направления фазу не сбрасывает
 * (поворот на ходу не дёргает шаг); смена состояния — сбрасывает.
 */
export class SpriteAnimator {
  private anim: AnimName = 'idle';
  private elapsed = 0;
  private facingSector = 0;

  constructor(private readonly clips: ClipSet) {}

  get hasClips(): boolean {
    return this.clips.size > 0;
  }

  get state(): AnimName {
    return this.anim;
  }

  get facing(): number {
    return this.facingSector;
  }

  /** `restart` перезапускает клип с нуля и при том же состоянии (например, новый выстрел). */
  set(anim: AnimName, facing: number, restart = false): void {
    if (anim !== this.anim || restart) this.elapsed = 0;
    this.anim = anim;
    this.facingSector = facing;
  }

  advance(dtMs: number): void {
    this.elapsed += dtMs;
  }

  /** Имя кадра сейчас; undefined — клипов нет (рисовать frames[0]). */
  frame(): string | undefined {
    const clip = resolveClip(this.clips, this.anim);
    return clip ? clipFrame(clip, this.facingSector, this.elapsed) : undefined;
  }

  /** Непетлевой клип проигран до конца. */
  get finished(): boolean {
    const clip = this.clips.get(this.anim);
    return !!clip && !clip.loop && this.elapsed >= clipDuration(clip, toClipDir(this.facingSector, clip.dirs));
  }
}
