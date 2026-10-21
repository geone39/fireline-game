import { Application, Assets, Container, Graphics, Sprite, Texture, UPDATE_PRIORITY } from 'pixi.js';
import { ArtSpriteView, FrameTextures } from './art-sprite';
import { indexSprites, parseArtManifest, type ArtFrame, type ArtSprite, type LegacyAnimMeta } from './atlas';
import { AtlasLoader, type ArtScreen } from './atlas-loader';
import { buildSpriteArt, type SpriteArt } from './sprite-anim';
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
  private readonly frames = new FrameTextures();
  private sprites = new Map<string, ArtSprite>();
  private readonly arts = new Map<string, SpriteArt>();
  private animMeta: Record<string, LegacyAnimMeta> = {};
  private readonly unloading = new Map<string, Promise<void>>();
  private readonly loader = new AtlasLoader<Texture>({
    resolve: (p) => (this.options.resolveAssetUrl ?? ((x: string) => x))(p),
    load: async (url) => {
      await this.unloading.get(url); // повторная загрузка не должна получить текстуру, которую выгружают
      return (await Assets.load(url)) as Texture;
    },
    unload: (_t, url) => {
      const p = Assets.unload(url)
        .catch(() => undefined)
        .finally(() => this.unloading.delete(url));
      this.unloading.set(url, p);
    },
    supportsWebp: detectWebp,
    onUnload: (name) => this.frames.drop(name),
    onError: (name, err) => console.warn(`atlas ${name}: не загрузился, остаются заглушки`, err),
  });
  private state: WorldState | null = null;
  private rendered = 0;
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
    // После отрисовки кадра (PixiJS рендерит сцену с приоритетом LOW): счётчик для замеров draw calls на кадр.
    this.app.ticker.add(() => this.rendered++, undefined, UPDATE_PRIORITY.UTILITY);
    this.app.ticker.add((t) => {
      if (this.mode === 'battle') {
        if (this.playback.tick(t.deltaMS)) this.redrawBattle();
        this.battleView.tickAnim(t.deltaMS);
      } else {
        this.tickBaseAnim(t.deltaMS);
      }
    });
  }

  /**
   * Регистрирует атласы манифеста и загружает те, что нужны текущему экрану (база/бой); остальные — лениво при
   * переходе. WebP с PNG-запасом, см. AtlasLoader. Нехватка поля манифеста не ошибка: рисуются заглушки.
   */
  async loadAtlas(manifest: AtlasManifest): Promise<void> {
    const m = parseArtManifest(manifest);
    this.sprites = indexSprites(m);
    this.arts.clear();
    this.animMeta = m._anims ?? {};
    this.resetViews();
    this.frames.clear();
    this.loader.setManifest(m.atlases);
    await this.enterScreen(this.mode, true);
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

  /** Сколько кадров отрисовано (для dev-замеров draw calls/fps; не часть интерфейса Renderer). */
  get framesRendered(): number {
    return this.rendered;
  }

  /** Средний fps по ticker PixiJS. */
  get fps(): number {
    return this.app.ticker.FPS;
  }

  /** Имена загруженных сейчас атласов (ленивая загрузка по экранам). */
  get loadedAtlases(): string[] {
    return this.loader.loadedNames;
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
    this.loader.dispose();
    this.frames.clear();
    this.app.destroy(true, { children: true });
  }

  private gridSize(): number | undefined {
    return (this.mode === 'battle' ? this.battle?.gridSize : this.state?.gridSize) ?? this.state?.gridSize;
  }

  private setMode(mode: 'base' | 'battle'): void {
    if (this.mode === mode) return;
    this.mode = mode;
    this.hover = null;
    void this.enterScreen(mode, false);
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
      const art = this.artFor(item.type);
      const wantSprite = art !== undefined;
      if (n && (n.view instanceof Sprite) !== wantSprite) {
        n.view.destroy();
        n = undefined;
      }
      if (!n) {
        const view = art ? this.makeSprite(art) : new Graphics();
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

  /** Экран грузит свои атласы (ленивая загрузка), отпускает чужие и пересобирает вид, если что-то подгрузилось. */
  private async enterScreen(screen: ArtScreen, force: boolean): Promise<void> {
    const fresh = await this.loader.acquire(screen);
    if (this.mode !== screen) return;
    const other: ArtScreen = screen === 'base' ? 'battle' : 'base';
    if (this.loader.release(other).length > 0) this.resetViews(other);
    if (!force && fresh.length === 0) return;
    this.resetViews(screen);
    if (screen === 'base') {
      if (this.state) this.render(this.state);
    } else this.redrawBattle();
  }

  /** Сбрасывает спрайты (после смены/выгрузки атласов они пересоздаются); без аргумента — оба экрана. */
  private resetViews(screen?: ArtScreen): void {
    if (screen !== 'battle') {
      for (const n of this.nodes.values()) n.view.destroy();
      this.nodes.clear();
    }
    if (screen !== 'base') this.battleView.resetSprites();
  }

  private tickBaseAnim(dtMs: number): void {
    for (const n of this.nodes.values()) {
      if (n.view instanceof ArtSpriteView && n.view.animated && n.view.visible) n.view.advance(dtMs);
    }
  }

  private artFor(type: string): SpriteArt | undefined {
    let art = this.arts.get(type);
    if (!art) {
      const def = this.sprites.get(type);
      if (!def) return undefined;
      art = buildSpriteArt(def, this.animMeta[def.id]);
      this.arts.set(type, art);
    }
    return this.loader.has(art.def.atlas) ? art : undefined;
  }

  /** Спрайт по id контента, если арт загружен; иначе undefined (рисуется заглушка-фигура). */
  private spriteFor(type: string): ArtSpriteView | undefined {
    const art = this.artFor(type);
    return art ? this.makeSprite(art) : undefined;
  }

  private makeSprite(art: SpriteArt): ArtSpriteView {
    const src = this.loader.get(art.def.atlas) as Texture;
    return new ArtSpriteView(art, (f: ArtFrame) => this.frames.get(art.def.atlas, src, f));
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

let webpSupport: boolean | undefined;

/** Поддерживает ли браузер WebP (проверка кодированием одного пикселя; результат кэшируется). */
export function detectWebp(): boolean {
  if (webpSupport !== undefined) return webpSupport;
  try {
    webpSupport = document.createElement('canvas').toDataURL('image/webp').startsWith('data:image/webp');
  } catch {
    webpSupport = false;
  }
  return webpSupport;
}
