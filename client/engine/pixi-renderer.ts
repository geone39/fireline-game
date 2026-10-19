import { Application, Assets, Container, Graphics, Rectangle, Sprite, Texture } from 'pixi.js';
import { indexSprites, parseArtManifest, type ArtSprite } from './atlas';
import { footprintOf, type BaseItem, type Rect } from './base-types';
import { centerOn, clampCamera, intersects, visibleRect, worldBounds, type WorldRect } from './camera';
import {
  BACKGROUND_COLOR,
  CULL_MARGIN_PX,
  FALLBACK_BUILDING_FILL,
  HOVER_ALPHA,
  HOVER_COLOR,
  SELECT_COLOR,
  TILE_FILL,
  TILE_FILL_ALT,
  TILE_STROKE,
  WATER_FILL,
  WATER_FILL_ALT,
} from './config';
import { sortByDepth } from './depth';
import { diamond, footprintCenter, footprintDiamond, tileToScreen } from './iso';
import { Interaction, selectedFootprint } from './interaction';
import { BattleInput } from './battle-input';
import { BattlePlayback } from './battle-playback';
import type { BattleEvent, BattleWorldState } from './battle-types';
import { BattleView } from './battle-view';
import type {
  AtlasManifest,
  BattleRenderer,
  Camera,
  Effect,
  InputEvent,
  InputHandler,
  WorldState,
} from './renderer';

interface Node {
  item: BaseItem;
  view: Sprite | Graphics;
  bounds: WorldRect;
}

export interface PixiRendererOptions {
  /** Превращает путь из манифеста (`atlases/buildings.png`) в URL. */
  resolveAssetUrl?: (path: string) => string;
}

export class PixiRenderer implements BattleRenderer {
  private readonly app = new Application();
  private readonly world = new Container();
  private readonly ground = new Graphics();
  private readonly itemsLayer = new Container();
  private readonly overlay = new Graphics();
  private readonly handlers = new Set<InputHandler>();
  private readonly nodes = new Map<string, Node>();
  private readonly textures = new Map<string, Texture>();
  private sprites = new Map<string, ArtSprite>();
  private atlasTextures = new Map<string, Texture>();
  private state: WorldState | null = null;
  private camera: Camera = { x: 0, y: 0, zoom: 1 };
  private hover: { x: number; y: number } | null = null;
  private groundKey = '';
  private interaction: Interaction | null = null;
  private mode: 'base' | 'battle' = 'base';
  private battle: BattleWorldState | null = null;
  private battleInput: BattleInput | null = null;
  private readonly battleView = new BattleView((type) => this.spriteFor(type));
  private readonly playback = new BattlePlayback(() => {
    this.redrawBattle();
    this.dispatch({ type: 'battleEventsDone' });
  });

  constructor(private readonly options: PixiRendererOptions = {}) {}

  async init(host: HTMLElement): Promise<void> {
    await this.app.init({ background: BACKGROUND_COLOR, resizeTo: host, antialias: true });
    host.appendChild(this.app.canvas);
    this.app.canvas.style.display = 'block';
    this.app.canvas.style.touchAction = 'none';
    this.itemsLayer.sortableChildren = true;
    this.world.addChild(this.ground, this.itemsLayer, this.overlay, this.battleView.container);
    this.app.stage.addChild(this.world);
    this.app.renderer.on('resize', () => this.setCamera(this.camera));
    this.bindInput(this.app.canvas);
    this.interaction = new Interaction({
      getCamera: () => this.camera,
      setCamera: (c) => this.setCamera(c),
      getWorld: () => this.state,
      emit: (e) => this.dispatch(e),
    });
    this.battleInput = new BattleInput({
      getCamera: () => this.camera,
      setCamera: (c) => this.setCamera(c),
      getState: () => this.playback.state,
      emit: (e) => this.dispatch(e),
    });
    this.app.ticker.add((t) => {
      if (this.mode === 'battle' && this.playback.tick(t.deltaMS)) this.redrawBattle();
    });
  }

  async loadAtlas(manifest: AtlasManifest): Promise<void> {
    const m = parseArtManifest(manifest);
    const resolve = this.options.resolveAssetUrl ?? ((p: string) => p);
    this.sprites = indexSprites(m);
    this.textures.clear();
    this.atlasTextures = new Map();
    for (const [name, a] of Object.entries(m.atlases)) {
      this.atlasTextures.set(name, (await Assets.load(resolve(a.image))) as Texture);
    }
    this.battleView.resetSprites();
    if (this.mode === 'battle') this.redrawBattle();
    else if (this.state) this.render(this.state);
  }

