// Контракт modules/client-engine/CONTRACT.md v0.1:
// interface Renderer { loadAtlas(m); render(world); on(input); setCamera(); play(fx) }

/** Манифест атласов (art/manifest.json). Форма уточняется в следующих задачах. */
export interface AtlasManifest {
  [key: string]: unknown;
}

/** Снимок мира из client-core/sim. Пока только размер сетки и подсвеченный тайл. */
export interface WorldState {
  gridSize: number;
  highlight?: { x: number; y: number };
}

export type InputEvent =
  | { type: 'tileTap'; x: number; y: number }
  | { type: 'pan'; dx: number; dy: number }
  | { type: 'zoom'; delta: number };

export type InputHandler = (e: InputEvent) => void;

export interface Camera {
  x: number;
  y: number;
  zoom: number;
}

export interface Effect {
  name: string;
  x: number;
  y: number;
}

export interface Renderer {
  loadAtlas(manifest: AtlasManifest): Promise<void>;
  render(world: WorldState): void;
  /** Подписка на ввод; возвращает функцию отписки. */
  on(handler: InputHandler): () => void;
  setCamera(camera: Camera): void;
  play(fx: Effect): void;
}
