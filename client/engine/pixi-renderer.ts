import { Application, Container, Graphics } from 'pixi.js';
import {
  BACKGROUND_COLOR,
  GRID_SIZE,
  HELLO_TILE,
  HELLO_TILE_FILL,
  TILE_FILL,
  TILE_FILL_ALT,
  TILE_STROKE,
} from './config';
import { diamond, tileToScreen } from './iso';
import type {
  AtlasManifest,
  Camera,
  Effect,
  InputHandler,
  Renderer,
  WorldState,
} from './renderer';

export class PixiRenderer implements Renderer {
  private readonly app = new Application();
  private readonly world = new Container();
  private readonly handlers = new Set<InputHandler>();

  async init(host: HTMLElement): Promise<void> {
    await this.app.init({ background: BACKGROUND_COLOR, resizeTo: host, antialias: true });
    host.appendChild(this.app.canvas);
    this.app.stage.addChild(this.world);
    this.app.canvas.style.display = 'block';
    const c = tileToScreen(HELLO_TILE.x, HELLO_TILE.y);
    this.setCamera({
      x: this.app.screen.width / 2 - c.x,
      y: this.app.screen.height / 2 - c.y,
      zoom: 1,
    });
  }

  async loadAtlas(manifest: AtlasManifest): Promise<void> {
    // Атласы появятся после art/manifest.json; в hello-tile рисуем примитивами.
    void manifest;
  }

  render(state: WorldState): void {
    for (const child of this.world.removeChildren()) child.destroy();
    const g = new Graphics();
    for (let ty = 0; ty < state.gridSize; ty++) {
      for (let tx = 0; tx < state.gridSize; tx++) {
        const p = tileToScreen(tx, ty);
        g.poly(diamond(p.x, p.y))
          .fill((tx + ty) % 2 === 0 ? TILE_FILL : TILE_FILL_ALT)
          .stroke({ width: 1, color: TILE_STROKE });
      }
    }
    if (state.highlight) {
      const p = tileToScreen(state.highlight.x, state.highlight.y);
      g.poly(diamond(p.x, p.y)).fill(HELLO_TILE_FILL);
    }
    this.world.addChild(g);
  }

  on(handler: InputHandler): () => void {
    this.handlers.add(handler);
    return () => {
      this.handlers.delete(handler);
    };
  }

  setCamera(camera: Camera): void {
    this.world.position.set(camera.x, camera.y);
    this.world.scale.set(camera.zoom);
  }

  play(fx: Effect): void {
    // Эффекты — отдельная задача.
    void fx;
  }

  destroy(): void {
    this.app.destroy(true, { children: true });
  }
}

export function helloWorld(): WorldState {
  return { gridSize: GRID_SIZE, highlight: { ...HELLO_TILE } };
}