  render(state: WorldState): void {
    this.state = state;
    this.setMode('base');
    this.drawGround(state);
    this.syncItems(state);
    this.drawOverlay();
    this.cull();
  }

  setBattle(state: BattleWorldState): void {
    this.battle = state;
    this.setMode('battle');
    this.drawGround(state);
    this.playback.setState(state);
    this.redrawBattle();
  }

  playEvents(events: readonly BattleEvent[]): void {
    this.playback.enqueue(events);
  }

  skipEvents(): void {
    this.playback.skip();
  }

  setSkipAnimations(on: boolean): void {
    this.playback.setInstant(on);
  }

  on(handler: InputHandler): () => void {
    this.handlers.add(handler);
    return () => {
      this.handlers.delete(handler);
    };
  }

  setCamera(camera: Camera): void {
    const size = this.gridSize();
    this.camera =
      size === undefined
        ? camera
        : clampCamera(camera, { width: this.app.screen.width, height: this.app.screen.height }, worldBounds(size));
    this.world.position.set(this.camera.x, this.camera.y);
    this.world.scale.set(this.camera.zoom);
    this.cull();
  }

  /** Камера по центру карты. Вызывается один раз после первого render(). */
  centerCamera(zoom = 1): void {
    const size = this.gridSize() ?? 0;
    const c = (size - 1) / 2;
    this.setCamera(centerOn(c, c, zoom, { width: this.app.screen.width, height: this.app.screen.height }));
  }

  play(fx: Effect): void {
    // Эффекты — отдельная задача.
    void fx;
  }

  destroy(): void {
    this.app.destroy(true, { children: true });
  }

  private gridSize(): number | undefined {
    return (this.mode === 'battle' ? this.battle?.gridSize : this.state?.gridSize) ?? this.state?.gridSize;
  }

  private setMode(mode: 'base' | 'battle'): void {
    if (this.mode === mode) return;
    this.mode = mode;
    this.hover = null;
    const battle = mode === 'battle';
    this.itemsLayer.visible = !battle;
    this.overlay.visible = !battle;
    this.battleView.show(battle);
    this.groundKey = '';
  }

  private redrawBattle(): void {
    const s = this.playback.state;
    if (this.mode === 'battle' && s) this.battleView.draw(s, this.playback, this.hover);
  }

  private dispatch(e: InputEvent): void {
    if (e.type === 'hover') {
      this.hover = e.tile;
      if (this.mode === 'battle') this.redrawBattle();
      else this.drawOverlay();
    }
    for (const h of this.handlers) h(e);
  }

  private input(): Interaction | BattleInput | null {
    return this.mode === 'battle' ? this.battleInput : this.interaction;
  }

  private bindInput(canvas: HTMLCanvasElement): void {
    const pos = (ev: PointerEvent | WheelEvent) => {
      const r = canvas.getBoundingClientRect();
      return { x: ev.clientX - r.left, y: ev.clientY - r.top };
    };
    canvas.addEventListener('pointerdown', (ev) => {
      canvas.setPointerCapture(ev.pointerId);
      const p = pos(ev);
      this.input()?.pointerDown(ev.pointerId, p.x, p.y);
    });
    canvas.addEventListener('pointermove', (ev) => {
      const p = pos(ev);
      this.input()?.pointerMove(ev.pointerId, p.x, p.y);
    });
    canvas.addEventListener('pointerup', (ev) => {
      const p = pos(ev);
      this.input()?.pointerUp(ev.pointerId, p.x, p.y);
    });
    canvas.addEventListener('pointercancel', (ev) => this.input()?.pointerCancel(ev.pointerId));
    canvas.addEventListener('pointerleave', () => this.input()?.pointerLeave());
    canvas.addEventListener(
      'wheel',
      (ev) => {
        ev.preventDefault();
        const p = pos(ev);
        this.input()?.wheel(ev.deltaY, p.x, p.y);
      },
      { passive: false },
    );
  }

  private drawGround(state: Pick<WorldState, 'gridSize' | 'water'>): void {
    const key = JSON.stringify([state.gridSize, state.water]);
    if (key === this.groundKey) return;
    this.groundKey = key;
    const water = (tx: number, ty: number) =>
      (state.water ?? []).some((r: Rect) => tx >= r.x && tx < r.x + r.w && ty >= r.y && ty < r.y + r.h);
    const g = this.ground.clear();
    for (let ty = 0; ty < state.gridSize; ty++) {
      for (let tx = 0; tx < state.gridSize; tx++) {
        const p = tileToScreen(tx, ty);
        const alt = (tx + ty) % 2 === 0;
        const fill = water(tx, ty) ? (alt ? WATER_FILL : WATER_FILL_ALT) : alt ? TILE_FILL : TILE_FILL_ALT;
        g.poly(diamond(p.x, p.y)).fill(fill).stroke({ width: 1, color: TILE_STROKE });
      }
    }
  }

