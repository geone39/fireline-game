import { Rectangle, Sprite, Texture } from 'pixi.js';
import type { ArtFrame } from './atlas';
import { SpriteAnimator, type AnimName, type SpriteArt } from './sprite-anim';

/** Кэш текстур кадров по атласам; освобождается вместе с атласом. Источник (картинку) не трогает — им владеет загрузчик. */
export class FrameTextures {
  private readonly byAtlas = new Map<string, Map<string, Texture>>();

  get(atlas: string, source: Texture, f: ArtFrame): Texture {
    let m = this.byAtlas.get(atlas);
    if (!m) this.byAtlas.set(atlas, (m = new Map()));
    let t = m.get(f.name);
    if (!t) {
      t = new Texture({ source: source.source, frame: new Rectangle(f.x, f.y, f.w, f.h) });
      m.set(f.name, t);
    }
    return t;
  }

  drop(atlas: string): void {
    const m = this.byAtlas.get(atlas);
    if (!m) return;
    for (const t of m.values()) t.destroy(false);
    this.byAtlas.delete(atlas);
  }

  clear(): void {
    for (const a of [...this.byAtlas.keys()]) this.drop(a);
  }
}

/**
 * Спрайт из манифеста art: якорь `ground`, кадры и анимация (если у спрайта есть клипы).
 * Время идёт только через `advance`; текстура меняется лишь при смене кадра.
 */
export class ArtSpriteView extends Sprite {
  private readonly animator: SpriteAnimator;
  private shown = '';

  constructor(
    readonly art: SpriteArt,
    private readonly textureOf: (f: ArtFrame) => Texture,
  ) {
    super(textureOf(art.def.frames[0] as ArtFrame));
    this.animator = new SpriteAnimator(art.clips);
    this.refresh();
  }

  /** Есть ли что проигрывать (иначе спрайт статичен и в тиках не участвует). */
  get animated(): boolean {
    return this.animator.hasClips;
  }

  get facing(): number {
    return this.animator.facing;
  }

  get hasDie(): boolean {
    return this.art.clips.has('die');
  }

  get finished(): boolean {
    return this.animator.finished;
  }

  /** Расстояние от точки «земли» до верха спрайта (px): для полосок hp. Якорь `top`, иначе высота над землёй. */
  get topOffset(): number {
    const a = this.art.def.anchors;
    return a.top ? a.ground.y - a.top.y : a.ground.y;
  }

  play(anim: AnimName, facing: number, restart = false): void {
    this.animator.set(anim, facing, restart);
    this.refresh();
  }

  advance(dtMs: number): void {
    if (!this.animator.hasClips) return;
    this.animator.advance(dtMs);
    this.refresh();
  }

  private refresh(): void {
    const name = this.animator.frame() ?? (this.art.def.frames[0] as ArtFrame).name;
    if (name === this.shown) return;
    const f = this.art.frames.get(name) ?? (this.art.def.frames[0] as ArtFrame);
    this.shown = f.name;
    this.texture = this.textureOf(f);
    const g = this.art.def.anchors.ground;
    this.anchor.set(g.x / f.w, g.y / f.h);
  }
}