  private syncItems(state: WorldState): void {
    const footprints = state.footprints ?? {};
    const items = state.items ?? [];
    const alive = new Set(items.map((i) => i.uid));
    for (const [uid, n] of this.nodes) {
      if (!alive.has(uid)) {
        n.view.destroy();
        this.nodes.delete(uid);
      }
    }
    sortByDepth(items, footprints).forEach((item, order) => {
      const { w, h } = footprintOf(item, footprints);
      const c = footprintCenter(item.x, item.y, w, h);
      const p = tileToScreen(c.x, c.y);
      let n = this.nodes.get(item.uid);
      const sprite = this.sprites.get(item.type);
      const wantSprite = sprite !== undefined && this.atlasTextures.has(sprite.atlas);
      if (n && (n.view instanceof Sprite) !== wantSprite) {
        n.view.destroy();
        n = undefined;
      }
      if (!n) {
        const view = wantSprite ? this.makeSprite(sprite) : new Graphics();
        this.itemsLayer.addChild(view);
        n = { item, view, bounds: { left: 0, top: 0, right: 0, bottom: 0 } };
        this.nodes.set(item.uid, n);
      }
      n.item = item;
      n.view.zIndex = order;
      n.view.position.set(p.x, p.y);
      if (n.view instanceof Graphics) {
        n.view.clear().poly(relativeDiamond(item.x, item.y, w, h, p)).fill(FALLBACK_BUILDING_FILL);
        const d = relativeDiamond(item.x, item.y, w, h, p);
        n.bounds = { left: p.x + minOf(d, 0), right: p.x + maxOf(d, 0), top: p.y + minOf(d, 1) - 24, bottom: p.y + maxOf(d, 1) };
      } else {
        const sw = n.view.width;
        const sh = n.view.height;
        n.bounds = {
          left: p.x - n.view.anchor.x * sw,
          top: p.y - n.view.anchor.y * sh,
          right: p.x + (1 - n.view.anchor.x) * sw,
          bottom: p.y + (1 - n.view.anchor.y) * sh,
        };
      }
    });
  }

  /** Спрайт по id контента, если арт загружен; иначе undefined (рисуется заглушка-фигура). */
  private spriteFor(type: string): Sprite | undefined {
    const def = this.sprites.get(type);
    if (!def || !this.atlasTextures.has(def.atlas)) return undefined;
    return this.makeSprite(def);
  }

  private makeSprite(def: ArtSprite): Sprite {
    let tex = this.textures.get(def.id);
    if (!tex) {
      const f = def.frames[0];
      const src = this.atlasTextures.get(def.atlas);
      if (!f || !src) throw new Error(`no frame for ${def.id}`);
      tex = new Texture({ source: src.source, frame: new Rectangle(f.x, f.y, f.w, f.h) });
      this.textures.set(def.id, tex);
    }
    const s = new Sprite(tex);
    s.anchor.set(def.anchors.ground.x / tex.width, def.anchors.ground.y / tex.height);
    return s;
  }

  private drawOverlay(): void {
    const g = this.overlay.clear();
    const sel = this.state ? selectedFootprint(this.state) : undefined;
    if (sel) {
      g.poly(footprintDiamond(sel.x, sel.y, sel.w, sel.h)).stroke({ width: 3, color: SELECT_COLOR });
    }
    if (this.hover) {
      const p = tileToScreen(this.hover.x, this.hover.y);
      g.poly(diamond(p.x, p.y)).fill({ color: HOVER_COLOR, alpha: HOVER_ALPHA });
    }
  }

  /** Culling: спрайты вне видимой области не участвуют в рендере. */
  private cull(): void {
    const view = visibleRect(
      this.camera,
      { width: this.app.screen.width, height: this.app.screen.height },
      CULL_MARGIN_PX,
    );
    for (const n of this.nodes.values()) n.view.visible = intersects(n.bounds, view);
  }
}

function relativeDiamond(x: number, y: number, w: number, h: number, origin: { x: number; y: number }): number[] {
  return footprintDiamond(x, y, w, h).map((v, i) => v - (i % 2 === 0 ? origin.x : origin.y));
}

function minOf(a: number[], off: number): number {
  let m = Infinity;
  for (let i = off; i < a.length; i += 2) m = Math.min(m, a[i] as number);
  return m;
}

function maxOf(a: number[], off: number): number {
  let m = -Infinity;
  for (let i = off; i < a.length; i += 2) m = Math.max(m, a[i] as number);
  return m;
}
